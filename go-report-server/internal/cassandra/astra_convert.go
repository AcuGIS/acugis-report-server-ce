package cassandra

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

// ConvertAstraValue converts a raw Astra Data API JSON value into a wire value
// that matches the Jasper/Java class declared for the Cassandra column type.
//
// JSON cannot carry java.math.BigDecimal distinctly, so decimals / varints /
// UUIDs / dates are emitted as strings (or numbers) that PreviewRunner coerces
// using the compiled report field class. Numeric int/long/float/double/boolean
// are emitted as native JSON types.
func ConvertAstraValue(cqlType string, raw any) any {
	if raw == nil {
		return nil
	}
	javaClass := MapJavaClass(cqlType)
	converted := convertByJavaClass(javaClass, normalizeTypeName(cqlType), raw)
	debugTypeLog("", cqlType, javaClass, raw, converted)
	return converted
}

// ConvertAstraRow converts all columns in a Data API row using schema types.
// Unknown columns fall back to best-effort JSON-native values.
func ConvertAstraRow(colTypes map[string]string, row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for name, raw := range row {
		if strings.HasPrefix(name, "$") {
			continue
		}
		if cqlType, ok := colTypes[name]; ok {
			out[name] = ConvertAstraValue(cqlType, raw)
			continue
		}
		out[name] = convertUnknownAstraValue(raw)
		debugTypeLog(name, "(unknown)", MapJavaClass(""), raw, out[name])
	}
	return out
}

func convertByJavaClass(javaClass, cqlType string, raw any) any {
	switch javaClass {
	case "java.math.BigDecimal":
		return decimalWire(raw)
	case "java.math.BigInteger":
		return integerWire(raw)
	case "java.lang.Byte":
		if n, ok := asInt64(raw); ok {
			return int64(int8(n))
		}
		return coerceStringNumber(raw)
	case "java.lang.Short":
		if n, ok := asInt64(raw); ok {
			return int64(int16(n))
		}
		return coerceStringNumber(raw)
	case "java.lang.Integer":
		if n, ok := asInt64(raw); ok {
			return int64(int32(n))
		}
		return coerceStringNumber(raw)
	case "java.lang.Long":
		if n, ok := asInt64(raw); ok {
			return n
		}
		return coerceStringNumber(raw)
	case "java.lang.Float", "java.lang.Double":
		if f, ok := asFloat64(raw); ok {
			return f
		}
		return coerceStringNumber(raw)
	case "java.lang.Boolean":
		return asBool(raw)
	case "java.util.UUID":
		return fmt.Sprint(raw)
	case "java.sql.Timestamp", "java.util.Date":
		return timestampWire(raw)
	case "java.time.LocalDate":
		return dateWire(raw)
	case "[B":
		return blobWire(raw)
	case "java.lang.String":
		return stringWire(cqlType, raw)
	default:
		return stringWire(cqlType, raw)
	}
}

func convertUnknownAstraValue(raw any) any {
	switch t := raw.(type) {
	case nil:
		return nil
	case bool:
		return t
	case float64:
		if t == math.Trunc(t) && t >= float64(math.MinInt64) && t <= float64(math.MaxInt64) {
			return int64(t)
		}
		return t
	case float32:
		return float64(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case string, int, int8, int16, int32, int64:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

func decimalWire(raw any) any {
	switch t := raw.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		// Avoid binary float artifacts for whole numbers; otherwise use precise-ish format.
		if t == math.Trunc(t) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", t)
	case *big.Rat:
		if t == nil {
			return nil
		}
		return t.FloatString(18)
	case *big.Float:
		if t == nil {
			return nil
		}
		return t.Text('f', -1)
	case map[string]any:
		// Astra occasionally wraps decimals; prefer explicit string fields.
		for _, k := range []string{"value", "$decimal", "decimal"} {
			if s, ok := t[k].(string); ok {
				return s
			}
		}
		b, _ := json.Marshal(t)
		return string(b)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func integerWire(raw any) any {
	switch t := raw.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", t)
	case *big.Int:
		if t == nil {
			return nil
		}
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func timestampWire(raw any) any {
	switch t := raw.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		// Epoch millis heuristic.
		if t > 1e11 {
			return time.UnixMilli(int64(t)).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(int64(t), 0).UTC().Format(time.RFC3339Nano)
	case int64:
		if t > 1e11 {
			return time.UnixMilli(t).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(t, 0).UTC().Format(time.RFC3339Nano)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func dateWire(raw any) any {
	switch t := raw.(type) {
	case string:
		s := strings.TrimSpace(t)
		if i := strings.IndexByte(s, 'T'); i > 0 {
			return s[:i]
		}
		if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
			return s[:10]
		}
		return s
	case float64:
		return time.UnixMilli(int64(t)).UTC().Format("2006-01-02")
	case int64:
		return time.UnixMilli(t).UTC().Format("2006-01-02")
	case time.Time:
		return t.UTC().Format("2006-01-02")
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func blobWire(raw any) any {
	switch t := raw.(type) {
	case string:
		return t
	case []byte:
		return base64.StdEncoding.EncodeToString(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

func stringWire(cqlType string, raw any) any {
	switch t := raw.(type) {
	case string:
		return t
	case bool, float64, float32, int, int8, int16, int32, int64:
		return fmt.Sprint(t)
	case json.Number:
		return t.String()
	default:
		if strings.HasPrefix(cqlType, "list") || strings.HasPrefix(cqlType, "set") ||
			strings.HasPrefix(cqlType, "map") || strings.HasPrefix(cqlType, "tuple") ||
			cqlType == "udt" {
			b, err := json.Marshal(t)
			if err != nil {
				return fmt.Sprint(t)
			}
			return string(b)
		}
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		// Prefer plain string when JSON encoded a quoted string.
		var s string
		if json.Unmarshal(b, &s) == nil {
			return s
		}
		return string(b)
	}
}

func asInt64(raw any) (int64, bool) {
	switch t := raw.(type) {
	case int:
		return int64(t), true
	case int8:
		return int64(t), true
	case int16:
		return int64(t), true
	case int32:
		return int64(t), true
	case int64:
		return t, true
	case float64:
		if t == math.Trunc(t) {
			return int64(t), true
		}
	case float32:
		if float64(t) == math.Trunc(float64(t)) {
			return int64(t), true
		}
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i, true
		}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

func asFloat64(raw any) (float64, bool) {
	switch t := raw.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f, true
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func asBool(raw any) any {
	switch t := raw.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "y", "t":
			return true
		case "false", "0", "no", "n", "f":
			return false
		}
	case float64:
		return t != 0
	case int, int64:
		return fmt.Sprint(t) != "0"
	}
	return raw
}

func coerceStringNumber(raw any) any {
	switch t := raw.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func astraDebugEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ACUGIS_CASSANDRA_DEBUG")))
	if v == "1" || v == "true" || v == "yes" {
		return true
	}
	v = strings.ToLower(strings.TrimSpace(os.Getenv("ACUGIS_DEBUG")))
	return v == "1" || v == "true" || v == "yes"
}

func debugTypeLog(field, cqlType, javaClass string, raw, converted any) {
	if !astraDebugEnabled() {
		return
	}
	rawType := fmt.Sprintf("%T", raw)
	convType := fmt.Sprintf("%T", converted)
	name := field
	if name == "" {
		name = "(value)"
	}
	log.Printf("[acugis-cassandra-astra] type field=%s cassandraType=%s jasperClass=%s rawGoType=%s convertedGoType=%s",
		name, cqlType, javaClass, rawType, convType)
}
