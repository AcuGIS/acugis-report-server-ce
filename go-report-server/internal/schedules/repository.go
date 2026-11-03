package schedules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool   *pgxpool.Pool
	schema string
}

// NewRepository constructs a schedule repository.
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

func (r *Repository) qualified(table string) string {
	return quoteIdent(r.schema) + "." + quoteIdent(table)
}

// TableAvailable reports whether jasper_schedule exists.
func (r *Repository) TableAvailable(ctx context.Context) bool {
	if !r.Configured() {
		return false
	}
	sql := `SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 LIMIT 1`
	var one int
	err := r.pool.QueryRow(ctx, sql, r.schema, "jasper_schedule").Scan(&one)
	return err == nil
}

// HasRepeatFields reports whether schedule_mode column exists.
func (r *Repository) HasRepeatFields(ctx context.Context) bool {
	if !r.Configured() || !r.TableAvailable(ctx) {
		return false
	}
	sql := `SELECT 1 FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND column_name = $3 LIMIT 1`
	var one int
	err := r.pool.QueryRow(ctx, sql, r.schema, "jasper_schedule", "schedule_mode").Scan(&one)
	return err == nil
}

// List returns all schedules ordered by id DESC.
func (r *Repository) List(ctx context.Context) ([]Schedule, error) {
	if !r.Configured() {
		return nil, ErrUnavailable
	}
	if !r.TableAvailable(ctx) {
		return []Schedule{}, nil
	}
	hasRepeat := r.HasRepeatFields(ctx)
	sql := `SELECT * FROM ` + r.qualified("jasper_schedule") + ` ORDER BY id DESC`
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()

	var out []Schedule
	for rows.Next() {
		s, err := scanSchedule(rows, hasRepeat)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Get returns one schedule by id.
func (r *Repository) Get(ctx context.Context, id int) (*Schedule, error) {
	if !r.Configured() || id <= 0 {
		return nil, ErrNotFound
	}
	if !r.TableAvailable(ctx) {
		return nil, ErrNotFound
	}
	hasRepeat := r.HasRepeatFields(ctx)
	sql := `SELECT * FROM ` + r.qualified("jasper_schedule") + ` WHERE id = $1`
	rows, err := r.pool.Query(ctx, sql, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	s, err := scanSchedule(rows, hasRepeat)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserts a schedule and returns the new id.
func (r *Repository) Create(ctx context.Context, data Schedule) (int, error) {
	if !r.Configured() {
		return 0, ErrUnavailable
	}
	if !r.TableAvailable(ctx) {
		return 0, ErrTableMissing
	}
	hasRepeat := r.HasRepeatFields(ctx)

	baseCols := `cron_period, cron_custom, report_name, datasource_name, format, filename,
		email, email_subject, email_body, email_template, noemail, url_opt_params, enabled`
	placeholders := `$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13`
	args := []any{
		defaultString(data.CronPeriod, "never"),
		data.CronCustom,
		data.ReportName,
		data.DatasourceName,
		defaultString(data.Format, "pdf"),
		data.Filename,
		data.Email,
		data.EmailSubject,
		data.EmailBody,
		data.EmailTemplate,
		data.NoEmail,
		data.URLOptParams,
		data.Enabled,
	}
	if hasRepeat {
		baseCols += `, schedule_mode, repeat_minutes, hourly_minute, daily_time, weekly_day, weekly_time, monthly_day, monthly_time, once_datetime`
		placeholders += `,$14,$15,$16,$17,$18,$19,$20,$21,$22`
		args = append(args,
			data.ScheduleMode,
			data.RepeatMinutes,
			data.HourlyMinute,
			data.DailyTime,
			data.WeeklyDay,
			data.WeeklyTime,
			data.MonthlyDay,
			data.MonthlyTime,
			data.OnceDatetime,
		)
	}

	sql := `INSERT INTO ` + r.qualified("jasper_schedule") + ` (` + baseCols + `) VALUES (` + placeholders + `) RETURNING id`
	var id int
	if err := r.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return id, nil
}

// Update updates an existing schedule.
func (r *Repository) Update(ctx context.Context, id int, data Schedule) error {
	if !r.Configured() || id <= 0 {
		return ErrNotFound
	}
	hasRepeat := r.HasRepeatFields(ctx)

	setClause := `cron_period = $2,
		cron_custom = $3,
		report_name = $4,
		datasource_name = $5,
		format = $6,
		filename = $7,
		email = $8,
		email_subject = $9,
		email_body = $10,
		email_template = $11,
		noemail = $12,
		url_opt_params = $13,
		enabled = $14`
	args := []any{
		id,
		defaultString(data.CronPeriod, "never"),
		data.CronCustom,
		data.ReportName,
		data.DatasourceName,
		defaultString(data.Format, "pdf"),
		data.Filename,
		data.Email,
		data.EmailSubject,
		data.EmailBody,
		data.EmailTemplate,
		data.NoEmail,
		data.URLOptParams,
		data.Enabled,
	}
	if hasRepeat {
		setClause += `,
		schedule_mode = $15,
		repeat_minutes = $16,
		hourly_minute = $17,
		daily_time = $18,
		weekly_day = $19,
		weekly_time = $20,
		monthly_day = $21,
		monthly_time = $22,
		once_datetime = $23`
		args = append(args,
			data.ScheduleMode,
			data.RepeatMinutes,
			data.HourlyMinute,
			data.DailyTime,
			data.WeeklyDay,
			data.WeeklyTime,
			data.MonthlyDay,
			data.MonthlyTime,
			data.OnceDatetime,
		)
	}

	sql := `UPDATE ` + r.qualified("jasper_schedule") + ` SET ` + setClause + `, updated_at = now() WHERE id = $1`
	tag, err := r.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a schedule by id.
func (r *Repository) Delete(ctx context.Context, id int) error {
	if !r.Configured() || id <= 0 {
		return ErrNotFound
	}
	sql := `DELETE FROM ` + r.qualified("jasper_schedule") + ` WHERE id = $1`
	tag, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSchedule(rows pgx.Rows, hasRepeat bool) (Schedule, error) {
	fieldDescs := rows.FieldDescriptions()
	values, err := rows.Values()
	if err != nil {
		return Schedule{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return rowToSchedule(fieldDescs, values, hasRepeat)
}

func rowToSchedule(fieldDescs []pgconn.FieldDescription, values []any, hasRepeat bool) (Schedule, error) {
	colIndex := map[string]int{}
	for i, fd := range fieldDescs {
		colIndex[string(fd.Name)] = i
	}
	get := func(name string) any {
		if i, ok := colIndex[name]; ok && i < len(values) {
			return values[i]
		}
		return nil
	}

	s := Schedule{
		ID:             intFromAny(get("id")),
		CronPeriod:     stringFromAny(get("cron_period")),
		CronCustom:     stringFromAny(get("cron_custom")),
		ReportName:     stringFromAny(get("report_name")),
		DatasourceName: stringFromAny(get("datasource_name")),
		Format:         stringFromAny(get("format")),
		Filename:       stringFromAny(get("filename")),
		Email:          stringFromAny(get("email")),
		EmailSubject:   stringFromAny(get("email_subject")),
		EmailBody:      stringFromAny(get("email_body")),
		EmailTemplate:  stringFromAny(get("email_template")),
		NoEmail:        boolFromAny(get("noemail")),
		URLOptParams:   stringFromAny(get("url_opt_params")),
		Enabled:        boolFromAny(get("enabled")),
	}
	if t, ok := get("created_at").(time.Time); ok {
		s.CreatedAt = t
	}
	if t, ok := get("updated_at").(time.Time); ok {
		s.UpdatedAt = t
	}
	if hasRepeat {
		s.ScheduleMode = stringFromAny(get("schedule_mode"))
		s.RepeatMinutes = intPtrFromAny(get("repeat_minutes"))
		s.HourlyMinute = intPtrFromAny(get("hourly_minute"))
		s.DailyTime = stringFromAny(get("daily_time"))
		s.WeeklyDay = stringFromAny(get("weekly_day"))
		s.WeeklyTime = stringFromAny(get("weekly_time"))
		s.MonthlyDay = intPtrFromAny(get("monthly_day"))
		s.MonthlyTime = stringFromAny(get("monthly_time"))
		s.OnceDatetime = stringFromAny(get("once_datetime"))
	}
	return s, nil
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func boolFromAny(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	default:
		return BoolValue(v, true)
	}
}

func intPtrFromAny(v any) *int {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case int32:
		i := int(n)
		return &i
	case int64:
		i := int(n)
		return &i
	case int:
		return &n
	default:
		return nil
	}
}
