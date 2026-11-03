package embeddedmap

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var reCQLField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// GeoServerFilterMapping maps a report parameter onto a GeoServer attribute filter.
type GeoServerFilterMapping struct {
	ReportParameter string `json:"reportParameter"`
	GeoServerField  string `json:"geoserverField"`
	Operator        string `json:"operator"`
}

// BuildCQLFilter builds a GeoServer CQL_FILTER from declarative mappings + report values.
// Returns "" when there are no usable clauses (omit CQL_FILTER entirely).
func BuildCQLFilter(mappings []GeoServerFilterMapping, reportParams map[string]any) string {
	var clauses []string
	for _, row := range mappings {
		reportParam := strings.TrimSpace(row.ReportParameter)
		field := strings.TrimSpace(row.GeoServerField)
		op := strings.TrimSpace(row.Operator)
		if op == "" {
			op = "="
		}
		if reportParam == "" || field == "" || !reCQLField.MatchString(field) {
			continue
		}
		raw, ok := reportParams[reportParam]
		if !ok {
			continue
		}
		values := filterValues(raw)
		if len(values) == 0 {
			continue
		}
		opKey := strings.ToUpper(op)
		if opKey == "LIKE" || opKey == "ILIKE" || opKey == "IN" {
			op = opKey
		}
		if clause := cqlClause(field, op, values); clause != "" {
			clauses = append(clauses, clause)
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	return strings.Join(clauses, " AND ")
}

func filterValues(raw any) []any {
	if raw == nil {
		return nil
	}
	switch t := raw.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return []any{s}
	case bool, int, int64, float64, float32:
		return []any{t}
	case []string:
		var out []any
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		var out []any
		for _, item := range t {
			if item == nil {
				continue
			}
			switch v := item.(type) {
			case string:
				s := strings.TrimSpace(v)
				if s != "" {
					out = append(out, s)
				}
			case bool, int, int64, float64, float32:
				out = append(out, v)
			default:
				s := strings.TrimSpace(fmt.Sprint(v))
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s == "" {
			return nil
		}
		return []any{s}
	}
}

func cqlLiteral(value any) string {
	switch t := value.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	}
	s := fmt.Sprint(value)
	if isUnquotedNumeric(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func isUnquotedNumeric(s string) bool {
	if s == "" || strings.Contains(s, " ") {
		return false
	}
	// Leading-zero codes stay quoted.
	if len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func cqlClause(field, operator string, values []any) string {
	if len(values) == 0 {
		return ""
	}
	op := operator
	multi := len(values) > 1
	opUpper := strings.ToUpper(op)
	if multi && opUpper != "LIKE" && opUpper != "ILIKE" {
		op = "IN"
		opUpper = "IN"
	}
	if opUpper == "IN" {
		lits := make([]string, 0, len(values))
		for _, v := range values {
			lits = append(lits, cqlLiteral(v))
		}
		return field + " IN (" + strings.Join(lits, ",") + ")"
	}
	if opUpper == "LIKE" || opUpper == "ILIKE" {
		if !multi {
			return field + " " + opUpper + " " + cqlLiteral(values[0])
		}
		parts := make([]string, 0, len(values))
		for _, v := range values {
			parts = append(parts, field+" "+opUpper+" "+cqlLiteral(v))
		}
		return "(" + strings.Join(parts, " OR ") + ")"
	}
	allowed := map[string]bool{"=": true, "<>": true, ">": true, ">=": true, "<": true, "<=": true}
	if !allowed[op] {
		op = "="
	}
	return field + op + cqlLiteral(values[0])
}
