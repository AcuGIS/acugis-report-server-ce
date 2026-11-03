package bigquery

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

func TestMapJavaClass(t *testing.T) {
	cases := map[string]string{
		"STRING":        "java.lang.String",
		"INT64":         "java.lang.Long",
		"INTEGER":       "java.lang.Long",
		"FLOAT64":       "java.lang.Double",
		"NUMERIC":       "java.math.BigDecimal",
		"BIGNUMERIC":    "java.math.BigDecimal",
		"BOOL":          "java.lang.Boolean",
		"BOOLEAN":       "java.lang.Boolean",
		"TIMESTAMP":     "java.sql.Timestamp",
		"DATE":          "java.time.LocalDate",
		"DATETIME":      "java.time.LocalDateTime",
		"TIME":          "java.time.LocalTime",
		"GEOGRAPHY":     "java.lang.String",
		"JSON":          "java.lang.String",
		"ARRAY<STRING>": "java.lang.String",
		"STRUCT":        "java.lang.String",
	}
	for in, want := range cases {
		if got := MapJavaClass(in); got != want {
			t.Errorf("MapJavaClass(%q)=%q want %q", in, got, want)
		}
	}
}

func TestJasperValueCivilDateNotJSONQuoted(t *testing.T) {
	d := civil.Date{Year: 2026, Month: time.March, Day: 22}
	got := jasperValue(d)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("type %T", got)
	}
	if s != "2026-03-22" {
		t.Fatalf("got %q — must not include JSON quotes", s)
	}
	row := map[string]any{"sale_date": convertValue(d)}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	ds, _ := decoded["sale_date"].(string)
	if ds != "2026-03-22" {
		t.Fatalf("JSON round-trip sale_date=%q raw=%s", ds, string(b))
	}
}

func TestJasperValueScalars(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{int64(10), int64(10)},
		{float64(2200.0), float64(2200.0)},
		{true, true},
		{"Madrid", "Madrid"},
		{civil.Time{Hour: 9, Minute: 5, Second: 1}, "09:05:01"},
		{bigquery.NullDate{Date: civil.Date{Year: 2026, Month: 3, Day: 22}, Valid: true}, "2026-03-22"},
		{bigquery.NullDate{Valid: false}, nil},
	}
	for _, tc := range cases {
		got := jasperValue(tc.in)
		if got != tc.want {
			t.Errorf("jasperValue(%v)=%v (%T) want %v", tc.in, got, got, tc.want)
		}
	}
	rat := big.NewRat(22001, 10)
	got := jasperValue(rat)
	if s, ok := got.(string); !ok || s == "" || s[0] == '"' {
		t.Fatalf("NUMERIC Rat got %#v", got)
	}
}

func TestConvertValueDoesNotDoubleEncodeDate(t *testing.T) {
	d := civil.Date{Year: 2026, Month: time.March, Day: 22}
	got := convertValue(d)
	if s, ok := got.(string); !ok || s != "2026-03-22" {
		t.Fatalf("convertValue DATE = %#v", got)
	}
}

func TestBindParameters(t *testing.T) {
	q, err := bindParameters("SELECT * FROM t WHERE state = $P{p_state} AND id = $P{p_id}", map[string]any{
		"p_state": "CA",
		"p_id":    int64(42),
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.SQL != "SELECT * FROM t WHERE state = @p_state AND id = @p_id" {
		t.Fatalf("SQL=%q", q.SQL)
	}
	if len(q.Parameters) != 2 {
		t.Fatalf("params=%d", len(q.Parameters))
	}
}

func TestEnsureReadOnly(t *testing.T) {
	if err := ensureReadOnly("SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := ensureReadOnly("DELETE FROM t"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLooksLikeServiceAccountJSON(t *testing.T) {
	if looksLikeServiceAccountJSON(`{"type":"service_account","private_key":"x","client_email":"a@b.com"}`) {
		// ok
	} else {
		t.Fatal("expected true")
	}
	if looksLikeServiceAccountJSON("not-json") {
		t.Fatal("expected false")
	}
}
