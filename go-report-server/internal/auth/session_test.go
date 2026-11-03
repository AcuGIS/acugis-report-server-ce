package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

func TestRandomSessionTokenShape(t *testing.T) {
	tok, err := auth.RandomSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if strings.ContainsAny(tok, "+/=") {
		t.Fatalf("token should be base64url without padding: %q", tok)
	}
	hash, err := auth.HashSessionToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Fatalf("hash len=%d", len(hash))
	}
}

func TestSessionCookieFlags(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	expires := time.Now().Add(time.Hour)
	auth.SetSessionCookie(rec, req, "jrs_session", "tok-value", expires)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "jrs_session" || c.Value != "tok-value" {
		t.Fatalf("cookie=%+v", c)
	}
	if !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || !c.Secure {
		t.Fatalf("flags HttpOnly=%v Path=%s SameSite=%v Secure=%v", c.HttpOnly, c.Path, c.SameSite, c.Secure)
	}

	rec2 := httptest.NewRecorder()
	auth.ClearSessionCookie(rec2, req, "jrs_session")
	cleared := rec2.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 && cleared[0].Value != "" {
		// MaxAge -1 or empty value is fine; value must be empty.
		if len(cleared) != 1 || cleared[0].Value != "" {
			t.Fatalf("clear cookie=%+v", cleared)
		}
	}
}

func TestCookieSecure(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if auth.CookieSecure(req) {
		t.Fatal("plain HTTP should not be secure")
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	if !auth.CookieSecure(req) {
		t.Fatal("X-Forwarded-Proto=https should be secure")
	}
}
