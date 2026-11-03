package duckdb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// MapJavaClass maps a DuckDB type name to a Jasper/Java class.
func MapJavaClass(duckType string) string {
	t := normalizeTypeName(duckType)
	switch {
	case t == "boolean", t == "bool":
		return "java.lang.Boolean"
	case t == "tinyint":
		return "java.lang.Byte"
	case t == "smallint":
		return "java.lang.Short"
	case t == "integer", t == "int", t == "int4":
		return "java.lang.Integer"
	case t == "bigint", t == "int8", t == "long":
		return "java.lang.Long"
	case t == "hugeint", t == "uhugeint":
		return "java.math.BigDecimal"
	case t == "float", t == "float4", t == "real":
		return "java.lang.Float"
	case t == "double", t == "float8":
		return "java.lang.Double"
	case strings.HasPrefix(t, "decimal"), strings.HasPrefix(t, "numeric"):
		return "java.math.BigDecimal"
	case t == "varchar", t == "char", t == "bpchar", t == "text", t == "string":
		return "java.lang.String"
	case t == "date":
		return "java.time.LocalDate"
	case t == "time", t == "timetz":
		return "java.lang.String"
	case t == "timestamp", t == "datetime":
		return "java.sql.Timestamp"
	case strings.HasPrefix(t, "timestamp with time zone"), t == "timestamptz":
		return "java.sql.Timestamp"
	case t == "uuid":
		return "java.util.UUID"
	case t == "blob", t == "bytea", t == "varbinary", t == "binary":
		return "[B"
	case t == "json", t == "jsonb":
		return "java.lang.String"
	case strings.HasPrefix(t, "list"), strings.HasPrefix(t, "array"),
		strings.HasPrefix(t, "struct"), strings.HasPrefix(t, "map"),
		strings.HasPrefix(t, "union"):
		return "java.lang.String"
	default:
		return "java.lang.String"
	}
}

func normalizeTypeName(duckType string) string {
	t := strings.TrimSpace(strings.ToLower(duckType))
	if i := strings.Index(t, "("); i > 0 {
		base := strings.TrimSpace(t[:i])
		if base == "decimal" || base == "numeric" || base == "varchar" || base == "char" {
			return base
		}
	}
	return t
}

func formatCell(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		if len(t) > 64 {
			return fmt.Sprintf("<blob %d bytes>", len(t))
		}
		return base64.StdEncoding.EncodeToString(t)
	case time.Time:
		if t.IsZero() {
			return ""
		}
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
			return t.Format("2006-01-02")
		}
		return t.UTC().Format(time.RFC3339Nano)
	case *big.Int:
		if t == nil {
			return ""
		}
		return t.String()
	case *big.Float:
		if t == nil {
			return ""
		}
		return t.Text('f', -1)
	case bool, int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

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
	case float32:
		return float64(t)
	case float64:
		return t
	case time.Time:
		if t.IsZero() {
			return nil
		}
		return t.UTC().Format(time.RFC3339Nano)
	case []byte:
		if t == nil {
			return nil
		}
		return base64.StdEncoding.EncodeToString(t)
	case *big.Int:
		if t == nil {
			return nil
		}
		return t.String()
	case *big.Float:
		if t == nil {
			return nil
		}
		f, _ := t.Float64()
		return f
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}
