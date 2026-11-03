package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/audit"
)

// apiUsageAuditMiddleware records selective /api/v1/* requests into audit_log.
//
// Exclusions (avoid analytics self-noise and low-value probes):
//   - non-/api/v1 paths
//   - OPTIONS / HEAD
//   - /api/v1/admin/audit*, /api/v1/admin/audit-export*, and /api/v1/admin/usage*
//   - /api/v1/diagnostics
//   - /api/v1/embedded-map/debug
//
// Never stores bodies, Authorization headers, PATs, cookies, or embed tokens.
func (s *Server) apiUsageAuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.shouldAuditAPIRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.recordAPIUsageAudit(r, rec.status, time.Since(start))
	})
}

func (s *Server) shouldAuditAPIRequest(r *http.Request) bool {
	if r == nil || !s.auditEnabled() {
		return false
	}
	if r.Method == http.MethodOptions || r.Method == http.MethodHead {
		return false
	}
	path := stripBasePath(r.URL.Path, s.cfg.BasePath)
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasPrefix(lower, "/api/v1/admin/audit"):
		return false
	case strings.HasPrefix(lower, "/api/v1/admin/audit-export"):
		return false
	case strings.HasPrefix(lower, "/api/v1/admin/usage"):
		return false
	case lower == "/api/v1/diagnostics":
		return false
	case strings.HasPrefix(lower, "/api/v1/embedded-map/debug"):
		return false
	}
	return true
}

func (s *Server) recordAPIUsageAudit(r *http.Request, status int, dur time.Duration) {
	route := apiRoutePattern(r, s.cfg.BasePath)
	method := strings.ToUpper(r.Method)
	label := method + " " + route
	st := audit.StatusSuccess
	if status >= 400 {
		st = audit.StatusFailure
	}
	details := map[string]any{
		audit.DetailsKeyMethod: method,
		audit.DetailsKeyRoute:  route,
		audit.DetailsKeyStatus: status,
	}
	if requestIsEmbedChannel(r) {
		details[audit.DetailsKeyChannel] = audit.DetailsChannelEmbed
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategorySystem,
		Action:     audit.ActionAPIRequest,
		ObjectType: audit.ObjectAPI,
		ObjectID:   route,
		ObjectName: label,
		Status:     st,
		DurationMS: audit.DurationMS(dur),
		Details:    details,
	})
}

func apiRoutePattern(r *http.Request, basePath string) string {
	if r == nil {
		return "/api/v1/*"
	}
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := strings.TrimSpace(rctx.RoutePattern()); p != "" {
			p = strings.TrimSuffix(p, "/")
			if p == "" {
				p = "/"
			}
			return p
		}
	}
	return normalizeAPIPathFallback(r.URL.Path, basePath)
}

func stripBasePath(path, basePath string) string {
	if bp := strings.TrimSuffix(basePath, "/"); bp != "" && strings.HasPrefix(path, bp) {
		path = strings.TrimPrefix(path, bp)
		if path == "" {
			path = "/"
		}
	}
	return path
}

// normalizeAPIPathFallback collapses numeric/UUID segments when chi pattern is unavailable.
func normalizeAPIPathFallback(path, basePath string) string {
	path = stripBasePath(path, basePath)
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "" {
			continue
		}
		if looksLikePathID(p) {
			parts[i] = "{id}"
		}
	}
	out := strings.Join(parts, "/")
	if out == "" {
		return "/"
	}
	return out
}

func looksLikePathID(seg string) bool {
	if seg == "" {
		return false
	}
	digit := true
	for _, c := range seg {
		if c < '0' || c > '9' {
			digit = false
			break
		}
	}
	if digit {
		return true
	}
	if len(seg) >= 32 && strings.Count(seg, "-") >= 4 {
		return true
	}
	return false
}
