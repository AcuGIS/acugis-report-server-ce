package audit

import (
	"context"
	"log/slog"
	"time"
)

// RetentionRunner periodically deletes audit events older than the retention window.
type RetentionRunner struct {
	svc      *Service
	interval time.Duration
	logger   *slog.Logger
	stop     chan struct{}
	done     chan struct{}
}

// NewRetentionRunner builds a background cleaner. Interval defaults to 24h.
func NewRetentionRunner(svc *Service, interval time.Duration, logger *slog.Logger) *RetentionRunner {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionRunner{
		svc:      svc,
		interval: interval,
		logger:   logger,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start launches the cleanup loop in a background goroutine.
func (r *RetentionRunner) Start() {
	if r == nil || r.svc == nil || !r.svc.Enabled() {
		if r != nil {
			close(r.done)
		}
		return
	}
	go r.loop()
}

// Stop signals the loop to exit and waits for it to finish.
func (r *RetentionRunner) Stop() {
	if r == nil {
		return
	}
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	<-r.done
}

func (r *RetentionRunner) loop() {
	defer close(r.done)
	// Run once shortly after startup, then on interval.
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-timer.C:
			r.runOnce()
			timer.Reset(r.interval)
		}
	}
}

func (r *RetentionRunner) runOnce() {
	deleted, err := r.svc.PurgeExpired(context.Background())
	if err != nil {
		r.logger.Error("audit retention cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		r.logger.Info("audit retention cleanup", "deleted", deleted, "retention_days", r.svc.RetentionDays())
	}
}

// PurgeExpired deletes events older than the retention window.
func (s *Service) PurgeExpired(ctx context.Context) (int64, error) {
	if s == nil || !s.opts.Enabled {
		return 0, nil
	}
	repo := s.repo
	if repo == nil || !repo.Configured() {
		return 0, nil
	}
	days := s.RetentionDays()
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	return repo.DeleteOlderThan(ctx, cutoff)
}
