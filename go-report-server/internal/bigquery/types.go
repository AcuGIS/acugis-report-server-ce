package bigquery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

// MapJavaClass maps a BigQuery type name to a Jasper/Java class.
func MapJavaClass(bqType string) string {
	t := normalizeTypeName(bqType)
	switch t {
	case "string", "bytes":
		return "java.lang.String"
	case "integer", "int64":
		return "java.lang.Long"
	case "float", "float64":
		return "java.lang.Double"
	case "numeric", "bignumeric":
		return "java.math.BigDecimal"
	case "boolean", "bool":
		return "java.lang.Boolean"
	case "timestamp":
		return "java.sql.Timestamp"
	case "date":
		return "java.time.LocalDate"
	case "datetime":
		return "java.time.LocalDateTime"
	case "time":
		return "java.time.LocalTime"
	case "geography", "json":
		return "java.lang.String"
	case "record", "struct":
		return "java.lang.String"
	}
	if strings.HasPrefix(t, "array") || strings.HasPrefix(t, "repeated") {
		return "java.lang.String"
	}
	return "java.lang.String"
}

func normalizeTypeName(bqType string) string {
	t := strings.TrimSpace(strings.ToLower(bqType))
	t = strings.TrimPrefix(t, "repeated ")
	if i := strings.Index(t, "<"); i > 0 {
		return strings.TrimSpace(t[:i])
	}
	if i := strings.IndexAny(t, " \t("); i > 0 {
		t = t[:i]
	}
	return t
}

func typeNameFromField(f *bigquery.FieldSchema) string {
	if f == nil {
		return "STRING"
	}
	name := string(f.Type)
	if f.Repeated {
		return "ARRAY<" + name + ">"
	}
	if f.Type == bigquery.RecordFieldType && len(f.Schema) > 0 {
		return "STRUCT"
	}
	return name
}

func formatCell(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return base64.StdEncoding.EncodeToString(t)
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339Nano)
	case *time.Time:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339Nano)
	case civil.Date:
		return t.String()
	case civil.Time:
		return t.String()
	case civil.DateTime:
		return t.String()
	case *big.Rat:
		if t == nil {
			return ""
		}
		return t.FloatString(9)
	case bool, int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", t)
	default:
		if s, ok := t.(fmt.Stringer); ok {
			return s.String()
		}
		return fmt.Sprintf("%v", t)
	}
}

// jasperValue converts a BigQuery value into a JSON-friendly typed value for Jasper fill.
//
// Scalars must keep their logical type (or an unquoted ISO string for temporal types).
// Never return json.Marshal(scalar) as a Go string — that embeds JSON quotes into the
// value (e.g. DATE becomes `"2026-03-22"` instead of 2026-03-22).
func jasperValue(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return t
	case int:
		return int64(t)
	case int8:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case uint:
		return int64(t)
	case uint8:
		return int64(t)
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		if t > uint64(^uint64(0)>>1) {
			return fmt.Sprintf("%d", t)
		}
		return int64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case *big.Rat:
		if t == nil {
			return nil
		}
		return t.FloatString(9)
	case *big.Int:
		if t == nil {
			return nil
		}
		if t.IsInt64() {
			return t.Int64()
		}
		return t.String()
	case time.Time:
		if t.IsZero() {
			return nil
		}
		return t.UTC().Format(time.RFC3339Nano)
	case *time.Time:
		if t == nil || t.IsZero() {
			return nil
		}
		return t.UTC().Format(time.RFC3339Nano)
	case civil.Date:
		return t.String()
	case civil.Time:
		return t.String()
	case civil.DateTime:
		return t.String()
	case []byte:
		if t == nil {
			return nil
		}
		return base64.StdEncoding.EncodeToString(t)
	case bigquery.NullInt64:
		if !t.Valid {
			return nil
		}
		return t.Int64
	case bigquery.NullFloat64:
		if !t.Valid {
			return nil
		}
		return t.Float64
	case bigquery.NullString:
		if !t.Valid {
			return nil
		}
		return t.StringVal
	case bigquery.NullBool:
		if !t.Valid {
			return nil
		}
		return t.Bool
	case bigquery.NullTimestamp:
		if !t.Valid {
			return nil
		}
		return t.Timestamp.UTC().Format(time.RFC3339Nano)
	case bigquery.NullDate:
		if !t.Valid {
			return nil
		}
		return t.Date.String()
	case bigquery.NullTime:
		if !t.Valid {
			return nil
		}
		return t.Time.String()
	case bigquery.NullDateTime:
		if !t.Valid {
			return nil
		}
		return t.DateTime.String()
	case bigquery.NullGeography:
		if !t.Valid {
			return nil
		}
		return t.GeographyVal
	case bigquery.NullJSON:
		if !t.Valid {
			return nil
		}
		return t.JSONVal
	case []any:
		return t
	case map[string]any:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// convertValue normalizes a BigQuery Value for Jasper collection rows.
func convertValue(v bigquery.Value) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []bigquery.Value:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = convertValue(item)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return fmt.Sprintf("%v", out)
		}
		return string(b)
	case map[string]bigquery.Value:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = convertValue(item)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return fmt.Sprintf("%v", out)
		}
		return string(b)
	default:
		return jasperValue(t)
	}
}
