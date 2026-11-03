package contentresource

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists jasper_content_resource rows via pgx.
type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a content resource repository.
func NewRepository(pool *pgxpool.Pool, schema string) *Repository {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	return &Repository{pool: pool, schema: schema}
}

// Configured reports whether queries can run.
func (r *Repository) Configured() bool {
	return r != nil && r.pool != nil
}

func (r *Repository) q(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// TableAvailable reports whether jasper_content_resource is readable.
func (r *Repository) TableAvailable(ctx context.Context) bool {
	if !r.Configured() {
		return false
	}
	var one int
	err := r.pool.QueryRow(ctx, `SELECT 1 FROM `+r.q("jasper_content_resource")+` LIMIT 1`).Scan(&one)
	return err == nil || err == pgx.ErrNoRows
}

// Insert creates a new resource row.
func (r *Repository) Insert(ctx context.Context, res *Resource) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if res == nil {
		return ErrInvalidInput
	}
	var createdBy any
	if res.CreatedBy != nil && *res.CreatedBy > 0 {
		createdBy = *res.CreatedBy
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO `+r.q("jasper_content_resource")+`
    (uuid, folder_id, display_name, resource_type, original_filename, extension,
     storage_filename, mime_type, sha256, size_bytes, width, height, has_alpha, created_by)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, created_at, updated_at
`, res.UUID, res.FolderID, res.DisplayName, res.ResourceType, res.OriginalFilename,
		res.Extension, res.StorageFilename, res.MimeType, res.SHA256, res.SizeBytes,
		res.Width, res.Height, res.HasAlpha, createdBy)
	if err := row.Scan(&res.ID, &res.CreatedAt, &res.UpdatedAt); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// GetByUUID loads a resource by UUID.
func (r *Repository) GetByUUID(ctx context.Context, uuid string) (*Resource, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, ErrInvalidInput
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, uuid::text, folder_id, display_name, resource_type, original_filename, extension,
       storage_filename, mime_type, sha256, size_bytes, width, height, has_alpha,
       created_by, created_at, updated_at
FROM `+r.q("jasper_content_resource")+`
WHERE uuid = $1::uuid
`, uuid)
	return scanResource(row)
}

// UpdateMetadata updates display name, type, and/or folder_id.
func (r *Repository) UpdateMetadata(ctx context.Context, uuid string, in UpdateInput) (*Resource, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	cur, err := r.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if name == "" {
			return nil, ErrInvalidInput
		}
		cur.DisplayName = name
	}
	if in.ResourceType != nil {
		rt, err := NormalizeType(*in.ResourceType)
		if err != nil {
			return nil, err
		}
		cur.ResourceType = rt
	}
	if in.FolderID != nil {
		if *in.FolderID <= 0 {
			return nil, ErrInvalidInput
		}
		cur.FolderID = *in.FolderID
	}
	_, err = r.pool.Exec(ctx, `
UPDATE `+r.q("jasper_content_resource")+`
SET display_name = $2,
    resource_type = $3,
    folder_id = $4,
    updated_at = now()
WHERE uuid = $1::uuid
`, cur.UUID, cur.DisplayName, cur.ResourceType, cur.FolderID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return r.GetByUUID(ctx, cur.UUID)
}

// UpdateContent replaces binary metadata fields while preserving UUID and identity fields.
func (r *Repository) UpdateContent(ctx context.Context, res *Resource) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	if res == nil || strings.TrimSpace(res.UUID) == "" {
		return ErrInvalidInput
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE `+r.q("jasper_content_resource")+`
SET original_filename = $2,
    extension = $3,
    storage_filename = $4,
    mime_type = $5,
    sha256 = $6,
    size_bytes = $7,
    width = $8,
    height = $9,
    has_alpha = $10,
    updated_at = now()
WHERE uuid = $1::uuid
`, res.UUID, res.OriginalFilename, res.Extension, res.StorageFilename, res.MimeType,
		res.SHA256, res.SizeBytes, res.Width, res.Height, res.HasAlpha)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteByUUID removes the DB row.
func (r *Repository) DeleteByUUID(ctx context.Context, uuid string) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return ErrInvalidInput
	}
	tag, err := r.pool.Exec(ctx, `
DELETE FROM `+r.q("jasper_content_resource")+`
WHERE uuid = $1::uuid
`, uuid)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns resources matching the filter (no ACL filtering).
func (r *Repository) List(ctx context.Context, f ListFilter) ([]Resource, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args := []any{}
	where := []string{"1=1"}
	argN := 1
	if f.FolderID > 0 {
		where = append(where, fmt.Sprintf("r.folder_id = $%d", argN))
		args = append(args, f.FolderID)
		argN++
	}
	if rt := strings.TrimSpace(f.ResourceType); rt != "" {
		where = append(where, fmt.Sprintf("r.resource_type = $%d", argN))
		args = append(args, strings.ToLower(rt))
		argN++
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, fmt.Sprintf("(lower(r.display_name) LIKE $%d OR lower(r.original_filename) LIKE $%d)", argN, argN))
		args = append(args, "%"+strings.ToLower(q)+"%")
		argN++
	}
	args = append(args, limit)
	sql := `
SELECT r.id, r.uuid::text, r.folder_id, r.display_name, r.resource_type, r.original_filename, r.extension,
       r.storage_filename, r.mime_type, r.sha256, r.size_bytes, r.width, r.height, r.has_alpha,
       r.created_by, r.created_at, r.updated_at
FROM ` + r.q("jasper_content_resource") + ` r
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY lower(r.display_name) ASC, r.created_at DESC
LIMIT $` + fmt.Sprintf("%d", argN)

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []Resource
	for rows.Next() {
		res, err := scanResourceRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *res)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanResource(row scannable) (*Resource, error) {
	var res Resource
	var createdBy *int64
	var width, height *int
	var hasAlpha *bool
	err := row.Scan(
		&res.ID, &res.UUID, &res.FolderID, &res.DisplayName, &res.ResourceType,
		&res.OriginalFilename, &res.Extension, &res.StorageFilename, &res.MimeType,
		&res.SHA256, &res.SizeBytes, &width, &height, &hasAlpha,
		&createdBy, &res.CreatedAt, &res.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	res.Width = width
	res.Height = height
	res.HasAlpha = hasAlpha
	res.CreatedBy = createdBy
	return &res, nil
}

func scanResourceRows(rows pgx.Rows) (*Resource, error) {
	return scanResource(rows)
}
