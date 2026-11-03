package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Repository performs user/group admin CRUD against PHP auth tables.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs an admin repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Configured reports whether a pool is available.
func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

// ListUsers returns all dashboard users with aggregated group names.
func (r *Repository) ListUsers(ctx context.Context) ([]UserRow, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `
SELECT u.id, u.username, u.email, u.is_admin, u.enabled, u.created_at,
       COALESCE(string_agg(g.name, ', ' ORDER BY g.name), '') AS group_names,
       COALESCE(u.password_hash, '') AS password_hash
FROM dashboard_users u
LEFT JOIN auth_group_members gm ON gm.user_id = u.id
LEFT JOIN auth_groups g ON g.id = gm.group_id
GROUP BY u.id, u.username, u.email, u.is_admin, u.enabled, u.created_at, u.password_hash
ORDER BY u.username ASC`)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []UserRow
	for rows.Next() {
		var row UserRow
		var passwordHash string
		if err := rows.Scan(&row.ID, &row.Username, &row.Email, &row.IsAdmin, &row.Enabled, &row.CreatedAt, &row.GroupNames, &passwordHash); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		row.HasLocalPassword = passwordHash != "" && passwordHash != "!oidc-no-local-password!"
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListAllGroups returns groups for user membership checkboxes.
func (r *Repository) ListAllGroups(ctx context.Context) ([]GroupRef, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `SELECT id, name FROM auth_groups ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []GroupRef
	for rows.Next() {
		var g GroupRef
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// UserGroupIDs returns group ids for a user.
func (r *Repository) UserGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if userID <= 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT group_id FROM auth_group_members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetUserGroups replaces group membership for a user.
func (r *Repository) SetUserGroups(ctx context.Context, userID int64, groupIDs []int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if userID <= 0 {
		return ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM auth_group_members WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING`, gid, userID); err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// SaveUser creates or updates a dashboard user (bcrypt password_hash on create / optional change).
func (r *Repository) SaveUser(ctx context.Context, in SaveUserInput, groupIDs []int64) (int64, error) {
	if !r.Configured() {
		return 0, ErrUnavailable
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(in.Email)
	if in.Username == "" {
		return 0, ErrInvalidInput
	}
	if in.ID <= 0 && in.Password == "" {
		return 0, ErrInvalidInput
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int64
	if in.ID > 0 {
		userID = in.ID
		if in.Password != "" {
			hash, herr := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
			if herr != nil {
				return 0, fmt.Errorf("%w: hash password: %v", ErrUnavailable, herr)
			}
			tag, err := tx.Exec(ctx, `
UPDATE dashboard_users SET username = $1, email = $2, password_hash = $3, is_admin = $4, enabled = $5
WHERE id = $6`, in.Username, in.Email, string(hash), in.IsAdmin, in.Enabled, in.ID)
			if err != nil {
				return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			if tag.RowsAffected() == 0 {
				return 0, fmt.Errorf("%w: user not found", ErrInvalidInput)
			}
		} else {
			tag, err := tx.Exec(ctx, `
UPDATE dashboard_users SET username = $1, email = $2, is_admin = $3, enabled = $4
WHERE id = $5`, in.Username, in.Email, in.IsAdmin, in.Enabled, in.ID)
			if err != nil {
				return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			if tag.RowsAffected() == 0 {
				return 0, fmt.Errorf("%w: user not found", ErrInvalidInput)
			}
		}
	} else {
		hash, herr := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if herr != nil {
			return 0, fmt.Errorf("%w: hash password: %v", ErrUnavailable, herr)
		}
		err := tx.QueryRow(ctx, `
INSERT INTO dashboard_users (username, email, password_hash, is_admin, enabled)
VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			in.Username, in.Email, string(hash), in.IsAdmin, in.Enabled,
		).Scan(&userID)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM auth_group_members WHERE user_id = $1`, userID); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING`, gid, userID); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return userID, nil
}

// DeleteUser removes a user, their sessions, and group memberships.
func (r *Repository) DeleteUser(ctx context.Context, userID, currentUserID int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if userID <= 0 {
		return ErrInvalidInput
	}
	if userID == currentUserID {
		return ErrSelfDelete
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
DELETE FROM dashboard_sessions
WHERE username = (SELECT username FROM dashboard_users WHERE id = $1)`, userID); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_group_members WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM dashboard_users WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: user not found", ErrInvalidInput)
	}
	return tx.Commit(ctx)
}

// ListGroups returns groups with aggregated member usernames.
func (r *Repository) ListGroups(ctx context.Context) ([]GroupRow, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `
SELECT g.id, g.name, COALESCE(g.description, '') AS description,
       COALESCE(string_agg(u.username, ', ' ORDER BY u.username), '') AS members
FROM auth_groups g
LEFT JOIN auth_group_members gm ON gm.group_id = g.id
LEFT JOIN dashboard_users u ON u.id = gm.user_id
GROUP BY g.id, g.name, g.description
ORDER BY g.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []GroupRow
	for rows.Next() {
		var row GroupRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &row.Members); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListAllUsers returns users for group membership checkboxes.
func (r *Repository) ListAllUsers(ctx context.Context) ([]UserRef, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `SELECT id, username FROM dashboard_users ORDER BY username ASC`)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []UserRef
	for rows.Next() {
		var u UserRef
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GroupMemberIDs returns user ids for a group.
func (r *Repository) GroupMemberIDs(ctx context.Context, groupID int64) ([]int64, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if groupID <= 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM auth_group_members WHERE group_id = $1`, groupID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetGroupMembers replaces membership for a group.
func (r *Repository) SetGroupMembers(ctx context.Context, groupID int64, userIDs []int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if groupID <= 0 {
		return ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM auth_group_members WHERE group_id = $1`, groupID); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	for _, uid := range userIDs {
		if uid <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING`, groupID, uid); err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return tx.Commit(ctx)
}

// SaveGroup creates or updates a group and replaces members when memberIDs is non-nil.
func (r *Repository) SaveGroup(ctx context.Context, in SaveGroupInput, memberIDs []int64) (int64, error) {
	if !r.Configured() {
		return 0, ErrUnavailable
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" {
		return 0, ErrInvalidInput
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var groupID int64
	if in.ID > 0 {
		groupID = in.ID
		tag, err := tx.Exec(ctx, `
UPDATE auth_groups SET name = $1, description = $2 WHERE id = $3`,
			in.Name, in.Description, in.ID)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		if tag.RowsAffected() == 0 {
			return 0, fmt.Errorf("%w: group not found", ErrInvalidInput)
		}
	} else {
		err := tx.QueryRow(ctx, `
INSERT INTO auth_groups (name, description) VALUES ($1, $2) RETURNING id`,
			in.Name, in.Description,
		).Scan(&groupID)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}

	if memberIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM auth_group_members WHERE group_id = $1`, groupID); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		for _, uid := range memberIDs {
			if uid <= 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO auth_group_members (group_id, user_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING`, groupID, uid); err != nil {
				return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return groupID, nil
}

// DeleteGroup removes a group unless it is Administrators or Public.
func (r *Repository) DeleteGroup(ctx context.Context, groupID int64) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if groupID <= 0 {
		return ErrInvalidInput
	}

	var name string
	err := r.pool.QueryRow(ctx, `SELECT name FROM auth_groups WHERE id = $1`, groupID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: group not found", ErrInvalidInput)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if strings.EqualFold(name, "Administrators") || strings.EqualFold(name, "Public") {
		return ErrProtectedGroup
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM resource_permissions WHERE group_id = $1`, groupID); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_group_members WHERE group_id = $1`, groupID); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM auth_groups WHERE id = $1`, groupID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: group not found", ErrInvalidInput)
	}
	return tx.Commit(ctx)
}

// IsProtectedGroupName reports whether a group name cannot be deleted.
func IsProtectedGroupName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "Administrators") ||
		strings.EqualFold(strings.TrimSpace(name), "Public")
}
