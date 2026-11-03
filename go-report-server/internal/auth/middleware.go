package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Middleware is OptionalAuthentication with default options.
// Prefer OptionalAuthentication / RequireAuthentication for new wiring.
func Middleware(cookieName string) func(http.Handler) http.Handler {
	svc := NewService(nil, Options{CookieName: cookieName, AuthDisabled: false})
	return OptionalAuthentication(svc, MiddlewareOptions{})
}

// MiddlewareOptions configures auth middleware responses.
type MiddlewareOptions struct {
	Logger *slog.Logger
}

// OptionalAuthentication mirrors PHP jrs_auth_try_refresh_current_user:
// attach a user when the cookie is valid; continue anonymously when absent or
// invalid; return 503 on database dependency failure when a cookie was supplied.
func OptionalAuthentication(svc *Service, opts MiddlewareOptions) func(http.Handler) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if svc != nil && svc.AuthDisabled() {
				ctx := WithUser(r.Context(), LocalDevAdmin())
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if svc == nil {
				next.ServeHTTP(w, r)
				return
			}

			res := svc.AuthenticateRequest(r.Context(), r)
			switch res.Status {
			case StatusAuthDisabled:
				ctx := WithUser(r.Context(), LocalDevAdmin())
				next.ServeHTTP(w, r.WithContext(ctx))
			case StatusAuthenticated:
				ctx := WithUser(r.Context(), res.User)
				next.ServeHTTP(w, r.WithContext(ctx))
			case StatusDatabaseError:
				// Fail closed when a credential was supplied (cookie or Bearer).
				cookie, _ := r.Cookie(svc.CookieName())
				hasCookie := cookie != nil && cookie.Value != ""
				_, hasBearer := ParseBearerToken(r.Header.Get("Authorization"))
				if hasCookie || hasBearer {
					logger.Error("authentication database failure",
						"error", "database unavailable",
					)
					writeJSONError(w, http.StatusServiceUnavailable, "application database unavailable")
					return
				}
				next.ServeHTTP(w, r)
			default:
				// Missing, invalid, expired, disabled → anonymous (PHP try_refresh).
				next.ServeHTTP(w, r)
			}
		})
	}
}

// RequireAuthentication mirrors PHP jrs_require_login for API / development
// routes: 401 JSON when unauthenticated. Never redirects to a login page.
func RequireAuthentication(svc *Service, opts MiddlewareOptions) func(http.Handler) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if svc != nil && svc.AuthDisabled() {
				ctx := WithUser(r.Context(), LocalDevAdmin())
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if svc == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "application database unavailable")
				return
			}
			res := svc.AuthenticateRequest(r.Context(), r)
			switch res.Status {
			case StatusAuthDisabled, StatusAuthenticated:
				user := res.User
				if user == nil {
					user = LocalDevAdmin()
				}
				ctx := WithUser(r.Context(), user)
				next.ServeHTTP(w, r.WithContext(ctx))
			case StatusDatabaseError:
				logger.Error("authentication database failure", "error", "database unavailable")
				writeJSONError(w, http.StatusServiceUnavailable, "application database unavailable")
			default:
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
			}
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
