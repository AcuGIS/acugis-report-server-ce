// Package collectionfile reads report-owned tabular documents into typed row maps
// for Jasper collection fill. CSV and JSON are implemented. XLSX can reuse the
// column kinds, the sample-based type merge, and the query subset later.
// Parsers take an io.Reader so they do not know where the document is stored.
package collectionfile

import (
	"encoding/json"
	"time"
)

// Kind is the inferred column type stored in row maps.
type Kind string

const (
	KindString   Kind = "string"
	KindInteger  Kind = "integer"
	KindDecimal  Kind = "decimal"
	KindBoolean  Kind = "boolean"
	KindDate     Kind = "date"
	KindDateTime Kind = "datetime"
)

// DefaultSampleLimit is the number of non-null values inspected per column.
const DefaultSampleLimit = 200

// MaxBytes is the largest file ParseCSV will read.
const MaxBytes = 32 << 20

// Options controls parsing. Delimiter is used when Auto is false.
// A zero Delimiter means comma.
type Options struct {
	Delimiter   rune
	Auto        bool
	SampleLimit int
}

// Column is one header and its inferred kind.
type Column struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
}

// Table is a typed in-memory sheet.
type Table struct {
	Columns   []Column         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	Delimiter rune             `json:"-"`
}

// Date is a calendar date. JSON is YYYY-MM-DD so Jasper can coerce java.sql.Date
// without treating the value as a timestamp string.
type Date struct {
	Time time.Time
}

// MarshalJSON encodes the date as YYYY-MM-DD.
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Time.UTC().Format("2006-01-02"))
}

// DateTime is an instant. JSON is RFC3339 so Jasper can coerce java.sql.Timestamp.
type DateTime struct {
	Time time.Time
}

// MarshalJSON encodes the timestamp in UTC RFC3339.
func (d DateTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Time.UTC().Format(time.RFC3339))
}

// JavaClass is the Jasper field class for a column kind.
func JavaClass(kind Kind) string {
	switch kind {
	case KindInteger:
		return "java.lang.Long"
	case KindDecimal:
		return "java.lang.Double"
	case KindBoolean:
		return "java.lang.Boolean"
	case KindDate:
		return "java.sql.Date"
	case KindDateTime:
		return "java.sql.Timestamp"
	default:
		return "java.lang.String"
	}
}
