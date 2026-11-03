package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/database"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

// jrs-schedule-dispatcher replaces systemd timers in container runtimes.
// It polls due schedules and execs the existing jrs-run-schedule command.
// Native Ubuntu installs continue to use systemd timers; this binary is unused there.
func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("postgres unavailable", "error", err)
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	defer db.Close()

	schema := cfg.Database.Schema
	repo := schedules.NewRepository(db.PGX(), schema)
	executions := schedules.NewExecutionRepository(db.PGX(), schema)
	dispatcher := schedules.NewDispatcher(schedules.DispatcherOptions{
		Repo:       repo,
		Executions: executions,
		Pool:       db.PGX(),
		Run:        schedules.ExecRunSchedule,
		Logger:     logger,
		Location:   time.Local,
	})

	healthAddr := schedules.DispatcherHealthAddrFromEnv()
	healthSrv := &http.Server{
		Addr:              healthAddr,
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           healthHandler(),
	}
	go func() {
		logger.Info("scheduler health listening", "addr", healthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("scheduler health server failed", "error", err)
		}
	}()

	err = dispatcher.RunLoop(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = healthSrv.Shutdown(shutdownCtx)
	if err != nil && err != context.Canceled {
		logger.Error("dispatcher exited", "error", err)
		return 1
	}
	return 0
}

func healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "service": "report-scheduler"})
	})
	return mux
}
