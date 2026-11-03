package geoserver

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Identifier scanning matches the ArcGIS quote-aware FROM parser so a feature
// type such as "LINZ NZ River Name Lines - Pilot" stays one name. Whitespace
// splitting is not used on table references.

var (
	reSelectFrom = regexp.MustCompile(`(?is)^\s*SELECT\s+(.+?)\s+FROM\s+(.+?)(?:\s+WHERE\s+(.+?))?(?:\s+ORDER\s+BY\s+(.+?))?\s*$`)
	reIdent      = regexp.MustCompile(`"((?:[^"]|"")*)"|([A-Za-z_][A-Za-z0-9_]*)`)
	reParamP     = regexp.MustCompile(`\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reParamColon = regexp.MustCompile(`(^|[^:A-Za-z0-9_]):([A-Za-z_][A-Za-z0-9_]*)`)
	reXin        = regexp.MustCompile(`(?i)\$X\{(IN|NOTIN)\s*,\s*([^,]+),\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	reEqNull     = regexp.MustCompile(`(?i)((?:"(?:[^"]|"")*"|[A-Za-z_][A-Za-z0-9_]*))\s*=\s*NULL\b`)
	reNeNull     = regexp.MustCompile(`(?i)((?:"(?:[^"]|"")*"|[A-Za-z_][A-Za-z0-9_]*))\s*(?:<>|!=)\s*NULL\b`)
	reQual       = regexp.MustCompile(`(?:"(?:[^"]|"")*"|[A-Za-z_][A-Za-z0-9_]*)\.("((?:[^"]|"")*)"|([A-Za-z_][A-Za-z0-9_]*))`)
)

type parsedSQL struct {
	Fields  []string
	Schema  string
	Table   string
	Alias   string
	Where   string
	OrderBy string
}

type wfsQuery struct {
	TypeName string
	Fields   []string
	CQL      string
	SortBy   string
}

func parseGeneratedSQL(sql string) (*parsedSQL, error) {
	sql = strings.TrimSpace(sql)
	sql = strings.TrimSuffix(sql, ";")
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return nil, fmt.Errorf("GeoServer query is empty")
	}
	if err := rejectUnsupportedSQL(sql); err != nil {
		return nil, err
	}
	m := reSelectFrom.FindStringSubmatch(sql)
	if m == nil {
		return nil, fmt.Errorf("unsupported GeoServer query SQL (expected a single-feature-type SELECT). GeoServer WFS does not accept arbitrary SQL")
	}
	selectList := strings.TrimSpace(m[1])
	fromClause := strings.TrimSpace(m[2])
	where := strings.TrimSpace(m[3])
	orderBy := strings.TrimSpace(m[4])

	schema, table, alias := splitTableRef(fromClause)
	if table == "" {
		return nil, fmt.Errorf("could not parse the GeoServer feature type in FROM")
	}
	if strings.Contains(selectList, "(") {
		return nil, fmt.Errorf("GeoServer WFS queries do not support SQL functions")
	}
	if strings.HasPrefix(strings.ToUpper(selectList), "DISTINCT") {
		return nil, fmt.Errorf("GeoServer WFS queries do not support DISTINCT")
	}

	fields := []string{}
	if selectList != "*" {
		for _, part := range splitSelectList(selectList) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if idx := strings.LastIndex(strings.ToUpper(part), " AS "); idx >= 0 {
				part = strings.TrimSpace(part[:idx])
			}
			name := columnNameFromExpr(part)
			if name == "" || name == "*" {
				return nil, fmt.Errorf("unsupported GeoServer select item %q", strings.TrimSpace(part))
			}
			fields = append(fields, name)
		}
		if len(fields) == 0 {
			return nil, fmt.Errorf("GeoServer query has no attribute fields")
		}
	}
	return &parsedSQL{
		Fields:  fields,
		Schema:  schema,
		Table:   table,
		Alias:   alias,
		Where:   where,
		OrderBy: orderBy,
	}, nil
}

func translateQuery(sql string, params map[string]any) (*wfsQuery, error) {
	parsed, err := parseGeneratedSQL(sql)
	if err != nil {
		return nil, err
	}
	cql, err := translateWhere(parsed.Where, params)
	if err != nil {
		return nil, err
	}
	sortBy, err := translateOrderBy(parsed.OrderBy)
	if err != nil {
		return nil, err
	}
	return &wfsQuery{
		TypeName: qualifiedTypeName(parsed.Schema, parsed.Table),
		Fields:   parsed.Fields,
		CQL:      cql,
		SortBy:   sortBy,
	}, nil
}

func qualifiedTypeName(schema, table string) string {
	schema = strings.TrimSpace(schema)
	table = strings.TrimSpace(table)
	if schema == "" || strings.EqualFold(schema, "default") {
		return table
	}
	return schema + ":" + table
}

func rejectUnsupportedSQL(sql string) error {
	for _, kw := range []struct{ word, msg string }{
		{"JOIN", "GeoServer WFS queries do not support JOINs across feature types"},
		{"UNION", "GeoServer WFS queries do not support UNION"},
		{"EXCEPT", "GeoServer WFS queries do not support EXCEPT"},
		{"INTERSECT", "GeoServer WFS queries do not support INTERSECT"},
		{"GROUP BY", "GeoServer WFS queries do not support GROUP BY"},
		{"HAVING", "GeoServer WFS queries do not support HAVING"},
		{"LIMIT", "GeoServer WFS queries do not support LIMIT; the preview and report row cap is applied by the provider"},
		{"OFFSET", "GeoServer WFS queries do not support OFFSET"},
		{"FETCH", "GeoServer WFS queries do not support FETCH"},
	} {
		if containsKeyword(sql, kw.word) {
			return fmt.Errorf("%s", kw.msg)
		}
	}
	return nil
}

func containsKeyword(sql, keyword string) bool {
	upper := strings.ToUpper(sql)
	kw := strings.ToUpper(keyword)
	inQuote := false
	for i := 0; i < len(upper); {
		if sql[i] == '\'' {
			if inQuote && i+1 < len(sql) && sql[i+1] == '\'' {
				i += 2
				continue
			}
			inQuote = !inQuote
			i++
			continue
		}
		if sql[i] == '"' && !inQuote {
			i++
			for i < len(sql) {
				if sql[i] == '"' {
					if i+1 < len(sql) && sql[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		if !inQuote && strings.HasPrefix(upper[i:], kw) {
			beforeOK := i == 0 || !isIdentByte(upper[i-1])
			after := i + len(kw)
			afterOK := after >= len(upper) || !isIdentByte(upper[after])
			if beforeOK && afterOK {
				return true
			}
		}
		i++
	}
	return false
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func splitTableRef(from string) (schema, table, alias string) {
	from = strings.TrimSpace(from)
	names, alias := scanQualifiedName(from)
	switch len(names) {
	case 0:
		return "", "", ""
	case 1:
		return "", names[0], alias
	case 2:
		return names[0], names[1], alias
	default:
		return "", "", ""
	}
}

// scanQualifiedName returns the dot-separated identifiers and the optional alias.
// Quoted identifiers keep internal spaces and hyphens. Doubled quotes are unescaped.
func scanQualifiedName(from string) (names []string, alias string) {
	i := 0
	n := len(from)

	skipSpace := func() {
		for i < n {
			r, size := utf8.DecodeRuneInString(from[i:])
			if !unicode.IsSpace(r) {
				return
			}
			i += size
		}
	}
	readIdent := func() (string, bool) {
		skipSpace()
		if i >= n {
			return "", false
		}
		if from[i] == '"' {
			i++
			var b strings.Builder
			for i < n {
				if from[i] == '"' {
					if i+1 < n && from[i+1] == '"' {
						b.WriteByte('"')
						i += 2
						continue
					}
					i++
					return b.String(), true
				}
				b.WriteByte(from[i])
				i++
			}
			return b.String(), true
		}
		r, size := utf8.DecodeRuneInString(from[i:])
		if !isSQLIdentStart(r) {
			return "", false
		}
		start := i
		i += size
		for i < n {
			r, size = utf8.DecodeRuneInString(from[i:])
			if !isSQLIdentCont(r) {
				break
			}
			i += size
		}
		return from[start:i], true
	}
	take := func() bool {
		name, ok := readIdent()
		if !ok {
			return false
		}
		if name != "" {
			names = append(names, name)
		}
		return true
	}
	if !take() {
		return nil, ""
	}
	for {
		save := i
		skipSpace()
		if i < n && from[i] == '.' {
			i++
			if !take() {
				i = save
				break
			}
			continue
		}
		i = save
		break
	}
	tok, ok := readIdent()
	if !ok {
		return names, ""
	}
	if strings.EqualFold(tok, "AS") {
		if a, ok := readIdent(); ok {
			return names, a
		}
		return names, ""
	}
	return names, tok
}

func isSQLIdentStart(r rune) bool {
	return r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isSQLIdentCont(r rune) bool {
	return isSQLIdentStart(r) || (r >= '0' && r <= '9')
}

func splitSelectList(s string) []string {
	var parts []string
	var b strings.Builder
	depth := 0
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"' && !inQuote:
			inQuote = true
			b.WriteRune(r)
		case r == '"' && inQuote:
			inQuote = false
			b.WriteRune(r)
		case inQuote:
			b.WriteRune(r)
		case r == '(':
			depth++
			b.WriteRune(r)
		case r == ')':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		case r == ',' && depth == 0:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func columnNameFromExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	idents := reIdent.FindAllStringSubmatch(expr, -1)
	if len(idents) == 0 {
		return ""
	}
	last := idents[len(idents)-1]
	if last[1] != "" {
		return strings.ReplaceAll(last[1], `""`, `"`)
	}
	return last[2]
}

func translateWhere(where string, params map[string]any) (string, error) {
	where = strings.TrimSpace(where)
	if where == "" {
		return "", nil
	}
	where = PromoteFromParamValues(where, params)
	var expandErr error
	where = reXin.ReplaceAllStringFunc(where, func(m string) string {
		if expandErr != nil {
			return m
		}
		sub := reXin.FindStringSubmatch(m)
		if len(sub) < 4 {
			expandErr = fmt.Errorf("could not read $X parameter clause %q", m)
			return m
		}
		clause, err := expandIn(sub[1], sub[2], sub[3], params)
		if err != nil {
			expandErr = err
			return m
		}
		return clause
	})
	if expandErr != nil {
		return "", expandErr
	}
	where = replaceScalarParams(where, params)
	where = stripQualifiedColumns(where)
	where = replaceOutsideQuotes(where, "!=", "<>")
	where = reNeNull.ReplaceAllString(where, "$1 IS NOT NULL")
	where = reEqNull.ReplaceAllString(where, "$1 IS NULL")
	if strings.Contains(where, "$P{") || strings.Contains(where, "$X{") {
		return "", fmt.Errorf("GeoServer query has a parameter clause that could not be translated")
	}
	if err := validateFilter(where); err != nil {
		return "", err
	}
	return strings.TrimSpace(where), nil
}

func expandIn(op, col, name string, params map[string]any) (string, error) {
	col = cqlIdent(columnNameFromExpr(col))
	if col == "" {
		return "", fmt.Errorf("GeoServer IN clause is missing a field")
	}
	lits := paramLiterals(params, name)
	op = strings.ToUpper(strings.TrimSpace(op))
	if len(lits) == 0 {
		if op == "NOTIN" {
			return "1=1", nil
		}
		return "1=0", nil
	}
	if op == "NOTIN" {
		return fmt.Sprintf("%s NOT IN (%s)", col, strings.Join(lits, ", ")), nil
	}
	return fmt.Sprintf("%s IN (%s)", col, strings.Join(lits, ", ")), nil
}

func replaceScalarParams(where string, params map[string]any) string {
	replace := func(name string) string {
		return cqlLiteral(paramScalar(params, name))
	}
	where = reParamP.ReplaceAllStringFunc(where, func(m string) string {
		sub := reParamP.FindStringSubmatch(m)
		if len(sub) < 2 {
			return "NULL"
		}
		return replace(sub[1])
	})
	where = reParamColon.ReplaceAllStringFunc(where, func(m string) string {
		sub := reParamColon.FindStringSubmatch(m)
		if len(sub) < 3 {
			return "NULL"
		}
		return sub[1] + replace(sub[2])
	})
	return where
}

func paramScalar(params map[string]any, name string) any {
	if params == nil {
		return nil
	}
	v, ok := params[name]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		if len(t) == 0 {
			return nil
		}
		return t[0]
	case []any:
		if len(t) == 0 {
			return nil
		}
		return t[0]
	default:
		return v
	}
}

func paramLiterals(params map[string]any, name string) []string {
	if params == nil {
		return nil
	}
	v, ok := params[name]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		if len(t) == 0 {
			return nil
		}
		out := make([]string, 0, len(t))
		for _, s := range t {
			out = append(out, cqlLiteral(s))
		}
		return out
	case []any:
		if len(t) == 0 {
			return nil
		}
		out := make([]string, 0, len(t))
		for _, item := range t {
			if item == nil {
				continue
			}
			out = append(out, cqlLiteral(item))
		}
		return out
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, cqlLiteral(p))
		}
		return out
	default:
		return []string{cqlLiteral(t)}
	}
}

// cqlLiteral quotes strings and leaves numbers and booleans unquoted.
// Leading-zero codes stay quoted. A blank string stays ”. Null is NULL.
func cqlLiteral(value any) string {
	switch t := value.(type) {
	case nil:
		return "NULL"
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", t)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case time.Time:
		return "'" + t.UTC().Format(time.RFC3339) + "'"
	case string:
		return cqlStringLiteral(t)
	default:
		return cqlStringLiteral(fmt.Sprint(t))
	}
}

func cqlStringLiteral(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "null") && strings.TrimSpace(s) != "" && !strings.EqualFold(s, "null") {
		// Preserve mixed-case text that is not the SQL null token.
	}
	if strings.TrimSpace(s) == "" && s != "" {
		// A whitespace-only string is still a string.
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	if s == "" {
		return "''"
	}
	if strings.EqualFold(s, "null") {
		return "NULL"
	}
	if isUnquotedNumeric(strings.TrimSpace(s)) {
		return strings.TrimSpace(s)
	}
	if strings.EqualFold(s, "true") || strings.EqualFold(s, "false") {
		return strings.ToLower(s)
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func isUnquotedNumeric(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	if len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' {
		return false
	}
	if len(s) > 2 && s[0] == '-' && s[1] == '0' && s[2] >= '0' && s[2] <= '9' {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func stripQualifiedColumns(where string) string {
	return reQual.ReplaceAllStringFunc(where, func(m string) string {
		sub := reQual.FindStringSubmatch(m)
		if len(sub) < 4 {
			return m
		}
		if sub[2] != "" {
			return cqlIdent(strings.ReplaceAll(sub[2], `""`, `"`))
		}
		return cqlIdent(sub[3])
	})
}

func cqlIdent(name string) string {
	name = strings.TrimSpace(name)
	if len(name) >= 2 && name[0] == '"' && name[len(name)-1] == '"' {
		name = strings.ReplaceAll(name[1:len(name)-1], `""`, `"`)
	}
	if name == "" {
		return ""
	}
	if isSimpleIdent(name) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func isSimpleIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !isSQLIdentStart(r) {
				return false
			}
			continue
		}
		if !isSQLIdentCont(r) {
			return false
		}
	}
	return true
}

func translateOrderBy(orderBy string) (string, error) {
	orderBy = strings.TrimSpace(orderBy)
	if orderBy == "" {
		return "", nil
	}
	parts := splitSelectList(orderBy)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dir := "A"
		upper := strings.ToUpper(p)
		if strings.HasSuffix(upper, " DESC") {
			dir = "D"
			p = strings.TrimSpace(p[:len(p)-5])
		} else if strings.HasSuffix(upper, " ASC") {
			p = strings.TrimSpace(p[:len(p)-4])
		}
		name := cqlIdent(columnNameFromExpr(p))
		if name == "" {
			return "", fmt.Errorf("could not read ORDER BY item %q", p)
		}
		// QueryEscape turns spaces into '+'. The trailing +A/+D is GeoServer's
		// sort direction separator and must stay a literal '+'.
		out = append(out, url.QueryEscape(name)+"+"+dir)
	}
	return strings.Join(out, ","), nil
}

func validateFilter(expr string) error {
	i := 0
	n := len(expr)
	for i < n {
		r, size := utf8.DecodeRuneInString(expr[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		switch r {
		case '\'':
			i++
			for i < n {
				if expr[i] == '\'' {
					if i+1 < n && expr[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '"':
			i++
			for i < n {
				if expr[i] == '"' {
					if i+1 < n && expr[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '(', ')', ',', '=', '<', '>', '+', '-', '.', '*':
			i += size
		default:
			if isSQLIdentStart(r) || (r >= '0' && r <= '9') {
				start := i
				i += size
				for i < n {
					rr, sz := utf8.DecodeRuneInString(expr[i:])
					if unicode.IsSpace(rr) || strings.ContainsRune("(),=<>'+-.", rr) {
						break
					}
					if !isSQLIdentCont(rr) && rr != '.' && rr != ':' {
						return fmt.Errorf("unsupported GeoServer filter near %q", snippet(expr, start))
					}
					i += sz
				}
				word := expr[start:i]
				if isSQLIdentStart(r) {
					if err := checkFilterWord(expr, i, word); err != nil {
						return err
					}
				}
				continue
			}
			return fmt.Errorf("unsupported GeoServer filter near %q", snippet(expr, i))
		}
	}
	return nil
}

func checkFilterWord(expr string, after int, word string) error {
	upper := strings.ToUpper(word)
	switch upper {
	case "SELECT", "FROM", "JOIN", "UNION", "GROUP", "ORDER", "HAVING", "LIMIT", "OFFSET",
		"DISTINCT", "EXISTS", "CASE", "CAST", "WITH", "FETCH", "OVER", "WINDOW":
		return fmt.Errorf("GeoServer WFS does not support SQL %s in a filter", upper)
	}
	j := after
	for j < len(expr) {
		r, size := utf8.DecodeRuneInString(expr[j:])
		if !unicode.IsSpace(r) {
			if r == '(' && !allowedFilterCall(upper) {
				return fmt.Errorf("GeoServer WFS does not support SQL function %s", word)
			}
			break
		}
		j += size
	}
	return nil
}

func allowedFilterCall(upper string) bool {
	switch upper {
	case "IN", "NOT":
		return true
	default:
		return false
	}
}

func snippet(s string, i int) string {
	if i < 0 {
		i = 0
	}
	end := i + 24
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}

func replaceOutsideQuotes(s, old, repl string) string {
	if old == "" || !strings.Contains(s, old) {
		return s
	}
	var b strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); {
		if s[i] == '\'' && !inDouble {
			b.WriteByte(s[i])
			if inSingle && i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			inSingle = !inSingle
			i++
			continue
		}
		if s[i] == '"' && !inSingle {
			b.WriteByte(s[i])
			if inDouble && i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			inDouble = !inDouble
			i++
			continue
		}
		if !inSingle && !inDouble && strings.HasPrefix(s[i:], old) {
			b.WriteString(repl)
			i += len(old)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
