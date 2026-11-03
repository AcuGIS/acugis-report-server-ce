package cassandra

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"reflect"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"gopkg.in/inf.v0"
)

// MapJavaClass maps a Cassandra CQL type name to a Jasper/Java class.
func MapJavaClass(cqlType string) string {
	t := normalizeTypeName(cqlType)
	switch t {
	case "ascii", "text", "varchar", "inet", "duration":
		return "java.lang.String"
	case "tinyint":
		return "java.lang.Byte"
	case "smallint":
		return "java.lang.Short"
	case "int":
		return "java.lang.Integer"
	case "bigint", "counter", "time":
		return "java.lang.Long"
	case "varint":
		return "java.math.BigInteger"
	case "decimal":
		return "java.math.BigDecimal"
	case "float":
		return "java.lang.Float"
	case "double":
		return "java.lang.Double"
	case "boolean":
		return "java.lang.Boolean"
	case "date":
		return "java.time.LocalDate"
	case "timestamp":
		return "java.sql.Timestamp"
	case "uuid", "timeuuid":
		return "java.util.UUID"
	case "blob":
		return "[B"
	}
	if strings.HasPrefix(t, "list<") || strings.HasPrefix(t, "set<") ||
		strings.HasPrefix(t, "map<") || strings.HasPrefix(t, "tuple<") ||
		strings.HasPrefix(t, "frozen<") || t == "udt" || strings.Contains(t, ".") {
		return "java.lang.String"
	}
	return "java.lang.String"
}

func normalizeTypeName(cqlType string) string {
	t := strings.TrimSpace(strings.ToLower(cqlType))
	if strings.HasPrefix(t, "frozen<") && strings.HasSuffix(t, ">") {
		inner := t[len("frozen<") : len(t)-1]
		return normalizeTypeName(inner)
	}
	if i := strings.Index(t, "<"); i > 0 {
		base := t[:i]
		switch base {
		case "list", "set", "map", "tuple":
			return t
		}
	}
	if i := strings.IndexAny(t, " \t("); i > 0 {
		t = t[:i]
	}
	return t
}

// scanTarget returns a pointer suitable for Iter.Scan based on CQL type info.
func scanTarget(ti gocql.TypeInfo) any {
	if ti == nil {
		var s *string
		return &s
	}
	switch ti.Type() {
	case gocql.TypeAscii, gocql.TypeVarchar, gocql.TypeText, gocql.TypeInet, gocql.TypeDuration:
		var s *string
		return &s
	case gocql.TypeTinyInt:
		var v *int8
		return &v
	case gocql.TypeSmallInt:
		var v *int16
		return &v
	case gocql.TypeInt:
		var v *int
		return &v
	case gocql.TypeBigInt, gocql.TypeCounter, gocql.TypeTime:
		var v *int64
		return &v
	case gocql.TypeVarint:
		var v *big.Int
		return &v
	case gocql.TypeDecimal:
		var v *inf.Dec
		return &v
	case gocql.TypeFloat:
		var v *float32
		return &v
	case gocql.TypeDouble:
		var v *float64
		return &v
	case gocql.TypeBoolean:
		var v *bool
		return &v
	case gocql.TypeTimestamp:
		var v *time.Time
		return &v
	case gocql.TypeDate:
		var v *time.Time
		return &v
	case gocql.TypeUUID, gocql.TypeTimeUUID:
		var v *gocql.UUID
		return &v
	case gocql.TypeBlob:
		var v []byte
		return &v
	default:
		var v any
		return &v
	}
}

func derefScan(ptr any) any {
	if ptr == nil {
		return nil
	}
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Ptr {
		return ptr
	}
	if rv.IsNil() {
		return nil
	}
	elem := rv.Elem()
	if elem.Kind() == reflect.Ptr {
		if elem.IsNil() {
			return nil
		}
		return elem.Interface()
	}
	if !elem.IsValid() {
		return nil
	}
	// []byte scanned into *[]byte via non-nil pointer to slice
	if elem.Kind() == reflect.Slice && elem.IsNil() {
		return nil
	}
	return elem.Interface()
}

// formatCell converts a scanned Cassandra value to a preview string.
func formatCell(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		if t == nil {
			return ""
		}
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
	case gocql.UUID:
		return t.String()
	case *gocql.UUID:
		if t == nil {
			return ""
		}
		return t.String()
	case net.IP:
		return t.String()
	case *big.Int:
		if t == nil {
			return ""
		}
		return t.String()
	case *inf.Dec:
		if t == nil {
			return ""
		}
		return t.String()
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

// jasperValue converts a scanned value into a JSON-friendly typed value for Jasper fill.
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
	case *big.Int:
		if t == nil {
			return nil
		}
		return t.String()
	case *inf.Dec:
		if t == nil {
			return nil
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
	case gocql.UUID:
		return t.String()
	case *gocql.UUID:
		if t == nil {
			return nil
		}
		return t.String()
	case []byte:
		if t == nil {
			return nil
		}
		return base64.StdEncoding.EncodeToString(t)
	case net.IP:
		return t.String()
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

func typeNameFromInfo(ti gocql.TypeInfo) string {
	if ti == nil {
		return "text"
	}
	switch ti.Type() {
	case gocql.TypeAscii:
		return "ascii"
	case gocql.TypeVarchar:
		return "varchar"
	case gocql.TypeText:
		return "text"
	case gocql.TypeTinyInt:
		return "tinyint"
	case gocql.TypeSmallInt:
		return "smallint"
	case gocql.TypeInt:
		return "int"
	case gocql.TypeBigInt:
		return "bigint"
	case gocql.TypeCounter:
		return "counter"
	case gocql.TypeVarint:
		return "varint"
	case gocql.TypeDecimal:
		return "decimal"
	case gocql.TypeFloat:
		return "float"
	case gocql.TypeDouble:
		return "double"
	case gocql.TypeBoolean:
		return "boolean"
	case gocql.TypeTimestamp:
		return "timestamp"
	case gocql.TypeDate:
		return "date"
	case gocql.TypeTime:
		return "time"
	case gocql.TypeUUID:
		return "uuid"
	case gocql.TypeTimeUUID:
		return "timeuuid"
	case gocql.TypeInet:
		return "inet"
	case gocql.TypeBlob:
		return "blob"
	case gocql.TypeDuration:
		return "duration"
	case gocql.TypeList:
		return "list"
	case gocql.TypeSet:
		return "set"
	case gocql.TypeMap:
		return "map"
	case gocql.TypeTuple:
		return "tuple"
	case gocql.TypeUDT:
		return "udt"
	default:
		return fmt.Sprintf("type_%d", int(ti.Type()))
	}
}
