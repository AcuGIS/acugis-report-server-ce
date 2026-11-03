package cassandra

import (
	"encoding/json"
	"testing"
)

func TestConvertAstraValueDecimal(t *testing.T) {
	cases := []any{
		"123.45",
		json.Number("123.45"),
		float64(123.45),
		123,
	}
	for _, raw := range cases {
		got := ConvertAstraValue("decimal", raw)
		s, ok := got.(string)
		if !ok || s == "" {
			t.Fatalf("decimal raw=%v (%T) -> %T %v (want non-empty string wire)", raw, raw, got, got)
		}
	}
}

func TestConvertAstraRowSampleTable(t *testing.T) {
	types := map[string]string{
		"customer_id":   "uuid",
		"customer_name": "text",
		"state":         "text",
		"city":          "text",
		"sales":         "decimal",
		"active":        "boolean",
		"created_at":    "timestamp",
	}
	row := map[string]any{
		"customer_id":   "550e8400-e29b-41d4-a716-446655440000",
		"customer_name": "Acme",
		"state":         "Texas",
		"city":          "Austin",
		"sales":         json.Number("1999.50"),
		"active":        true,
		"created_at":    "2024-01-15T12:00:00.000Z",
	}
	out := ConvertAstraRow(types, row)
	if MapJavaClass(types["sales"]) != "java.math.BigDecimal" {
		t.Fatalf("sales class=%s", MapJavaClass(types["sales"]))
	}
	if _, ok := out["sales"].(string); !ok {
		t.Fatalf("sales wire should be string for BigDecimal coercion, got %T", out["sales"])
	}
	if out["sales"] != "1999.50" {
		t.Fatalf("sales=%v", out["sales"])
	}
	if out["active"] != true {
		t.Fatalf("active=%v", out["active"])
	}
	if out["customer_id"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("uuid=%v", out["customer_id"])
	}
	if MapJavaClass(types["customer_id"]) != "java.util.UUID" {
		t.Fatalf("uuid class")
	}
	if MapJavaClass(types["created_at"]) != "java.sql.Timestamp" {
		t.Fatalf("timestamp class")
	}
	if MapJavaClass(types["active"]) != "java.lang.Boolean" {
		t.Fatalf("boolean class")
	}
}

func TestConvertAstraValueIntAndBool(t *testing.T) {
	if got := ConvertAstraValue("int", float64(42)); got != int64(42) {
		t.Fatalf("int=%v (%T)", got, got)
	}
	if got := ConvertAstraValue("bigint", json.Number("99")); got != int64(99) {
		t.Fatalf("bigint=%v (%T)", got, got)
	}
	if got := ConvertAstraValue("boolean", "true"); got != true {
		t.Fatalf("bool=%v", got)
	}
}
