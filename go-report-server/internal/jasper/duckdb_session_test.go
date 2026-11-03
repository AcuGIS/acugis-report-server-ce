package jasper_test

import (
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

func TestAttachDuckDBSessionFormURLEncoded(t *testing.T) {
	session := map[string]any{
		"mode":         "database",
		"databasePath": "/data/analytics.duckdb",
	}
	method, rawQuery, ct, body, err := jasper.AttachDuckDBSession(
		"_repName=demo&_dataSource=Duck1&_repFormat=pdf&p_state=CA",
		session,
	)
	if err != nil {
		t.Fatal(err)
	}
	if method != "POST" {
		t.Fatalf("method=%s", method)
	}
	if rawQuery != "" {
		t.Fatalf("rawQuery should be empty, got %q", rawQuery)
	}
	if !strings.Contains(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("ct=%s", ct)
	}
	b, _ := io.ReadAll(body)
	vals, err := url.ParseQuery(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("_repName") != "demo" || vals.Get("_dataSource") != "Duck1" {
		t.Fatalf("missing report params: %v", vals)
	}
	if vals.Get("_collectionData") != "" {
		t.Fatal("must not attach collection data for DuckDB JDBC")
	}
	if !strings.Contains(vals.Get("_duckdbSession"), `"database"`) {
		t.Fatalf("session missing: %s", vals.Get("_duckdbSession"))
	}
	if vals.Get("p_state") != "CA" {
		t.Fatalf("param lost: %v", vals)
	}
}
