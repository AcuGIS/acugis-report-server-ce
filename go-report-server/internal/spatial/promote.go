package spatial

import (
	"regexp"
	"strings"
)

// Column ref capture matching IsSafeJasperColumnRef (without anchors).
const jasperColumnCapture = `(?:(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)\.){0,2}(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)`

var (
	reEqParamRight = regexp.MustCompile(`(?i)(` + jasperColumnCapture + `)\s*=\s*\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reEqParamLeft  = regexp.MustCompile(`(?i)\$P\{([A-Za-z_][A-Za-z0-9_]*)\}\s*=\s*(` + jasperColumnCapture + `)`)
	reInParam      = regexp.MustCompile(`(?i)(` + jasperColumnCapture + `)\s+IN\s*\(\s*\$P\{([A-Za-z_][A-Za-z0-9_]*)\}\s*\)`)
)

// PromoteCollectionEqualityToXin rewrites scalar equality / IN ($P{name}) against a
// Collection parameter into $X{IN, column, name}. Cascade lookup SQL often uses
// "col = $P{parent}" from Designer guidance; multi-select parents need $X{IN}.
func PromoteCollectionEqualityToXin(sql string, filterContext map[string]any, jrxmlParams map[string]ParamMeta) string {
	if strings.TrimSpace(sql) == "" {
		return sql
	}
	isColl := func(name string) bool {
		return paramIsCollection(name, filterContext, jrxmlParams)
	}

	sql = reEqParamRight.ReplaceAllStringFunc(sql, func(match string) string {
		m := reEqParamRight.FindStringSubmatch(match)
		if len(m) < 3 || !isColl(m[2]) {
			return match
		}
		return "$X{IN, " + m[1] + ", " + m[2] + "}"
	})
	sql = reEqParamLeft.ReplaceAllStringFunc(sql, func(match string) string {
		m := reEqParamLeft.FindStringSubmatch(match)
		if len(m) < 3 || !isColl(m[1]) {
			return match
		}
		return "$X{IN, " + m[2] + ", " + m[1] + "}"
	})
	sql = reInParam.ReplaceAllStringFunc(sql, func(match string) string {
		m := reInParam.FindStringSubmatch(match)
		if len(m) < 3 || !isColl(m[2]) {
			return match
		}
		return "$X{IN, " + m[1] + ", " + m[2] + "}"
	})
	return sql
}

func paramIsCollection(name string, filterContext map[string]any, jrxmlParams map[string]ParamMeta) bool {
	if meta, ok := jrxmlParams[name]; ok && meta.Kind == "collection" {
		return true
	}
	if raw, ok := filterContext[name]; ok {
		if _, isArr := asAnySlice(raw); isArr {
			return true
		}
	}
	return false
}
