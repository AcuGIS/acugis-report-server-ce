package schedules

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RecipientRepository manages jasper_recipient rows.
type RecipientRepository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRecipientRepository constructs a recipient repository.
func NewRecipientRepository(pool *pgxpool.Pool, schema string) *RecipientRepository {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	return &RecipientRepository{pool: pool, schema: schema}
}

func (r *RecipientRepository) qualified(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

// List returns all recipients ordered by name, email.
func (r *RecipientRepository) List(ctx context.Context) ([]Recipient, error) {
	if r == nil || r.pool == nil {
		return nil, ErrUnavailable
	}
	sql := `SELECT id, name, email, notes FROM ` + r.qualified("jasper_recipient") + ` ORDER BY name, email`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []Recipient
	for rows.Next() {
		var rec Recipient
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Email, &rec.Notes); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Get returns one recipient by id.
func (r *RecipientRepository) Get(ctx context.Context, id int) (*Recipient, error) {
	if r == nil || r.pool == nil || id <= 0 {
		return nil, ErrNotFound
	}
	sql := `SELECT id, name, email, notes FROM ` + r.qualified("jasper_recipient") + ` WHERE id = $1`
	var rec Recipient
	err := r.pool.QueryRow(ctx, sql, id).Scan(&rec.ID, &rec.Name, &rec.Email, &rec.Notes)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &rec, nil
}

// Save inserts or updates a recipient.
func (r *RecipientRepository) Save(ctx context.Context, id *int, name, email, notes string) error {
	if r == nil || r.pool == nil || strings.TrimSpace(email) == "" {
		return ErrUnavailable
	}
	if id != nil && *id > 0 {
		sql := `UPDATE ` + r.qualified("jasper_recipient") + ` SET name = $2, email = $3, notes = $4, updated_at = now() WHERE id = $1`
		_, err := r.pool.Exec(ctx, sql, *id, name, email, notes)
		return err
	}
	sql := `INSERT INTO ` + r.qualified("jasper_recipient") + ` (name, email, notes) VALUES ($1,$2,$3)`
	_, err := r.pool.Exec(ctx, sql, name, email, notes)
	return err
}

// Delete removes a recipient by id.
func (r *RecipientRepository) Delete(ctx context.Context, id int) error {
	if r == nil || r.pool == nil || id <= 0 {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM `+r.qualified("jasper_recipient")+` WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
