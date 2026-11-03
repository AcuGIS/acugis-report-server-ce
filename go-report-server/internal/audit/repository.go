package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUnavailable indicates the audit repository has no database pool.
var ErrUnavailable = errors.New("audit: repository unavailable")

// ErrNotFound indicates no audit event matched the lookup.
var ErrNotFound = errors.New("audit: event not found")

// Repository persists and queries audit_log rows.
type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a repository against the app DB pool.
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

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// EnsureSchema creates the audit_log table and indexes if missing (idempotent).
func (r *Repository) EnsureSchema(ctx context.Context) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	table := r.q("audit_log")
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ` + table + ` (
    id              BIGSERIAL PRIMARY KEY,
    event_id        UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id         BIGINT NULL,
    username        VARCHAR(255) NULL,
    session_id      VARCHAR(255) NULL,
    event_category  VARCHAR(50) NOT NULL,
    event_action    VARCHAR(100) NOT NULL,
    object_type     VARCHAR(50) NULL,
    object_id       VARCHAR(255) NULL,
    object_name     VARCHAR(500) NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'success',
    ip_address      VARCHAR(64) NULL,
    user_agent      TEXT NULL,
    duration_ms     BIGINT NULL,
    output_format   VARCHAR(32) NULL,
    output_size     BIGINT NULL,
    details         JSONB NULL
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_log_event_id ON ` + table + ` (event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON ` + table + ` (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON ` + table + ` (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_username ON ` + table + ` (username)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_event_category ON ` + table + ` (event_category)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_event_action ON ` + table + ` (event_action)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_object ON ` + table + ` (object_type, object_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_status ON ` + table + ` (status)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_category_action_created ON ` + table + ` (event_category, event_action, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := r.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("audit ensure schema: %w", err)
		}
	}
	return nil
}

// Insert appends one audit event.
func (r *Repository) Insert(ctx context.Context, e Event) error {
	if !r.Configured() {
		return ErrUnavailable
	}
	var details any
	if e.Details != nil {
		b, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("audit marshal details: %w", err)
		}
		details = b
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO `+r.q("audit_log")+` (
    event_id, created_at, user_id, username, session_id,
    event_category, event_action, object_type, object_id, object_name,
    status, ip_address, user_agent, duration_ms, output_format, output_size, details
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
    $11, NULLIF($12, ''), NULLIF($13, ''), $14, NULLIF($15, ''), $16, $17
)`,
		e.EventID,
		e.CreatedAt,
		e.UserID,
		nullStr(e.Username),
		nullStr(e.SessionID),
		e.Category,
		e.Action,
		e.ObjectType,
		e.ObjectID,
		e.ObjectName,
		e.Status,
		e.IPAddress,
		e.UserAgent,
		e.DurationMS,
		e.OutputFormat,
		e.OutputSize,
		details,
	)
	return err
}

func nullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// Record is a persisted audit row returned to API/UI callers.
type Record struct {
	ID           int64          `json:"id"`
	EventID      uuid.UUID      `json:"event_id"`
	CreatedAt    time.Time      `json:"created_at"`
	UserID       *int64         `json:"user_id,omitempty"`
	Username     string         `json:"username,omitempty"`
	SessionID    string         `json:"session_id,omitempty"`
	Category     string         `json:"event_category"`
	Action       string         `json:"event_action"`
	ObjectType   string         `json:"object_type,omitempty"`
	ObjectID     string         `json:"object_id,omitempty"`
	ObjectName   string         `json:"object_name,omitempty"`
	Status       string         `json:"status"`
	IPAddress    string         `json:"ip_address,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	DurationMS   *int64         `json:"duration_ms,omitempty"`
	OutputFormat string         `json:"output_format,omitempty"`
	OutputSize   *int64         `json:"output_size,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
}

// ListFilter holds server-side filter/pagination options.
type ListFilter struct {
	From       *time.Time
	To         *time.Time
	UserID     *int64
	Username   string
	Category   string
	Action     string
	ObjectType string
	Status     string
	Search     string // username, object name/id, event_id
	Sort       string // created_at | duration_ms
	Order      string // asc | desc
	Limit      int
	Offset     int
}

// ListResult is a page of audit records.
type ListResult struct {
	Items  []Record `json:"items"`
	Total  int64    `json:"total"`
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}

func (r *Repository) buildWhere(f ListFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(clause string, val any) {
		args = append(args, val)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("created_at <= $%d", *f.To)
	}
	if f.UserID != nil {
		add("user_id = $%d", *f.UserID)
	}
	if u := strings.TrimSpace(f.Username); u != "" {
		add("username ILIKE $%d", u)
	}
	if c := strings.TrimSpace(f.Category); c != "" {
		add("event_category = $%d", c)
	}
	if a := strings.TrimSpace(f.Action); a != "" {
		add("event_action = $%d", a)
	}
	if ot := strings.TrimSpace(f.ObjectType); ot != "" {
		add("object_type = $%d", ot)
	}
	if st := strings.TrimSpace(f.Status); st != "" {
		add("status = $%d", st)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+s+"%")
		n := len(args)
		// Also try exact UUID match when search looks like a UUID.
		if id, err := uuid.Parse(s); err == nil {
			args = append(args, id)
			clauses = append(clauses, fmt.Sprintf(
				`(username ILIKE $%d OR object_name ILIKE $%d OR object_id ILIKE $%d OR event_id::text ILIKE $%d OR event_id = $%d)`,
				n, n, n, n, n+1,
			))
		} else {
			clauses = append(clauses, fmt.Sprintf(
				`(username ILIKE $%d OR object_name ILIKE $%d OR object_id ILIKE $%d OR event_id::text ILIKE $%d)`,
				n, n, n, n,
			))
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) orderBy(f ListFilter) string {
	col := "created_at"
	switch strings.ToLower(strings.TrimSpace(f.Sort)) {
	case "duration_ms", "duration":
		col = "duration_ms"
	case "username", "user":
		col = "username"
	case "status":
		col = "status"
	case "category":
		col = "event_category"
	case "action":
		col = "event_action"
	}
	dir := "DESC"
	if strings.EqualFold(strings.TrimSpace(f.Order), "asc") {
		dir = "ASC"
	}
	return col + " " + dir + " NULLS LAST, id DESC"
}

// List returns a filtered, paginated page of audit events.
func (r *Repository) List(ctx context.Context, f ListFilter) (ListResult, error) {
	out := ListResult{Limit: f.Limit, Offset: f.Offset, Items: []Record{}}
	if !r.Configured() {
		return out, ErrUnavailable
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	out.Limit = f.Limit
	out.Offset = f.Offset

	where, args := r.buildWhere(f)
	countSQL := `SELECT COUNT(*) FROM ` + r.q("audit_log") + ` ` + where
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&out.Total); err != nil {
		return out, err
	}

	args = append(args, f.Limit, f.Offset)
	listSQL := `
SELECT id, event_id, created_at, user_id, COALESCE(username, ''), COALESCE(session_id, ''),
       event_category, event_action, COALESCE(object_type, ''), COALESCE(object_id, ''), COALESCE(object_name, ''),
       status, COALESCE(ip_address, ''), COALESCE(user_agent, ''), duration_ms, COALESCE(output_format, ''), output_size, details
FROM ` + r.q("audit_log") + ` ` + where + `
ORDER BY ` + r.orderBy(f) + `
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, rec)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRecord(row scannable) (Record, error) {
	var rec Record
	var details []byte
	err := row.Scan(
		&rec.ID, &rec.EventID, &rec.CreatedAt, &rec.UserID, &rec.Username, &rec.SessionID,
		&rec.Category, &rec.Action, &rec.ObjectType, &rec.ObjectID, &rec.ObjectName,
		&rec.Status, &rec.IPAddress, &rec.UserAgent, &rec.DurationMS, &rec.OutputFormat, &rec.OutputSize, &details,
	)
	if err != nil {
		return rec, err
	}
	if len(details) > 0 {
		_ = json.Unmarshal(details, &rec.Details)
	}
	return rec, nil
}

// GetByEventID loads one event by UUID.
func (r *Repository) GetByEventID(ctx context.Context, eventID uuid.UUID) (Record, error) {
	var zero Record
	if !r.Configured() {
		return zero, ErrUnavailable
	}
	row := r.pool.QueryRow(ctx, `
SELECT id, event_id, created_at, user_id, COALESCE(username, ''), COALESCE(session_id, ''),
       event_category, event_action, COALESCE(object_type, ''), COALESCE(object_id, ''), COALESCE(object_name, ''),
       status, COALESCE(ip_address, ''), COALESCE(user_agent, ''), duration_ms, COALESCE(output_format, ''), output_size, details
FROM `+r.q("audit_log")+`
WHERE event_id = $1`, eventID)
	rec, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, ErrNotFound
	}
	return rec, err
}

// SummaryMetrics holds lightweight dashboard metrics.
type SummaryMetrics struct {
	PeriodStart            time.Time `json:"period_start"`
	PeriodEnd              time.Time `json:"period_end"`
	ReportExecutions       int64     `json:"report_executions"`
	FailedExecutions       int64     `json:"failed_executions"`
	ActiveUsers            int64     `json:"active_users"`
	Exports                int64     `json:"exports"`
	AverageExecutionTimeMS *float64  `json:"average_execution_time_ms,omitempty"`
}

// Summary aggregates metrics for the given window.
func (r *Repository) Summary(ctx context.Context, from, to time.Time) (SummaryMetrics, error) {
	out := SummaryMetrics{PeriodStart: from, PeriodEnd: to}
	if !r.Configured() {
		return out, ErrUnavailable
	}
	err := r.pool.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE event_category = 'report' AND event_action IN ('execute', 'export')),
  COUNT(*) FILTER (WHERE event_category = 'report' AND event_action IN ('execute', 'export') AND status = 'failure'),
  COUNT(DISTINCT username) FILTER (WHERE username IS NOT NULL AND username <> ''),
  COUNT(*) FILTER (WHERE event_category = 'report' AND event_action = 'export'),
  AVG(duration_ms) FILTER (WHERE event_category = 'report' AND event_action IN ('execute', 'export') AND duration_ms IS NOT NULL)
FROM `+r.q("audit_log")+`
WHERE created_at >= $1 AND created_at <= $2`, from, to).Scan(
		&out.ReportExecutions,
		&out.FailedExecutions,
		&out.ActiveUsers,
		&out.Exports,
		&out.AverageExecutionTimeMS,
	)
	return out, err
}

// SlowReportRow is an aggregated slow-report metric.
type SlowReportRow struct {
	ReportID         string  `json:"report_id"`
	ReportName       string  `json:"report_name"`
	Executions       int64   `json:"executions"`
	AverageDurationMS float64 `json:"average_duration_ms"`
	MaximumDurationMS int64   `json:"maximum_duration_ms"`
	Failures         int64   `json:"failures"`
}

// SlowReports returns reports ordered by average duration (DB aggregation).
func (r *Repository) SlowReports(ctx context.Context, from, to time.Time, limit int) ([]SlowReportRow, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
SELECT
  COALESCE(NULLIF(object_id, ''), object_name, '(unknown)') AS report_id,
  COALESCE(NULLIF(object_name, ''), object_id, '(unknown)') AS report_name,
  COUNT(*) AS executions,
  COALESCE(AVG(duration_ms), 0) AS avg_ms,
  COALESCE(MAX(duration_ms), 0) AS max_ms,
  COUNT(*) FILTER (WHERE status = 'failure') AS failures
FROM `+r.q("audit_log")+`
WHERE created_at >= $1 AND created_at <= $2
  AND event_category = 'report'
  AND event_action IN ('execute', 'export')
  AND (object_id IS NOT NULL OR object_name IS NOT NULL)
GROUP BY 1, 2
ORDER BY avg_ms DESC NULLS LAST
LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SlowReportRow
	for rows.Next() {
		var row SlowReportRow
		if err := rows.Scan(&row.ReportID, &row.ReportName, &row.Executions, &row.AverageDurationMS, &row.MaximumDurationMS, &row.Failures); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if out == nil {
		out = []SlowReportRow{}
	}
	return out, rows.Err()
}

// DeleteOlderThan removes events strictly older than cutoff. Returns rows deleted.
func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if !r.Configured() {
		return 0, ErrUnavailable
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM `+r.q("audit_log")+` WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ListForExport returns up to maxRows matching the filter (no offset; for CSV).
func (r *Repository) ListForExport(ctx context.Context, f ListFilter, maxRows int) ([]Record, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if maxRows <= 0 {
		maxRows = 10000
	}
	if maxRows > 50000 {
		maxRows = 50000
	}
	f.Limit = maxRows
	f.Offset = 0
	res, err := r.List(ctx, f)
	return res.Items, err
}
