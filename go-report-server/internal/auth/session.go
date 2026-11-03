package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

// SessionTTLSeconds matches PHP JRS_AUTH_SESSION_TTL_SECONDS (7 days).
const SessionTTLSeconds = 604800

// RandomSessionToken mirrors PHP jrs_auth_random_session_token:
// base64url of 32 random bytes without padding.
func RandomSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b[:]), "="), nil
}

// CookieSecure reports whether the session cookie should set Secure,
// matching PHP jrs_auth_set_session_cookie (HTTPS or X-Forwarded-Proto=https).
func CookieSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
	return proto == "https"
}

// SetSessionCookie writes the raw jrs_session cookie (PHP jrs_auth_set_session_cookie).
func SetSessionCookie(w http.ResponseWriter, r *http.Request, cookieName, token string, expiresAt time.Time) {
	if w == nil || token == "" {
		return
	}
	if cookieName == "" {
		cookieName = "jrs_session"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   CookieSecure(r),
	})
}

// ClearSessionCookie expires the session cookie (PHP jrs_auth_clear_session_cookie).
func ClearSessionCookie(w http.ResponseWriter, r *http.Request, cookieName string) {
	if w == nil {
		return
	}
	if cookieName == "" {
		cookieName = "jrs_session"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   CookieSecure(r),
	})
}
