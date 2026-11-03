package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/explorer"
)

func (s *Server) auditEnabled() bool {
	return s != nil && s.deps.Audit != nil && s.deps.Audit.Enabled()
}

func (s *Server) recordAudit(r *http.Request, e audit.Event) {
	if !s.auditEnabled() || r == nil {
		return
	}
	s.deps.Audit.RecordRequest(r, e)
}

func (s *Server) recordAuditLogin(r *http.Request, username string, success bool) {
	status := audit.StatusSuccess
	action := audit.ActionLoginSuccess
	if !success {
		status = audit.StatusFailure
		action = audit.ActionLoginFailed
	}
	e := audit.Event{
		Category:   audit.CategoryAuthentication,
		Action:     action,
		ObjectType: audit.ObjectSession,
		ObjectName: username,
		Username:   username,
		Status:     status,
	}
	s.recordAudit(r, e)
}

func (s *Server) recordAuditLogout(r *http.Request) {
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryAuthentication,
		Action:     audit.ActionLogout,
		ObjectType: audit.ObjectSession,
		Status:     audit.StatusSuccess,
	})
}

func (s *Server) recordReportExecution(r *http.Request, reportName, format, status string, start time.Time, outputSize int64, errMsg string) {
	action := audit.ExportFormatAction(format)
	details := map[string]any{}
	if s.deps.Audit != nil && s.deps.Audit.LogParameters() {
		if params := reportParamsFromRequest(r); len(params) > 0 {
			details["parameters"] = audit.SanitizeParams(params)
		}
	}
	if errMsg != "" {
		details["error"] = audit.SanitizeErrorMessage(errMsg)
	}
	if requestIsEmbedChannel(r) {
		details[audit.DetailsKeyChannel] = audit.DetailsChannelEmbed
	}
	var sizePtr *int64
	if outputSize > 0 {
		sizePtr = audit.Int64Ptr(outputSize)
	}
	e := audit.Event{
		Category:     audit.CategoryReport,
		Action:       action,
		ObjectType:   audit.ObjectReport,
		ObjectID:     reportName,
		ObjectName:   reportName,
		Status:       status,
		DurationMS:   audit.DurationMS(time.Since(start)),
		OutputFormat: strings.ToLower(strings.TrimSpace(format)),
		OutputSize:   sizePtr,
		Details:      details,
	}
	s.recordAudit(r, e)
}

// recordExplorerContentOpen records a meaningful Dashboard or View open.
// Call only for full content loads (not summary/meta plumbing requests).
func (s *Server) recordExplorerContentOpen(r *http.Request, view *explorer.SavedView, channel string) {
	if view == nil {
		return
	}
	view.NormalizeDashboardFlag()
	objType := audit.ObjectView
	if view.IsDashboard {
		objType = audit.ObjectDashboard
	}
	details := map[string]any{}
	if channel != "" {
		details[audit.DetailsKeyChannel] = channel
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategoryAnalysis,
		Action:     audit.ActionOpen,
		ObjectType: objType,
		ObjectID:   explorerViewRef(view),
		ObjectName: view.Name,
		Status:     audit.StatusSuccess,
		Details:    details,
	})
}

// recordEmbedTokenMint audits embed-token issuance without recording the token value.
func (s *Server) recordEmbedTokenMint(r *http.Request, contentType, contentID, origin string, success bool, errMsg string) {
	status := audit.StatusSuccess
	action := audit.ActionTokenSuccess
	if !success {
		status = audit.StatusFailure
		action = audit.ActionTokenFailed
	}
	objType := audit.ObjectToken
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "report", "map_report":
		objType = audit.ObjectReport
	case "dashboard":
		objType = audit.ObjectDashboard
	case "view":
		objType = audit.ObjectView
	}
	details := map[string]any{
		audit.DetailsKeyOperation: "embed_token_mint",
		audit.DetailsKeyChannel:   audit.DetailsChannelEmbed,
		"content_type":            strings.ToLower(strings.TrimSpace(contentType)),
	}
	if origin = strings.TrimSpace(origin); origin != "" {
		details[audit.DetailsKeyOrigin] = origin
	}
	if errMsg != "" {
		details["error"] = audit.SanitizeErrorMessage(errMsg)
	}
	s.recordAudit(r, audit.Event{
		Category:   audit.CategorySecurity,
		Action:     action,
		ObjectType: objType,
		ObjectID:   strings.TrimSpace(contentID),
		ObjectName: strings.TrimSpace(contentID),
		Status:     status,
		Details:    details,
	})
}

// recordEmbedContentView audits a successful embedded page serve.
func (s *Server) recordEmbedContentView(r *http.Request, objectType, objectID, objectName, origin string) {
	details := map[string]any{
		audit.DetailsKeyChannel: audit.DetailsChannelEmbed,
	}
	if origin = strings.TrimSpace(origin); origin != "" {
		details[audit.DetailsKeyOrigin] = origin
	}
	category := audit.CategoryReport
	action := audit.ActionReportView
	switch objectType {
	case audit.ObjectDashboard, audit.ObjectView:
		category = audit.CategoryAnalysis
		action = audit.ActionOpen
	}
	s.recordAudit(r, audit.Event{
		Category:   category,
		Action:     action,
		ObjectType: objectType,
		ObjectID:   objectID,
		ObjectName: objectName,
		Status:     audit.StatusSuccess,
		Details:    details,
	})
}

func reportParamsFromRequest(r *http.Request) map[string]any {
	if r == nil {
		return nil
	}
	q := r.URL.Query()
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if r.PostForm != nil {
			for k, vals := range r.PostForm {
				q[k] = vals
			}
		}
	}
	return queryToParamMap(q)
}

func queryToParamMap(q url.Values) map[string]any {
	if len(q) == 0 {
		return nil
	}
	skip := map[string]bool{
		"report": true, "reportName": true, "format": true, "output": true,
		"_acugisResourceBase": true, "et": true,
	}
	out := make(map[string]any)
	for k, vals := range q {
		if skip[k] || strings.HasPrefix(k, "_") {
			continue
		}
		if len(vals) == 1 {
			out[k] = vals[0]
		} else if len(vals) > 1 {
			cp := make([]any, len(vals))
			for i, v := range vals {
				cp[i] = v
			}
			out[k] = cp
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// requestIsEmbedChannel reports whether the request is embed-authenticated or on /embed/*.
func requestIsEmbedChannel(r *http.Request) bool {
	if r == nil {
		return false
	}
	path := r.URL.Path
	if strings.Contains(path, "/embed/") || strings.HasPrefix(path, "/api/v1/embed") {
		return true
	}
	if et := strings.TrimSpace(r.URL.Query().Get("et")); embedtoken.IsEmbedToken(et) {
		return true
	}
	if c, err := r.Cookie(embedTokenCookieName); err == nil && c != nil && embedtoken.IsEmbedToken(c.Value) {
		return true
	}
	if bearer, ok := auth.ParseBearerToken(r.Header.Get("Authorization")); ok && embedtoken.IsEmbedToken(bearer) {
		return true
	}
	return false
}
