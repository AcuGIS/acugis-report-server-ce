package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/acl"
	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

func (s *Server) handleDevReport(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	user := auth.FromContext(r.Context())

	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		s.writeGatewayError(w, jasper.ErrBadRequest("method not allowed"), start, r, user, "", "")
		return
	}

	q := r.URL.Query()
	reportName := jasper.ExtractReportName(q)
	format := jasper.ExtractFormat(q)

	if !jasper.IsValidReportKey(reportName) {
		s.writeGatewayError(w, jasper.ErrBadRequest("Invalid report."), start, r, user, reportName, format)
		return
	}

	if err := s.authorizeReportView(r.Context(), user, reportName); err != nil {
		s.writeGatewayError(w, err, start, r, user, reportName, format)
		return
	}
	if err := s.enforceEmbedContentScope(r, reportName); err != nil {
		s.writeGatewayError(w, jasper.ErrForbidden(err.Error()), start, r, user, reportName, format)
		return
	}
	// Only a validated embed grant on the canonical execute path may be framed
	// by the effective Embedded Analytics origins. /dev/report stays SAMEORIGIN.
	if embedGrant(r) != nil && s.isReportExecutePath(r.URL.Path) {
		s.applyAuthenticatedEmbedFraming(w)
	}

	if s.deps.Reports != nil {
		if def, err := s.deps.Reports.Get(reportName); err == nil && def != nil && mapreport.IsMapReport(def) {
			if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
				s.writeGatewayError(w, jasper.ErrForbidden(errFeatureUnavailable.Message), start, r, user, reportName, format)
				return
			}
		}
	}

	if s.deps.Gateway == nil {
		s.writeGatewayError(w, jasper.ErrUnavailable("Jasper engine URL is not configured", nil), start, r, user, reportName, format)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Gateway.Timeout())
	defer cancel()

	rawQuery := r.URL.RawQuery
	var replaceBody io.Reader
	var replaceCT string
	if embedGrant(r) != nil {
		q, body, ct, rerr := rewriteEmbedExecute(r, reportName, format, embedParameters(r))
		if rerr != nil {
			s.writeGatewayError(w, jasper.ErrBadRequest(rerr.Error()), start, r, user, reportName, format)
			return
		}
		rawQuery = q
		replaceBody = body
		replaceCT = ct
	}
	embedExtra := http.Header{}
	if folderID, ok := s.lookupReportItem(r.Context(), reportName); ok {
		// Execution reads the existing sidecar. It does not rewrite a valid
		// Designer or legacy resource base down to the catalog folder.
		slash := s.slashForFolder(r.Context(), folderID)
		if base := s.resourceBaseForExecute(reportName, slash); base != "" {
			q := r.URL.Query()
			if q.Get("_acugisResourceBase") == "" {
				vals, _ := url.ParseQuery(rawQuery)
				vals.Set("_acugisResourceBase", base)
				rawQuery = vals.Encode()
			}
		}
	}
	if s.deps.EmbeddedMap != nil && s.deps.Reports != nil {
		if def, err := s.deps.Reports.Get(reportName); err == nil && def != nil {
			if def.Raw == nil {
				def.Raw = map[string]any{}
			}
			if v, ok := def.Raw["report"]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" || fmt.Sprint(v) == "<nil>" {
				def.Raw["report"] = reportName
			}
			queryParams := mapreport.ParseQueryMulti(rawQuery)
			// Do not touch .jrxml / delete .jasper on execute. That forced a full
			// Tomcat recompile on every run. Layout sync belongs on edit save;
			// Tomcat already recompiles when JRXML is newer than .jasper.
			mapResult := s.deps.EmbeddedMap.AppendToQuery(ctx, def, rawQuery, queryParams)
			if emb := embeddedmap.ReportEmbeddedMap(def); emb != nil {
				if mapResult.Error != "" {
					s.logger.Error("embedded map", "report", reportName, "error", mapResult.Error)
					embedExtra.Set("X-JRS-Embedded-Map", "error")
					errHdr := strings.ReplaceAll(mapResult.Error, "\n", " ")
					if len(errHdr) > 200 {
						errHdr = errHdr[:200]
					}
					embedExtra.Set("X-JRS-Embedded-Map-Error", errHdr)
				} else {
					embedExtra.Set("X-JRS-Embedded-Map", "ok")
					pathHdr := mapResult.Path
					if pathHdr == "" {
						pathHdr = "(missing)"
					}
					if len(pathHdr) > 240 {
						pathHdr = pathHdr[:240]
					}
					embedExtra.Set("X-JRS-Embedded-Map-Path", pathHdr)
					s.logger.Info("embedded map", "report", reportName, "path", mapResult.Path)
				}
			}
			if mapResult.Query != "" {
				rawQuery = mapResult.Query
			}
		}
	}

	imageProxy := s.mountPath() + "/report-image"
	execReq := jasper.ExecuteRequest{
		Method:         r.Method,
		ReportName:     reportName,
		Format:         format,
		RawQuery:       rawQuery,
		Header:         r.Header.Clone(),
		ContentType:    r.Header.Get("Content-Type"),
		ImageProxyBase: imageProxy,
	}
	if replaceBody != nil {
		execReq.Body = replaceBody
		if replaceCT != "" {
			execReq.ContentType = replaceCT
		}
	} else if r.Method == http.MethodPost {
		execReq.Body = r.Body
	}

	// Provider reports are prepared in Go (collection rows or a DuckDB JDBC
	// session) before Tomcat fill. Ordinary JDBC/SQL reports fall through.
	if used, method, outQuery, ct, body, cerr := s.prepareReportProviderFill(ctx, reportName, rawQuery); used {
		if cerr != nil {
			s.writeGatewayError(w, jasper.ErrBadRequest(cerr.Error()), start, r, user, reportName, format)
			return
		}
		execReq.Method = method
		execReq.RawQuery = outQuery
		execReq.ContentType = ct
		execReq.Body = body
	}

	// Component export is armed only by handleEmbedReportComponent's request
	// context. The marker header is applied in the gateway from EmbedComponent.
	// Every other caller, including /reports/execute, loses _componentId here
	// even if the browser or a rewritten body put it on the query.
	if id, ok := trustedEmbedComponentID(r.Context()); ok {
		execReq.RawQuery = sealEmbedComponentQuery(execReq.RawQuery, id)
		execReq.EmbedComponent = true
	} else {
		execReq.RawQuery = stripEmbedComponentQuery(execReq.RawQuery)
	}

	resp, err := s.deps.Gateway.ExecuteReport(ctx, execReq)
	if err != nil {
		s.writeGatewayError(w, err, start, r, user, reportName, format)
		return
	}
	defer resp.Close()

	bytesWritten, status := s.streamProxyResponse(w, resp, embedExtra)
	s.recordReportExecution(r, reportName, format, audit.StatusSuccess, start, int64(bytesWritten), "")
	jasper.LogRequest(s.logger, jasper.RequestLog{
		RequestID:  RequestIDFromContext(r.Context()),
		RemoteIP:   remoteHost(r.RemoteAddr),
		User:       user.DisplayName(),
		ReportName: reportName,
		Format:     format,
		Method:     r.Method,
		Path:       r.URL.Path,
		Status:     status,
		Bytes:      bytesWritten,
		Duration:   time.Since(start),
	})
}

func (s *Server) handleDevReportImage(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	user := auth.FromContext(r.Context())

	if r.Method != http.MethodGet {
		s.writeGatewayError(w, jasper.ErrBadRequest("method not allowed"), start, r, user, "", "image")
		return
	}

	q := r.URL.Query()
	reportName := jasper.ExtractReportName(q)
	if !jasper.IsValidReportKey(reportName) {
		// Match PHP report_image.php: invalid key → 403 with empty body semantics.
		s.writeGatewayError(w, jasper.ErrForbidden("Invalid report."), start, r, user, reportName, "image")
		return
	}

	if err := s.authorizeReportView(r.Context(), user, reportName); err != nil {
		s.writeGatewayError(w, err, start, r, user, reportName, "image")
		return
	}
	if err := s.enforceEmbedContentScope(r, reportName); err != nil {
		s.writeGatewayError(w, jasper.ErrForbidden(err.Error()), start, r, user, reportName, "image")
		return
	}

	if s.deps.Gateway == nil {
		s.writeGatewayError(w, jasper.ErrUnavailable("Jasper image URL is not configured", nil), start, r, user, reportName, "image")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.deps.Gateway.Timeout())
	defer cancel()

	resp, err := s.deps.Gateway.FetchReportImage(ctx, jasper.ImageRequest{
		ReportName: reportName,
		RawQuery:   r.URL.RawQuery,
		Header:     r.Header.Clone(),
	})
	if err != nil {
		s.writeGatewayError(w, err, start, r, user, reportName, "image")
		return
	}
	defer resp.Close()

	// Tomcat ReportImageServlet historically defaulted unknown extensions
	// (including .svg QR codes) to image/png. With X-Content-Type-Options:
	// nosniff the browser then refuses to render the bytes.
	if ct := contentTypeForReportImage(q.Get("image")); ct != "" {
		if resp.Header == nil {
			resp.Header = make(http.Header)
		}
		resp.Header.Set("Content-Type", ct)
	}

	bytesWritten, status := s.streamProxyResponse(w, resp, nil)
	s.recordReportExecution(r, reportName, "image", audit.StatusSuccess, start, int64(bytesWritten), "")
	jasper.LogRequest(s.logger, jasper.RequestLog{
		RequestID:  RequestIDFromContext(r.Context()),
		RemoteIP:   remoteHost(r.RemoteAddr),
		User:       user.DisplayName(),
		ReportName: reportName,
		Format:     "image",
		Method:     r.Method,
		Path:       r.URL.Path,
		Status:     status,
		Bytes:      bytesWritten,
		Duration:   time.Since(start),
	})
}

// authorizeReportView mirrors PHP jrs_require_report_runtime_permission for view:
// auth-disabled allows; missing ACL repository → 503; deny → 403 (anonymous Public may still pass).
func (s *Server) authorizeReportView(ctx context.Context, user *auth.User, reportName string) error {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return nil
	}
	if s.deps.ACL == nil {
		return jasper.ErrUnavailable("application database unavailable", nil)
	}
	if !s.deps.ACL.Configured() && !s.deps.ACL.AuthDisabled() {
		return jasper.ErrUnavailable("application database unavailable", nil)
	}

	ok, err := s.deps.ACL.CanViewReport(ctx, user, reportName)
	if err != nil {
		if errors.Is(err, acl.ErrDatabase) {
			return jasper.ErrUnavailable("application database unavailable", err)
		}
		return jasper.Translate(err)
	}
	if !ok {
		return jasper.ErrForbidden("Permission denied")
	}
	return nil
}

func (s *Server) handleDevJasperStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	payload := map[string]any{
		"service": "acugis-report-server-go",
		"gateway": "not_configured",
	}

	if s.deps.Gateway == nil {
		s.writeJSON(w, http.StatusOK, payload)
		return
	}

	st := s.deps.Gateway.Status()
	connectivity := st.Connectivity
	if s.deps.Jasper != nil {
		h := s.deps.Jasper.CheckHealth(r.Context())
		switch h.Reachability {
		case jasper.ReachabilityReachable:
			if connectivity == "" || connectivity == "unknown" {
				connectivity = "reachable"
			}
		case jasper.ReachabilityUnreachable:
			connectivity = "unreachable"
		}
	}

	authDisabled := s.cfg.AuthDisabled
	if s.deps.Auth != nil {
		authDisabled = s.deps.Auth.AuthDisabled()
	}

	payload = map[string]any{
		"service":                              "acugis-report-server-go",
		"configured":                           st.Configured,
		"report_url":                           st.ReportURL,
		"image_url":                            st.ImageURL,
		"timeout":                              st.Timeout,
		"connectivity":                         connectivity,
		"last_successful_at":                   st.LastSuccessfulAt,
		"last_error_at":                        st.LastErrorAt,
		"last_error_summary":                   st.LastErrorSummary,
		"last_report_name":                     st.LastReportName,
		"last_format":                          st.LastFormat,
		"last_status_code":                     st.LastStatusCode,
		"authentication_repository_configured": s.deps.Auth != nil && s.deps.Auth.RepositoryConfigured(),
		"acl_repository_configured":            s.deps.ACL != nil && s.deps.ACL.Configured(),
		"auth_disabled":                        authDisabled,
	}
	s.writeJSON(w, http.StatusOK, payload)
}

func (s *Server) streamProxyResponse(w http.ResponseWriter, resp *jasper.ProxyResponse, extra http.Header) (int64, int) {
	dst := w.Header()
	for k, vals := range resp.Header {
		dst.Del(k)
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
	// Apply after Jasper headers so X-JRS-* diagnostics are not dropped.
	for k, vals := range extra {
		dst.Del(k)
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
	// Avoid caching proxied binary content via the global no-store default when
	// Jasper did not supply Cache-Control — keep middleware default.

	status := resp.StatusCode
	if status <= 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)

	written, err := io.Copy(w, resp.Body)
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn("stream jasper response",
			"error", err,
			"bytes", written,
		)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return written, status
}

func (s *Server) writeGatewayError(w http.ResponseWriter, err error, start time.Time, r *http.Request, user *auth.User, reportName, format string) {
	ge, ok := jasper.AsError(err)
	if !ok {
		ge = jasper.Translate(err)
	}
	status := ge.HTTPStatus()
	msg := ge.PublicMessage()
	if ge.Code == jasper.CodeUnavailable {
		msg = "Jasper engine unavailable. Start Tomcat (docker/report-server on :9099) or set JRS_JASPER_INTERNAL_REPORT_URL; check GET /dev/jasper-status. To use PHP execute from the Go viewer, set JRS_REPORT_ENDPOINT=/acugis-report-server/reports/execute_report.php"
	}

	if ge.Internal != nil {
		s.logger.Error("jasper gateway error",
			"request_id", RequestIDFromContext(r.Context()),
			"error_code", string(ge.Code),
			"error_summary", msg,
			"internal", jasper.FormatInternal(ge.Internal),
			"report_name", reportName,
			"format", format,
		)
	}

	jasper.LogRequest(s.logger, jasper.RequestLog{
		RequestID:    RequestIDFromContext(r.Context()),
		RemoteIP:     remoteHost(r.RemoteAddr),
		User:         user.DisplayName(),
		ReportName:   reportName,
		Format:       format,
		Method:       r.Method,
		Path:         r.URL.Path,
		Status:       status,
		Bytes:        0,
		Duration:     time.Since(start),
		ErrorCode:    string(ge.Code),
		ErrorSummary: msg,
	})

	if reportName != "" {
		s.recordReportExecution(r, reportName, format, audit.StatusFailure, start, 0, msg)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.Error(w, msg, status)
}

// isReportExecutePath reports whether path is the canonical /reports/execute
// route, after the mount prefix is removed. The legacy *.php alias is
// redirected or rewritten before the handler runs.
func (s *Server) isReportExecutePath(path string) bool {
	base := ""
	if s != nil {
		base = strings.TrimSuffix(s.cfg.BasePath, "/")
	}
	p := path
	if base != "" && base != "/" && strings.HasPrefix(p, base) {
		p = strings.TrimPrefix(p, base)
	}
	if p == "" {
		p = "/"
	}
	return p == "/reports/execute"
}

func (s *Server) mountPath() string {
	if s.cfg.BasePath == "/" || s.cfg.BasePath == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.BasePath, "/")
}

// contentTypeForReportImage maps Jasper HTML image names to MIME types.
// Empty means leave the upstream Content-Type unchanged.
func contentTypeForReportImage(image string) string {
	name := strings.ToLower(strings.TrimSpace(image))
	switch {
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".gif"):
		return "image/gif"
	default:
		return ""
	}
}
