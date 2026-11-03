package schedules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecutionRepository manages jasper_schedule_execution rows.
type ExecutionRepository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewExecutionRepository constructs an execution repository.
func NewExecutionRepository(pool *pgxpool.Pool, schema string) *ExecutionRepository {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	return &ExecutionRepository{pool: pool, schema: schema}
}

func (r *ExecutionRepository) qualified(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

// Create inserts a running execution row and returns its id.
func (r *ExecutionRepository) Create(ctx context.Context, scheduleID int) (int, error) {
	if r == nil || r.pool == nil {
		return 0, ErrUnavailable
	}
	var sid any
	if scheduleID > 0 {
		sid = scheduleID
	}
	sql := `INSERT INTO ` + r.qualified("jasper_schedule_execution") + ` (schedule_id, status) VALUES ($1, $2) RETURNING id`
	var id int
	err := r.pool.QueryRow(ctx, sql, sid, "running").Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return id, nil
}

// Finish updates an execution row with final status.
func (r *ExecutionRepository) Finish(ctx context.Context, executionID int, status, outputFile, emailStatus, errorMessage string) error {
	if r == nil || r.pool == nil || executionID <= 0 {
		return nil
	}
	sql := `UPDATE ` + r.qualified("jasper_schedule_execution") + ` SET
		status = $2,
		finished_at = now(),
		output_file = $3,
		email_status = $4,
		error_message = $5
	WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, executionID, status, outputFile, emailStatus, errorMessage)
	return err
}

// List returns execution history, optionally filtered by schedule id.
func (r *ExecutionRepository) List(ctx context.Context, scheduleID *int, limit int) ([]Execution, error) {
	if r == nil || r.pool == nil {
		return nil, ErrUnavailable
	}
	if limit <= 0 {
		limit = 200
	}

	base := `SELECT e.id, e.schedule_id, e.status, e.started_at, e.finished_at, e.output_file, e.email_status, e.error_message,
		COALESCE(s.report_name, ''), COALESCE(s.filename, '')
		FROM ` + r.qualified("jasper_schedule_execution") + ` e
		LEFT JOIN ` + r.qualified("jasper_schedule") + ` s ON s.id = e.schedule_id`

	var (
		rows pgxRows
		err  error
	)
	if scheduleID != nil && *scheduleID > 0 {
		rows, err = r.pool.Query(ctx, base+` WHERE e.schedule_id = $1 ORDER BY e.started_at DESC LIMIT $2`, *scheduleID, limit)
	} else {
		rows, err = r.pool.Query(ctx, base+` ORDER BY e.started_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []Execution
	for rows.Next() {
		var ex Execution
		var scheduleIDVal *int32
		var finishedAt *time.Time
		if err := rows.Scan(
			&ex.ID, &scheduleIDVal, &ex.Status, &ex.StartedAt, &finishedAt,
			&ex.OutputFile, &ex.EmailStatus, &ex.ErrorMessage,
			&ex.ReportName, &ex.ScheduleFilename,
		); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		if scheduleIDVal != nil {
			v := int(*scheduleIDVal)
			ex.ScheduleID = &v
		}
		ex.FinishedAt = finishedAt
		out = append(out, ex)
	}
	return out, rows.Err()
}

// LatestForSchedule returns the most recent execution for a schedule.
func (r *ExecutionRepository) LatestForSchedule(ctx context.Context, scheduleID int) (*Execution, error) {
	sid := scheduleID
	list, err := r.List(ctx, &sid, 1)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

type pgxRows interface {
	Close()
	Next() bool
	Scan(dest ...any) error
	Err() error
}
