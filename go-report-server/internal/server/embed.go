package server

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embedtoken"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

// embedTokenResolver adapts config + auth into auth.EmbedTokenResolver.
type embedTokenResolver struct {
	secret string
}

func (r embedTokenResolver) ResolveEmbedToken(_ context.Context, raw string) (*auth.User, error) {
	claims, err := embedtoken.ParseAndValidate(r.secret, raw)
	if err != nil {
		return nil, err
	}
	return auth.NewEmbedUser(claims.UserID, claims.Username, string(claims.ContentType), claims.ContentID, claims.Origin, claims.Parameters), nil
}

type embedTokenRequest struct {
	ContentType string          `json:"content_type"`
	ContentID   string          `json:"content_id"`
	TTLSeconds  int             `json:"ttl_seconds"`
	Origin      string          `json:"origin"`
	Parameters  json.RawMessage `json:"parameters"`
}

type embedTokenResponse struct {
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
	ContentType string    `json:"content_type"`
	ContentID   string    `json:"content_id"`
}

// handleEmbedTokenMint issues a short-lived emb_ token. Requires session or PAT.
// Embedding the viewer ALWAYS requires the returned embed token — never a PAT.
func (s *Server) handleEmbedTokenMint(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.writeEmbedCORS(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.writeEmbedCORS(w, r)

	user := auth.FromContext(r.Context())
	if !auth.IsAuthenticated(r.Context()) && !(s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled())) {
		s.writeEmbedError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required to mint embed tokens")
		return
	}
	if strings.TrimSpace(s.cfg.EmbedTokenSecret) == "" {
		s.writeEmbedError(w, http.StatusServiceUnavailable, "SERVER_ERROR", "embed token signing is not configured")
		return
	}

	var req embedTokenRequest
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeEmbedError(w, http.StatusBadRequest, "INVALID_PARAMETER", "invalid request body")
		return
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			s.writeEmbedError(w, http.StatusBadRequest, "INVALID_PARAMETER", "invalid JSON body")
			return
		}
	}
	ct := embedtoken.ContentType(strings.TrimSpace(strings.ToLower(req.ContentType)))
	id := strings.TrimSpace(req.ContentID)
	if !embedtoken.ValidContentType(ct) || id == "" {
		s.recordEmbedTokenMint(r, req.ContentType, id, req.Origin, false, "content_type and content_id are required")
		s.writeEmbedError(w, http.StatusBadRequest, "INVALID_PARAMETER", "content_type and content_id are required")
		return
	}
	if req.Origin != "" && !s.embedOriginAllowed(req.Origin) {
		s.recordEmbedTokenMint(r, string(ct), id, req.Origin, false, "origin is not allowed for embedding")
		s.writeEmbedError(w, http.StatusForbidden, "EMBED_ORIGIN_DENIED", "origin is not allowed for embedding")
		return
	}

	bound, err := embedtoken.ParseMintParameters(req.Parameters)
	if err != nil {
		s.recordEmbedTokenMint(r, string(ct), id, req.Origin, false, err.Error())
		s.writeEmbedError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if err := s.authorizeEmbedContent(r, user, ct, id); err != nil {
		s.recordEmbedTokenMint(r, string(ct), id, req.Origin, false, err.Error())
		s.writeEmbedAuthz(w, err)
		return
	}

	ttl := s.cfg.EmbedTokenTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	username := ""
	var uid int64
	if user != nil {
		username = user.Username
		uid = user.ID
	}
	tok, exp, err := embedtoken.Issue(s.cfg.EmbedTokenSecret, embedtoken.Claims{
		UserID:      uid,
		Username:    username,
		ContentType: ct,
		ContentID:   id,
		Permission:  "view",
		Origin:      strings.TrimSpace(req.Origin),
		Parameters:  bound,
	}, ttl)
	if err != nil {
		s.logger.Error("mint embed token", "error", err)
		s.recordEmbedTokenMint(r, string(ct), id, req.Origin, false, "could not mint embed token")
		s.writeEmbedError(w, http.StatusInternalServerError, "SERVER_ERROR", "could not mint embed token")
		return
	}
	s.recordEmbedTokenMint(r, string(ct), id, req.Origin, true, "")
	s.writeJSON(w, http.StatusOK, embedTokenResponse{
		Token:       tok,
		ExpiresAt:   exp,
		ContentType: string(ct),
		ContentID:   id,
	})
}

func (s *Server) authorizeEmbedContent(r *http.Request, user *auth.User, ct embedtoken.ContentType, id string) error {
	switch ct {
	case embedtoken.ContentReport, embedtoken.ContentMapReport:
		key := jasper.NormalizeReportKey(id)
		def, err := s.loadAuthorizedReport(r, user, key)
		if err != nil {
			return err
		}
		if ct == embedtoken.ContentMapReport && !mapreport.IsMapReport(def) {
			return errReportNotFound
		}
		if ct == embedtoken.ContentMapReport {
			if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
				return err
			}
		}
		return nil
	case embedtoken.ContentDashboard, embedtoken.ContentView:
		if ct == embedtoken.ContentDashboard {
			if err := s.requireEditionFeature(edition.FeatureDashboards); err != nil {
				return err
			}
		}
		if ct == embedtoken.ContentView {
			if err := s.requireEditionFeature(edition.FeatureDataExplorer); err != nil {
				return err
			}
		}
		if s.deps.ACL == nil {
			if s.cfg.AuthDisabled {
				return nil
			}
			return errStoreUnavailable
		}
		ok, err := s.deps.ACL.CanViewExplorerView(r.Context(), user, id)
		if err != nil {
			return errStoreUnavailable
		}
		if !ok {
			return errPermissionDenied
		}
		return nil
	default:
		return errReportInvalid
	}
}

func (s *Server) handleEmbedReport(w http.ResponseWriter, r *http.Request) {
	s.serveEmbedReport(w, r, embedtoken.ContentReport, chi.URLParam(r, "key"))
}

func (s *Server) serveEmbedReport(w http.ResponseWriter, r *http.Request, ct embedtoken.ContentType, rawKey string) {
	key := jasper.NormalizeReportKey(rawKey)
	claims, user, err := s.requireEmbedToken(w, r, ct, key)
	if err != nil {
		s.writeEmbedAuthzHTML(w, err)
		return
	}
	_ = claims
	r = r.WithContext(auth.WithUser(r.Context(), user))
	s.renderViewPageEmbed(w, r, key)
}

// requireEmbedToken enforces that embedding ALWAYS uses a valid emb_ token
// scoped to the requested content. Session/PAT alone are not accepted on /embed/*.
func (s *Server) requireEmbedToken(w http.ResponseWriter, r *http.Request, ct embedtoken.ContentType, contentID string) (embedtoken.Claims, *auth.User, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("et"))
	if raw == "" {
		if bearer, ok := auth.ParseBearerToken(r.Header.Get("Authorization")); ok {
			raw = bearer
		}
	}
	if raw == "" {
		return embedtoken.Claims{}, nil, &httpError{Status: http.StatusUnauthorized, Message: "AUTH_REQUIRED: embed token required"}
	}
	if strings.HasPrefix(raw, auth.PersonalAccessTokenPrefix) {
		return embedtoken.Claims{}, nil, &httpError{Status: http.StatusUnauthorized, Message: "AUTH_REQUIRED: personal access tokens cannot be used for embedding; mint an emb_ token via POST /api/v1/embed/token"}
	}
	if !embedtoken.IsEmbedToken(raw) {
		return embedtoken.Claims{}, nil, &httpError{Status: http.StatusUnauthorized, Message: "TOKEN_INVALID: expected emb_ embed token"}
	}
	if strings.TrimSpace(s.cfg.EmbedTokenSecret) == "" {
		return embedtoken.Claims{}, nil, &httpError{Status: http.StatusServiceUnavailable, Message: "SERVER_ERROR: embed tokens not configured"}
	}
	claims, err := embedtoken.ParseAndValidate(s.cfg.EmbedTokenSecret, raw)
	if err != nil {
		if errors.Is(err, embedtoken.ErrExpiredToken) {
			return embedtoken.Claims{}, nil, &httpError{Status: http.StatusUnauthorized, Message: "TOKEN_EXPIRED: embed token expired"}
		}
		return embedtoken.Claims{}, nil, &httpError{Status: http.StatusUnauthorized, Message: "TOKEN_INVALID: invalid embed token"}
	}
	if err := embedtoken.MatchContent(claims, ct, contentID); err != nil {
		return embedtoken.Claims{}, nil, &httpError{Status: http.StatusForbidden, Message: "ACCESS_DENIED: token is not scoped to this content"}
	}
	if claims.Origin != "" {
		// Optional origin binding on the token.
		reqOrigin := strings.TrimSpace(r.Header.Get("Origin"))
		if reqOrigin == "" {
			if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
				// best-effort: leave enforcement to CSP frame-ancestors
			}
		} else if !strings.EqualFold(reqOrigin, claims.Origin) && !s.embedOriginAllowed(reqOrigin) {
			return embedtoken.Claims{}, nil, &httpError{Status: http.StatusForbidden, Message: "EMBED_ORIGIN_DENIED"}
		}
	}
	user := auth.NewEmbedUser(claims.UserID, claims.Username, string(claims.ContentType), claims.ContentID, claims.Origin, claims.Parameters)
	// Still enforce live ACL — token alone is not a permanent grant.
	if err := s.authorizeEmbedContent(r.WithContext(auth.WithUser(r.Context(), user)), user, ct, contentID); err != nil {
		return embedtoken.Claims{}, nil, err
	}
	// Persist token for same-origin fetches/iframes inside the embed (not for host pages).
	s.setEmbedTokenCookie(w, r, raw)
	return claims, user, nil
}

const embedTokenCookieName = "jrs_embed_token"

func (s *Server) setEmbedTokenCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := auth.CookieSecure(r)
	sameSite := http.SameSiteNoneMode
	if !secure {
		// Browsers reject SameSite=None without Secure; Lax is fine for http://localhost.
		sameSite = http.SameSiteLaxMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     embedTokenCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.cfg.EmbedTokenTTL.Seconds()),
		HttpOnly: true,
		SameSite: sameSite,
		Secure:   secure,
	})
}

func (s *Server) writeEmbedCORS(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || !s.embedOriginAllowed(origin) {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Vary", "Origin")
}

func (s *Server) writeEmbedError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]string{"code": code, "error": message, "message": message})
}

func (s *Server) writeEmbedAuthz(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		code := "SERVER_ERROR"
		switch he.Status {
		case http.StatusUnauthorized:
			code = "AUTH_REQUIRED"
		case http.StatusForbidden:
			code = "ACCESS_DENIED"
		case http.StatusNotFound:
			code = "CONTENT_NOT_FOUND"
		case http.StatusBadRequest:
			code = "INVALID_PARAMETER"
		}
		s.writeEmbedError(w, he.Status, code, he.Message)
		return
	}
	s.writeEmbedError(w, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
}

func (s *Server) writeEmbedAuthzHTML(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		http.Error(w, he.Message, he.Status)
		return
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func (s *Server) renderViewPageEmbed(w http.ResponseWriter, r *http.Request, key string) {
	user := auth.FromContext(r.Context())
	def, err := s.loadAuthorizedReport(r, user, key)
	if err != nil {
		s.writeEmbedAuthzHTML(w, err)
		return
	}
	if mapreport.IsMapReport(def) {
		if err := s.requireEditionFeature(edition.FeatureMapReports); err != nil {
			s.writeEmbedAuthzHTML(w, err)
			return
		}
	}
	ds := strings.TrimSpace(r.URL.Query().Get("datasource"))
	if ds == "" {
		ds = def.Datasource()
	}
	isMap := mapreport.IsMapReport(def)
	reportEndpoint := s.browserReportEndpoint()
	chrome := s.materialChrome(def.DisplayName())
	boundNames, _ := json.Marshal(boundParameterNames(embedParameters(r)))
	data := viewPageData{
		materialChromeData:     chrome,
		IsMap:                  isMap,
		ReportJS:               mustJSON(def.Key),
		DatasourceJS:           mustJSON(ds),
		ReportEndpointJS:       mustJSON(reportEndpoint),
		IsMapJS:                template.JS(boolJS(isMap)),
		CloseURL:               "",
		EmbedMode:              true,
		EmbedAllowedOriginsJS:  s.embedAllowedOriginsJS(),
		EmbedBoundParametersJS: template.JS(boundNames),
		EmbedToolbar:           queryFlagDefaultTrue(r, "toolbar"),
		EmbedParameters:        queryFlagDefaultTrue(r, "parameters"),
		EmbedExport:            queryFlagDefaultTrue(r, "export"),
		EmbedZoom:              queryFlagDefaultTrue(r, "zoom"),
		EmbedClose:             queryFlagDefaultFalse(r, "close"),
		EmbedAutoResize:        queryFlagDefaultFalse(r, "autoResize"),
		ReportsBaseURL:         s.goURL("/reports"),
		MapReportCSP:           mapReportCSP(isMap),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := viewTemplate.Execute(w, data); err != nil {
		s.logger.Error("render embed view", "error", err)
		return
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	s.recordEmbedContentView(r, audit.ObjectReport, key, def.DisplayName(), origin)
}

func queryFlagDefaultTrue(r *http.Request, name string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return true
	}
	return !(v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "off"))
}

func queryFlagDefaultFalse(r *http.Request, name string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on")
}

func (s *Server) handleSDKAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(chi.URLParam(r, "file"))
	if name == "" {
		// /sdk/v1/{file}
		name = strings.TrimSpace(chi.URLParam(r, "*"))
	}
	name = filepath.Base(name)
	switch name {
	case "acugis-report.js", "acugis-report.min.js", "acugis-report.esm.js", "index.d.ts":
	default:
		http.NotFound(w, r)
		return
	}
	candidates := []string{
		filepath.Join(s.cfg.Paths.AppRoot, "sdk", "javascript", "dist", name),
		filepath.Join(s.cfg.Paths.AppRoot, "go-report-server", "internal", "server", "sdkassets", name),
	}
	// Also try relative to this module when running from go-report-server.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "sdkassets", name),
			filepath.Join(wd, "..", "..", "sdk", "javascript", "dist", name),
			filepath.Join(wd, "..", "sdk", "javascript", "dist", name),
		)
	}
	var path string
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			path = c
			break
		}
	}
	if path == "" {
		http.Error(w, "SDK asset not found — run npm run build in sdk/javascript", http.StatusNotFound)
		return
	}
	switch {
	case strings.HasSuffix(name, ".min.js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".esm.js"), strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".d.ts"):
		w.Header().Set("Content-Type", "text/typescript; charset=utf-8")
	}
	if strings.Contains(r.URL.Path, "/sdk/v1/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	http.ServeFile(w, r, path)
}

// isEmbedPath reports whether the request targets an embed or embed-API route.
func (s *Server) isEmbedPath(path string) bool {
	base := strings.TrimSuffix(s.cfg.BasePath, "/")
	p := path
	if base != "" && base != "/" && strings.HasPrefix(p, base) {
		p = strings.TrimPrefix(p, base)
	}
	return strings.HasPrefix(p, "/embed/") || strings.HasPrefix(p, "/api/v1/embed")
}
