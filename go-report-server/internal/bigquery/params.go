package bigquery

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
)

var (
	rePParam = regexp.MustCompile(`(?i)\$P\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	reXIn    = regexp.MustCompile(`(?i)\$X\{\s*(IN|NOTIN)\s*,\s*([^,]+)\s*,\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
)

type preparedQuery struct {
	SQL        string
	Parameters []bigquery.QueryParameter
}

// bindParameters rewrites Jasper $P{} / $X{} into BigQuery named @params.
func bindParameters(sql string, params map[string]any) (preparedQuery, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return preparedQuery{}, fmt.Errorf("BigQuery query is empty")
	}

	var bqParams []bigquery.QueryParameter
	used := map[string]int{}
	out := sql

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
			if op == "NOTIN" {
				return "TRUE"
			}
			return "FALSE"
		}
		markers := make([]string, len(list))
		for i, v := range list {
			pname := uniqueParamName(name, used)
			markers[i] = "@" + pname
			bqParams = append(bqParams, bigquery.QueryParameter{
				Name:  pname,
				Value: coerceBindValue(v),
			})
		}
		joined := strings.Join(markers, ", ")
		if op == "NOTIN" {
			return fmt.Sprintf("%s NOT IN UNNEST([%s])", col, joined)
		}
		return fmt.Sprintf("%s IN UNNEST([%s])", col, joined)
	})

	out = rePParam.ReplaceAllStringFunc(out, func(m string) string {
		sub := rePParam.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		name := sub[1]
		v, ok := params[name]
		if !ok {
			pname := uniqueParamName(name, used)
			bqParams = append(bqParams, bigquery.QueryParameter{Name: pname, Value: nil})
			return "@" + pname
		}
		if sl := coerceSlice(v); len(sl) > 1 || (len(sl) == 1 && looksLikeCollection(v)) {
			if len(sl) == 0 {
				pname := uniqueParamName(name, used)
				bqParams = append(bqParams, bigquery.QueryParameter{Name: pname, Value: nil})
				return "@" + pname
			}
			markers := make([]string, len(sl))
			for i, item := range sl {
				pname := uniqueParamName(name, used)
				markers[i] = "@" + pname
				bqParams = append(bqParams, bigquery.QueryParameter{
					Name:  pname,
					Value: coerceBindValue(item),
				})
			}
			return strings.Join(markers, ", ")
		}
		pname := uniqueParamName(name, used)
		bqParams = append(bqParams, bigquery.QueryParameter{
			Name:  pname,
			Value: coerceBindValue(v),
		})
		return "@" + pname
	})

	return preparedQuery{SQL: out, Parameters: bqParams}, nil
}

func uniqueParamName(base string, used map[string]int) string {
	n := used[base]
	used[base] = n + 1
	if n == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, n)
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
		if b, err := strconv.ParseBool(s); err == nil && (s == "true" || s == "false" || s == "TRUE" || s == "FALSE") {
			return b
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && strings.ContainsAny(s, ".eE") {
			return f
		}
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

func ensureReadOnly(sql string) error {
	trimmed := stripSQLComments(sql)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return fmt.Errorf("BigQuery query is empty")
	}
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		verb := firstWord(upper)
		switch verb {
		case "INSERT", "UPDATE", "DELETE", "MERGE", "DROP", "ALTER", "CREATE", "TRUNCATE", "GRANT", "REVOKE":
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

func stripSQLComments(sql string) string {
	lines := strings.Split(sql, "\n")
	var b strings.Builder
	for _, line := range lines {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	out := b.String()
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
