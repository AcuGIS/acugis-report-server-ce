package audit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/audit"
)

func TestRecordLoginAndReportExecution(t *testing.T) {
	mem := &audit.MemoryWriter{}
	svc := audit.NewServiceWithWriter(mem, audit.Options{Enabled: true, LogParameters: true})

	req := httptest.NewRequest(http.MethodPost, "/login.php", nil)
	req.Header.Set("User-Agent", "audit-test-agent")
	req.RemoteAddr = "192.168.1.20:443"

	svc.RecordRequest(req, audit.Event{
		Category: audit.CategoryAuthentication,
		Action:   audit.ActionLoginSuccess,
		Username: "admin",
		Status:   audit.StatusSuccess,
	})
	svc.RecordRequest(req, audit.Event{
		Category: audit.CategoryAuthentication,
		Action:   audit.ActionLoginFailed,
		Username: "admin",
		Status:   audit.StatusFailure,
	})
	svc.RecordRequest(req, audit.Event{
		Category: audit.CategoryAuthentication,
		Action:   audit.ActionLogout,
		Status:   audit.StatusSuccess,
	})

	dur := int64(1240)
	size := int64(4096)
	svc.RecordRequest(req, audit.Event{
		Category:     audit.CategoryReport,
		Action:       audit.ActionReportExecute,
		ObjectType:   audit.ObjectReport,
		ObjectID:     "sales_by_region",
		ObjectName:   "Sales by Region",
		Status:       audit.StatusSuccess,
		DurationMS:   &dur,
		OutputFormat: "html",
		OutputSize:   &size,
		Details: map[string]any{
			"parameters": map[string]any{
				"p_country": []any{"USA"},
				"password":  "should-not-leak",
			},
		},
	})

	events := mem.Snapshot()
	if len(events) != 4 {
		t.Fatalf("got %d events", len(events))
	}

	login := events[0]
	if login.Action != audit.ActionLoginSuccess || login.Username != "admin" {
		t.Fatalf("login: %+v", login)
	}
	if login.IPAddress == "" {
		t.Fatal("expected IP from request")
	}
	if login.UserAgent != "audit-test-agent" {
		t.Fatalf("ua=%q", login.UserAgent)
	}

	fail := events[1]
	if fail.Status != audit.StatusFailure || fail.Action != audit.ActionLoginFailed {
		t.Fatalf("fail login: %+v", fail)
	}
	// Ensure password never appears in details
	for _, e := range events {
		if e.Details != nil {
			b := stringify(e.Details)
			if strings.Contains(strings.ToLower(b), "should-not-leak") {
				t.Fatalf("password leaked into audit: %s", b)
			}
		}
	}

	rep := events[3]
	if rep.ObjectID != "sales_by_region" || rep.OutputFormat != "html" {
		t.Fatalf("report: %+v", rep)
	}
	if rep.DurationMS == nil || *rep.DurationMS != 1240 {
		t.Fatalf("duration: %v", rep.DurationMS)
	}
	if rep.Details == nil {
		t.Fatal("expected sanitized details")
	}
	params, _ := rep.Details["parameters"].(map[string]any)
	if params["password"] != "[redacted]" {
		t.Fatalf("password not redacted in details: %#v", params)
	}
}

func stringify(v any) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(
		strings.TrimSpace(toStr(v)), "\n", " "), "\t", " "))
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		parts := make([]string, 0, len(t))
		for k, val := range t {
			parts = append(parts, k+"="+toStr(val))
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func TestMemoryRetentionDeletesExpiredOnly(t *testing.T) {
	store := &audit.MemoryStore{}
	now := time.Now().UTC()
	_ = store.Insert(context.Background(), audit.Event{
		Category:  audit.CategorySystem,
		Action:    "old",
		CreatedAt: now.Add(-120 * 24 * time.Hour),
	})
	_ = store.Insert(context.Background(), audit.Event{
		Category:  audit.CategorySystem,
		Action:    "recent",
		CreatedAt: now.Add(-10 * 24 * time.Hour),
	})

	cutoff := now.Add(-90 * 24 * time.Hour)
	deleted, err := store.DeleteOlderThan(context.Background(), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d", deleted)
	}
	events := store.Snapshot()
	if len(events) != 1 || events[0].Action != "recent" {
		t.Fatalf("remaining=%+v", events)
	}
}
