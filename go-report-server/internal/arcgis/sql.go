package arcgis

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SQL translation matches AcuGIS Report Designer (internal/provider/arcgis).
// Report Server must not grow a second ArcGIS WHERE dialect.

var (
	reSelectFrom   = regexp.MustCompile(`(?is)^\s*SELECT\s+(.+?)\s+FROM\s+(.+?)(?:\s+WHERE\s+(.+?))?(?:\s+GROUP\s+BY\s+.+?)?(?:\s+ORDER\s+BY\s+(.+?))?\s*$`)
	reIdent        = regexp.MustCompile(`"([^"]+)"|([A-Za-z_][A-Za-z0-9_]*)`)
	reParamP       = regexp.MustCompile(`\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reParamColon   = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)
	reXin          = regexp.MustCompile(`(?i)\$X\{(IN|NOTIN)\s*,\s*([^,]+),\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	reEqParamRight = regexp.MustCompile(`(?i)((?:(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)\.){0,2}(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*))\s*=\s*\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reEqParamLeft  = regexp.MustCompile(`(?i)\$P\{([A-Za-z_][A-Za-z0-9_]*)\}\s*=\s*((?:(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)\.){0,2}(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*))`)
	reInParam      = regexp.MustCompile(`(?i)((?:(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*)\.){0,2}(?:"[A-Za-z_][\w]*"|` + "`[A-Za-z_][\\w]*`" + `|\[[A-Za-z_][\w]*\]|[A-Za-z_][\w]*))\s+IN\s*\(\s*\$P\{([A-Za-z_][A-Za-z0-9_]*)\}\s*\)`)
)

type parsedSQL struct {
	Fields  []string
	Schema  string
	Table   string
	Where   string
	OrderBy string
}

func parseGeneratedSQL(sql string) (*parsedSQL, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return nil, fmt.Errorf("empty SQL")
	}
	sql = strings.TrimSuffix(sql, ";")
	m := reSelectFrom.FindStringSubmatch(sql)
	if m == nil {
		return nil, fmt.Errorf("unsupported ArcGIS query SQL (expected single-table SELECT)")
	}
	selectList := strings.TrimSpace(m[1])
	fromClause := strings.TrimSpace(m[2])
	where := strings.TrimSpace(m[3])
	orderBy := strings.TrimSpace(m[4])

	if strings.Contains(strings.ToUpper(fromClause), " JOIN ") {
		return nil, fmt.Errorf("ArcGIS Feature Server queries do not support JOINs; use a single layer")
	}

	schema, table := splitTableRef(fromClause)
	if table == "" {
		return nil, fmt.Errorf("could not parse FROM table")
	}

	fields := []string{}
	if selectList != "*" {
		parts := splitSelectList(selectList)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if idx := strings.LastIndex(strings.ToUpper(p), " AS "); idx >= 0 {
				p = strings.TrimSpace(p[:idx])
			}
			name := columnNameFromExpr(p)
			if name != "" && name != "*" {
				fields = append(fields, name)
			}
		}
	}

	return &parsedSQL{
		Fields:  fields,
		Schema:  schema,
		Table:   table,
		Where:   where,
		OrderBy: orderBy,
	}, nil
}

func splitTableRef(from string) (schema, table string) {
	from = strings.TrimSpace(from)
	parts := strings.Fields(from)
	if len(parts) == 0 {
		return "", ""
	}
	ref := parts[0]
	idents := reIdent.FindAllStringSubmatch(ref, -1)
	names := make([]string, 0, len(idents))
	for _, id := range idents {
		if id[1] != "" {
			names = append(names, id[1])
		} else if id[2] != "" {
			names = append(names, id[2])
		}
	}
	switch len(names) {
	case 0:
		return "", ""
	case 1:
		return schemaLayers, names[0]
	default:
		return names[0], names[1]
	}
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
	if i := strings.Index(expr, "("); i >= 0 && strings.HasSuffix(expr, ")") {
		inner := expr[i+1 : len(expr)-1]
		return columnNameFromExpr(inner)
	}
	idents := reIdent.FindAllStringSubmatch(expr, -1)
	if len(idents) == 0 {
		return ""
	}
	last := idents[len(idents)-1]
	if last[1] != "" {
		return last[1]
	}
	return last[2]
}

func isMultiParamValue(v any) bool {
	switch v.(type) {
	case []string, []any:
		return true
	default:
		return false
	}
}

// promoteFromParamValues rewrites col = $P{name} into $X{IN, col, name}
// when the supplied value is a slice. A slice is never joined into one scalar.
func promoteFromParamValues(sql string, params map[string]any) string {
	if strings.TrimSpace(sql) == "" {
		return sql
	}
	isCollection := func(name string) bool {
		if params == nil {
			return false
		}
		v, ok := params[name]
		return ok && isMultiParamValue(v)
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

func rewriteWhereForArcGIS(where string, params map[string]any) string {
	where = strings.TrimSpace(where)
	if where == "" {
		return "1=1"
	}
	where = promoteFromParamValues(where, params)

	where = reXin.ReplaceAllStringFunc(where, func(m string) string {
		sub := reXin.FindStringSubmatch(m)
		if len(sub) < 4 {
			return "1=0"
		}
		op := strings.ToUpper(sub[1])
		col := strings.TrimSpace(sub[2])
		name := sub[3]
		vals := paramValues(params, name)
		if len(vals) == 0 {
			if op == "NOTIN" {
				return "1=1"
			}
			return "1=0"
		}
		quoted := make([]string, 0, len(vals))
		for _, v := range vals {
			quoted = append(quoted, quoteSQLLiteral(v))
		}
		inList := strings.Join(quoted, ", ")
		if op == "NOTIN" {
			return fmt.Sprintf("%s NOT IN (%s)", col, inList)
		}
		return fmt.Sprintf("%s IN (%s)", col, inList)
	})

	replaceParam := func(name string) string {
		if params == nil {
			return "NULL"
		}
		v, ok := params[name]
		if !ok || v == nil {
			return "NULL"
		}
		switch t := v.(type) {
		case []string:
			if len(t) == 0 {
				return "NULL"
			}
			return quoteSQLLiteral(t[0])
		case []any:
			if len(t) == 0 {
				return "NULL"
			}
			return quoteSQLLiteral(fmt.Sprintf("%v", t[0]))
		default:
			return quoteSQLLiteral(fmt.Sprintf("%v", t))
		}
	}

	where = reParamP.ReplaceAllStringFunc(where, func(m string) string {
		sub := reParamP.FindStringSubmatch(m)
		if len(sub) < 2 {
			return "NULL"
		}
		return replaceParam(sub[1])
	})
	where = reParamColon.ReplaceAllStringFunc(where, func(m string) string {
		sub := reParamColon.FindStringSubmatch(m)
		if len(sub) < 2 {
			return "NULL"
		}
		return replaceParam(sub[1])
	})

	where = rewriteILike(where)
	where = stripQualifiedColumns(where)
	return where
}

func rewriteILike(where string) string {
	re := regexp.MustCompile(`(?i)((?:"[^"]+"|\w+)(?:\.(?:"[^"]+"|\w+))?)\s+ILIKE\s+('[^']*'|"[^"]*"|NULL)`)
	for {
		loc := re.FindStringSubmatchIndex(where)
		if loc == nil {
			break
		}
		col := where[loc[2]:loc[3]]
		val := where[loc[4]:loc[5]]
		repl := fmt.Sprintf("UPPER(%s) LIKE UPPER(%s)", col, val)
		where = where[:loc[0]] + repl + where[loc[1]:]
	}
	return where
}

func stripQualifiedColumns(where string) string {
	re := regexp.MustCompile(`(?:"[^"]+"|\w+)\.("([^"]+)"|(\w+))`)
	return re.ReplaceAllStringFunc(where, func(m string) string {
		sub := re.FindStringSubmatch(m)
		if len(sub) < 4 {
			return m
		}
		if sub[2] != "" {
			return `"` + sub[2] + `"`
		}
		return sub[3]
	})
}

func rewriteOrderBy(orderBy string) string {
	orderBy = strings.TrimSpace(orderBy)
	if orderBy == "" {
		return ""
	}
	parts := strings.Split(orderBy, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		desc := false
		upper := strings.ToUpper(p)
		if strings.HasSuffix(upper, " DESC") {
			desc = true
			p = strings.TrimSpace(p[:len(p)-5])
		} else if strings.HasSuffix(upper, " ASC") {
			p = strings.TrimSpace(p[:len(p)-4])
		}
		name := columnNameFromExpr(p)
		if name == "" {
			continue
		}
		if desc {
			out = append(out, name+" DESC")
		} else {
			out = append(out, name)
		}
	}
	return strings.Join(out, ", ")
}

func quoteSQLLiteral(v string) string {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "null") {
		return "NULL"
	}
	if v == "" {
		return "''"
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func paramValues(params map[string]any, name string) []string {
	if params == nil {
		return nil
	}
	v, ok := params[name]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		out := make([]string, len(t))
		copy(out, t)
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, fmt.Sprintf("%v", x))
		}
		return out
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	default:
		s := strings.TrimSpace(fmt.Sprintf("%v", t))
		if s == "" {
			return nil
		}
		return []string{s}
	}
}
