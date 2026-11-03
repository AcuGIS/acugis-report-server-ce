package spatial

import (
	"regexp"
	"strings"
)

var sqlKeywords = map[string]bool{
	"WHERE": true, "JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true, "FULL": true,
	"CROSS": true, "ON": true, "USING": true, "GROUP": true, "ORDER": true, "LIMIT": true,
	"OFFSET": true, "HAVING": true, "UNION": true, "EXCEPT": true, "INTERSECT": true,
	"AND": true, "OR": true, "SET": true, "FETCH": true, "FOR": true, "WINDOW": true, "AS": true,
}

// sqlIdentRE matches one SQL identifier:
//   "double-quoted" | `backtick` | [bracket] | bare
// Capture groups per match: 1=dq, 2=bt, 3=br, 4=bare (4 groups total).
const sqlIdentGroups = 4

var sqlIdentRE = `(?:"((?:[^"]|"")*)"|` + "`((?:[^`]|``)*)`" + `|\[([^\]]+)\]|([A-Za-z_][\w]*))`

// TableRef is a schema/table/alias triple from SQL.
type TableRef struct {
	Schema string
	Table  string
	Alias  string
}

// SQLPrimaryTable extracts the primary FROM table (PHP jrs_sql_primary_table).
func SQLPrimaryTable(sql string) TableRef {
	out := TableRef{}
	if strings.TrimSpace(sql) == "" {
		return out
	}
	normalized := NeutralizeJasperSQL(sql)
	re := regexp.MustCompile(`(?i)\bFROM\s+` + sqlIdentRE + `(?:\s*\.\s*` + sqlIdentRE + `)?` + `(?:\s+(?:AS\s+)?` + sqlIdentRE + `)?`)
	m := re.FindStringSubmatch(normalized)
	if rel := relationFromMatch(m); rel != nil {
		return *rel
	}
	return out
}

// SQLTableAliases parses FROM/JOIN aliases (PHP jrs_sql_table_aliases).
func SQLTableAliases(sql string) map[string]TableRef {
	normalized := NeutralizeJasperSQL(sql)
	aliases := map[string]TableRef{}
	fromRE := regexp.MustCompile(`(?i)\bFROM\s+` + sqlIdentRE + `(?:\s*\.\s*` + sqlIdentRE + `)?` + `(?:\s+(?:AS\s+)?` + sqlIdentRE + `)?`)
	if m := fromRE.FindStringSubmatch(normalized); m != nil {
		if rel := relationFromMatch(m); rel != nil {
			alias := rel.Alias
			if alias == "" {
				alias = rel.Table
			}
			aliases[alias] = TableRef{Schema: rel.Schema, Table: rel.Table}
			aliases[rel.Table] = TableRef{Schema: rel.Schema, Table: rel.Table}
		}
	}
	joinRE := regexp.MustCompile(`(?i)\bJOIN\s+` + sqlIdentRE + `(?:\s*\.\s*` + sqlIdentRE + `)?` + `(?:\s+(?:AS\s+)?` + sqlIdentRE + `)?`)
	for _, m := range joinRE.FindAllStringSubmatch(normalized, -1) {
		rel := relationFromMatch(m)
		if rel == nil {
			continue
		}
		alias := rel.Alias
		if alias == "" {
			alias = rel.Table
		}
		aliases[alias] = TableRef{Schema: rel.Schema, Table: rel.Table}
		aliases[rel.Table] = TableRef{Schema: rel.Schema, Table: rel.Table}
	}
	return aliases
}

// SQLRelationList returns unique schema/table pairs from FROM/JOIN (primary first).
func SQLRelationList(sql string) []TableRef {
	aliases := SQLTableAliases(sql)
	seen := map[string]bool{}
	var out []TableRef
	// Prefer primary FROM order: walk aliases map is random; re-parse for order.
	normalized := NeutralizeJasperSQL(sql)
	fromRE := regexp.MustCompile(`(?i)\bFROM\s+` + sqlIdentRE + `(?:\s*\.\s*` + sqlIdentRE + `)?` + `(?:\s+(?:AS\s+)?` + sqlIdentRE + `)?`)
	joinRE := regexp.MustCompile(`(?i)\bJOIN\s+` + sqlIdentRE + `(?:\s*\.\s*` + sqlIdentRE + `)?` + `(?:\s+(?:AS\s+)?` + sqlIdentRE + `)?`)
	appendRel := func(rel *TableRef) {
		if rel == nil || rel.Table == "" {
			return
		}
		key := strings.ToLower(rel.Schema) + "." + strings.ToLower(rel.Table)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, TableRef{Schema: rel.Schema, Table: rel.Table, Alias: rel.Alias})
	}
	if m := fromRE.FindStringSubmatch(normalized); m != nil {
		appendRel(relationFromMatch(m))
	}
	for _, m := range joinRE.FindAllStringSubmatch(normalized, -1) {
		appendRel(relationFromMatch(m))
	}
	_ = aliases
	return out
}

// SQLGeometryRef builds a geometry column reference using PostgreSQL quoting
// (PHP jrs_sql_geometry_ref). Prefer SQLGeometryRefDialect for other engines.
func SQLGeometryRef(sql, schema, table, geometryColumn string) string {
	return SQLGeometryRefDialect(sql, schema, table, geometryColumn, pgSpatial{})
}

// SQLGeometryRefDialect builds a geometry column reference with dialect quoting.
func SQLGeometryRefDialect(sql, schema, table, geometryColumn string, d SpatialDialect) string {
	if d == nil {
		d = pgSpatial{}
	}
	if !IsSafeSQLIdent(table) || !IsSafeSQLIdent(geometryColumn) {
		return ""
	}
	if schema != "" && !IsSafeSQLIdent(schema) {
		return ""
	}
	aliases := SQLTableAliases(sql)
	geom := d.QuoteIdent(geometryColumn)
	for alias, info := range aliases {
		if strings.EqualFold(info.Table, table) && !strings.EqualFold(alias, table) {
			if IsSafeSQLIdent(alias) {
				return d.QuoteIdent(alias) + "." + geom
			}
		}
	}
	if _, ok := aliases[table]; ok {
		return d.QuoteIdent(table) + "." + geom
	}
	if schema != "" {
		return d.QuoteIdent(schema) + "." + d.QuoteIdent(table) + "." + geom
	}
	return d.QuoteIdent(table) + "." + geom
}

func relationFromMatch(m []string) *TableRef {
	// Full match + 3 idents × 4 groups = 13 elements (indices 0..12).
	if len(m) < 1+sqlIdentGroups {
		return nil
	}
	first := identFromGroups(m, 1)
	if first == "" {
		return nil
	}
	second := ""
	if len(m) >= 1+2*sqlIdentGroups {
		second = identFromGroups(m, 1+sqlIdentGroups)
	}
	aliasRaw := ""
	if len(m) >= 1+3*sqlIdentGroups {
		aliasRaw = identFromGroups(m, 1+2*sqlIdentGroups)
	}
	var schema, table string
	if second != "" {
		schema = first
		table = second
	} else {
		table = first
	}
	alias := ""
	if aliasRaw != "" && !sqlKeywords[strings.ToUpper(aliasRaw)] {
		alias = aliasRaw
	}
	return &TableRef{Schema: schema, Table: table, Alias: alias}
}

// identFromGroups reads one identifier starting at quotedGroup (dq group index).
// Layout: dq, backtick, bracket, bare.
func identFromGroups(m []string, start int) string {
	if start+3 >= len(m) {
		return ""
	}
	if dq := m[start]; dq != "" {
		return strings.ReplaceAll(dq, `""`, `"`)
	}
	if bt := m[start+1]; bt != "" {
		return strings.ReplaceAll(bt, "``", "`")
	}
	if br := m[start+2]; br != "" {
		return br
	}
	return m[start+3]
}
