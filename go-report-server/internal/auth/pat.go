package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// PersonalAccessTokenPrefix is the visible prefix on every issued PAT (Report Server).
	// Cloud uses "agp_"; Report Server uses "jrs_" with the same architecture.
	PersonalAccessTokenPrefix = "jrs_"

	// PersonalAccessTokenSecretBytes is the entropy of the secret portion.
	PersonalAccessTokenSecretBytes = 32

	// PersonalAccessTokenPrefixDisplayLen is how many characters of the plaintext
	// are stored for UI identification (never enough to reconstruct the secret).
	PersonalAccessTokenPrefixDisplayLen = 12

	// LastUsedUpdateInterval throttles last-used timestamp writes (Cloud parity).
	LastUsedUpdateInterval = 5 * time.Minute
)

var (
	ErrTokenUnsupported = errors.New("authentication: token auth unavailable")
	ErrTokenInvalid     = errors.New("authentication: invalid token")
	ErrTokenNotFound    = errors.New("authentication: token not found")
)

// PersonalAccessToken is public metadata for a PAT (never includes a secret).
type PersonalAccessToken struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedByIP string     `json:"created_by_ip,omitempty"`
	LastUsedIP  string     `json:"last_used_ip,omitempty"`
	Active      bool       `json:"active"`
}

// PersonalAccessTokenCreate holds fields required to persist a new token.
type PersonalAccessTokenCreate struct {
	UserID      int64
	Name        string
	Description string
	Scopes      []string
	ExpiresAt   *time.Time
	CreatedByIP string
}

// PersonalAccessTokenUpdate holds mutable metadata (never regenerates the secret).
type PersonalAccessTokenUpdate struct {
	Name         *string
	Description  *string
	ExpiresAt    *time.Time // used when SetExpires is true
	SetExpires   bool       // when true, write ExpiresAt (nil clears expiration)
}

// TokenStore persists hashed personal access tokens.
type TokenStore interface {
	CreatePAT(ctx context.Context, tokenHash, tokenPrefix string, input PersonalAccessTokenCreate) (PersonalAccessToken, error)
	ListPATsByUser(ctx context.Context, userID int64) ([]PersonalAccessToken, error)
	GetPAT(ctx context.Context, userID, tokenID int64) (PersonalAccessToken, error)
	LookupActivePAT(ctx context.Context, tokenHash string) (user *User, tokenID int64, lastUsedAt *time.Time, err error)
	UpdatePAT(ctx context.Context, userID, tokenID int64, input PersonalAccessTokenUpdate) (PersonalAccessToken, error)
	RevokePAT(ctx context.Context, userID, tokenID int64) error
	DeletePAT(ctx context.Context, userID, tokenID int64) error
	TouchPATLastUsed(ctx context.Context, tokenID int64, at time.Time, ip string) error
}

func (s *Service) tokenStore() (TokenStore, bool) {
	if s == nil || s.repo == nil {
		return nil, false
	}
	store, ok := s.repo.(TokenStore)
	if !ok || store == nil {
		return nil, false
	}
	return store, true
}

// CreatePersonalAccessToken issues a new PAT. The plaintext is returned once.
func (s *Service) CreatePersonalAccessToken(ctx context.Context, userID int64, name, description string, expiresAt *time.Time, createdByIP string) (PersonalAccessToken, string, error) {
	store, ok := s.tokenStore()
	if !ok {
		return PersonalAccessToken{}, "", ErrTokenUnsupported
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return PersonalAccessToken{}, "", errors.New("name is required")
	}
	if userID <= 0 {
		return PersonalAccessToken{}, "", errors.New("user id is required")
	}
	if expiresAt != nil && !expiresAt.IsZero() && !expiresAt.After(time.Now()) {
		return PersonalAccessToken{}, "", errors.New("expiration must be in the future")
	}

	secret, err := randomPATSecret(PersonalAccessTokenSecretBytes)
	if err != nil {
		return PersonalAccessToken{}, "", err
	}
	plaintext := PersonalAccessTokenPrefix + secret
	prefix := plaintext
	if len(prefix) > PersonalAccessTokenPrefixDisplayLen {
		prefix = prefix[:PersonalAccessTokenPrefixDisplayLen]
	}

	hash, err := HashSessionToken(plaintext)
	if err != nil {
		return PersonalAccessToken{}, "", err
	}

	record, err := store.CreatePAT(ctx, hash, prefix, PersonalAccessTokenCreate{
		UserID:      userID,
		Name:        name,
		Description: strings.TrimSpace(description),
		Scopes:      []string{},
		ExpiresAt:   expiresAt,
		CreatedByIP: strings.TrimSpace(createdByIP),
	})
	if err != nil {
		return PersonalAccessToken{}, "", err
	}
	s.logger.Info("personal access token created",
		"user_id", userID,
		"token_id", record.ID,
		"token_prefix", prefix,
	)
	return record, plaintext, nil
}

// ListPersonalAccessTokens returns non-revoked tokens for a user (no secrets).
func (s *Service) ListPersonalAccessTokens(ctx context.Context, userID int64) ([]PersonalAccessToken, error) {
	store, ok := s.tokenStore()
	if !ok {
		return nil, ErrTokenUnsupported
	}
	return store.ListPATsByUser(ctx, userID)
}

// GetPersonalAccessToken returns one token owned by the user (no secret).
func (s *Service) GetPersonalAccessToken(ctx context.Context, userID, tokenID int64) (PersonalAccessToken, error) {
	store, ok := s.tokenStore()
	if !ok {
		return PersonalAccessToken{}, ErrTokenUnsupported
	}
	if userID <= 0 || tokenID <= 0 {
		return PersonalAccessToken{}, ErrTokenNotFound
	}
	return store.GetPAT(ctx, userID, tokenID)
}

// UpdatePersonalAccessToken updates name/description/expiration without regenerating the secret.
func (s *Service) UpdatePersonalAccessToken(ctx context.Context, userID, tokenID int64, input PersonalAccessTokenUpdate) (PersonalAccessToken, error) {
	store, ok := s.tokenStore()
	if !ok {
		return PersonalAccessToken{}, ErrTokenUnsupported
	}
	if userID <= 0 || tokenID <= 0 {
		return PersonalAccessToken{}, ErrTokenNotFound
	}
	if input.Name != nil {
		n := strings.TrimSpace(*input.Name)
		if n == "" {
			return PersonalAccessToken{}, errors.New("name is required")
		}
		input.Name = &n
	}
	if input.Description != nil {
		d := strings.TrimSpace(*input.Description)
		input.Description = &d
	}
	if input.SetExpires && input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now()) {
		return PersonalAccessToken{}, errors.New("expiration must be in the future")
	}
	return store.UpdatePAT(ctx, userID, tokenID, input)
}

// RevokePersonalAccessToken soft-revokes a token owned by the user.
func (s *Service) RevokePersonalAccessToken(ctx context.Context, userID, tokenID int64) error {
	store, ok := s.tokenStore()
	if !ok {
		return ErrTokenUnsupported
	}
	if userID <= 0 || tokenID <= 0 {
		return ErrTokenNotFound
	}
	return store.RevokePAT(ctx, userID, tokenID)
}

// DeletePersonalAccessToken permanently removes a token row (typically after revoke).
func (s *Service) DeletePersonalAccessToken(ctx context.Context, userID, tokenID int64) error {
	store, ok := s.tokenStore()
	if !ok {
		return ErrTokenUnsupported
	}
	if userID <= 0 || tokenID <= 0 {
		return ErrTokenNotFound
	}
	return store.DeletePAT(ctx, userID, tokenID)
}

// RotatePersonalAccessToken revokes the old token and issues a new one with the same metadata.
// Returns the new metadata and plaintext secret (once).
func (s *Service) RotatePersonalAccessToken(ctx context.Context, userID, tokenID int64, createdByIP string) (PersonalAccessToken, string, error) {
	store, ok := s.tokenStore()
	if !ok {
		return PersonalAccessToken{}, "", ErrTokenUnsupported
	}
	old, err := store.GetPAT(ctx, userID, tokenID)
	if err != nil {
		return PersonalAccessToken{}, "", err
	}
	if old.RevokedAt != nil {
		return PersonalAccessToken{}, "", ErrTokenNotFound
	}
	if err := store.RevokePAT(ctx, userID, tokenID); err != nil {
		return PersonalAccessToken{}, "", err
	}
	return s.CreatePersonalAccessToken(ctx, userID, old.Name, old.Description, old.ExpiresAt, createdByIP)
}

// ValidatePersonalAccessToken authenticates a raw PAT and returns the user.
// Invalid, revoked, and expired tokens all yield ErrTokenInvalid (no existence leak).
func (s *Service) ValidatePersonalAccessToken(ctx context.Context, rawToken, clientIP string) (*User, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || !hasAnyPrefix(rawToken, s.acceptedPATPrefixes()) {
		return nil, ErrTokenInvalid
	}
	store, ok := s.tokenStore()
	if !ok {
		return nil, ErrTokenUnsupported
	}
	hash, err := HashSessionToken(rawToken)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	user, tokenID, lastUsedAt, err := store.LookupActivePAT(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	if user == nil || !user.Enabled {
		return nil, ErrTokenInvalid
	}
	maybeTouchLastUsed(ctx, store, tokenID, lastUsedAt, clientIP)
	return user, nil
}

func maybeTouchLastUsed(ctx context.Context, store TokenStore, tokenID int64, lastUsedAt *time.Time, clientIP string) {
	now := time.Now().UTC()
	if lastUsedAt != nil && now.Sub(lastUsedAt.UTC()) < LastUsedUpdateInterval {
		return
	}
	_ = store.TouchPATLastUsed(ctx, tokenID, now, strings.TrimSpace(clientIP))
}

func randomPATSecret(size int) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("invalid token size")
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hasAnyPrefix(token string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(token, p) {
			return true
		}
	}
	return false
}

// ParseBearerToken extracts the credential from an Authorization header value.
// Returns ("", false) when the header is absent; ("", true) when present but malformed;
// (token, true) when well-formed.
func ParseBearerToken(authorization string) (token string, present bool) {
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(authorization) < len(prefix) || !strings.EqualFold(authorization[:len(prefix)], prefix) {
		return "", true
	}
	token = strings.TrimSpace(authorization[len(prefix):])
	if token == "" {
		return "", true
	}
	return token, true
}

// RedactAuthorization returns a log-safe Authorization header value.
func RedactAuthorization(authorization string) string {
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		return ""
	}
	if token, present := ParseBearerToken(authorization); present {
		if token == "" {
			return "Bearer [invalid]"
		}
		return "Bearer [redacted]"
	}
	scheme, _, _ := strings.Cut(authorization, " ")
	if scheme == "" {
		return "[redacted]"
	}
	return scheme + " [redacted]"
}

// ClientIPFromRequest returns the best-effort client IP for audit fields.
func ClientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		// Strip port when present (also handles IPv6 "[::1]:123" poorly; fine for audit).
		if !strings.Contains(host, "]") {
			return host[:i]
		}
	}
	return host
}
