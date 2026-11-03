package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/auth"
)

type memoryTokenStore struct {
	byHash map[string]memoryPAT
	byID   map[int64]memoryPAT
	nextID int64
	users  map[int64]*auth.User
}

type memoryPAT struct {
	token    auth.PersonalAccessToken
	hash     string
	lastUsed *time.Time
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{
		byHash: map[string]memoryPAT{},
		byID:   map[int64]memoryPAT{},
		nextID: 1,
		users: map[int64]*auth.User{
			7: {ID: 7, Username: "pat-user", Email: "pat@example.com", Enabled: true, Groups: []string{"Editors"}},
			1: {ID: 1, Username: "cookie-user", Enabled: true},
		},
	}
}

func (s *memoryTokenStore) Configured() bool { return true }

func (s *memoryTokenStore) ResolveUserFromSession(_ context.Context, tokenHash string) (*auth.User, error) {
	if tokenHash == "" {
		return nil, auth.ErrInvalidSession
	}
	// Session "session-raw-token" hashed maps to cookie-user when present in test helper.
	want, _ := auth.HashSessionToken("session-raw-token")
	if tokenHash == want {
		return s.users[1], nil
	}
	return nil, auth.ErrInvalidSession
}

func (s *memoryTokenStore) CreatePAT(_ context.Context, tokenHash, tokenPrefix string, input auth.PersonalAccessTokenCreate) (auth.PersonalAccessToken, error) {
	id := s.nextID
	s.nextID++
	now := time.Now().UTC()
	token := auth.PersonalAccessToken{
		ID:          id,
		UserID:      input.UserID,
		Name:        input.Name,
		Description: input.Description,
		TokenPrefix: tokenPrefix,
		Scopes:      append([]string(nil), input.Scopes...),
		ExpiresAt:   input.ExpiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedByIP: input.CreatedByIP,
		Active:      true,
	}
	if token.Scopes == nil {
		token.Scopes = []string{}
	}
	rec := memoryPAT{token: token, hash: tokenHash}
	s.byHash[tokenHash] = rec
	s.byID[id] = rec
	return token, nil
}

func (s *memoryTokenStore) ListPATsByUser(_ context.Context, userID int64) ([]auth.PersonalAccessToken, error) {
	out := make([]auth.PersonalAccessToken, 0)
	for _, rec := range s.byID {
		if rec.token.UserID == userID && rec.token.RevokedAt == nil {
			out = append(out, rec.token)
		}
	}
	return out, nil
}

func (s *memoryTokenStore) GetPAT(_ context.Context, userID, tokenID int64) (auth.PersonalAccessToken, error) {
	rec, ok := s.byID[tokenID]
	if !ok || rec.token.UserID != userID {
		return auth.PersonalAccessToken{}, auth.ErrTokenNotFound
	}
	return rec.token, nil
}

func (s *memoryTokenStore) LookupActivePAT(_ context.Context, tokenHash string) (*auth.User, int64, *time.Time, error) {
	rec, ok := s.byHash[tokenHash]
	if !ok || rec.token.RevokedAt != nil {
		return nil, 0, nil, auth.ErrTokenNotFound
	}
	if rec.token.ExpiresAt != nil && !rec.token.ExpiresAt.After(time.Now()) {
		return nil, 0, nil, auth.ErrTokenNotFound
	}
	user := s.users[rec.token.UserID]
	if user == nil || !user.Enabled {
		return nil, 0, nil, auth.ErrTokenNotFound
	}
	return user, rec.token.ID, rec.lastUsed, nil
}

func (s *memoryTokenStore) UpdatePAT(_ context.Context, userID, tokenID int64, input auth.PersonalAccessTokenUpdate) (auth.PersonalAccessToken, error) {
	rec, ok := s.byID[tokenID]
	if !ok || rec.token.UserID != userID || rec.token.RevokedAt != nil {
		return auth.PersonalAccessToken{}, auth.ErrTokenNotFound
	}
	if input.Name != nil {
		rec.token.Name = *input.Name
	}
	if input.Description != nil {
		rec.token.Description = *input.Description
	}
	if input.SetExpires {
		rec.token.ExpiresAt = input.ExpiresAt
	}
	rec.token.UpdatedAt = time.Now().UTC()
	s.byID[tokenID] = rec
	s.byHash[rec.hash] = rec
	return rec.token, nil
}

func (s *memoryTokenStore) RevokePAT(_ context.Context, userID, tokenID int64) error {
	rec, ok := s.byID[tokenID]
	if !ok || rec.token.UserID != userID || rec.token.RevokedAt != nil {
		return auth.ErrTokenNotFound
	}
	now := time.Now().UTC()
	rec.token.RevokedAt = &now
	rec.token.Active = false
	s.byID[tokenID] = rec
	s.byHash[rec.hash] = rec
	return nil
}

func (s *memoryTokenStore) DeletePAT(_ context.Context, userID, tokenID int64) error {
	rec, ok := s.byID[tokenID]
	if !ok || rec.token.UserID != userID {
		return auth.ErrTokenNotFound
	}
	delete(s.byID, tokenID)
	delete(s.byHash, rec.hash)
	return nil
}

func (s *memoryTokenStore) TouchPATLastUsed(_ context.Context, tokenID int64, at time.Time, ip string) error {
	rec, ok := s.byID[tokenID]
	if !ok {
		return auth.ErrTokenNotFound
	}
	rec.lastUsed = &at
	rec.token.LastUsedAt = &at
	rec.token.LastUsedIP = ip
	s.byID[tokenID] = rec
	s.byHash[rec.hash] = rec
	return nil
}

func newTestPATService(store *memoryTokenStore) *auth.Service {
	return auth.NewService(store, auth.Options{CookieName: "jrs_session"})
}

func TestParseBearerToken(t *testing.T) {
	token, present := auth.ParseBearerToken("")
	if present || token != "" {
		t.Fatalf("empty header should be absent")
	}
	token, present = auth.ParseBearerToken("Basic abc")
	if !present || token != "" {
		t.Fatalf("non-bearer should be present+malformed")
	}
	token, present = auth.ParseBearerToken("Bearer  jrs_secret ")
	if !present || token != "jrs_secret" {
		t.Fatalf("got token=%q present=%v", token, present)
	}
}

func TestPersonalAccessTokenLifecycle(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTestPATService(store)

	meta, secret, err := svc.CreatePersonalAccessToken(context.Background(), 7, "CI Server", "for CI", nil, "127.0.0.1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(secret, auth.PersonalAccessTokenPrefix) {
		t.Fatalf("secret missing prefix: %q", secret)
	}
	if meta.Name != "CI Server" || meta.ID == 0 {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	user, err := svc.ValidatePersonalAccessToken(context.Background(), secret, "10.0.0.1")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if user.Username != "pat-user" || user.ID != 7 {
		t.Fatalf("unexpected user: %+v", user)
	}

	if _, err := svc.ValidatePersonalAccessToken(context.Background(), "jrs_nope", ""); err == nil {
		t.Fatal("expected invalid token error")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	res := svc.AuthenticateRequest(context.Background(), req)
	if res.Status != auth.StatusAuthenticated || res.User == nil || res.User.Username != "pat-user" {
		t.Fatalf("bearer auth: %+v", res)
	}

	if err := svc.RevokePersonalAccessToken(context.Background(), 7, meta.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.ValidatePersonalAccessToken(context.Background(), secret, ""); err == nil {
		t.Fatal("expected revoked token to fail")
	}
	res = svc.AuthenticateRequest(context.Background(), req)
	if res.Status == auth.StatusAuthenticated {
		t.Fatalf("revoked bearer should be unauthenticated: %+v", res)
	}
}

func TestBearerPreferredOverSession(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTestPATService(store)

	_, secret, err := svc.CreatePersonalAccessToken(context.Background(), 7, "PAT", "", nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "session-raw-token"})
	req.Header.Set("Authorization", "Bearer "+secret)
	res := svc.AuthenticateRequest(context.Background(), req)
	if res.Status != auth.StatusAuthenticated || res.User == nil || res.User.Username != "pat-user" {
		t.Fatalf("expected bearer user, got %+v", res)
	}
}

func TestInvalidBearerDoesNotFallThroughToSession(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTestPATService(store)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jrs_session", Value: "session-raw-token"})
	req.Header.Set("Authorization", "Bearer jrs_invalid")
	res := svc.AuthenticateRequest(context.Background(), req)
	if res.Status == auth.StatusAuthenticated {
		t.Fatalf("invalid bearer must not use cookie: %+v", res)
	}
}

func TestExpiredPersonalAccessToken(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTestPATService(store)
	past := time.Now().Add(-time.Hour)
	if _, _, err := svc.CreatePersonalAccessToken(context.Background(), 7, "Old", "", &past, ""); err == nil {
		t.Fatal("expected rejection of past expiration on create")
	}

	future := time.Now().Add(time.Hour)
	meta, secret, err := svc.CreatePersonalAccessToken(context.Background(), 7, "Soon", "", &future, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rec := store.byID[meta.ID]
	expired := time.Now().Add(-time.Minute)
	rec.token.ExpiresAt = &expired
	store.byID[meta.ID] = rec
	store.byHash[rec.hash] = rec

	if _, err := svc.ValidatePersonalAccessToken(context.Background(), secret, ""); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestRotateAndUpdateToken(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTestPATService(store)
	meta, secret, err := svc.CreatePersonalAccessToken(context.Background(), 7, "A", "desc", nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	name := "Renamed"
	updated, err := svc.UpdatePersonalAccessToken(context.Background(), 7, meta.ID, auth.PersonalAccessTokenUpdate{Name: &name})
	if err != nil || updated.Name != "Renamed" {
		t.Fatalf("update: %+v err=%v", updated, err)
	}

	newMeta, newSecret, err := svc.RotatePersonalAccessToken(context.Background(), 7, meta.ID, "1.2.3.4")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if newSecret == secret {
		t.Fatal("rotated secret should differ")
	}
	if _, err := svc.ValidatePersonalAccessToken(context.Background(), secret, ""); err == nil {
		t.Fatal("old secret should be revoked")
	}
	if _, err := svc.ValidatePersonalAccessToken(context.Background(), newSecret, ""); err != nil {
		t.Fatalf("new secret should work: %v", err)
	}
	if newMeta.Name != "Renamed" {
		t.Fatalf("rotated name=%q", newMeta.Name)
	}
}

func TestRedactAuthorization(t *testing.T) {
	if got := auth.RedactAuthorization("Bearer jrs_secret"); got != "Bearer [redacted]" {
		t.Fatalf("got %q", got)
	}
}
