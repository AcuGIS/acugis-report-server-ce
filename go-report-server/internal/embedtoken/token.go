// Package embedtoken issues and validates short-lived, content-scoped embed tokens.
//
// Embedding ALWAYS requires an emb_ token. Personal access tokens (jrs_) and
// interactive sessions may mint embed tokens via POST /api/v1/embed/token, but
// must not be passed to the browser Embedding SDK or placed in iframe URLs.
package embedtoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Prefix identifies embed tokens (distinct from PAT prefix jrs_).
	Prefix = "emb_"

	// DefaultTTL is used when callers omit an explicit lifetime.
	DefaultTTL = 15 * time.Minute

	// MaxTTL caps minted token lifetime.
	MaxTTL = 24 * time.Hour
)

var (
	ErrInvalidToken = errors.New("embedtoken: invalid token")
	ErrExpiredToken = errors.New("embedtoken: token expired")
	ErrWrongContent = errors.New("embedtoken: content mismatch")
	ErrEmptySecret  = errors.New("embedtoken: empty signing secret")
)

// ContentType is the embedded content kind.
type ContentType string

const (
	ContentReport    ContentType = "report"
	ContentDashboard ContentType = "dashboard"
	ContentView      ContentType = "view"
	ContentMapReport ContentType = "map-report"
)

// Claims are carried inside a signed embed token.
type Claims struct {
	UserID      int64       `json:"uid"`
	Username    string      `json:"un"`
	ContentType ContentType `json:"ct"`
	ContentID   string      `json:"cid"`
	Permission  string      `json:"perm"`
	Origin      string      `json:"origin,omitempty"`
	// Parameters are bound parameters. Omitted on tokens minted without them,
	// so previously issued tokens keep validating. Values are canonical strings
	// (one element for a scalar, more than one for a multi-value parameter).
	Parameters map[string][]string `json:"parameters,omitempty"`
	IssuedAt   int64               `json:"iat"`
	ExpiresAt  int64               `json:"exp"`
}

// IsEmbedToken reports whether raw looks like an embed token (prefix only).
func IsEmbedToken(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), Prefix)
}

// ValidContentType reports whether ct is a supported embed content type.
func ValidContentType(ct ContentType) bool {
	switch ct {
	case ContentReport, ContentDashboard, ContentView, ContentMapReport:
		return true
	default:
		return false
	}
}

// Issue creates a signed emb_ token for the given claims and TTL.
func Issue(secret string, claims Claims, ttl time.Duration) (token string, expiresAt time.Time, err error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", time.Time{}, ErrEmptySecret
	}
	if !ValidContentType(claims.ContentType) {
		return "", time.Time{}, fmt.Errorf("%w: content type", ErrInvalidToken)
	}
	claims.ContentID = strings.TrimSpace(claims.ContentID)
	if claims.ContentID == "" {
		return "", time.Time{}, fmt.Errorf("%w: content id", ErrInvalidToken)
	}
	if claims.Permission == "" {
		claims.Permission = "view"
	}
	if claims.Parameters != nil {
		norm, nerr := NormalizeBoundParameters(claims.Parameters)
		if nerr != nil {
			return "", time.Time{}, nerr
		}
		claims.Parameters = norm
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}
	now := time.Now().UTC()
	expiresAt = now.Add(ttl)
	claims.IssuedAt = now.Unix()
	claims.ExpiresAt = expiresAt.Unix()

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return Prefix + payloadB64 + "." + sigB64, expiresAt, nil
}

// ParseAndValidate verifies signature and expiry. It does not check ACL.
func ParseAndValidate(secret, raw string) (Claims, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return Claims{}, ErrEmptySecret
	}
	raw = strings.TrimSpace(raw)
	if !IsEmbedToken(raw) {
		return Claims{}, ErrInvalidToken
	}
	body := strings.TrimPrefix(raw, Prefix)
	parts := strings.Split(body, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Claims{}, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, got) {
		return Claims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if !ValidContentType(claims.ContentType) || strings.TrimSpace(claims.ContentID) == "" {
		return Claims{}, ErrInvalidToken
	}
	if claims.ExpiresAt <= time.Now().UTC().Unix() {
		return Claims{}, ErrExpiredToken
	}
	if len(claims.Parameters) > 0 {
		norm, nerr := NormalizeBoundParameters(claims.Parameters)
		if nerr != nil {
			return Claims{}, ErrInvalidToken
		}
		claims.Parameters = norm
	}
	return claims, nil
}

// MatchContent ensures the token is scoped to the requested content.
func MatchContent(claims Claims, contentType ContentType, contentID string) error {
	if claims.ContentType != contentType {
		return ErrWrongContent
	}
	if strings.TrimSpace(claims.ContentID) != strings.TrimSpace(contentID) {
		return ErrWrongContent
	}
	return nil
}

// RandomSecret returns a hex-ish secret suitable for development.
func RandomSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
