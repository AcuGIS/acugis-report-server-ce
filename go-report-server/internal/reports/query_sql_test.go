package reports_test

import (
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestQuerySQLString(t *testing.T) {
	def := &reports.Definition{Raw: map[string]any{"query": "SELECT * FROM customers"}}
	if got := def.QuerySQL(); got != "SELECT * FROM customers" {
		t.Fatalf("got %q", got)
	}
}

func TestQuerySQLManualSqlNested(t *testing.T) {
	def := &reports.Definition{Raw: map[string]any{
		"query": map[string]any{
			"mode":      "sql",
			"manualSql": "SELECT id, sales FROM sales_by_customer WHERE state = :state",
		},
	}}
	want := "SELECT id, sales FROM sales_by_customer WHERE state = :state"
	if got := def.QuerySQL(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestQuerySQLGeneratedFallback(t *testing.T) {
	def := &reports.Definition{Raw: map[string]any{
		"query": map[string]any{
			"mode":         "visual",
			"generatedSql": "SELECT * FROM t",
		},
	}}
	if got := def.QuerySQL(); got != "SELECT * FROM t" {
		t.Fatalf("got %q", got)
	}
}

func TestQuerySQLTopLevelCQL(t *testing.T) {
	def := &reports.Definition{Raw: map[string]any{"cql": "SELECT * FROM ks.t"}}
	if got := def.QuerySQL(); got != "SELECT * FROM ks.t" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractQueryFromProjectJSON(t *testing.T) {
	raw := []byte(`{
		"query": {
			"mode": "sql",
			"manualSql": "SELECT customer_id, sales FROM sales_by_customer"
		}
	}`)
	got := reports.ExtractQueryFromProjectJSON(raw)
	if got != "SELECT customer_id, sales FROM sales_by_customer" {
		t.Fatalf("got %q", got)
	}
}

func TestQuerySQLEmpty(t *testing.T) {
	def := &reports.Definition{Raw: map[string]any{}}
	if got := def.QuerySQL(); got != "" {
		t.Fatalf("got %q", got)
	}
}
