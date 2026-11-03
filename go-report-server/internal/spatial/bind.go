package spatial

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// BindResult is the output of BindJasperSQL.
type BindResult struct {
	SQL    string
	Params []any
	Debug  []map[string]any
}

var (
	reBindX  = regexp.MustCompile(`\$X\{([A-Za-z]+)\s*,\s*([^,]+?)\s*,\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)
	reBindPB = regexp.MustCompile(`\$P!\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reBindP  = regexp.MustCompile(`\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reRemain = regexp.MustCompile(`\$[PX]!?\{`)
)

// BindJasperSQL expands Jasper $P / $P! / $X placeholders for PostgreSQL (PHP jrs_bind_jasper_sql).
func BindJasperSQL(sql string, filterContext map[string]any, jrxmlParams map[string]ParamMeta) (*BindResult, error) {
	return BindJasperSQLDialect(sql, filterContext, jrxmlParams, pgSpatial{})
}

// BindJasperSQLDialect expands Jasper placeholders using the given spatial dialect's
// parameter style ($N, ?, @pN, or :N) and cast expressions.
func BindJasperSQLDialect(sql string, filterContext map[string]any, jrxmlParams map[string]ParamMeta, d SpatialDialect) (*BindResult, error) {
	if sql == "" {
		return nil, fmt.Errorf("Report SQL is empty.")
	}
	if d == nil {
		d = pgSpatial{}
	}
	// Multi-select parents referenced as "col = $P{name}" must become $X{IN}
	// before binding; $P{} can only carry a scalar.
	sql = PromoteCollectionEqualityToXin(sql, filterContext, jrxmlParams)

	params := make([]any, 0)
	debug := make([]map[string]any, 0)
	var bindErr error

	bindValue := func(value any) string {
		params = append(params, value)
		return d.Placeholder(len(params))
	}

	sql = reBindX.ReplaceAllStringFunc(sql, func(match string) string {
		if bindErr != nil {
			return "TRUE"
		}
		m := reBindX.FindStringSubmatch(match)
		if len(m) < 4 {
			return "TRUE"
		}
		fn := strings.ToUpper(m[1])
		column := strings.TrimSpace(m[2])
		name := m[3]
		if !IsSafeJasperColumnRef(column) {
			bindErr = fmt.Errorf("Unsafe column reference in $X{%s}: %s", fn, column)
			return "TRUE"
		}
		raw := filterContext[name]
		values := flattenFilterValues(raw)
		javaType := "java.lang.String"
		if meta, ok := jrxmlParams[name]; ok {
			javaType = meta.Class
		} else if len(values) > 1 {
			javaType = "java.util.ArrayList"
		}

		if len(values) == 0 {
			clause := "0 = 0"
			if fn == "NOTIN" || fn == "NOT IN" {
				clause = "1 = 0"
			}
			debug = append(debug, map[string]any{
				"parameter": name, "java_type": javaType, "values": []any{},
				"generated_sql": clause, "bindings": []any{},
			})
			return clause
		}

		placeholders := make([]string, 0, len(values))
		bindings := make([]any, 0, len(values))
		for _, v := range values {
			placeholders = append(placeholders, bindValue(v))
			bindings = append(bindings, v)
		}
		qMarks := strings.Repeat("?,", len(bindings))
		qMarks = strings.TrimSuffix(qMarks, ",")
		pgList := strings.Join(placeholders, ", ")

		var clause, debugSQL string
		switch fn {
		case "IN":
			clause = column + " IN (" + pgList + ")"
			debugSQL = column + " IN (" + qMarks + ")"
		case "NOTIN":
			clause = column + " NOT IN (" + pgList + ")"
			debugSQL = column + " NOT IN (" + qMarks + ")"
		case "EQUAL":
			clause = column + " = " + placeholders[0]
			debugSQL = column + " = ?"
		case "NOTEQUAL":
			clause = column + " <> " + placeholders[0]
			debugSQL = column + " <> ?"
		case "LESS":
			clause = column + " < " + placeholders[0]
			debugSQL = column + " < ?"
		case "GREATER":
			clause = column + " > " + placeholders[0]
			debugSQL = column + " > ?"
		case "LESSOREQUAL":
			clause = column + " <= " + placeholders[0]
			debugSQL = column + " <= ?"
		case "GREATEROREQUAL":
			clause = column + " >= " + placeholders[0]
			debugSQL = column + " >= ?"
		case "BETWEEN":
			if len(placeholders) < 2 {
				clause = "0 = 0"
				debugSQL = clause
			} else {
				clause = column + " BETWEEN " + placeholders[0] + " AND " + placeholders[1]
				debugSQL = column + " BETWEEN ? AND ?"
			}
		default:
			bindErr = fmt.Errorf("Unsupported $X function: %s", fn)
			return "TRUE"
		}
		debug = append(debug, map[string]any{
			"parameter": name, "java_type": javaType, "values": bindings,
			"generated_sql": debugSQL, "bindings": bindings,
		})
		return clause
	})
	if bindErr != nil {
		return nil, bindErr
	}

	sql = reBindPB.ReplaceAllStringFunc(sql, func(match string) string {
		if bindErr != nil {
			return "NULL"
		}
		m := reBindPB.FindStringSubmatch(match)
		if len(m) < 2 {
			return "NULL"
		}
		name := m[1]
		raw, ok := filterContext[name]
		if !ok {
			return "NULL"
		}
		javaType := javaTypeFor(name, raw, jrxmlParams)
		if arr, isArr := asAnySlice(raw); isArr {
			parts := make([]string, 0, len(arr))
			bindings := make([]any, 0, len(arr))
			for _, v := range arr {
				if v == nil || v == "" {
					continue
				}
				bindings = append(bindings, v)
				parts = append(parts, "'"+strings.ReplaceAll(fmt.Sprint(v), "'", "''")+"'")
			}
			clause := "NULL"
			if len(parts) > 0 {
				clause = strings.Join(parts, ", ")
			}
			debug = append(debug, map[string]any{
				"parameter": name, "java_type": javaType, "values": bindings,
				"generated_sql": "$P!{" + name + "} => " + clause, "bindings": bindings,
			})
			return clause
		}
		if raw == nil {
			return "NULL"
		}
		var clause string
		switch t := raw.(type) {
		case bool:
			if t {
				clause = "TRUE"
			} else {
				clause = "FALSE"
			}
		case int:
			clause = strconv.Itoa(t)
		case int64:
			clause = strconv.FormatInt(t, 10)
		case float64:
			clause = strconv.FormatFloat(t, 'f', -1, 64)
		default:
			s := fmt.Sprint(raw)
			if IsSafeSQLIdent(s) {
				clause = s
			} else {
				clause = "'" + strings.ReplaceAll(s, "'", "''") + "'"
			}
		}
		debug = append(debug, map[string]any{
			"parameter": name, "java_type": javaType, "values": []any{raw},
			"generated_sql": "$P!{" + name + "} => " + clause, "bindings": []any{raw},
		})
		return clause
	})
	if bindErr != nil {
		return nil, bindErr
	}

	sql = reBindP.ReplaceAllStringFunc(sql, func(match string) string {
		m := reBindP.FindStringSubmatch(match)
		if len(m) < 2 {
			return "NULL"
		}
		name := m[1]
		raw, ok := filterContext[name]
		if !ok {
			// MySQL ST_GeomFromText(wkt, srid) errors when srid is SQL NULL
			// (even on the unused side of "p_geometry IS NULL OR …").
			if name == "p_srid" {
				return "4326"
			}
			return "NULL"
		}
		javaType := javaTypeFor(name, raw, jrxmlParams)
		if arr, isArr := asAnySlice(raw); isArr {
			bindings := make([]any, 0, len(arr))
			for _, v := range arr {
				if v == nil || v == "" {
					continue
				}
				bindings = append(bindings, v)
			}
			if len(bindings) == 0 {
				if name == "p_srid" {
					return "4326"
				}
				return "NULL"
			}
			raw = bindings[0]
			debug = append(debug, map[string]any{
				"parameter": name, "java_type": javaType, "values": bindings,
				"generated_sql": name + " = ? (WARNING: Collection passed to $P{}; use $X{IN,...}. Using first value only.)",
				"bindings":      []any{raw},
			})
		}
		if raw == nil || raw == "" {
			if name == "p_srid" {
				return "4326"
			}
			return "NULL"
		}
		kind := "string"
		if meta, ok := jrxmlParams[name]; ok {
			kind = meta.Kind
		}
		if name == "p_srid" {
			kind = "integer"
		}
		var ph string
		switch kind {
		case "boolean":
			if b, ok := raw.(bool); ok {
				if b {
					ph = bindValue("t")
				} else {
					ph = bindValue("f")
				}
			} else {
				ph = bindValue(raw)
			}
		case "integer":
			switch t := raw.(type) {
			case int:
				if name == "p_srid" && t == 0 {
					return "4326"
				}
				ph = d.CastInteger(bindValue(t))
			case int64:
				if name == "p_srid" && t == 0 {
					return "4326"
				}
				ph = d.CastInteger(bindValue(int(t)))
			case float64:
				if name == "p_srid" && int(t) == 0 {
					return "4326"
				}
				ph = d.CastInteger(bindValue(int(t)))
			case string:
				if matched, _ := regexp.MatchString(`^-?\d+$`, t); matched {
					n, _ := strconv.Atoi(t)
					if name == "p_srid" && n == 0 {
						return "4326"
					}
					ph = d.CastInteger(bindValue(n))
				} else {
					ph = bindValue(raw)
				}
			default:
				ph = bindValue(raw)
			}
		case "double":
			switch t := raw.(type) {
			case int:
				ph = d.CastFloat(bindValue(float64(t)))
			case int64:
				ph = d.CastFloat(bindValue(float64(t)))
			case float64:
				ph = d.CastFloat(bindValue(t))
			case string:
				if f, err := strconv.ParseFloat(t, 64); err == nil {
					ph = d.CastFloat(bindValue(f))
				} else {
					ph = bindValue(raw)
				}
			default:
				ph = bindValue(raw)
			}
		default:
			ph = bindValue(raw)
		}
		if len(debug) == 0 || debug[len(debug)-1]["parameter"] != name {
			debug = append(debug, map[string]any{
				"parameter": name, "java_type": javaType, "values": []any{raw},
				"generated_sql": name + " = ?", "bindings": []any{raw},
			})
		}
		return ph
	})

	if reRemain.MatchString(sql) {
		return nil, fmt.Errorf("Unresolved Jasper parameter placeholders remain in SQL.")
	}
	return &BindResult{SQL: sql, Params: params, Debug: debug}, nil
}

func flattenFilterValues(raw any) []any {
	out := make([]any, 0)
	if arr, ok := asAnySlice(raw); ok {
		for _, v := range arr {
			if v == nil || v == "" {
				continue
			}
			out = append(out, v)
		}
		return out
	}
	if raw != nil && raw != "" {
		out = append(out, raw)
	}
	return out
}

func asAnySlice(raw any) ([]any, bool) {
	switch t := raw.(type) {
	case []any:
		return t, true
	case []string:
		out := make([]any, len(t))
		for i, v := range t {
			out[i] = v
		}
		return out, true
	default:
		return nil, false
	}
}

func javaTypeFor(name string, raw any, jrxmlParams map[string]ParamMeta) string {
	if meta, ok := jrxmlParams[name]; ok {
		return meta.Class
	}
	if _, ok := asAnySlice(raw); ok {
		return "java.util.ArrayList"
	}
	return "java.lang.String"
}
