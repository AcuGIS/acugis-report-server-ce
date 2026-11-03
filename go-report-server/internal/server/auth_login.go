package server

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/audit"
	"github.com/acugis/acugis-report-server-go/internal/auth"
)

type loginSSOView struct {
	LocalEnabled bool
	ShowSSO      bool
	ShowOIDC     bool
	ShowSAML     bool
	OIDCLabel    string
	SAMLLabel    string
	OIDCLoginURL string
	SAMLLoginURL string
	SSOOnly      bool
}

func (s *Server) defaultLoginNext() string {
	return s.goURL("/reports/")
}

// safeLoginNext mirrors PHP jrs_login_safe_next: only same-origin absolute paths.
func safeLoginNext(next, defaultNext string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return defaultNext
	}
	return next
}

func (s *Server) loginURL() string {
	if u := strings.TrimSpace(s.cfg.CentralLoginURL); u != "" {
		return u
	}
	return s.goURL("/login")
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.qcartaProfile() || strings.TrimSpace(s.cfg.CentralLoginURL) != "" {
		s.handleCentralLoginRedirect(w, r)
		return
	}
	defaultNext := s.defaultLoginNext()
	next := safeLoginNext(r.URL.Query().Get("next"), defaultNext)
	errorMsg := strings.TrimSpace(r.URL.Query().Get("error"))
	breakGlass := r.URL.Query().Get("local") == "1"

	authDisabled := s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled())
	if authDisabled {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}

	user := auth.FromContext(r.Context())
	if auth.IsAuthenticated(r.Context()) && user != nil && user.ID > 0 {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			next = safeLoginNext(r.FormValue("next"), next)
		}
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}

	show := s.loginSSOState(r)
	showLocalForm := show.LocalEnabled || breakGlass
	if show.SSOOnly && !breakGlass && r.Method == http.MethodGet && show.ShowSSO && errorMsg == "" {
		// SSO-only primary UX: show page with SSO button (do not auto-bounce — allows break-glass link).
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		next = safeLoginNext(r.FormValue("next"), next)
		breakGlass = breakGlass || r.FormValue("local") == "1"

		if !show.LocalEnabled && !breakGlass {
			errorMsg = "Local authentication is disabled. Use SSO or break-glass recovery."
			s.recordAuditLogin(r, username, false)
		} else if username == "" || password == "" {
			errorMsg = "Username and password are required."
			s.recordAuditLogin(r, username, false)
		} else if s.deps.Auth == nil {
			errorMsg = "Invalid username or password."
			s.recordAuditLogin(r, username, false)
		} else if err := s.deps.Auth.Login(r.Context(), w, r, username, password); err != nil {
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				s.logger.Error("login failed", "error", err)
			}
			errorMsg = "Invalid username or password."
			s.recordAuditLogin(r, username, false)
		} else {
			if !show.LocalEnabled && breakGlass {
				s.recordAudit(r, audit.Event{
					Category:   audit.CategoryAuthentication,
					Action:     audit.ActionLocalBreakGlassLogin,
					ObjectType: audit.ObjectSession,
					ObjectName: username,
					Username:   username,
					Status:     audit.StatusSuccess,
				})
			}
			s.recordAuditLogin(r, username, true)
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		showLocalForm = true
	}

	s.renderLoginPage(w, next, errorMsg, loginPageOpts{
		ShowLocalForm: showLocalForm,
		ShowSSO:       show.ShowSSO,
		ShowOIDC:      show.ShowOIDC,
		ShowSAML:      show.ShowSAML,
		OIDCLabel:     show.OIDCLabel,
		SAMLLabel:     show.SAMLLabel,
		SSOOnly:       show.SSOOnly && !breakGlass,
		BreakGlass:    breakGlass || (!show.LocalEnabled && showLocalForm),
		OIDCLoginURL:  show.OIDCLoginURL + url.QueryEscape(next),
		SAMLLoginURL:  show.SAMLLoginURL + url.QueryEscape(next),
		BreakGlassURL: s.goURL("/login") + "?local=1&next=" + url.QueryEscape(next),
	})
}

type loginPageOpts struct {
	ShowLocalForm bool
	ShowSSO       bool
	ShowOIDC      bool
	ShowSAML      bool
	OIDCLabel     string
	SAMLLabel     string
	SSOOnly       bool
	BreakGlass    bool
	OIDCLoginURL  string
	SAMLLoginURL  string
	BreakGlassURL string
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.discardDesignerSessions(r)
	s.recordAuditLogout(r)
	var idpLogout string
	if s.deps.OIDC != nil {
		if u, ok := s.deps.OIDC.EndSessionURL(r.Context(), s.absoluteURLFromRequest(r, "/login")); ok {
			idpLogout = u
			s.recordAudit(r, audit.Event{
				Category:   audit.CategoryAuthentication,
				Action:     audit.ActionOIDCLogout,
				ObjectType: audit.ObjectSession,
				Status:     audit.StatusSuccess,
			})
		}
	}
	if s.deps.Auth != nil {
		s.deps.Auth.Logout(r.Context(), w, r)
	} else {
		auth.ClearSessionCookie(w, r, s.cfg.SessionCookieName)
	}
	if idpLogout != "" {
		http.Redirect(w, r, idpLogout, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.loginURL(), http.StatusSeeOther)
}

func (s *Server) handleAPIAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.cfg.AuthDisabled || (s.deps.Auth != nil && s.deps.Auth.AuthDisabled()) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "auth disabled"})
		return
	}

	// API login remains local-password only (CLI/SDK). Honor local_auth_enabled.
	if s.deps.OIDC != nil {
		if cfg, err := s.deps.OIDC.LoadSettings(r.Context()); err == nil && !cfg.LocalAuthEnabled {
			s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "local authentication is disabled"})
			return
		}
	}

	username := ""
	password := ""
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			username = strings.TrimSpace(body.Username)
			password = body.Password
		}
	} else {
		_ = r.ParseForm()
		username = strings.TrimSpace(r.FormValue("username"))
		password = r.FormValue("password")
	}

	if username == "" || password == "" {
		s.recordAuditLogin(r, username, false)
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	if s.deps.Auth == nil {
		s.recordAuditLogin(r, username, false)
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	if err := s.deps.Auth.Login(r.Context(), w, r, username, password); err != nil {
		s.recordAuditLogin(r, username, false)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
			return
		}
		s.logger.Error("api login failed", "error", err)
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	s.recordAuditLogin(r, username, true)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAPIAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.discardDesignerSessions(r)
	s.recordAuditLogout(r)
	if s.deps.Auth != nil {
		s.deps.Auth.Logout(r.Context(), w, r)
	} else {
		auth.ClearSessionCookie(w, r, s.cfg.SessionCookieName)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) renderLoginPage(w http.ResponseWriter, next, errorMsg string, opts loginPageOpts) {
	title := html.EscapeString(productName())
	safeError := html.EscapeString(errorMsg)
	safeNext := html.EscapeString(next)
	errorBlock := ""
	if errorMsg != "" {
		errorBlock = `<div class="error-banner">` + safeError + `</div>`
	}

	localBlock := ""
	if opts.ShowLocalForm {
		breakNote := ""
		localHidden := ""
		if opts.BreakGlass {
			breakNote = `<p class="login-subtitle" style="margin-bottom:16px;">Emergency local administrator sign-in. Requires an existing local admin account and password.</p>`
			localHidden = `<input type="hidden" name="local" value="1">`
		}
		localBlock = breakNote + `
        <form method="post" action="">
            <input type="hidden" name="next" value="` + safeNext + `">
            ` + localHidden + `
            <div class="input-field">
                <input id="username" name="username" type="text" required autocomplete="username" autofocus>
                <label for="username">Username</label>
            </div>
            <div class="input-field">
                <input id="password" name="password" type="password" required autocomplete="current-password">
                <label for="password">Password</label>
            </div>
            <button type="submit" class="btn waves-effect waves-light teal darken-1" style="width:100%;">
                Sign in
            </button>
        </form>`
	}

	divider := ""
	if opts.ShowLocalForm && opts.ShowSSO {
		divider = `<div class="login-divider"><span>or</span></div>`
	}

	ssoBlock := ""
	if opts.ShowOIDC {
		ssoBlock += `
        <a class="btn waves-effect waves-light blue darken-1" style="width:100%;margin-top:8px;" href="` + html.EscapeString(opts.OIDCLoginURL) + `">
            ` + html.EscapeString(opts.OIDCLabel) + `
        </a>`
	}
	if opts.ShowSAML {
		ssoBlock += `
        <a class="btn waves-effect waves-light indigo darken-1" style="width:100%;margin-top:8px;" href="` + html.EscapeString(opts.SAMLLoginURL) + `">
            ` + html.EscapeString(opts.SAMLLabel) + `
        </a>`
	}

	breakGlassLink := ""
	if opts.SSOOnly && !opts.BreakGlass {
		breakGlassLink = `<p class="break-glass"><a href="` + html.EscapeString(opts.BreakGlassURL) + `">Local administrator recovery</a></p>`
	}

	subtitle := "Sign in to manage reports and schedules."
	if opts.SSOOnly && !opts.ShowLocalForm {
		subtitle = "Sign in with your organization identity provider."
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Sign in - ` + title + `</title>
    <link href="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/css/materialize.min.css" rel="stylesheet">
    <link href="https://fonts.googleapis.com/icon?family=Material+Icons" rel="stylesheet">
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&display=swap" rel="stylesheet">
    <style>
        body {
            background: #f5f5f5;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            font-family: Roboto, sans-serif;
        }
        .login-card {
            width: 100%;
            max-width: 420px;
            margin: 24px;
            border-radius: 8px;
        }
        .login-brand {
            font-size: 1.35rem;
            font-weight: 500;
            color: #263238;
            margin-bottom: 8px;
        }
        .login-subtitle {
            color: #607d8b;
            margin-bottom: 24px;
        }
        .error-banner {
            background: #ffebee;
            color: #c62828;
            border-radius: 4px;
            padding: 12px 14px;
            margin-bottom: 20px;
        }
        .login-divider {
            display: flex;
            align-items: center;
            text-align: center;
            color: #90a4ae;
            margin: 20px 0 12px;
            font-size: 0.85rem;
        }
        .login-divider::before, .login-divider::after {
            content: "";
            flex: 1;
            border-bottom: 1px solid #cfd8dc;
        }
        .login-divider span { padding: 0 12px; }
        .break-glass {
            margin-top: 20px;
            text-align: center;
            font-size: 0.85rem;
        }
        .break-glass a { color: #607d8b; }
    </style>
</head>
<body>
<div class="card login-card">
    <div class="card-content">
        <div class="login-brand">` + title + `</div>
        <p class="login-subtitle">` + html.EscapeString(subtitle) + `</p>
        ` + errorBlock + `
        ` + localBlock + `
        ` + divider + `
        ` + ssoBlock + `
        ` + breakGlassLink + `
    </div>
</div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/materialize/1.0.0/js/materialize.min.js"></script>
<script>
document.addEventListener('DOMContentLoaded', function () {
    if (window.M) {
        M.updateTextFields();
    }
});
</script>
</body>
</html>`))
}
