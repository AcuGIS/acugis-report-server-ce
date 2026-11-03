package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// Designer integration headers. The browser must not be able to set the
// trusted values: the proxy overwrites them from the authenticated Report
// Server user and the report key in the public URL.
const (
	headerDesignerToken     = "X-AcuGIS-Designer-Internal-Token"
	headerDesignerUserID    = "X-AcuGIS-Designer-User-ID"
	headerDesignerUsername  = "X-AcuGIS-Designer-Username"
	headerDesignerReportKey = "X-AcuGIS-Designer-Report-Key"
	headerDesignerSession   = "X-AcuGIS-Designer-Session"
)

// Personal access tokens: OptionalAuthentication already accepts a Bearer PAT
// before the jrs_session cookie, and these routes use that same user. No
// Designer-specific scope is added. The internal Designer endpoints are not
// mounted on Report Server, so a PAT cannot call them directly.
//
// CSRF: Designer routes are same-origin on Report Server. The proxy does not
// add a second origin, does not emit CORS, and does not change the session
// cookie. This is the existing Report Server cookie model, not a new CSRF token.

var (
	errDesignerUnavailable  = &httpError{Status: http.StatusServiceUnavailable, Message: "Designer unavailable"}
	errNotDesignerEditable  = &httpError{Status: http.StatusConflict, Message: "Report is not Designer-editable."}
	errDesignerBodyTooLarge = &httpError{Status: http.StatusRequestEntityTooLarge, Message: "request body too large"}
	designerSessionID       = regexp.MustCompile(`^[0-9a-f]{32}$`)
	designerWindowsPath     = regexp.MustCompile(`[A-Za-z]:\\`)
	designerUnixPath        = regexp.MustCompile(`(?:^|[\s"'=])/(?:tmp|var|home|Users|etc|opt|usr|root)/`)
	designerSecretAssign    = regexp.MustCompile(`(?i)(password|passwd|pwd|passphrase|token|secret|api_key|apikey)\s*[=:]\s*\S+`)
	designerPEM             = regexp.MustCompile(`-----BEGIN `)
	designerStack           = regexp.MustCompile(`(?i)(\.go:\d|goroutine \d|panic:|stack trace)`)
	designerCodeToken       = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	designerRequestIDSafe   = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

type designerOpenResult struct {
	ID     string `json:"id"`
	Reused bool   `json:"reused"`
}

func (s *Server) requireDesignerUser(r *http.Request, user *auth.User) error {
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		return errAuthRequired
	}
	if user == nil || !auth.IsAuthenticated(r.Context()) || user.ID <= 0 || !user.Enabled {
		return errAuthRequired
	}
	return nil
}

// authorizeDesignerEdit is the Edit Report policy: service edit AND report edit.
// Administrators pass through the existing Report Server ACL bypass.
func (s *Server) authorizeDesignerEdit(r *http.Request, user *auth.User, key string) error {
	if err := s.requireServiceEdit(r, user); err != nil {
		if user != nil && user.ID > 0 {
			s.logger.Info("Designer authorization denied", "user", user.DisplayName(), "report", key)
		}
		return err
	}
	ok, err := s.canEditReport(r, user, key)
	if err != nil {
		return err
	}
	if !ok && !s.isPlatformAdmin(user) {
		s.logger.Info("Designer authorization denied", "user", user.DisplayName(), "report", key)
		return errPermissionDenied
	}
	return nil
}

func (s *Server) handleDesignerPage(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeDesignerResult(w, r, err)
		return
	}
	if !s.allowDesignerRequest(w, r, false) {
		return
	}
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	if !jasper.IsValidReportKey(key) {
		s.writeDesignerResult(w, r, errReportInvalid)
		return
	}
	if s.deps.Reports == nil {
		s.writeDesignerResult(w, r, errStoreUnavailable)
		return
	}
	if _, err := s.deps.Reports.Get(key); err != nil {
		if errors.Is(err, reports.ErrNotFound) || errors.Is(err, reports.ErrInvalidKey) {
			s.writeDesignerResult(w, r, errReportNotFound)
			return
		}
		s.logger.Error("designer load report", "report", key)
		s.writeDesignerResult(w, r, errStoreUnavailable)
		return
	}
	native, err := s.designerNative(key)
	if err != nil {
		s.writeDesignerResult(w, r, err)
		return
	}
	if !native {
		ok, jerr := s.designerEditableJRXML(key)
		if jerr != nil {
			s.writeDesignerResult(w, r, jerr)
			return
		}
		if !ok {
			s.writeDesignerResult(w, r, errNotDesignerEditable)
			return
		}
	}
	if err := s.authorizeDesignerEdit(r, user, key); err != nil {
		s.writeDesignerResult(w, r, err)
		return
	}
	opened, err := s.openDesignerSession(r.Context(), user, key)
	if err != nil {
		if errors.Is(err, errNotDesignerEditable) {
			s.writeDesignerResult(w, r, errNotDesignerEditable)
			return
		}
		s.logger.Warn("Designer upstream unavailable", "user", user.DisplayName(), "report", key)
		s.writeDesignerResult(w, r, errDesignerUnavailable)
		return
	}
	s.logger.Info("Designer session opened", "user", user.DisplayName(), "report", key, "reused", opened.Reused)
	if wantsJSONResponse(r) {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"designer":  true,
			"reportKey": key,
			"sessionId": opened.ID,
			"reused":    opened.Reused,
		})
		return
	}
	reportName := key
	if def, err := s.deps.Reports.Get(key); err == nil && def != nil {
		if name := strings.TrimSpace(stringify(def.Raw["name"])); name != "" {
			reportName = name
		}
	}
	if err := s.renderDesignerSPA(w, key, opened.ID, reportName, s.designerFolderChoices(r, user)); err != nil {
		s.logger.Warn("Designer frontend unavailable", "user", user.DisplayName(), "report", key)
		s.writeDesignerResult(w, r, errDesignerUnavailable)
	}
}

func (s *Server) handleDesignerProxy(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requireDesignerUser(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	if !s.allowDesignerRequest(w, r, true) {
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE, HEAD")
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	if !jasper.IsValidReportKey(key) {
		s.writeAuthzJSON(w, errReportInvalid)
		return
	}
	if err := s.authorizeDesignerEdit(r, user, key); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	rest, ok := designerProxyRemainder(chi.URLParam(r, "*"))
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	pathSession := designerPathSession(rest)
	headerSession := strings.TrimSpace(r.Header.Get(headerDesignerSession))
	if pathSession != "" && headerSession != "" && pathSession != headerSession {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Session identifier mismatch"})
		return
	}
	forwardSession := pathSession
	if forwardSession == "" {
		forwardSession = headerSession
	}
	if r.ContentLength > s.designerMaxBody() {
		s.writeAuthzJSON(w, errDesignerBodyTooLarge)
		return
	}
	if !s.designerConfigured() {
		s.logger.Warn("Designer upstream unavailable", "user", user.DisplayName(), "report", key)
		s.writeAuthzJSON(w, errDesignerUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.designerTimeout())
	defer cancel()
	resp, err := s.designerRequest(ctx, r.Method, "/api/designer/"+rest, r.Body, r.Header.Get("Content-Type"), r.Header.Get("Accept"), user, key, forwardSession, filteredQuery(r))
	if err != nil {
		s.logger.Warn("Designer upstream unavailable", "user", user.DisplayName(), "report", key)
		s.writeAuthzJSON(w, errDesignerUnavailable)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		s.writeAuthzJSON(w, errDesignerUnavailable)
		return
	}
	if resp.StatusCode >= 400 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		s.writeDesignerUpstreamError(w, r, resp.StatusCode, buf)
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" && !strings.ContainsAny(cd, "\r\n") {
		w.Header().Set("Content-Disposition", cd)
	}
	for _, h := range []string{"X-Content-Type-Options", "Cache-Control", "Content-Security-Policy"} {
		if v := resp.Header.Get(h); v != "" && !strings.ContainsAny(v, "\r\n") {
			w.Header().Set(h, v)
		}
	}
	if resp.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}

func (s *Server) writeDesignerResult(w http.ResponseWriter, r *http.Request, err error) {
	if wantsJSONResponse(r) {
		s.writeAuthzJSON(w, err)
		return
	}
	s.writeAuthzHTML(w, r, err)
}

// designerUpstreamError is the structured JSON body Designer returns from
// writeError / writeSafeConnectionErr. Only these fields are read. Extra
// upstream fields are never copied to the browser.
type designerUpstreamError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Error    string `json:"error"`
	Category string `json:"category"`
	Errors   []struct {
		Line    int    `json:"line"`
		Column  int    `json:"column"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (s *Server) writeDesignerUpstreamError(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	body = s.scrubDesignerBody(body)
	var parsed designerUpstreamError
	parsedOK := json.Unmarshal(body, &parsed) == nil
	if !parsedOK {
		parsed = designerUpstreamError{}
	}
	switch parsed.Code {
	case "session_expired":
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired"})
	case "forbidden":
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Permission denied"})
	case "session_mismatch":
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Session identifier mismatch"})
	case "not_editable":
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": "Report is not Designer-editable."})
	case "session_not_found":
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "session expired"})
	case "unauthenticated":
		// Internal-token and Designer authentication failures stay generic.
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Designer unavailable"})
	default:
		// 501 is an intentional Designer policy response, such as datasource
		// administration staying in Report Server. Other 5xx responses are
		// forwarded only when they match the safe Designer API contract.
		// Transport failures never reach this function: designerRequest
		// returns an error and the caller writes Designer unavailable.
		if status >= 500 && status != http.StatusNotImplemented {
			if parsedOK {
				if out, ok := s.safeDesignerApplicationError(status, parsed); ok {
					s.writeJSON(w, status, out)
					return
				}
			}
			s.logTranslatedDesignerUpstream(r, status, parsed.Code)
			s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Designer unavailable"})
			return
		}
		msg := parsed.Error
		if msg == "" {
			msg = parsed.Message
		}
		if msg == "" {
			msg = "Designer request failed"
		}
		safe := s.scrubPublic(msg)
		out := map[string]any{
			"error":   safe,
			"message": safe,
		}
		if cat := strings.TrimSpace(parsed.Category); cat != "" && !strings.ContainsAny(cat, `\/`) {
			out["category"] = cat
		}
		if len(parsed.Errors) > 0 {
			diags := make([]map[string]any, 0, len(parsed.Errors))
			for _, d := range parsed.Errors {
				text := strings.TrimSpace(s.scrubPublic(d.Message))
				if text == "" {
					continue
				}
				item := map[string]any{"message": text}
				if d.Line > 0 {
					item["line"] = d.Line
				}
				if d.Column > 0 {
					item["column"] = d.Column
				}
				diags = append(diags, item)
			}
			if len(diags) > 0 {
				out["errors"] = diags
			}
		}
		s.writeJSON(w, status, out)
	}
}

// safeDesignerApplicationError forwards a Designer 5xx only when it matches
// the structured contract Designer already sanitizes. writeSafeConnectionErr
// is that contract for Connection Manager: 502 code=sql category=sql, 504
// code=timeout with a fixed message, and 503 code=not_available with a fixed
// message. Other entries are the fixed strings writeError / writeAPIError /
// writeMapErr already publish. Variable text is accepted only for the sql
// contract, and only after a second safety check. Arbitrary 5xx bodies are
// not copied.
func (s *Server) safeDesignerApplicationError(status int, parsed designerUpstreamError) (map[string]any, bool) {
	code := strings.TrimSpace(parsed.Code)
	msg := strings.TrimSpace(parsed.Message)
	if msg == "" {
		msg = strings.TrimSpace(parsed.Error)
	}
	msg = strings.TrimSpace(s.scrubPublic(msg))
	if code == "sql" && status == http.StatusBadGateway && strings.TrimSpace(parsed.Category) == "sql" && designerMessageSafe(msg) {
		return map[string]any{
			"code":     "sql",
			"error":    msg,
			"message":  msg,
			"category": "sql",
		}, true
	}
	if !knownDesignerFixedError(status, code, msg) {
		return nil, false
	}
	return map[string]any{
		"code":    code,
		"error":   msg,
		"message": msg,
	}, true
}

func knownDesignerFixedError(status int, code, msg string) bool {
	switch code {
	case "timeout":
		return status == http.StatusGatewayTimeout && (msg == "The database did not respond in time" || msg == "The map service did not respond in time")
	case "not_available":
		return status == http.StatusServiceUnavailable && msg == "Report Server datasources are unavailable"
	case "storage_error":
		return status == http.StatusInternalServerError && msg == "Could not read or write the report"
	case "runtime":
		return status == http.StatusInternalServerError && (msg == "Request failed" || msg == "Could not start export" || msg == "Jasper runtime is not available" || msg == "Could not prepare the export")
	case "validation_error":
		return status == http.StatusBadGateway && msg == "authentication failed: credentials required or invalid"
	default:
		return false
	}
}

func (s *Server) logTranslatedDesignerUpstream(r *http.Request, status int, code string) {
	path := ""
	requestID := ""
	if r != nil {
		if r.URL != nil {
			path = r.URL.Path
		}
		requestID = safeDesignerRequestID(RequestIDFromContext(r.Context()))
	}
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn("Designer upstream error translated",
		"upstream_status", status,
		"code", safeDesignerLogCode(code),
		"path", path,
		"request_id", requestID,
	)
}

func (s *Server) openDesignerSession(ctx context.Context, user *auth.User, key string) (designerOpenResult, error) {
	if !s.designerConfigured() {
		return designerOpenResult{}, errDesignerUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, s.designerTimeout())
	defer cancel()
	resp, err := s.designerRequest(callCtx, http.MethodPost, "/internal/designer/open", nil, "", "", user, key, "", "")
	if err != nil {
		return designerOpenResult{}, errDesignerUnavailable
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusConflict {
		return designerOpenResult{}, errNotDesignerEditable
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return designerOpenResult{}, errDesignerUnavailable
	}
	var out struct {
		ID        string `json:"id"`
		ReportKey string `json:"reportKey"`
		Reused    bool   `json:"reused"`
	}
	if err := json.Unmarshal(buf, &out); err != nil || !designerSessionID.MatchString(out.ID) || out.ReportKey != key {
		return designerOpenResult{}, errDesignerUnavailable
	}
	return designerOpenResult{ID: out.ID, Reused: out.Reused}, nil
}

// discardDesignerSessions tells the loopback Designer to drop this user's
// sessions. Logout still succeeds when Designer is down; the public proxy
// will not forward later requests without a Report Server session.
func (s *Server) discardDesignerSessions(r *http.Request) {
	if s == nil || r == nil || !s.designerConfigured() || !s.designerEntitled() {
		return
	}
	user := auth.FromContext(r.Context())
	if user == nil || user.ID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	resp, err := s.designerRequest(ctx, http.MethodPost, "/internal/designer/logout", nil, "", "", user, "", "", "")
	if err != nil {
		s.logger.Warn("Designer upstream unavailable", "user", user.DisplayName())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
}

func (s *Server) designerRequest(ctx context.Context, method, path string, body io.Reader, contentType, accept string, user *auth.User, reportKey, sessionID, rawQuery string) (*http.Response, error) {
	base, err := config.ParseLoopbackURL(s.cfg.Designer.Upstream)
	if err != nil || strings.TrimSpace(s.cfg.Designer.Token) == "" {
		return nil, errDesignerUnavailable
	}
	if body != nil {
		body = io.LimitReader(body, s.designerMaxBody())
	}
	u := *base
	u.Path = path
	u.RawQuery = rawQuery
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, errDesignerUnavailable
	}
	req.Header.Set(headerDesignerToken, s.cfg.Designer.Token)
	if user != nil && user.ID > 0 {
		req.Header.Set(headerDesignerUserID, strconv.FormatInt(user.ID, 10))
		req.Header.Set(headerDesignerUsername, sanitizeHeader(user.Username))
	}
	if reportKey != "" {
		req.Header.Set(headerDesignerReportKey, reportKey)
	}
	if sessionID != "" {
		req.Header.Set(headerDesignerSession, sessionID)
	}
	if contentType != "" && !strings.ContainsAny(contentType, "\r\n") {
		req.Header.Set("Content-Type", contentType)
	}
	if accept != "" && !strings.ContainsAny(accept, "\r\n") {
		req.Header.Set("Accept", accept)
	}
	client := &http.Client{
		Timeout: s.designerTimeout(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client.Do(req)
}

func (s *Server) designerConfigured() bool {
	if s == nil || strings.TrimSpace(s.cfg.Designer.Token) == "" || strings.TrimSpace(s.cfg.Designer.Upstream) == "" {
		return false
	}
	_, err := config.ParseLoopbackURL(s.cfg.Designer.Upstream)
	return err == nil
}

func (s *Server) designerTimeout() time.Duration {
	if s != nil && s.cfg.Designer.Timeout > 0 {
		return s.cfg.Designer.Timeout
	}
	return 120 * time.Second
}

func (s *Server) designerMaxBody() int64 {
	if s != nil && s.cfg.Designer.MaxBody > 0 {
		return s.cfg.Designer.MaxBody
	}
	return 64 << 20
}

func (s *Server) designerNative(key string) (bool, error) {
	if !jasper.IsValidReportKey(key) {
		return false, errReportInvalid
	}
	dir := strings.TrimSpace(s.cfg.Paths.JRXMLDir)
	if dir == "" {
		return false, errStoreUnavailable
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return false, errReportInvalid
	}
	name := key + ".project.json"
	full := filepath.Join(root, name)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.ContainsRune(rel, filepath.Separator) {
		return false, errReportInvalid
	}
	st, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errStoreUnavailable
	}
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > 32<<20 {
		return false, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return false, errStoreUnavailable
	}
	return jsonObject(data), nil
}

// designerEditableJRXML reports whether the published report has a JRXML
// design document. A compiled Jasper file without JRXML is not editable.
func (s *Server) designerEditableJRXML(key string) (bool, error) {
	if !jasper.IsValidReportKey(key) {
		return false, errReportInvalid
	}
	dir := strings.TrimSpace(s.cfg.Paths.JRXMLDir)
	if dir == "" {
		return false, errStoreUnavailable
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return false, errReportInvalid
	}
	name := key + ".jrxml"
	full := filepath.Join(root, name)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.ContainsRune(rel, filepath.Separator) {
		return false, errReportInvalid
	}
	st, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errStoreUnavailable
	}
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > 32<<20 {
		return false, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return false, errStoreUnavailable
	}
	return editableJRXMLBytes(data), nil
}

func editableJRXMLBytes(data []byte) bool {
	if len(data) == 0 || len(data) > 32<<20 {
		return false
	}
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return false
	}
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	return bytes.Contains(bytes.ToLower(head), []byte("<jasperreport"))
}

func jsonObject(data []byte) bool {
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return false
	}
	var v any
	if err := json.Unmarshal(trim, &v); err != nil {
		return false
	}
	_, ok := v.(map[string]any)
	return ok
}

// designerProxyRemainder is the public Designer API suffix after
// /api/designer/{reportKey}/. The proxy forwards it unchanged as
// /api/designer/{suffix}. Session routes carry the session id in the path.
// Connection Manager and map discovery do not: the session id travels in
// X-AcuGIS-Designer-Session. Anything else, including /internal/designer, is
// rejected here and never forwarded.
func designerProxyRemainder(star string) (string, bool) {
	rest := strings.Trim(star, "/")
	if rest == "" || strings.Contains(rest, "..") || strings.ContainsAny(rest, "\\:") {
		return "", false
	}
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "" {
			return "", false
		}
	}
	switch parts[0] {
	case "sessions":
		if len(parts) < 2 || !designerSessionID.MatchString(parts[1]) {
			return "", false
		}
		return rest, true
	case "templates", "themes", "reports":
		// Catalog reads. Subpaths such as reports/import stay on Report Server
		// handlers, not this proxy.
		if len(parts) != 1 {
			return "", false
		}
		return rest, true
	case "connections":
		// List, create, update, delete, duplicate, test, schemas, tables,
		// columns, foreign keys, joins, preview, geometry, and ArcGIS browse.
		return rest, true
	case "maps":
		if len(parts) == 2 && parts[1] == "discover" {
			return rest, true
		}
		return "", false
	default:
		return "", false
	}
}

func designerPathSession(rest string) string {
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 && parts[0] == "sessions" && designerSessionID.MatchString(parts[1]) {
		return parts[1]
	}
	return ""
}

func filteredQuery(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	q := r.URL.Query()
	// path is a logical Designer resource (images/logo.png). It is not a
	// routing or filesystem instruction. Report identity stays in the URL and
	// the trusted report-key header.
	for _, k := range []string{"report", "reportKey", "report_key", "url", "target", "upstream"} {
		q.Del(k)
	}
	return q.Encode()
}

func sanitizeHeader(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	v = strings.TrimSpace(v)
	if len(v) > 200 {
		v = v[:200]
	}
	return v
}

func (s *Server) scrubDesignerBody(body []byte) []byte {
	return []byte(s.scrubPublic(string(body)))
}

func (s *Server) scrubPublic(msg string) string {
	if s != nil {
		if tok := s.cfg.Designer.Token; tok != "" {
			msg = strings.ReplaceAll(msg, tok, "[redacted]")
		}
		if up := strings.TrimSpace(s.cfg.Designer.Upstream); up != "" {
			msg = strings.ReplaceAll(msg, up, "[redacted]")
		}
	}
	msg = strings.ReplaceAll(msg, "127.0.0.1", "[redacted]")
	msg = strings.ReplaceAll(msg, "localhost", "[redacted]")
	return msg
}

// designerMessageSafe is the second check on a variable Connection Manager
// sql message. Designer writeSafeConnectionErr already redacts secrets and
// paths. Messages that still look like stacks, paths, or credentials are
// refused rather than forwarded.
func designerMessageSafe(msg string) bool {
	msg = strings.TrimSpace(msg)
	if msg == "" || len(msg) > 512 {
		return false
	}
	if strings.ContainsAny(msg, "\r\n\x00") || strings.Contains(msg, "../") || strings.Contains(msg, `..\`) {
		return false
	}
	if designerWindowsPath.MatchString(msg) || designerUnixPath.MatchString(msg) {
		return false
	}
	if designerSecretAssign.MatchString(msg) || designerPEM.MatchString(msg) || designerStack.MatchString(msg) {
		return false
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "jdbc:") || strings.Contains(lower, "x-acugis-designer-internal-token") {
		return false
	}
	return true
}

func safeDesignerLogCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "unstructured"
	}
	if !designerCodeToken.MatchString(code) {
		return "untrusted"
	}
	return code
}

func safeDesignerRequestID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if !designerRequestIDSafe.MatchString(id) {
		return "untrusted"
	}
	return id
}
