package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

func (s *Server) middlewareStack() []func(http.Handler) http.Handler {
	authMW := auth.OptionalAuthentication(s.deps.Auth, auth.MiddlewareOptions{Logger: s.logger})
	if s.deps.Auth == nil {
		// Fallback when Auth is not wired: cookie detect only is unsafe for
		// protected routes; handlers still enforce ACL when ACL is set.
		authMW = auth.OptionalAuthentication(
			auth.NewService(nil, auth.Options{
				CookieName:   s.cfg.SessionCookieName,
				AuthDisabled: s.cfg.AuthDisabled,
				Logger:       s.logger,
			}),
			auth.MiddlewareOptions{Logger: s.logger},
		)
	}
	return []func(http.Handler) http.Handler{
		s.recoverer,
		s.requestID,
		s.realIP,
		s.secureHeaders,
		s.phpCanonicalMiddleware,
		authMW,
		s.requestLogger,
		s.apiUsageAuditMiddleware,
	}
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered",
					"error", rec,
					"stack", string(debug.Stack()),
					"request_id", RequestIDFromContext(r.Context()),
				)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := clientIP(r); ip != "" {
			r.RemoteAddr = net.JoinHostPort(ip, "0")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// strict-origin omits URL paths and query strings from Referer.
		// no-referrer makes browsers send the literal Origin "null" on POST
		// (Fetch: append a request Origin header). The license page is a
		// normal same-document form, so that policy rejected activation
		// before the request reached license.Activate.
		h.Set("Referrer-Policy", "strict-origin")
		if s.isEmbedPath(r.URL.Path) {
			// Designated embed routes only: allow framing by configured origins.
			// Do not set X-Frame-Options (conflicts with CSP frame-ancestors).
			h.Set("Content-Security-Policy", "frame-ancestors "+s.embedFrameAncestors())
			h.Set("Cache-Control", "no-store")
		} else if strings.HasPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(s.cfg.BasePath, "/")), "/sdk/") ||
			strings.Contains(r.URL.Path, "/sdk/") {
			// SDK assets set their own Cache-Control in the handler.
			h.Set("X-Frame-Options", "SAMEORIGIN")
		} else {
			// SAMEORIGIN (not DENY): Jasper HTML must load in the viewer iframe on
			// the same origin. PHP execute_report.php does not send X-Frame-Options: DENY.
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		user := auth.FromContext(r.Context())
		s.logger.Info("request",
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote_ip", remoteHost(r.RemoteAddr)),
			slog.String("user", user.DisplayName()),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}

// RequestIDFromContext returns the request ID set by middleware.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func remoteHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
