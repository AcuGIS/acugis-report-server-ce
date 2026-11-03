package folders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists content folders and polymorphic items.
type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a folders repository.
func NewRepository(pool *pgxpool.Pool, schema string) *Repository {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	return &Repository{pool: pool, schema: schema}
}

// Configured reports whether a pool is available.
func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

func (r *Repository) q(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// EnsureHome creates the root Home folder and ACL resource if missing.
func (r *Repository) EnsureHome(ctx context.Context) (*Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	f, err := r.GetRoot(ctx)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO `+r.q("content_folders")+` (parent_id, name, description, icon, sort_order, acl_inherit)
VALUES (NULL, 'Home', 'Root content folder', 'home', 0, true)
RETURNING id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
`)
	home, err := scanFolder(row)
	if err != nil {
		// Concurrent insert — re-read.
		if f2, err2 := r.GetRoot(ctx); err2 == nil {
			return f2, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	_ = r.ensureFolderResource(ctx, home.ID, home.Name, 0)
	_ = r.grantDefaultFolderPermissions(ctx, home.ID)
	return home, nil
}

// GetRoot returns the Home folder (parent_id IS NULL).
func (r *Repository) GetRoot(ctx context.Context) (*Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_folders")+`
WHERE parent_id IS NULL
LIMIT 1
`)
	return scanFolder(row)
}

// Get returns a folder by id.
func (r *Repository) Get(ctx context.Context, id int64) (*Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_folders")+`
WHERE id = $1
`, id)
	return scanFolder(row)
}

// Create inserts a child folder.
func (r *Repository) Create(ctx context.Context, in CreateFolderInput) (*Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || in.ParentID <= 0 {
		return nil, ErrBadRequest
	}
	icon := strings.TrimSpace(in.Icon)
	if icon == "" {
		icon = "folder"
	}
	inherit := true
	if in.ACLInherit != nil {
		inherit = *in.ACLInherit
	}
	var createdBy any
	if in.CreatedBy > 0 {
		createdBy = in.CreatedBy
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO `+r.q("content_folders")+`
    (parent_id, name, description, icon, sort_order, acl_inherit, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
`, in.ParentID, name, strings.TrimSpace(in.Description), icon, in.SortOrder, inherit, createdBy)
	f, err := scanFolder(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	_ = r.ensureFolderResource(ctx, f.ID, f.Name, in.CreatedBy)
	_ = r.grantDefaultFolderPermissions(ctx, f.ID)
	return f, nil
}

// Update patches folder metadata. Root rename is rejected.
func (r *Repository) Update(ctx context.Context, id int64, in UpdateFolderInput) (*Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	cur, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	name := cur.Name
	desc := cur.Description
	icon := cur.Icon
	sortOrder := cur.SortOrder
	inherit := cur.ACLInherit
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, ErrBadRequest
		}
		if cur.ParentID == nil && !strings.EqualFold(name, "Home") {
			return nil, ErrRootImmutable
		}
	}
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}
	if in.Icon != nil {
		icon = strings.TrimSpace(*in.Icon)
		if icon == "" {
			icon = "folder"
		}
	}
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	if in.ACLInherit != nil {
		inherit = *in.ACLInherit
	}
	row := r.pool.QueryRow(ctx, `
UPDATE `+r.q("content_folders")+`
SET name = $2, description = $3, icon = $4, sort_order = $5, acl_inherit = $6, updated_at = now()
WHERE id = $1
RETURNING id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
`, id, name, desc, icon, sortOrder, inherit)
	f, err := scanFolder(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	_ = r.ensureFolderResource(ctx, f.ID, f.Name, 0)
	return f, nil
}

// Move reparents a folder. Rejects cycles and moving the root.
func (r *Repository) Move(ctx context.Context, id int64, in MoveFolderInput) (*Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	cur, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.ParentID == nil {
		return nil, ErrRootImmutable
	}
	if in.ParentID <= 0 {
		return nil, ErrBadRequest
	}
	if in.ParentID == id {
		return nil, ErrCycle
	}
	parent, err := r.Get(ctx, in.ParentID)
	if err != nil {
		return nil, err
	}
	ok, err := r.WouldCreateCycle(ctx, id, parent.ID)
	if err != nil {
		return nil, err
	}
	if ok {
		return nil, ErrCycle
	}
	sortOrder := cur.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	row := r.pool.QueryRow(ctx, `
UPDATE `+r.q("content_folders")+`
SET parent_id = $2, sort_order = $3, updated_at = now()
WHERE id = $1
RETURNING id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
`, id, in.ParentID, sortOrder)
	f, err := scanFolder(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return f, nil
}

// WouldCreateCycle reports whether making newParentID the parent of folderID
// would introduce a cycle (newParent is folder or a descendant of folder).
func (r *Repository) WouldCreateCycle(ctx context.Context, folderID, newParentID int64) (bool, error) {
	if folderID == newParentID {
		return true, nil
	}
	// Walk ancestors of newParent; if we hit folderID, cycle.
	cur := newParentID
	for i := 0; i < 10000; i++ {
		if cur == folderID {
			return true, nil
		}
		f, err := r.Get(ctx, cur)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		if f.ParentID == nil {
			return false, nil
		}
		cur = *f.ParentID
	}
	return true, ErrCycle
}

// Delete removes a folder. Non-recursive requires empty folder.
func (r *Repository) Delete(ctx context.Context, id int64, opts DeleteOptions) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	f, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if f.ParentID == nil {
		return ErrRootImmutable
	}
	folders, items, err := r.childCounts(ctx, id)
	if err != nil {
		return err
	}
	if folders+items > 0 {
		if !opts.Recursive {
			return ErrNotEmpty
		}
		if !opts.AllowRecursive {
			return ErrRecursiveDenied
		}
		if !opts.Confirm {
			return ErrConfirmRequired
		}
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM `+r.q("content_folders")+` WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	// Best-effort ACL cleanup.
	_, _ = r.pool.Exec(ctx, `
DELETE FROM `+r.q("resources")+`
WHERE resource_type = 'content_folder' AND resource_key = $1
`, fmt.Sprintf("%d", id))
	return nil
}

func (r *Repository) childCounts(ctx context.Context, id int64) (folders, items int, err error) {
	err = r.pool.QueryRow(ctx, `
SELECT
  (SELECT COUNT(*)::int FROM `+r.q("content_folders")+` WHERE parent_id = $1),
  (SELECT COUNT(*)::int FROM `+r.q("content_items")+` WHERE folder_id = $1)
`, id).Scan(&folders, &items)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return folders, items, nil
}

// ListChildren returns direct child folders sorted folders-first order.
func (r *Repository) ListChildren(ctx context.Context, parentID int64) ([]Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_folders")+`
WHERE parent_id = $1
ORDER BY sort_order ASC, lower(name) ASC
`, parentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// ListAll returns every folder (for tree building).
func (r *Repository) ListAll(ctx context.Context) ([]Folder, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, parent_id, name, description, icon, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_folders")+`
ORDER BY sort_order ASC, lower(name) ASC
`)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []Folder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// AncestorChain returns [root, …, folder] inclusive.
func (r *Repository) AncestorChain(ctx context.Context, id int64) ([]Folder, error) {
	var chain []Folder
	cur := id
	seen := map[int64]bool{}
	for i := 0; i < 10000; i++ {
		if seen[cur] {
			return nil, ErrCycle
		}
		seen[cur] = true
		f, err := r.Get(ctx, cur)
		if err != nil {
			return nil, err
		}
		chain = append(chain, *f)
		if f.ParentID == nil {
			break
		}
		cur = *f.ParentID
	}
	// Reverse to root-first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// PathString builds "Home > A > B" for a folder id.
func (r *Repository) PathString(ctx context.Context, id int64) (string, error) {
	chain, err := r.AncestorChain(ctx, id)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(chain))
	for _, f := range chain {
		parts = append(parts, f.Name)
	}
	return strings.Join(parts, " > "), nil
}

// ListItems returns content items in a folder.
func (r *Repository) ListItems(ctx context.Context, folderID int64) ([]Item, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, folder_id, content_type, content_key, display_name, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_items")+`
WHERE folder_id = $1
ORDER BY sort_order ASC, lower(display_name) ASC, content_type ASC, content_key ASC
`, folderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}

// GetItemByKey looks up membership by type+key.
func (r *Repository) GetItemByKey(ctx context.Context, contentType, contentKey string) (*Item, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	contentType = strings.TrimSpace(contentType)
	contentKey = strings.TrimSpace(contentKey)
	if contentType == "" || contentKey == "" {
		return nil, ErrItemNotFound
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, folder_id, content_type, content_key, display_name, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_items")+`
WHERE content_type = $1 AND content_key = $2
`, contentType, contentKey)
	return scanItem(row)
}

// GetItemByID looks up a content_items row by primary key.
func (r *Repository) GetItemByID(ctx context.Context, id int64) (*Item, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if id <= 0 {
		return nil, ErrItemNotFound
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, folder_id, content_type, content_key, display_name, sort_order, acl_inherit, created_by, created_at, updated_at
FROM `+r.q("content_items")+`
WHERE id = $1
`, id)
	return scanItem(row)
}

// UpsertItem places or moves content into a folder.
func (r *Repository) UpsertItem(ctx context.Context, in MoveItemInput) (*Item, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	ct := strings.TrimSpace(in.ContentType)
	ck := strings.TrimSpace(in.ContentKey)
	if ct == "" || ck == "" || in.FolderID <= 0 {
		return nil, ErrBadRequest
	}
	if _, err := r.Get(ctx, in.FolderID); err != nil {
		return nil, err
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		display = ck
	}
	inherit := true
	if in.ACLInherit != nil {
		inherit = *in.ACLInherit
	}
	sortOrder := 0
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	var createdBy any
	if in.CreatedBy > 0 {
		createdBy = in.CreatedBy
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO `+r.q("content_items")+`
    (folder_id, content_type, content_key, display_name, sort_order, acl_inherit, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (content_type, content_key) DO UPDATE
SET folder_id = EXCLUDED.folder_id,
    display_name = EXCLUDED.display_name,
    sort_order = EXCLUDED.sort_order,
    acl_inherit = EXCLUDED.acl_inherit,
    updated_at = now()
RETURNING id, folder_id, content_type, content_key, display_name, sort_order, acl_inherit, created_by, created_at, updated_at
`, in.FolderID, ct, ck, display, sortOrder, inherit, createdBy)
	return scanItem(row)
}

// DeleteItem removes membership (does not delete the underlying resource).
func (r *Repository) DeleteItem(ctx context.Context, contentType, contentKey string) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	tag, err := r.pool.Exec(ctx, `
DELETE FROM `+r.q("content_items")+`
WHERE content_type = $1 AND content_key = $2
`, strings.TrimSpace(contentType), strings.TrimSpace(contentKey))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrItemNotFound
	}
	return nil
}

// ClearFavoritesForContent removes all users' favorites for a content key.
func (r *Repository) ClearFavoritesForContent(ctx context.Context, contentType, contentKey string) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	ct := strings.TrimSpace(contentType)
	ck := strings.TrimSpace(contentKey)
	if ct == "" || ck == "" {
		return ErrBadRequest
	}
	_, err := r.pool.Exec(ctx, `
DELETE FROM `+r.q("content_favorites")+`
WHERE content_type = $1 AND content_key = $2
`, ct, ck)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// SearchFoldersAndItems finds folders/items matching query (case-insensitive).
func (r *Repository) SearchFoldersAndItems(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	like := "%" + strings.ToLower(q) + "%"
	var hits []SearchHit

	frows, err := r.pool.Query(ctx, `
SELECT id, name, description, icon
FROM `+r.q("content_folders")+`
WHERE lower(name) LIKE $1 OR lower(description) LIKE $1
ORDER BY lower(name) ASC
LIMIT $2
`, like, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer frows.Close()
	for frows.Next() {
		var id int64
		var name, desc, icon string
		if err := frows.Scan(&id, &name, &desc, &icon); err != nil {
			return nil, err
		}
		path, _ := r.PathString(ctx, id)
		hits = append(hits, SearchHit{
			Kind:        "folder",
			ID:          id,
			Name:        name,
			Description: desc,
			FolderPath:  path,
			Icon:        icon,
		})
	}
	if err := frows.Err(); err != nil {
		return nil, err
	}

	irows, err := r.pool.Query(ctx, `
SELECT id, folder_id, content_type, content_key, display_name
FROM `+r.q("content_items")+`
WHERE lower(display_name) LIKE $1 OR lower(content_key) LIKE $1
ORDER BY lower(display_name) ASC
LIMIT $2
`, like, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer irows.Close()
	for irows.Next() {
		var id, folderID int64
		var ct, ck, name string
		if err := irows.Scan(&id, &folderID, &ct, &ck, &name); err != nil {
			return nil, err
		}
		path, _ := r.PathString(ctx, folderID)
		hits = append(hits, SearchHit{
			Kind:        "item",
			ID:          id,
			FolderID:    folderID,
			ContentType: ct,
			ContentKey:  ck,
			Name:        name,
			FolderPath:  path,
		})
	}
	return hits, irows.Err()
}

// ListFavorites returns favorite keys for a user.
func (r *Repository) ListFavorites(ctx context.Context, userID int64) ([]Item, error) {
	if !r.Configured() || userID <= 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
SELECT content_type, content_key, created_at
FROM `+r.q("content_favorites")+`
WHERE user_id = $1
ORDER BY created_at DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		var created time.Time
		if err := rows.Scan(&it.ContentType, &it.ContentKey, &created); err != nil {
			return nil, err
		}
		it.CreatedAt = created
		out = append(out, it)
	}
	return out, rows.Err()
}

// SetFavorite adds or removes a favorite.
func (r *Repository) SetFavorite(ctx context.Context, userID int64, contentType, contentKey string, favorite bool) error {
	if !r.Configured() || userID <= 0 {
		return ErrUnavailable
	}
	ct := strings.TrimSpace(contentType)
	ck := strings.TrimSpace(contentKey)
	if ct == "" || ck == "" {
		return ErrBadRequest
	}
	if favorite {
		_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.q("content_favorites")+` (user_id, content_type, content_key)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, content_type, content_key) DO NOTHING
`, userID, ct, ck)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return nil
	}
	_, err := r.pool.Exec(ctx, `
DELETE FROM `+r.q("content_favorites")+`
WHERE user_id = $1 AND content_type = $2 AND content_key = $3
`, userID, ct, ck)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// IsFavorite reports whether the user favorited the key.
func (r *Repository) IsFavorite(ctx context.Context, userID int64, contentType, contentKey string) (bool, error) {
	if !r.Configured() || userID <= 0 {
		return false, nil
	}
	var ok bool
	err := r.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM `+r.q("content_favorites")+`
  WHERE user_id = $1 AND content_type = $2 AND content_key = $3
)
`, userID, contentType, contentKey).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return ok, nil
}

// FolderIDsInChain returns folder id + ancestors (self first, then parents).
func (r *Repository) FolderIDsInChain(ctx context.Context, folderID int64) ([]int64, error) {
	chain, err := r.AncestorChain(ctx, folderID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(chain))
	// AncestorChain is root-first; reverse for nearest-first inheritance walk.
	for i := len(chain) - 1; i >= 0; i-- {
		ids = append(ids, chain[i].ID)
	}
	return ids, nil
}

// GetFolderACLInherit returns acl_inherit for a folder.
func (r *Repository) GetFolderMeta(ctx context.Context, id int64) (aclInherit bool, parentID *int64, err error) {
	f, err := r.Get(ctx, id)
	if err != nil {
		return false, nil, err
	}
	return f.ACLInherit, f.ParentID, nil
}

func (r *Repository) ensureFolderResource(ctx context.Context, folderID int64, displayName string, createdBy int64) error {
	key := fmt.Sprintf("%d", folderID)
	var cb any
	if createdBy > 0 {
		cb = createdBy
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.q("resources")+` (resource_type, resource_key, display_name, service, created_by)
VALUES ('content_folder', $1, $2, 'jasper_reports', $3)
ON CONFLICT (resource_type, resource_key) DO UPDATE
SET display_name = EXCLUDED.display_name
`, key, displayName, cb)
	return err
}

func (r *Repository) grantDefaultFolderPermissions(ctx context.Context, folderID int64) error {
	key := fmt.Sprintf("%d", folderID)
	_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.q("resource_permissions")+` (resource_id, group_id, permission)
SELECT r.id, g.id, v.permission
FROM `+r.q("resources")+` r
CROSS JOIN (VALUES
    ('Administrators', 'admin'),
    ('Editors', 'edit'),
    ('Public', 'view')
) AS v(group_name, permission)
JOIN `+r.q("auth_groups")+` g ON g.name = v.group_name
WHERE r.resource_type = 'content_folder' AND r.resource_key = $1
ON CONFLICT (resource_id, group_id, permission) DO NOTHING
`, key)
	return err
}

// ReplaceFolderPermissions clears non-admin grants and applies group view (or given permission).
func (r *Repository) ReplaceFolderPermissions(ctx context.Context, folderID int64, groupNames []string, permission string) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	key := fmt.Sprintf("%d", folderID)
	permission = strings.ToLower(strings.TrimSpace(permission))
	if permission == "" {
		permission = "view"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer tx.Rollback(ctx)

	var resourceID int64
	err = tx.QueryRow(ctx, `
SELECT id FROM `+r.q("resources")+`
WHERE resource_type = 'content_folder' AND resource_key = $1
`, key).Scan(&resourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := r.ensureFolderResource(ctx, folderID, key, 0); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
SELECT id FROM `+r.q("resources")+`
WHERE resource_type = 'content_folder' AND resource_key = $1
`, key).Scan(&resourceID)
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	_, err = tx.Exec(ctx, `
DELETE FROM `+r.q("resource_permissions")+` WHERE resource_id = $1
`, resourceID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	// Always restore Administrators admin.
	_, err = tx.Exec(ctx, `
INSERT INTO `+r.q("resource_permissions")+` (resource_id, group_id, permission)
SELECT $1, g.id, 'admin'
FROM `+r.q("auth_groups")+` g
WHERE g.name = 'Administrators'
ON CONFLICT DO NOTHING
`, resourceID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	for _, name := range uniqueNames(groupNames) {
		if strings.EqualFold(name, "Administrators") {
			continue
		}
		_, err = tx.Exec(ctx, `
INSERT INTO `+r.q("resource_permissions")+` (resource_id, group_id, permission)
SELECT $1, g.id, $3
FROM `+r.q("auth_groups")+` g
WHERE g.name = $2
ON CONFLICT DO NOTHING
`, resourceID, name, permission)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return tx.Commit(ctx)
}

// ListFolderPermissionGroups returns group names with any permission on the folder.
func (r *Repository) ListFolderPermissionGroups(ctx context.Context, folderID int64) ([]string, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	key := fmt.Sprintf("%d", folderID)
	rows, err := r.pool.Query(ctx, `
SELECT DISTINCT g.name
FROM `+r.q("resources")+` r
JOIN `+r.q("resource_permissions")+` p ON p.resource_id = r.id
JOIN `+r.q("auth_groups")+` g ON g.id = p.group_id
WHERE r.resource_type = 'content_folder' AND r.resource_key = $1
ORDER BY g.name
`, key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanFolder(row scannable) (*Folder, error) {
	var f Folder
	var parentID *int64
	var createdBy *int64
	err := row.Scan(&f.ID, &parentID, &f.Name, &f.Description, &f.Icon, &f.SortOrder, &f.ACLInherit, &createdBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	f.ParentID = parentID
	f.CreatedBy = createdBy
	return &f, nil
}

func scanItem(row scannable) (*Item, error) {
	var it Item
	var createdBy *int64
	err := row.Scan(&it.ID, &it.FolderID, &it.ContentType, &it.ContentKey, &it.DisplayName, &it.SortOrder, &it.ACLInherit, &createdBy, &it.CreatedAt, &it.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrItemNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	it.CreatedBy = createdBy
	return &it, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func uniqueNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out
}
