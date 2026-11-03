package schedules

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultDispatcherPollInterval is the idle cadence between due-checks.
	DefaultDispatcherPollInterval = 15 * time.Second
	// DefaultDispatcherHealthAddr is the HTTP bind for GET /health.
	DefaultDispatcherHealthAddr = ":9089"
	// dispatcherLockClass is the PostgreSQL advisory-lock class (ASCII "JRS\x01").
	dispatcherLockClass int32 = 0x4a525301
)

// RunFunc executes one schedule id (production: jrs-run-schedule <id>).
type RunFunc func(ctx context.Context, scheduleID int) error

// Dispatcher polls enabled schedules and invokes the existing runner command
// when a timer spec is due. It replaces systemd timers in container runtimes
// without duplicating report/email execution logic.
type Dispatcher struct {
	repo       *Repository
	executions *ExecutionRepository
	pool       *pgxpool.Pool
	run        RunFunc
	poll       time.Duration
	logger     *slog.Logger
	location   *time.Location
}

// DispatcherOptions configures a Dispatcher.
type DispatcherOptions struct {
	Repo       *Repository
	Executions *ExecutionRepository
	Pool       *pgxpool.Pool
	Run        RunFunc
	Poll       time.Duration
	Logger     *slog.Logger
	Location   *time.Location
}

// NewDispatcher constructs a schedule dispatcher.
func NewDispatcher(opts DispatcherOptions) *Dispatcher {
	poll := opts.Poll
	if poll <= 0 {
		poll = DispatcherPollIntervalFromEnv()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	run := opts.Run
	if run == nil {
		run = ExecRunSchedule
	}
	return &Dispatcher{
		repo:       opts.Repo,
		executions: opts.Executions,
		pool:       opts.Pool,
		run:        run,
		poll:       poll,
		logger:     logger,
		location:   loc,
	}
}

// PollInterval returns the configured poll cadence.
func (d *Dispatcher) PollInterval() time.Duration {
	if d == nil || d.poll <= 0 {
		return DefaultDispatcherPollInterval
	}
	return d.poll
}

// RunLoop polls until ctx is cancelled. One schedule's failure does not stop later ones.
func (d *Dispatcher) RunLoop(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("dispatcher unavailable")
	}
	d.logger.Info("schedule dispatcher started",
		"poll", d.poll.String(),
		"tz", d.location.String(),
		"runner", RunnerCommandFromEnv(),
	)
	d.Tick(ctx)
	ticker := time.NewTicker(d.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.logger.Info("schedule dispatcher stopping", "reason", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			d.Tick(ctx)
		}
	}
}

// Tick evaluates every enabled schedule once. Errors are logged and skipped.
func (d *Dispatcher) Tick(ctx context.Context) {
	if d == nil || d.repo == nil {
		return
	}
	list, err := d.repo.List(ctx)
	if err != nil {
		d.logger.Error("list schedules failed", "error", err)
		return
	}
	now := time.Now().In(d.location)
	for _, schedule := range list {
		if ctx.Err() != nil {
			return
		}
		if err := d.maybeRun(ctx, schedule, now); err != nil {
			d.logger.Error("schedule tick failed", "schedule_id", schedule.ID, "error", err)
		}
	}
}

func (d *Dispatcher) maybeRun(ctx context.Context, schedule Schedule, now time.Time) error {
	if !NeedsSystemd(schedule) {
		return nil
	}
	spec := BuildTimerSpec(schedule)
	last, err := d.latest(ctx, schedule.ID)
	if err != nil {
		return err
	}
	if !IsDue(schedule, spec, now, last) {
		return nil
	}

	unlock, locked, err := d.tryLock(ctx, schedule.ID)
	if err != nil {
		return err
	}
	if !locked {
		d.logger.Info("schedule already locked, skipping", "schedule_id", schedule.ID)
		return nil
	}
	defer unlock()

	last, err = d.latest(ctx, schedule.ID)
	if err != nil {
		return err
	}
	if !IsDue(schedule, spec, now, last) {
		return nil
	}

	d.logger.Info("dispatching schedule",
		"schedule_id", schedule.ID,
		"mode", EffectiveMode(schedule),
		"spec_type", spec.Type,
		"calendar", spec.Calendar,
		"interval", spec.Interval,
	)
	if err := d.run(ctx, schedule.ID); err != nil {
		d.logger.Error("schedule execution failed", "schedule_id", schedule.ID, "error", err)
		return nil
	}
	d.logger.Info("schedule execution finished", "schedule_id", schedule.ID)
	return nil
}

func (d *Dispatcher) latest(ctx context.Context, scheduleID int) (*Execution, error) {
	if d.executions == nil {
		return nil, nil
	}
	return d.executions.LatestForSchedule(ctx, scheduleID)
}

func (d *Dispatcher) tryLock(ctx context.Context, scheduleID int) (unlock func(), locked bool, err error) {
	if d.pool == nil {
		return func() {}, true, nil
	}
	var ok bool
	err = d.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, $2)`, dispatcherLockClass, int32(scheduleID)).Scan(&ok)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return func() {}, false, nil
	}
	unlocked := false
	return func() {
		if unlocked {
			return
		}
		unlocked = true
		_, _ = d.pool.Exec(context.Background(), `SELECT pg_advisory_unlock($1, $2)`, dispatcherLockClass, int32(scheduleID))
	}, true, nil
}

// DispatcherPollIntervalFromEnv reads JRS_SCHEDULER_POLL_INTERVAL (Go duration).
func DispatcherPollIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("JRS_SCHEDULER_POLL_INTERVAL"))
	if raw == "" {
		return DefaultDispatcherPollInterval
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultDispatcherPollInterval
	}
	return d
}

// DispatcherHealthAddrFromEnv reads JRS_SCHEDULER_LISTEN_ADDR.
func DispatcherHealthAddrFromEnv() string {
	addr := strings.TrimSpace(os.Getenv("JRS_SCHEDULER_LISTEN_ADDR"))
	if addr == "" {
		return DefaultDispatcherHealthAddr
	}
	return addr
}

// RunnerCommandFromEnv is the existing oneshot command (jrs-run-schedule).
func RunnerCommandFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("JRS_SCHEDULE_RUNNER")); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "jrs-run-schedule")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		if runtime.GOOS == "windows" {
			win := candidate + ".exe"
			if info, err := os.Stat(win); err == nil && !info.IsDir() {
				return win
			}
		}
	}
	if path, err := exec.LookPath("jrs-run-schedule"); err == nil {
		return path
	}
	return "jrs-run-schedule"
}

// ExecRunSchedule invokes the existing jrs-run-schedule <id> command.
func ExecRunSchedule(ctx context.Context, scheduleID int) error {
	bin := RunnerCommandFromEnv()
	cmd := exec.CommandContext(ctx, bin, strconv.Itoa(scheduleID))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %d: %w", bin, scheduleID, err)
	}
	return nil
}

// ScheduleRuntime returns "dispatcher" when JRS_SCHEDULE_RUNTIME=dispatcher,
// otherwise "systemd" (native Ubuntu default).
func ScheduleRuntime() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("JRS_SCHEDULE_RUNTIME")))
	if v == "dispatcher" {
		return "dispatcher"
	}
	return "systemd"
}
