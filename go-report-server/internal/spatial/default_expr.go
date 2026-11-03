package spatial

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reQuotedString   = regexp.MustCompile(`"((?:\\.|[^"\\])*)"|'((?:\\.|[^'\\])*)'`)
	reAsListCall     = regexp.MustCompile(`(?i)(?:java\.util\.)?(?:Arrays\.asList|List\.of|Collections\.singletonList|Collections\.singleton)\s*\(`)
	reEmptyListCall  = regexp.MustCompile(`(?i)(?:java\.util\.)?Collections\.emptyList\s*\(\s*\)`)
	reIntegerValueOf = regexp.MustCompile(`(?i)(?:java\.lang\.)?Integer\.valueOf\s*\(\s*(-?\d+)\s*\)`)
	reLongValueOf    = regexp.MustCompile(`(?i)(?:java\.lang\.)?Long\.valueOf\s*\(\s*(-?\d+)\s*\)`)
	reBooleanValueOf = regexp.MustCompile(`(?i)(?:java\.lang\.)?Boolean\.(?:valueOf|TRUE|FALSE)\b(?:\s*\(\s*(true|false)\s*\))?`)
)

// ParseDefaultValueExpression evaluates common JRXML defaultValueExpression forms
// into a JSON-friendly value (string, bool, number, or []string for collections).
// Complex / unsupported expressions return nil (Jasper fill still evaluates them
// when the parameter is omitted from the request).
func ParseDefaultValueExpression(expr string) any {
	expr = strings.TrimSpace(expr)
	if expr == "" || strings.EqualFold(expr, "null") {
		return nil
	}

	if reEmptyListCall.MatchString(expr) {
		return []string{}
	}

	if reAsListCall.MatchString(expr) {
		vals := extractQuotedStrings(expr)
		if vals == nil {
			vals = []string{}
		}
		return vals
	}

	if m := reIntegerValueOf.FindStringSubmatch(expr); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reLongValueOf.FindStringSubmatch(expr); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reBooleanValueOf.FindStringSubmatch(expr); len(m) > 0 {
		token := strings.ToLower(m[0])
		if strings.Contains(token, "false") {
			return false
		}
		if strings.Contains(token, "true") {
			return true
		}
		if len(m) >= 2 && strings.EqualFold(m[1], "false") {
			return false
		}
		if len(m) >= 2 && strings.EqualFold(m[1], "true") {
			return true
		}
	}

	if unquoted, ok := unquoteLiteral(expr); ok {
		return unquoted
	}

	lower := strings.ToLower(expr)
	switch lower {
	case "true", "boolean.true":
		return true
	case "false", "boolean.false":
		return false
	}

	if matched, _ := regexp.MatchString(`^-?\d+$`, expr); matched {
		n, _ := strconv.Atoi(expr)
		return n
	}
	if f, err := strconv.ParseFloat(expr, 64); err == nil && strings.ContainsAny(expr, ".eE") {
		return f
	}

	// Unsupported expression (method calls, new ..., etc.)
	return nil
}

func extractQuotedStrings(expr string) []string {
	matches := reQuotedString.FindAllStringSubmatch(expr, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		out = append(out, unescapeJavaString(raw))
	}
	return out
}

func unquoteLiteral(expr string) (string, bool) {
	if len(expr) < 2 {
		return "", false
	}
	quote := expr[0]
	if (quote != '"' && quote != '\'') || expr[len(expr)-1] != quote {
		return "", false
	}
	return unescapeJavaString(expr[1 : len(expr)-1]), true
}

func unescapeJavaString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"', '\'', '\\':
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
