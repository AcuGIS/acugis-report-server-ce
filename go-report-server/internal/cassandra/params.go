package cassandra

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	rePParam = regexp.MustCompile(`(?i)\$P\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	// $X{IN, column, param} / $X{NOTIN, column, param} — Cassandra-safe expansion.
	reXIn = regexp.MustCompile(`(?i)\$X\{\s*(IN|NOTIN)\s*,\s*([^,]+)\s*,\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
)

// preparedCQL is a CQL statement with ? bind markers and ordered values.
type preparedCQL struct {
	CQL    string
	Values []any
}

// bindParameters rewrites AcuGIS/Jasper parameter expressions into Cassandra bind markers.
// Preserves original report CQL elsewhere; this only prepares the execution form.
func bindParameters(cql string, params map[string]any) (preparedCQL, error) {
	cql = strings.TrimSpace(cql)
	if cql == "" {
		return preparedCQL{}, fmt.Errorf("CQL query is empty")
	}

	var values []any
	out := cql

	// Expand $X{IN|NOTIN, col, param} first.
	out = reXIn.ReplaceAllStringFunc(out, func(m string) string {
		sub := reXIn.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		op := strings.ToUpper(sub[1])
		col := strings.TrimSpace(sub[2])
		name := sub[3]
		list := coerceSlice(params[name])
		if len(list) == 0 {
			// Empty IN → always-false / always-true predicates without bind markers.
			if op == "NOTIN" {
				return "true"
			}
			return "false"
		}
		markers := make([]string, len(list))
		for i, v := range list {
			markers[i] = "?"
			values = append(values, coerceBindValue(v))
		}
		joined := strings.Join(markers, ", ")
		if op == "NOTIN" {
			return fmt.Sprintf("%s NOT IN (%s)", col, joined)
		}
		return fmt.Sprintf("%s IN (%s)", col, joined)
	})

	// Replace $P{name} with ? and collect values in appearance order.
	out = rePParam.ReplaceAllStringFunc(out, func(m string) string {
		sub := rePParam.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		name := sub[1]
		v, ok := params[name]
		if !ok {
			values = append(values, nil)
			return "?"
		}
		// Multi-value param used with bare $P{} in an IN list: expand markers.
		if sl := coerceSlice(v); len(sl) > 1 || (len(sl) == 1 && looksLikeCollection(v)) {
			if len(sl) == 0 {
				values = append(values, nil)
				return "?"
			}
			markers := make([]string, len(sl))
			for i, item := range sl {
				markers[i] = "?"
				values = append(values, coerceBindValue(item))
			}
			return strings.Join(markers, ", ")
		}
		values = append(values, coerceBindValue(v))
		return "?"
	})

	return preparedCQL{CQL: out, Values: values}, nil
}

func looksLikeCollection(v any) bool {
	switch v.(type) {
	case []any, []string, []int, []int64, []float64:
		return true
	case string:
		s := strings.TrimSpace(v.(string))
		return strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")
	default:
		return false
	}
}

func coerceSlice(v any) []any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		return t
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	case []int:
		out := make([]any, len(t))
		for i, n := range t {
			out[i] = n
		}
		return out
	case []int64:
		out := make([]any, len(t))
		for i, n := range t {
			out[i] = n
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		// Comma-separated or JSON-ish array.
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			inner := strings.TrimSpace(s[1 : len(s)-1])
			if inner == "" {
				return nil
			}
			parts := splitCSV(inner)
			out := make([]any, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				p = strings.Trim(p, `"'`)
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}
		if strings.Contains(s, ",") {
			parts := splitCSV(s)
			out := make([]any, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				p = strings.Trim(p, `"'`)
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}
		return []any{s}
	default:
		return []any{v}
	}
}

func splitCSV(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\'' {
			inQuote = !inQuote
			cur.WriteByte(c)
			continue
		}
		if c == ',' && !inQuote {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	parts = append(parts, cur.String())
	return parts
}

func coerceBindValue(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return ""
		}
		// Try UUID
		if u, err := uuid.Parse(s); err == nil {
			return u
		}
		// Try bool
		if b, err := strconv.ParseBool(s); err == nil && (s == "true" || s == "false" || s == "TRUE" || s == "FALSE") {
			return b
		}
		// Try int
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		// Try float
		if f, err := strconv.ParseFloat(s, 64); err == nil && strings.ContainsAny(s, ".eE") {
			return f
		}
		// Try RFC3339 timestamp
		if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return tm
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm
		}
		if tm, err := time.Parse("2006-01-02", s); err == nil {
			return tm
		}
		return s
	case float32:
		return float64(t)
	case float64, int, int8, int16, int32, int64, bool, time.Time:
		return t
	default:
		return t
	}
}

// ensureReadOnly rejects non-SELECT CQL used for report datasets.
func ensureReadOnly(cql string) error {
	trimmed := stripCQLComments(cql)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return fmt.Errorf("CQL query is empty")
	}
	upper := strings.ToUpper(trimmed)
	// Allow leading USE keyspace; still require SELECT as the statement intent.
	if strings.HasPrefix(upper, "USE ") {
		rest := strings.TrimSpace(trimmed[3:])
		if i := strings.IndexAny(rest, ";\n"); i >= 0 {
			rest = strings.TrimSpace(rest[i+1:])
		} else {
			return fmt.Errorf("report datasets require a SELECT query")
		}
		upper = strings.ToUpper(rest)
		trimmed = rest
	}
	if !strings.HasPrefix(upper, "SELECT") {
		verb := firstWord(upper)
		switch verb {
		case "INSERT", "UPDATE", "DELETE", "TRUNCATE", "DROP", "ALTER", "CREATE", "BATCH", "GRANT", "REVOKE":
			return fmt.Errorf("report datasets are read-only; %s is not allowed", verb)
		default:
			return fmt.Errorf("report datasets require a SELECT query")
		}
	}
	return nil
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t\n\r"); i > 0 {
		return s[:i]
	}
	return s
}

func stripCQLComments(cql string) string {
	lines := strings.Split(cql, "\n")
	var b strings.Builder
	for _, line := range lines {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	out := b.String()
	// Strip /* */ blocks.
	for {
		start := strings.Index(out, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(out[start+2:], "*/")
		if end < 0 {
			out = out[:start]
			break
		}
		out = out[:start] + out[start+2+end+2:]
	}
	return out
}

// wrapLimitForPreview appends LIMIT when the user query has none (preview/metadata only).
func wrapLimitForPreview(cql string, limit int) string {
	if limit <= 0 {
		return cql
	}
	upper := strings.ToUpper(stripCQLComments(cql))
	if strings.Contains(upper, " LIMIT ") || strings.HasSuffix(strings.TrimSpace(upper), "LIMIT") {
		return cql
	}
	trimmed := strings.TrimRight(strings.TrimSpace(cql), ";")
	return fmt.Sprintf("%s LIMIT %d", trimmed, limit)
}
