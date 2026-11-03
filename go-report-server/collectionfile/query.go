package collectionfile

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Query subset implemented for CSV v1:
//
//	SELECT col [, col]* | SELECT *
//	FROM one table reference (the name is not used to locate the file)
//	WHERE comparisons combined with AND / OR and parentheses
//	  field =  literal | field <> literal | field != literal
//	  field =  $P{name} | field <> $P{name}
//	  field <  <=  >  >=  literal or $P{name} when both sides are numeric,
//	    date/datetime, or string
//	  field IN (literals or $P) | field NOT IN (...)
//	  $X{IN, field, name} | $X{NOTIN, field, name}
//	  field IS NULL | field IS NOT NULL
//	ORDER BY field [ASC|DESC], ...
//
// A null compared with = or <> does not match (SQL unknown). A blank string
// matches only a blank string. $X{IN}, $X{NOTIN}, IN ($P{}), and NOT IN ($P{})
// take a list: []string, []any, a JSON array string, or a comma-separated
// string. One value stays a one-element list; the SQL is not rewritten to
// equality. An empty or null list makes $X{IN} / IN ($P{}) match every row
// and $X{NOTIN} / NOT IN ($P{}) match none. A slice used with = uses the
// first value and is not comma-split.
//
// JOIN, UNION, GROUP BY, HAVING, DISTINCT, LIMIT, OFFSET, LIKE, ILIKE,
// BETWEEN, aggregates, and other SQL functions are rejected.

// Query runs the v1 subset and returns the projected rows.
// limit <= 0 means no row cap. The cap is applied after filter and sort.
func Query(table *Table, sql string, params map[string]any, limit int) (*Table, error) {
	if table == nil {
		return nil, fmt.Errorf("CSV file has no header row")
	}
	params = NormalizeParams(sql, params)
	stmt, err := parseQuery(sql)
	if err != nil {
		return nil, err
	}
	kinds := map[string]Kind{}
	for _, c := range table.Columns {
		kinds[c.Name] = c.Kind
	}
	outCols := make([]Column, 0, len(stmt.fields))
	if stmt.star {
		outCols = append(outCols, table.Columns...)
	} else {
		for _, f := range stmt.fields {
			kind, ok := kinds[f.source]
			if !ok {
				return nil, fmt.Errorf("CSV query references unknown field %q", f.source)
			}
			outCols = append(outCols, Column{Name: f.name, Kind: kind})
		}
	}
	aliasSource := map[string]string{}
	for _, f := range stmt.fields {
		aliasSource[f.name] = f.source
	}
	for i, o := range stmt.order {
		if src, ok := aliasSource[o.name]; ok {
			stmt.order[i].name = src
		}
		if _, ok := kinds[stmt.order[i].name]; !ok {
			return nil, fmt.Errorf("CSV query references unknown field %q", o.name)
		}
	}
	filtered := make([]map[string]any, 0, len(table.Rows))
	for _, row := range table.Rows {
		ok, err := evalPred(stmt.where, row, kinds, params)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, row)
		}
	}
	if len(stmt.order) > 0 {
		sort.SliceStable(filtered, func(i, j int) bool {
			return rowLess(filtered[i], filtered[j], stmt.order, table.Columns)
		})
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	rows := make([]map[string]any, len(filtered))
	for i, row := range filtered {
		rows[i] = projectRow(row, stmt)
	}
	return &Table{Columns: outCols, Rows: rows, Delimiter: table.Delimiter}, nil
}

type selectField struct {
	source string
	name   string
}

type orderField struct {
	name string
	desc bool
}

type statement struct {
	star   bool
	fields []selectField
	where  pred
	order  []orderField
}

type pred interface{ isPred() }

type predCmp struct {
	col string
	op  string
	val valueExpr
}

type predIn struct {
	col  string
	not  bool
	vals []valueExpr
}

type predNull struct {
	col string
	not bool
}

type predAnd struct{ left, right pred }
type predOr struct{ left, right pred }

func (predCmp) isPred()  {}
func (predIn) isPred()   {}
func (predNull) isPred() {}
func (predAnd) isPred()  {}
func (predOr) isPred()   {}

type valueExpr struct {
	param string
	lit   any
	isLit bool
}

func parseQuery(sql string) (*statement, error) {
	sql = strings.TrimSpace(sql)
	sql = strings.TrimSuffix(sql, ";")
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return nil, fmt.Errorf("CSV query is empty")
	}
	if err := rejectUnsupported(sql); err != nil {
		return nil, err
	}
	p := &parser{s: sql}
	p.skip()
	if !p.consumeKeyword("SELECT") {
		return nil, fmt.Errorf("CSV query must be a single SELECT")
	}
	stmt := &statement{}
	p.skip()
	if p.peek() == '*' {
		p.i++
		stmt.star = true
	} else {
		for {
			f, err := p.parseSelectItem()
			if err != nil {
				return nil, err
			}
			stmt.fields = append(stmt.fields, f)
			p.skip()
			if p.peek() != ',' {
				break
			}
			p.i++
		}
		if len(stmt.fields) == 0 {
			return nil, fmt.Errorf("CSV query has no fields")
		}
	}
	p.skip()
	if !p.consumeKeyword("FROM") {
		return nil, fmt.Errorf("CSV query is missing FROM")
	}
	if _, err := p.parseTableRef(); err != nil {
		return nil, err
	}
	p.skip()
	if p.consumeKeyword("WHERE") {
		w, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		stmt.where = w
		p.skip()
	}
	if p.consumeKeyword("ORDER") {
		p.skip()
		if !p.consumeKeyword("BY") {
			return nil, fmt.Errorf("CSV query has ORDER without BY")
		}
		for {
			name, err := p.parseColumn()
			if err != nil {
				return nil, err
			}
			p.skip()
			desc := false
			if p.consumeKeyword("DESC") {
				desc = true
			} else {
				p.consumeKeyword("ASC")
			}
			stmt.order = append(stmt.order, orderField{name: name, desc: desc})
			p.skip()
			if p.peek() != ',' {
				break
			}
			p.i++
		}
	}
	p.skip()
	if p.i < len(p.s) {
		return nil, fmt.Errorf("CSV query has unsupported syntax near %q", p.rest())
	}
	return stmt, nil
}

func rejectUnsupported(sql string) error {
	checks := []struct{ word, msg string }{
		{"JOIN", "CSV queries do not support JOIN"},
		{"UNION", "CSV queries do not support UNION"},
		{"EXCEPT", "CSV queries do not support EXCEPT"},
		{"INTERSECT", "CSV queries do not support INTERSECT"},
		{"GROUP", "CSV queries do not support GROUP BY"},
		{"HAVING", "CSV queries do not support HAVING"},
		{"LIMIT", "CSV queries do not support LIMIT"},
		{"OFFSET", "CSV queries do not support OFFSET"},
		{"FETCH", "CSV queries do not support FETCH"},
		{"DISTINCT", "CSV queries do not support DISTINCT"},
		{"LIKE", "CSV queries do not support LIKE"},
		{"ILIKE", "CSV queries do not support ILIKE"},
		{"BETWEEN", "CSV queries do not support BETWEEN"},
	}
	for _, c := range checks {
		if containsKeyword(sql, c.word) {
			return fmt.Errorf("%s", c.msg)
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

type parser struct {
	s string
	i int
}

func (p *parser) peek() byte {
	if p.i >= len(p.s) {
		return 0
	}
	return p.s[p.i]
}

func (p *parser) rest() string {
	s := strings.TrimSpace(p.s[p.i:])
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func (p *parser) skip() {
	for p.i < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.i:])
		if !unicode.IsSpace(r) {
			return
		}
		p.i += size
	}
}

func (p *parser) consumeKeyword(word string) bool {
	p.skip()
	if p.i+len(word) > len(p.s) || !strings.EqualFold(p.s[p.i:p.i+len(word)], word) {
		return false
	}
	after := p.i + len(word)
	if after < len(p.s) && isIdentByte(p.s[after]) {
		return false
	}
	p.i = after
	return true
}

func (p *parser) parseSelectItem() (selectField, error) {
	name, err := p.parseColumn()
	if err != nil {
		return selectField{}, err
	}
	p.skip()
	if p.peek() == '(' {
		return selectField{}, fmt.Errorf("CSV queries do not support SQL functions")
	}
	alias := name
	if p.consumeKeyword("AS") {
		a, err := p.parseIdent()
		if err != nil {
			return selectField{}, fmt.Errorf("CSV query is missing an alias")
		}
		alias = a
	}
	return selectField{source: name, name: alias}, nil
}

func (p *parser) parseTableRef() (string, error) {
	names := []string{}
	name, err := p.parseIdent()
	if err != nil {
		return "", fmt.Errorf("CSV query is missing a table name")
	}
	names = append(names, name)
	for {
		p.skip()
		if p.peek() != '.' {
			break
		}
		p.i++
		next, err := p.parseIdent()
		if err != nil {
			return "", fmt.Errorf("CSV query has a broken table name")
		}
		names = append(names, next)
	}
	p.skip()
	// Optional alias. Stop when the next token is a clause keyword.
	if p.i < len(p.s) && (p.peek() == '"' || isIdentStart(rune(p.peek()))) {
		save := p.i
		tok, err := p.parseIdent()
		if err == nil && !isClause(tok) {
			if strings.EqualFold(tok, "AS") {
				if _, err := p.parseIdent(); err != nil {
					return "", fmt.Errorf("CSV query is missing a table alias")
				}
			}
		} else {
			p.i = save
		}
	}
	return names[len(names)-1], nil
}

func isClause(tok string) bool {
	switch strings.ToUpper(tok) {
	case "WHERE", "ORDER", "GROUP", "HAVING", "LIMIT", "OFFSET", "UNION", "JOIN":
		return true
	default:
		return false
	}
}

func (p *parser) parseColumn() (string, error) {
	name, err := p.parseIdent()
	if err != nil {
		return "", fmt.Errorf("CSV query is missing a field name")
	}
	for {
		p.skip()
		if p.peek() != '.' {
			break
		}
		p.i++
		next, err := p.parseIdent()
		if err != nil {
			return "", fmt.Errorf("CSV query has a broken field name")
		}
		name = next
	}
	return name, nil
}

func (p *parser) parseIdent() (string, error) {
	p.skip()
	if p.i >= len(p.s) {
		return "", fmt.Errorf("CSV query ended unexpectedly")
	}
	if p.s[p.i] == '"' {
		p.i++
		var b strings.Builder
		for p.i < len(p.s) {
			if p.s[p.i] == '"' {
				if p.i+1 < len(p.s) && p.s[p.i+1] == '"' {
					b.WriteByte('"')
					p.i += 2
					continue
				}
				p.i++
				return b.String(), nil
			}
			b.WriteByte(p.s[p.i])
			p.i++
		}
		return "", fmt.Errorf("CSV query has an unterminated identifier")
	}
	r, size := utf8.DecodeRuneInString(p.s[p.i:])
	if !isIdentStart(r) {
		return "", fmt.Errorf("CSV query has unsupported syntax near %q", p.rest())
	}
	start := p.i
	p.i += size
	for p.i < len(p.s) {
		r, size = utf8.DecodeRuneInString(p.s[p.i:])
		if !isIdentCont(r) {
			break
		}
		p.i += size
	}
	return p.s[start:p.i], nil
}

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isIdentCont(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

func (p *parser) parseOr() (pred, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.consumeKeyword("OR") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = predOr{left, right}
	}
	return left, nil
}

func (p *parser) parseAnd() (pred, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.consumeKeyword("AND") {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = predAnd{left, right}
	}
	return left, nil
}

func (p *parser) parsePrimary() (pred, error) {
	p.skip()
	if p.peek() == '(' {
		p.i++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skip()
		if p.peek() != ')' {
			return nil, fmt.Errorf("CSV query is missing a closing parenthesis")
		}
		p.i++
		return inner, nil
	}
	if strings.HasPrefix(p.s[p.i:], "$X{") || strings.HasPrefix(strings.ToUpper(p.s[p.i:]), "$X{") {
		return p.parseXin()
	}
	col, err := p.parseColumn()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.peek() == '(' {
		return nil, fmt.Errorf("CSV queries do not support SQL functions")
	}
	if p.consumeKeyword("IS") {
		not := p.consumeKeyword("NOT")
		if !p.consumeKeyword("NULL") {
			return nil, fmt.Errorf("CSV query has IS without NULL")
		}
		return predNull{col: col, not: not}, nil
	}
	not := false
	if p.consumeKeyword("NOT") {
		not = true
	}
	if p.consumeKeyword("IN") {
		vals, err := p.parseValueList()
		if err != nil {
			return nil, err
		}
		return predIn{col: col, not: not, vals: vals}, nil
	}
	if not {
		return nil, fmt.Errorf("CSV queries do not support NOT except NOT IN and IS NOT NULL")
	}
	op, err := p.parseCmpOp()
	if err != nil {
		return nil, err
	}
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return predCmp{col: col, op: op, val: val}, nil
}

func (p *parser) parseCmpOp() (string, error) {
	p.skip()
	ops := []string{"<>", "!=", "<=", ">=", "=", "<", ">"}
	for _, op := range ops {
		if strings.HasPrefix(p.s[p.i:], op) {
			p.i += len(op)
			if op == "!=" {
				return "<>", nil
			}
			return op, nil
		}
	}
	return "", fmt.Errorf("CSV query has an unsupported comparison near %q", p.rest())
}

func (p *parser) parseXin() (pred, error) {
	p.skip()
	if len(p.s)-p.i < 3 || !strings.EqualFold(p.s[p.i:p.i+3], "$X{") {
		return nil, fmt.Errorf("CSV query has a broken $X clause")
	}
	p.i += 3
	p.skip()
	not := false
	if p.consumeKeyword("NOTIN") {
		not = true
	} else if !p.consumeKeyword("IN") {
		return nil, fmt.Errorf("CSV queries only support $X{IN} and $X{NOTIN}")
	}
	p.skip()
	if p.peek() != ',' {
		return nil, fmt.Errorf("CSV query has a broken $X clause")
	}
	p.i++
	col, err := p.parseColumn()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.peek() != ',' {
		return nil, fmt.Errorf("CSV query has a broken $X clause")
	}
	p.i++
	name, err := p.parseIdent()
	if err != nil {
		return nil, fmt.Errorf("CSV query $X clause is missing a parameter name")
	}
	p.skip()
	if p.peek() != '}' {
		return nil, fmt.Errorf("CSV query has a broken $X clause")
	}
	p.i++
	return predIn{col: col, not: not, vals: []valueExpr{{param: name}}}, nil
}

func (p *parser) parseValueList() ([]valueExpr, error) {
	p.skip()
	if p.peek() != '(' {
		return nil, fmt.Errorf("CSV query IN clause is missing '('")
	}
	p.i++
	var vals []valueExpr
	for {
		p.skip()
		if p.peek() == ')' {
			p.i++
			break
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
		p.skip()
		if p.peek() == ',' {
			p.i++
			continue
		}
		if p.peek() == ')' {
			p.i++
			break
		}
		return nil, fmt.Errorf("CSV query IN clause has unsupported syntax near %q", p.rest())
	}
	return vals, nil
}

func (p *parser) parseValue() (valueExpr, error) {
	p.skip()
	if strings.HasPrefix(p.s[p.i:], "$P{") {
		p.i += 3
		name, err := p.parseIdent()
		if err != nil {
			return valueExpr{}, fmt.Errorf("CSV query is missing a parameter name")
		}
		p.skip()
		if p.peek() != '}' {
			return valueExpr{}, fmt.Errorf("CSV query has a broken $P clause")
		}
		p.i++
		return valueExpr{param: name}, nil
	}
	if p.peek() == '\'' {
		s, err := p.parseString()
		if err != nil {
			return valueExpr{}, err
		}
		return valueExpr{isLit: true, lit: s}, nil
	}
	if t, ok := p.tryDateLiteral(); ok {
		return valueExpr{isLit: true, lit: t}, nil
	}
	if p.consumeKeyword("NULL") {
		return valueExpr{isLit: true, lit: nil}, nil
	}
	if p.consumeKeyword("TRUE") {
		return valueExpr{isLit: true, lit: true}, nil
	}
	if p.consumeKeyword("FALSE") {
		return valueExpr{isLit: true, lit: false}, nil
	}
	start := p.i
	if p.peek() == '-' || p.peek() == '+' || (p.peek() >= '0' && p.peek() <= '9') {
		p.i++
		for p.i < len(p.s) {
			c := p.s[p.i]
			if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
				p.i++
				continue
			}
			break
		}
		tok := p.s[start:p.i]
		if isIntegerToken(tok) {
			n, err := strconv.ParseInt(tok, 10, 64)
			if err != nil {
				return valueExpr{}, fmt.Errorf("CSV query has an invalid number %q", tok)
			}
			return valueExpr{isLit: true, lit: n}, nil
		}
		if isDecimalToken(tok) || isIntegerToken(strings.TrimPrefix(tok, "+")) {
			f, err := strconv.ParseFloat(tok, 64)
			if err != nil {
				return valueExpr{}, fmt.Errorf("CSV query has an invalid number %q", tok)
			}
			return valueExpr{isLit: true, lit: f}, nil
		}
		return valueExpr{}, fmt.Errorf("CSV query has an invalid number %q", tok)
	}
	if t, ok := p.tryDateLiteral(); ok {
		return valueExpr{isLit: true, lit: t}, nil
	}
	return valueExpr{}, fmt.Errorf("CSV query has an unsupported value near %q", p.rest())
}

func (p *parser) tryDateLiteral() (any, bool) {
	rest := p.s[p.i:]
	if len(rest) < 10 || rest[4] != '-' || rest[7] != '-' {
		return nil, false
	}
	// Unquoted ISO dates are accepted as literals.
	end := 10
	if len(rest) > 10 && (rest[10] == 'T' || rest[10] == ' ') {
		for end < len(rest) && !unicode.IsSpace(rune(rest[end])) && rest[end] != ')' && rest[end] != ',' {
			end++
		}
	}
	tok := rest[:end]
	if k, v, ok := typedToken(tok); ok && (k == KindDate || k == KindDateTime) {
		p.i += end
		return v, true
	}
	return nil, false
}

func (p *parser) parseString() (string, error) {
	if p.peek() != '\'' {
		return "", fmt.Errorf("CSV query expected a string")
	}
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		if p.s[p.i] == '\'' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '\'' {
				b.WriteByte('\'')
				p.i += 2
				continue
			}
			p.i++
			return b.String(), nil
		}
		b.WriteByte(p.s[p.i])
		p.i++
	}
	return "", fmt.Errorf("CSV query has an unterminated string")
}

func projectRow(row map[string]any, stmt *statement) map[string]any {
	if stmt.star {
		out := make(map[string]any, len(row))
		for k, v := range row {
			out[k] = v
		}
		return out
	}
	out := make(map[string]any, len(stmt.fields))
	for _, f := range stmt.fields {
		out[f.name] = row[f.source]
	}
	return out
}

func evalPred(pr pred, row map[string]any, kinds map[string]Kind, params map[string]any) (bool, error) {
	if pr == nil {
		return true, nil
	}
	switch t := pr.(type) {
	case predAnd:
		ok, err := evalPred(t.left, row, kinds, params)
		if err != nil || !ok {
			return ok, err
		}
		return evalPred(t.right, row, kinds, params)
	case predOr:
		ok, err := evalPred(t.left, row, kinds, params)
		if err != nil || ok {
			return ok, err
		}
		return evalPred(t.right, row, kinds, params)
	case predNull:
		_, ok := kinds[t.col]
		if !ok {
			return false, fmt.Errorf("CSV query references unknown field %q", t.col)
		}
		isNull := row[t.col] == nil
		if t.not {
			return !isNull, nil
		}
		return isNull, nil
	case predCmp:
		kind, ok := kinds[t.col]
		if !ok {
			return false, fmt.Errorf("CSV query references unknown field %q", t.col)
		}
		right, err := resolveValue(t.val, params, false)
		if err != nil {
			return false, err
		}
		return compare(kind, row[t.col], right, t.op)
	case predIn:
		kind, ok := kinds[t.col]
		if !ok {
			return false, fmt.Errorf("CSV query references unknown field %q", t.col)
		}
		items, paramOnly, err := inItems(t.vals, params)
		if err != nil {
			return false, err
		}
		if paramOnly && len(items) == 0 {
			// Optional Jasper collection: empty $X{IN} / IN ($P{}) is 0 = 0
			// (every row, including nulls). Empty $X{NOTIN} / NOT IN ($P{})
			// is 1 = 0 (no rows). A literal empty IN () is not parameter-backed
			// and keeps SQL's match-nothing behavior below.
			return !t.not, nil
		}
		if row[t.col] == nil {
			return false, nil
		}
		found := false
		for _, item := range items {
			ok, err := compare(kind, row[t.col], item, "=")
			if err != nil {
				return false, err
			}
			if ok {
				found = true
				break
			}
		}
		if t.not {
			return !found, nil
		}
		return found, nil
	default:
		return false, fmt.Errorf("CSV query has an unsupported filter")
	}
}

func resolveValue(v valueExpr, params map[string]any, multi bool) (any, error) {
	if v.param == "" {
		return v.lit, nil
	}
	var raw any
	ok := false
	if params != nil {
		raw, ok = params[v.param]
	}
	if multi {
		if !ok {
			raw = nil
		}
		list := coerceMultiStrings(raw)
		out := make([]any, len(list))
		for i := range list {
			out[i] = list[i]
		}
		return out, nil
	}
	if !ok || raw == nil {
		return nil, nil
	}
	switch list := raw.(type) {
	case []string:
		if len(list) == 0 {
			return nil, nil
		}
		return list[0], nil
	case []any:
		if len(list) == 0 {
			return nil, nil
		}
		return list[0], nil
	default:
		return raw, nil
	}
}

func inItems(vals []valueExpr, params map[string]any) ([]any, bool, error) {
	paramOnly := len(vals) > 0
	var items []any
	for _, v := range vals {
		if v.param == "" {
			paramOnly = false
			if v.lit != nil {
				items = append(items, v.lit)
			}
			continue
		}
		got, err := resolveValue(v, params, true)
		if err != nil {
			return nil, false, err
		}
		switch list := got.(type) {
		case []any:
			items = append(items, list...)
		default:
			if got != nil {
				items = append(items, got)
			}
		}
	}
	return items, paramOnly, nil
}

var (
	reXinParam = regexp.MustCompile(`(?i)\$X\{\s*(?:IN|NOTIN)\s*,\s*[^,]+,\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	reInPParam = regexp.MustCompile(`(?i)\b(?:NOT\s+)?IN\s*\(\s*\$P\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\s*\)`)
)

// NormalizeParams copies params and turns values referenced by $X{IN},
// $X{NOTIN}, IN ($P{}), and NOT IN ($P{}) into []string. A single selection
// is a one-element list. Scalar $P comparisons are left unchanged. The SQL
// text is not modified.
func NormalizeParams(sql string, params map[string]any) map[string]any {
	names := multiParamNames(sql)
	if len(names) == 0 {
		return params
	}
	out := make(map[string]any, len(params)+len(names))
	for k, v := range params {
		out[k] = v
	}
	for name := range names {
		out[name] = coerceMultiStrings(out[name])
	}
	return out
}

func multiParamNames(sql string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, re := range []*regexp.Regexp{reXinParam, reInPParam} {
		for _, m := range re.FindAllStringSubmatch(sql, -1) {
			if len(m) > 1 && m[1] != "" {
				names[m[1]] = struct{}{}
			}
		}
	}
	return names
}

// coerceMultiStrings expands a collection parameter. nil, "", and "[]" are an
// empty list. A JSON array string or a comma-separated string becomes one
// element per item. An existing slice is trimmed, not joined and split again,
// so a value that itself contains a comma stays one element.
func coerceMultiStrings(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return []string{}
	case []string:
		return trimMulti(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if item == nil {
				continue
			}
			parts = append(parts, fmt.Sprint(item))
		}
		return trimMulti(parts)
	case string:
		return splitMultiString(v)
	default:
		return splitMultiString(fmt.Sprint(v))
	}
}

func splitMultiString(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return []string{}
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		var strs []string
		if err := json.Unmarshal([]byte(s), &strs); err == nil {
			return trimMulti(strs)
		}
		var anys []any
		if err := json.Unmarshal([]byte(s), &anys); err == nil {
			return coerceMultiStrings(anys)
		}
	}
	if strings.Contains(s, ",") {
		return trimMulti(strings.Split(s, ","))
	}
	return []string{s}
}

func trimMulti(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func compare(kind Kind, left, right any, op string) (bool, error) {
	if left == nil || right == nil {
		return false, nil
	}
	lv, err := coerce(kind, left)
	if err != nil {
		return false, err
	}
	rv, err := coerce(kind, right)
	if err != nil {
		return false, err
	}
	if lv == nil || rv == nil {
		return false, nil
	}
	if op != "=" && op != "<>" && kind == KindBoolean {
		return false, fmt.Errorf("CSV queries cannot compare boolean field with %s", op)
	}
	cmp, err := cmpValues(kind, lv, rv)
	if err != nil {
		return false, err
	}
	switch op {
	case "=":
		return cmp == 0, nil
	case "<>":
		return cmp != 0, nil
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("CSV query has unsupported operator %s", op)
	}
}

func cmpValues(kind Kind, left, right any) (int, error) {
	switch kind {
	case KindInteger:
		l, r := left.(int64), right.(int64)
		switch {
		case l < r:
			return -1, nil
		case l > r:
			return 1, nil
		default:
			return 0, nil
		}
	case KindDecimal:
		l, r := left.(float64), right.(float64)
		switch {
		case l < r:
			return -1, nil
		case l > r:
			return 1, nil
		default:
			return 0, nil
		}
	case KindBoolean:
		l, r := left.(bool), right.(bool)
		if l == r {
			return 0, nil
		}
		if !l && r {
			return -1, nil
		}
		return 1, nil
	case KindDate, KindDateTime:
		l, r := asTime(left), asTime(right)
		switch {
		case l.Before(r):
			return -1, nil
		case l.After(r):
			return 1, nil
		default:
			return 0, nil
		}
	default:
		l, r := fmt.Sprint(left), fmt.Sprint(right)
		return strings.Compare(l, r), nil
	}
}

func asTime(v any) time.Time {
	switch t := v.(type) {
	case Date:
		return t.Time
	case DateTime:
		return t.Time
	case time.Time:
		return t
	default:
		return time.Time{}
	}
}

func coerce(kind Kind, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch kind {
	case KindString:
		switch t := v.(type) {
		case string:
			return t, nil
		case Date:
			return t.Time.UTC().Format("2006-01-02"), nil
		case DateTime:
			return t.Time.UTC().Format(time.RFC3339), nil
		case time.Time:
			return t.UTC().Format(time.RFC3339), nil
		case bool:
			if t {
				return "true", nil
			}
			return "false", nil
		case int64:
			return strconv.FormatInt(t, 10), nil
		case int:
			return strconv.Itoa(t), nil
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64), nil
		default:
			return fmt.Sprint(t), nil
		}
	case KindInteger:
		switch t := v.(type) {
		case int64:
			return t, nil
		case int:
			return int64(t), nil
		case float64:
			if t != float64(int64(t)) {
				return nil, fmt.Errorf("CSV query cannot compare an integer field with decimal %v", t)
			}
			return int64(t), nil
		case string:
			if t == "" {
				return nil, nil
			}
			if !isIntegerToken(t) {
				return nil, fmt.Errorf("CSV query cannot compare an integer field with %q", t)
			}
			n, err := strconv.ParseInt(t, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("CSV query cannot compare an integer field with %q", t)
			}
			return n, nil
		default:
			return nil, fmt.Errorf("CSV query cannot compare an integer field with %T", v)
		}
	case KindDecimal:
		switch t := v.(type) {
		case float64:
			return t, nil
		case int64:
			return float64(t), nil
		case int:
			return float64(t), nil
		case string:
			if t == "" {
				return nil, nil
			}
			if !isDecimalToken(t) && !isIntegerToken(t) {
				return nil, fmt.Errorf("CSV query cannot compare a decimal field with %q", t)
			}
			f, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return nil, fmt.Errorf("CSV query cannot compare a decimal field with %q", t)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("CSV query cannot compare a decimal field with %T", v)
		}
	case KindBoolean:
		switch t := v.(type) {
		case bool:
			return t, nil
		case string:
			switch strings.ToLower(t) {
			case "":
				return nil, nil
			case "true":
				return true, nil
			case "false":
				return false, nil
			default:
				return nil, fmt.Errorf("CSV query cannot compare a boolean field with %q", t)
			}
		default:
			return nil, fmt.Errorf("CSV query cannot compare a boolean field with %T", v)
		}
	case KindDate:
		tm, err := coerceTime(v, true)
		if err != nil {
			return nil, err
		}
		if tm == nil {
			return nil, nil
		}
		return Date{Time: *tm}, nil
	case KindDateTime:
		tm, err := coerceTime(v, false)
		if err != nil {
			return nil, err
		}
		if tm == nil {
			return nil, nil
		}
		return DateTime{Time: *tm}, nil
	default:
		return v, nil
	}
}

func coerceTime(v any, dateOnly bool) (*time.Time, error) {
	switch t := v.(type) {
	case Date:
		u := t.Time.UTC()
		return &u, nil
	case DateTime:
		u := t.Time.UTC()
		return &u, nil
	case time.Time:
		u := t.UTC()
		return &u, nil
	case string:
		if t == "" {
			return nil, nil
		}
		parsed, ok := parseDateTime(t)
		if !ok {
			what := "datetime"
			if dateOnly {
				what = "date"
			}
			return nil, fmt.Errorf("CSV query cannot compare a %s field with %q", what, t)
		}
		if dateOnly && !isDateOnly(t) && len(t) > 10 {
			// A datetime parameter compared to a date column uses the calendar date.
			parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
		}
		return &parsed, nil
	default:
		what := "datetime"
		if dateOnly {
			what = "date"
		}
		return nil, fmt.Errorf("CSV query cannot compare a %s field with %T", what, v)
	}
}

func rowLess(a, b map[string]any, order []orderField, cols []Column) bool {
	kinds := map[string]Kind{}
	for _, c := range cols {
		kinds[c.Name] = c.Kind
	}
	for _, o := range order {
		av, bv := a[o.name], b[o.name]
		if av == nil && bv == nil {
			continue
		}
		if av == nil {
			return false
		}
		if bv == nil {
			return true
		}
		kind := kinds[o.name]
		if kind == "" {
			kind = KindString
		}
		cmp, err := cmpValues(kind, av, bv)
		if err != nil || cmp == 0 {
			continue
		}
		if o.desc {
			return cmp > 0
		}
		return cmp < 0
	}
	return false
}
