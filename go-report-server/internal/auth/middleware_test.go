package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

type mockResolver struct {
	configured bool
	user       *auth.User
	err        error
	lastHash   string
}

func (m *mockResolver) Configured() bool { return m.configured }

func (m *mockResolver) ResolveUserFromSession(_ context.Context, tokenHash string) (*auth.User, error) {
	m.lastHash = tokenHash
	if m.err != nil {
		return nil, m.err
	}
	return m.user, nil
}

func TestOptionalAuthenticationAnonymous(t *testing.T) {
	repo := &mockResolver{configured: true}
	svc := auth.NewService(repo, auth.Options{CookieName: "jrs_session"})
	var got *auth.User
	h := auth.OptionalAuthentication(svc, auth.MiddlewareOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got == nil || !got.Anonymous() {
		t.Fatalf("user=%+v", got)
	}
}

func TestOptionalAuthenticationValidSession(t *testing.T) {
	repo := &mockResolver{
		configured: true,
		user: &auth.User{ID: 7, Username: "alice", Email: "a@example.com", Enabled: true, Groups: []string{"Editors"}},
	}
	svc := auth.NewService(repo, auth.Options{CookieName: "jrs_session"})
	var got *auth.User
	h := auth.OptionalAuthentication(svc, auth.MiddlewareOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "jrs-test-token-not-a-secret"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got == nil || got.Username != "alice" || got.ID != 7 {
		t.Fatalf("user=%+v", got)
	}
	wantHash, _ := auth.HashSessionToken("jrs-test-token-not-a-secret")
	if repo.lastHash != wantHash {
		t.Fatalf("hash=%s want=%s", repo.lastHash, wantHash)
	}
}

func TestOptionalAuthenticationInvalidContinuesAnonymous(t *testing.T) {
	repo := &mockResolver{configured: true, err: auth.ErrInvalidSession}
	svc := auth.NewService(repo, auth.Options{CookieName: "jrs_session"})
	var got *auth.User
	var authenticated bool
	h := auth.OptionalAuthentication(svc, auth.MiddlewareOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
		authenticated = auth.IsAuthenticated(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "unknown-token"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if authenticated || (got != nil && !got.Anonymous()) {
		t.Fatalf("expected anonymous, got %+v authenticated=%v", got, authenticated)
	}
}

func TestOptionalAuthenticationDatabaseFailureWithCookie(t *testing.T) {
	repo := &mockResolver{configured: true, err: auth.ErrDatabase}
	svc := auth.NewService(repo, auth.Options{CookieName: "jrs_session"})
	h := auth.OptionalAuthentication(svc, auth.MiddlewareOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "any-token"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "application database unavailable" {
		t.Fatalf("body=%v", body)
	}
}

func TestRequireAuthenticationUnauthorized(t *testing.T) {
	repo := &mockResolver{configured: true, err: auth.ErrInvalidSession}
	svc := auth.NewService(repo, auth.Options{CookieName: "jrs_session"})
	h := auth.RequireAuthentication(svc, auth.MiddlewareOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "bad"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRequireAuthenticationAuthDisabled(t *testing.T) {
	svc := auth.NewService(nil, auth.Options{AuthDisabled: true})
	var got *auth.User
	h := auth.RequireAuthentication(svc, auth.MiddlewareOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if got == nil || got.Username != "local-dev" || !got.IsAdmin {
		t.Fatalf("user=%+v", got)
	}
}

func TestServiceExpiredAndDisabled(t *testing.T) {
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "tok"})

	expired := auth.NewService(&mockResolver{configured: true, err: auth.ErrExpiredSession}, auth.Options{})
	res := expired.AuthenticateRequest(ctx, req)
	if res.Status != auth.StatusExpired {
		t.Fatalf("expired status=%s", res.Status)
	}

	disabled := auth.NewService(&mockResolver{configured: true, err: auth.ErrDisabledUser}, auth.Options{})
	res = disabled.AuthenticateRequest(ctx, req)
	if res.Status != auth.StatusDisabledUser {
		t.Fatalf("disabled status=%s", res.Status)
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	if auth.IsAuthenticated(ctx) {
		t.Fatal("expected unauthenticated")
	}
	u := &auth.User{ID: 1, Username: "bob", Enabled: true}
	ctx = auth.WithUser(ctx, u)
	got, ok := auth.UserFromContext(ctx)
	if !ok || got.Username != "bob" {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
	if !auth.IsAuthenticated(ctx) {
		t.Fatal("expected authenticated")
	}
}

func TestGroupsIncludeAdmin(t *testing.T) {
	if !auth.GroupsIncludeAdmin([]string{"Administrators"}) {
		t.Fatal("Administrators")
	}
	if auth.GroupsIncludeAdmin([]string{"Editors", "Public"}) {
		t.Fatal("non-admin")
	}
}

func TestIsAuthError(t *testing.T) {
	if !auth.IsAuthError(auth.ErrNoSession) {
		t.Fatal("ErrNoSession")
	}
	if auth.IsAuthError(errors.New("other")) {
		t.Fatal("other")
	}
}
