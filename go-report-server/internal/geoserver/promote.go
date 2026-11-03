package geoserver

import (
	"regexp"
	"strings"
)

// Column ref capture matching common Jasper / SQL identifiers (quoted or bare).
const columnCapture = `(?:(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)\.){0,2}(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)`

var (
	reEqParamRight = regexp.MustCompile(`(?i)(` + columnCapture + `)\s*=\s*\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reEqParamLeft  = regexp.MustCompile(`(?i)\$P\{([A-Za-z_][A-Za-z0-9_]*)\}\s*=\s*(` + columnCapture + `)`)
	reInParam      = regexp.MustCompile(`(?i)(` + columnCapture + `)\s+IN\s*\(\s*\$P\{([A-Za-z_][A-Za-z0-9_]*)\}\s*\)`)
)

// IsMultiParamValue reports whether a preview param value is multi-valued
// (slice). Comma-separated strings are not treated as multi here — callers that
// only have strings should promote based on parameter metadata instead.
func IsMultiParamValue(v any) bool {
	switch v.(type) {
	case []string, []any:
		return true
	default:
		return false
	}
}

// PromoteCollectionEqualityToXin rewrites "col = $P{name}" / "$P{name} = col" /
// "col IN ($P{name})" into "$X{IN, col, name}" when isCollection(name) is true.
// Cascade lookup SQL often uses scalar equality; multi-select parents need $X{IN}.
func PromoteCollectionEqualityToXin(sql string, isCollection func(name string) bool) string {
	if strings.TrimSpace(sql) == "" || isCollection == nil {
		return sql
	}

	sql = reEqParamRight.ReplaceAllStringFunc(sql, func(match string) string {
		m := reEqParamRight.FindStringSubmatch(match)
		if len(m) < 3 || !isCollection(m[2]) {
			return match
		}
		return "$X{IN, " + m[1] + ", " + m[2] + "}"
	})
	sql = reEqParamLeft.ReplaceAllStringFunc(sql, func(match string) string {
		m := reEqParamLeft.FindStringSubmatch(match)
		if len(m) < 3 || !isCollection(m[1]) {
			return match
		}
		return "$X{IN, " + m[2] + ", " + m[1] + "}"
	})
	sql = reInParam.ReplaceAllStringFunc(sql, func(match string) string {
		m := reInParam.FindStringSubmatch(match)
		if len(m) < 3 || !isCollection(m[2]) {
			return match
		}
		return "$X{IN, " + m[1] + ", " + m[2] + "}"
	})
	return sql
}

// PromoteFromParamValues promotes equality for any param whose preview value is a slice.
func PromoteFromParamValues(sql string, params map[string]any) string {
	return PromoteCollectionEqualityToXin(sql, func(name string) bool {
		if params == nil {
			return false
		}
		v, ok := params[name]
		return ok && IsMultiParamValue(v)
	})
}
