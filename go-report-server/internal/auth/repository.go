package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Repository performs read-only session and user lookups against PHP tables.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a read-only auth repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Configured reports whether a pool is available.
func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

const resolveUserSQL = `
SELECT u.id,
    u.username,
    COALESCE(u.email, '') AS email,
    COALESCE(u.is_admin, FALSE) AS is_admin,
    COALESCE(u.enabled, TRUE) AS enabled,
    s.expires_at,
    COALESCE(
        jsonb_agg(g.name ORDER BY g.name) FILTER (WHERE g.name IS NOT NULL),
        u.groups,
        '[]'::jsonb
    ) AS groups_json
FROM dashboard_sessions s
JOIN dashboard_users u ON u.username = s.username
LEFT JOIN auth_group_members gm ON gm.user_id = u.id
LEFT JOIN auth_groups g ON g.id = gm.group_id
WHERE s.token_hash = $1
  AND s.expires_at > NOW()
  AND COALESCE(u.enabled, TRUE) = TRUE
GROUP BY u.id, u.username, u.email, u.groups, u.is_admin, u.enabled, u.created_at, s.expires_at
`

// ResolveUserFromSession mirrors PHP jrs_auth_lookup_user_by_session:
// token hash must match, session must not be expired, user must be enabled.
//
// Distinguishes expired / disabled / unknown for diagnostics without leaking
// token material. Read-only: does not refresh expires_at.
func (r *Repository) ResolveUserFromSession(ctx context.Context, tokenHash string) (*User, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	if tokenHash == "" {
		return nil, ErrEmptyToken
	}

	var (
		id        int64
		username  string
		email     string
		isAdmin   bool
		enabled   bool
		expiresAt time.Time
		groupsRaw []byte
	)
	err := r.pool.QueryRow(ctx, resolveUserSQL, tokenHash).Scan(
		&id, &username, &email, &isAdmin, &enabled, &expiresAt, &groupsRaw,
	)
	if err == nil {
		groups, gerr := decodeGroupsJSON(groupsRaw)
		if gerr != nil {
			return nil, fmt.Errorf("%w: decode groups: %v", ErrDatabase, gerr)
		}
		return &User{
			ID:       id,
			Username: username,
			Email:    email,
			IsAdmin:  isAdmin,
			Enabled:  enabled,
			Groups:   groups,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}

	// No live session+enabled user. Probe why without mutating anything.
	return nil, r.classifyMissingSession(ctx, tokenHash)
}

func (r *Repository) classifyMissingSession(ctx context.Context, tokenHash string) error {
	var expiresAt time.Time
	var enabled *bool
	err := r.pool.QueryRow(ctx, `
SELECT s.expires_at, u.enabled
FROM dashboard_sessions s
LEFT JOIN dashboard_users u ON u.username = s.username
WHERE s.token_hash = $1
LIMIT 1
`, tokenHash).Scan(&expiresAt, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidSession
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if time.Now().After(expiresAt) {
		return ErrExpiredSession
	}
	if enabled != nil && !*enabled {
		return ErrDisabledUser
	}
	// Session exists and is not expired, but join failed (e.g. missing user).
	return ErrInvalidSession
}

// FindSessionByTokenHash returns a session row when the hash exists (including expired).
func (r *Repository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	if tokenHash == "" {
		return nil, ErrEmptyToken
	}
	var s Session
	err := r.pool.QueryRow(ctx, `
SELECT username, expires_at
FROM dashboard_sessions
WHERE token_hash = $1
LIMIT 1
`, tokenHash).Scan(&s.Username, &s.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return &s, nil
}

// FindUserByID loads a user by primary key (read-only).
func (r *Repository) FindUserByID(ctx context.Context, id int64) (*User, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	if id <= 0 {
		return nil, ErrInvalidSession
	}
	var u User
	var groupsRaw []byte
	err := r.pool.QueryRow(ctx, `
SELECT u.id,
    u.username,
    COALESCE(u.email, '') AS email,
    COALESCE(u.is_admin, FALSE) AS is_admin,
    COALESCE(u.enabled, TRUE) AS enabled,
    COALESCE(
        (SELECT jsonb_agg(g.name ORDER BY g.name)
         FROM auth_group_members gm
         JOIN auth_groups g ON g.id = gm.group_id
         WHERE gm.user_id = u.id),
        u.groups,
        '[]'::jsonb
    ) AS groups_json
FROM dashboard_users u
WHERE u.id = $1
LIMIT 1
`, id).Scan(&u.ID, &u.Username, &u.Email, &u.IsAdmin, &u.Enabled, &groupsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	groups, gerr := decodeGroupsJSON(groupsRaw)
	if gerr != nil {
		return nil, fmt.Errorf("%w: decode groups: %v", ErrDatabase, gerr)
	}
	u.Groups = groups
	return &u, nil
}

// AuthenticatePassword mirrors PHP jrs_auth_login credential check (bcrypt via password_verify).
// Returns ErrInvalidCredentials for unknown/disabled users or bad passwords (no user enumeration).
func (r *Repository) AuthenticatePassword(ctx context.Context, username, password string) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return ErrInvalidCredentials
	}

	var hash string
	var enabled bool
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(password_hash, ''), COALESCE(enabled, TRUE)
FROM dashboard_users
WHERE username = $1
LIMIT 1
`, username).Scan(&hash, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if !enabled || hash == "" {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// CreateSession inserts a dashboard_sessions row (PHP jrs_auth_create_session INSERT).
func (r *Repository) CreateSession(ctx context.Context, tokenHash, username string, expiresAt time.Time) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	tokenHash = strings.TrimSpace(tokenHash)
	username = strings.TrimSpace(username)
	if tokenHash == "" || username == "" {
		return ErrEmptyToken
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO dashboard_sessions (token_hash, username, expires_at)
VALUES ($1, $2, $3)
`, tokenHash, username, expiresAt)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return nil
}

// DeleteSession removes a session by token hash (PHP jrs_auth_logout DELETE).
func (r *Repository) DeleteSession(ctx context.Context, tokenHash string) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM dashboard_sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return nil
}

func decodeGroupsJSON(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(decoded))
	for _, v := range decoded {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}
