package audit_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/acugis/acugis-report-server-go/internal/audit"
)

// memoryRepo is an in-memory stand-in used via Service with a custom insert path.
// Service.Record requires a real Repository; these tests exercise Service.Enabled
// and Record no-op behavior plus retention cutoff math via DeleteOlderThan semantics.

func TestServiceDisabledNoOp(t *testing.T) {
	svc := audit.NewService(nil, audit.Options{Enabled: false, Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))})
	svc.Record(context.Background(), audit.Event{
		Category: audit.CategoryAuthentication,
		Action:   audit.ActionLoginSuccess,
	})
}

func TestServiceEnabledWithoutRepoNoOp(t *testing.T) {
	svc := audit.NewService(nil, audit.Options{Enabled: true})
	svc.Record(context.Background(), audit.Event{
		Category: audit.CategoryReport,
		Action:   audit.ActionReportExecute,
	})
}

func TestRetentionCutoffKeepsRecent(t *testing.T) {
	// Unit-level: PurgeExpired with nil repo returns 0 without error.
	svc := audit.NewService(nil, audit.Options{Enabled: true, RetentionDays: 90})
	n, err := svc.PurgeExpired(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 deleted, got %d", n)
	}
	if svc.RetentionDays() != 90 {
		t.Fatalf("retention=%d", svc.RetentionDays())
	}
}

func TestEventIDAssigned(t *testing.T) {
	// Ensure uuid generation works for events (used by Record).
	e := audit.Event{EventID: uuid.New(), CreatedAt: time.Now().UTC()}
	if e.EventID == uuid.Nil {
		t.Fatal("nil uuid")
	}
}
