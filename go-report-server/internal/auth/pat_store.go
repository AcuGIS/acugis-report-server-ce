package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreatePAT(ctx context.Context, tokenHash, tokenPrefix string, input PersonalAccessTokenCreate) (PersonalAccessToken, error) {
	if !r.Configured() {
		return PersonalAccessToken{}, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	scopes, err := json.Marshal(normalizeScopes(input.Scopes))
	if err != nil {
		return PersonalAccessToken{}, fmt.Errorf("%w: encode scopes: %v", ErrDatabase, err)
	}
	var token PersonalAccessToken
	var scopesRaw []byte
	err = r.pool.QueryRow(ctx, `
INSERT INTO personal_access_tokens (
	user_id, name, description, token_hash, token_prefix, scopes, expires_at, created_by_ip
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
RETURNING id, user_id, name, description, token_prefix, scopes,
	expires_at, last_used_at, created_at, updated_at, revoked_at, created_by_ip, last_used_ip
`, input.UserID, input.Name, input.Description, tokenHash, tokenPrefix, scopes, input.ExpiresAt, input.CreatedByIP).Scan(
		&token.ID,
		&token.UserID,
		&token.Name,
		&token.Description,
		&token.TokenPrefix,
		&scopesRaw,
		&token.ExpiresAt,
		&token.LastUsedAt,
		&token.CreatedAt,
		&token.UpdatedAt,
		&token.RevokedAt,
		&token.CreatedByIP,
		&token.LastUsedIP,
	)
	if err != nil {
		return PersonalAccessToken{}, fmt.Errorf("%w: create personal access token: %v", ErrDatabase, err)
	}
	token.Scopes, err = decodeScopes(scopesRaw)
	if err != nil {
		return PersonalAccessToken{}, err
	}
	token.Active = token.RevokedAt == nil && (token.ExpiresAt == nil || token.ExpiresAt.After(time.Now()))
	return token, nil
}

func (r *Repository) ListPATsByUser(ctx context.Context, userID int64) ([]PersonalAccessToken, error) {
	if !r.Configured() {
		return nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, user_id, name, description, token_prefix, scopes,
	expires_at, last_used_at, created_at, updated_at, revoked_at, created_by_ip, last_used_ip
FROM personal_access_tokens
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY created_at DESC, id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: list personal access tokens: %v", ErrDatabase, err)
	}
	defer rows.Close()

	now := time.Now()
	out := make([]PersonalAccessToken, 0)
	for rows.Next() {
		token, err := scanPAT(rows)
		if err != nil {
			return nil, err
		}
		token.Active = token.RevokedAt == nil && (token.ExpiresAt == nil || token.ExpiresAt.After(now))
		out = append(out, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: list personal access tokens: %v", ErrDatabase, err)
	}
	return out, nil
}

func (r *Repository) GetPAT(ctx context.Context, userID, tokenID int64) (PersonalAccessToken, error) {
	if !r.Configured() {
		return PersonalAccessToken{}, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, user_id, name, description, token_prefix, scopes,
	expires_at, last_used_at, created_at, updated_at, revoked_at, created_by_ip, last_used_ip
FROM personal_access_tokens
WHERE id = $1 AND user_id = $2
LIMIT 1
`, tokenID, userID)
	token, err := scanPAT(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PersonalAccessToken{}, ErrTokenNotFound
	}
	if err != nil {
		return PersonalAccessToken{}, fmt.Errorf("%w: get personal access token: %v", ErrDatabase, err)
	}
	token.Active = token.RevokedAt == nil && (token.ExpiresAt == nil || token.ExpiresAt.After(time.Now()))
	return token, nil
}

func (r *Repository) LookupActivePAT(ctx context.Context, tokenHash string) (*User, int64, *time.Time, error) {
	if !r.Configured() {
		return nil, 0, nil, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil, 0, nil, ErrTokenNotFound
	}

	var (
		tokenID    int64
		lastUsedAt *time.Time
		id         int64
		username   string
		email      string
		isAdmin    bool
		enabled    bool
		groupsRaw  []byte
	)
	err := r.pool.QueryRow(ctx, `
SELECT t.id,
	t.last_used_at,
	u.id,
	u.username,
	COALESCE(u.email, '') AS email,
	COALESCE(u.is_admin, FALSE) AS is_admin,
	COALESCE(u.enabled, TRUE) AS enabled,
	COALESCE(
		jsonb_agg(g.name ORDER BY g.name) FILTER (WHERE g.name IS NOT NULL),
		u.groups,
		'[]'::jsonb
	) AS groups_json
FROM personal_access_tokens t
JOIN dashboard_users u ON u.id = t.user_id
LEFT JOIN auth_group_members gm ON gm.user_id = u.id
LEFT JOIN auth_groups g ON g.id = gm.group_id
WHERE t.token_hash = $1
	AND t.revoked_at IS NULL
	AND (t.expires_at IS NULL OR t.expires_at > NOW())
	AND COALESCE(u.enabled, TRUE) = TRUE
GROUP BY t.id, t.last_used_at, u.id, u.username, u.email, u.groups, u.is_admin, u.enabled
`, tokenHash).Scan(
		&tokenID,
		&lastUsedAt,
		&id,
		&username,
		&email,
		&isAdmin,
		&enabled,
		&groupsRaw,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, 0, nil, fmt.Errorf("%w: lookup personal access token: %v", ErrDatabase, err)
	}
	groups, gerr := decodeGroupsJSON(groupsRaw)
	if gerr != nil {
		return nil, 0, nil, fmt.Errorf("%w: decode groups: %v", ErrDatabase, gerr)
	}
	if GroupsIncludeAdmin(groups) {
		isAdmin = true
	}
	return &User{
		ID:       id,
		Username: username,
		Email:    email,
		IsAdmin:  isAdmin,
		Enabled:  enabled,
		Groups:   groups,
	}, tokenID, lastUsedAt, nil
}

func (r *Repository) UpdatePAT(ctx context.Context, userID, tokenID int64, input PersonalAccessTokenUpdate) (PersonalAccessToken, error) {
	if !r.Configured() {
		return PersonalAccessToken{}, fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	// Load current then apply patches in one UPDATE with COALESCE pattern.
	cur, err := r.GetPAT(ctx, userID, tokenID)
	if err != nil {
		return PersonalAccessToken{}, err
	}
	if cur.RevokedAt != nil {
		return PersonalAccessToken{}, ErrTokenNotFound
	}

	name := cur.Name
	if input.Name != nil {
		name = *input.Name
	}
	description := cur.Description
	if input.Description != nil {
		description = *input.Description
	}
	expiresAt := cur.ExpiresAt
	if input.SetExpires {
		expiresAt = input.ExpiresAt
	}

	row := r.pool.QueryRow(ctx, `
UPDATE personal_access_tokens
SET name = $3,
	description = $4,
	expires_at = $5,
	updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
RETURNING id, user_id, name, description, token_prefix, scopes,
	expires_at, last_used_at, created_at, updated_at, revoked_at, created_by_ip, last_used_ip
`, tokenID, userID, name, description, expiresAt)
	token, err := scanPAT(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PersonalAccessToken{}, ErrTokenNotFound
	}
	if err != nil {
		return PersonalAccessToken{}, fmt.Errorf("%w: update personal access token: %v", ErrDatabase, err)
	}
	token.Active = token.RevokedAt == nil && (token.ExpiresAt == nil || token.ExpiresAt.After(time.Now()))
	return token, nil
}

func (r *Repository) RevokePAT(ctx context.Context, userID, tokenID int64) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE personal_access_tokens
SET revoked_at = NOW(), updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
`, tokenID, userID)
	if err != nil {
		return fmt.Errorf("%w: revoke personal access token: %v", ErrDatabase, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (r *Repository) DeletePAT(ctx context.Context, userID, tokenID int64) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	tag, err := r.pool.Exec(ctx, `
DELETE FROM personal_access_tokens
WHERE id = $1 AND user_id = $2
`, tokenID, userID)
	if err != nil {
		return fmt.Errorf("%w: delete personal access token: %v", ErrDatabase, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (r *Repository) TouchPATLastUsed(ctx context.Context, tokenID int64, at time.Time, ip string) error {
	if !r.Configured() {
		return fmt.Errorf("%w: pool not configured", ErrDatabase)
	}
	_, err := r.pool.Exec(ctx, `
UPDATE personal_access_tokens
SET last_used_at = $2, last_used_ip = $3
WHERE id = $1
`, tokenID, at, ip)
	if err != nil {
		return fmt.Errorf("%w: touch personal access token: %v", ErrDatabase, err)
	}
	return nil
}

type patScanner interface {
	Scan(dest ...any) error
}

func scanPAT(row patScanner) (PersonalAccessToken, error) {
	var token PersonalAccessToken
	var scopesRaw []byte
	err := row.Scan(
		&token.ID,
		&token.UserID,
		&token.Name,
		&token.Description,
		&token.TokenPrefix,
		&scopesRaw,
		&token.ExpiresAt,
		&token.LastUsedAt,
		&token.CreatedAt,
		&token.UpdatedAt,
		&token.RevokedAt,
		&token.CreatedByIP,
		&token.LastUsedIP,
	)
	if err != nil {
		return PersonalAccessToken{}, err
	}
	scopes, err := decodeScopes(scopesRaw)
	if err != nil {
		return PersonalAccessToken{}, err
	}
	token.Scopes = scopes
	return token, nil
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func decodeScopes(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var scopes []string
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return nil, fmt.Errorf("%w: decode scopes: %v", ErrDatabase, err)
	}
	if scopes == nil {
		scopes = []string{}
	}
	return scopes, nil
}
