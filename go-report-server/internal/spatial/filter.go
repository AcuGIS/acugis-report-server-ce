package spatial

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var reservedFilterKeys = map[string]bool{
	"SUBREPORT_DIR": true,
	"REPORT_DIR":    true,
	"report":        true,
}

// ParseQueryMulti preserves repeated keys as []string (PHP jrs_parse_query_string_multi).
func ParseQueryMulti(rawQuery string) map[string]any {
	out := map[string]any{}
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return out
	}
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}
		var rawKey, rawVal string
		if eq := strings.IndexByte(pair, '='); eq < 0 {
			rawKey = pair
		} else {
			rawKey = pair[:eq]
			rawVal = pair[eq+1:]
		}
		key, err := url.QueryUnescape(strings.ReplaceAll(rawKey, "+", " "))
		if err != nil {
			key = rawKey
		}
		if strings.HasSuffix(key, "[]") {
			key = strings.TrimSuffix(key, "[]")
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		val, err := url.QueryUnescape(strings.ReplaceAll(rawVal, "+", " "))
		if err != nil {
			val = rawVal
		}
		if existing, ok := out[key]; ok {
			switch t := existing.(type) {
			case string:
				out[key] = []string{t, val}
			case []string:
				out[key] = append(t, val)
			default:
				out[key] = []string{val}
			}
		} else {
			out[key] = val
		}
	}
	return out
}

// CollectFilterContext mirrors PHP jrs_collect_filter_context.
func CollectFilterContext(query map[string]any, jrxmlParams map[string]ParamMeta, reportParams []map[string]any) map[string]any {
	const clearSentinel = "__JRS_CLEAR__"
	ctx := map[string]any{}
	for key, raw := range query {
		key = strings.TrimSpace(key)
		if key == "" || strings.HasPrefix(key, "_") || reservedFilterKeys[key] {
			continue
		}
		if !IsSafeSQLIdent(key) {
			continue
		}
		values := normalizeRawValues(raw)
		clean := make([]string, 0, len(values))
		sawClear := false
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if v == clearSentinel {
				sawClear = true
				continue
			}
			clean = append(clean, v)
		}

		meta, hasMeta := jrxmlParams[key]
		kind := "string"
		if hasMeta {
			kind = meta.Kind
		}
		if key == "p_srid" {
			kind = "integer"
		}
		forceCollection := kind == "collection" || ReportParamIsMultiple(reportParams, key) || len(clean) > 1
		if len(clean) == 0 {
			// Explicit cascade/user clear → empty Collection (match-all for $X{IN}).
			// Blank-only key= is omitted so JRXML defaults can apply.
			if sawClear && (forceCollection || kind == "collection") {
				ctx[key] = []any{}
			}
			continue
		}
		if forceCollection {
			itemKind := kind
			if kind == "collection" {
				itemKind = "string"
				if hasMeta {
					class := strings.ToLower(meta.Class)
					switch {
					case strings.Contains(class, "integer"), strings.Contains(class, "long"):
						itemKind = "integer"
					case strings.Contains(class, "double"), strings.Contains(class, "float"), strings.Contains(class, "decimal"):
						itemKind = "double"
					}
				}
			}
			list := make([]any, 0, len(clean))
			for _, v := range clean {
				list = append(list, CoerceFilterScalar(v, itemKind))
			}
			ctx[key] = list
			continue
		}
		ctx[key] = CoerceFilterScalar(clean[0], kind)
	}
	applyMissingJRXMLDefaults(ctx, jrxmlParams)
	return ctx
}

// applyMissingJRXMLDefaults fills parameters that were omitted from the request
// with parsed JRXML defaultValueExpression values. Explicit empty collections
// already in ctx (from key=) are left alone so cascade clears still match-all.
func applyMissingJRXMLDefaults(ctx map[string]any, jrxmlParams map[string]ParamMeta) {
	if ctx == nil || len(jrxmlParams) == 0 {
		return
	}
	for name, meta := range jrxmlParams {
		if meta.Default == nil {
			continue
		}
		if _, exists := ctx[name]; exists {
			continue
		}
		if skipPublishParams[name] || strings.HasPrefix(name, "REPORT_") {
			continue
		}
		if coerced := coerceJRXMLDefault(meta); coerced != nil {
			ctx[name] = coerced
		}
	}
}

func coerceJRXMLDefault(meta ParamMeta) any {
	itemKind := meta.Kind
	if meta.Kind == "collection" {
		itemKind = "string"
		class := strings.ToLower(meta.Class)
		switch {
		case strings.Contains(class, "integer"), strings.Contains(class, "long"):
			itemKind = "integer"
		case strings.Contains(class, "double"), strings.Contains(class, "float"), strings.Contains(class, "decimal"):
			itemKind = "double"
		}
	}
	switch d := meta.Default.(type) {
	case []string:
		out := make([]any, 0, len(d))
		for _, v := range d {
			out = append(out, CoerceFilterScalar(v, itemKind))
		}
		return out
	case []any:
		out := make([]any, 0, len(d))
		for _, v := range d {
			out = append(out, CoerceFilterScalar(stringifyAny(v), itemKind))
		}
		return out
	case string:
		if meta.Kind == "collection" {
			return []any{CoerceFilterScalar(d, itemKind)}
		}
		return CoerceFilterScalar(d, meta.Kind)
	case bool:
		return d
	case int:
		return d
	case float64:
		if meta.Kind == "integer" {
			return int(d)
		}
		return d
	default:
		s := strings.TrimSpace(stringifyAny(d))
		if s == "" {
			return nil
		}
		if meta.Kind == "collection" {
			return []any{CoerceFilterScalar(s, itemKind)}
		}
		return CoerceFilterScalar(s, meta.Kind)
	}
}

func normalizeRawValues(raw any) []string {
	switch t := raw.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, v := range t {
			if v == nil {
				continue
			}
			out = append(out, strings.TrimSpace(stringifyAny(v)))
		}
		return out
	case string:
		return []string{t}
	case nil:
		return nil
	default:
		return []string{stringifyAny(t)}
	}
}

// CoerceFilterScalar mirrors PHP jrs_coerce_filter_scalar.
func CoerceFilterScalar(value, kind string) any {
	switch kind {
	case "integer":
		if matched, _ := regexp.MatchString(`^-?\d+$`, value); matched {
			n, _ := strconv.Atoi(value)
			return n
		}
		return value
	case "double":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
		return value
	case "boolean":
		lower := strings.ToLower(value)
		switch lower {
		case "1", "true", "t", "yes", "y", "on":
			return true
		case "0", "false", "f", "no", "n", "off":
			return false
		default:
			return value
		}
	default:
		return value
	}
}

// ReportParamIsMultiple mirrors PHP jrs_report_param_is_multiple.
func ReportParamIsMultiple(reportParams []map[string]any, name string) bool {
	for _, p := range reportParams {
		n, _ := p["name"].(string)
		if !strings.EqualFold(strings.TrimSpace(n), name) {
			continue
		}
		if truthy(p["multiple"]) {
			return true
		}
		if t, ok := p["type"].(string); ok && strings.EqualFold(t, "collection") {
			return true
		}
	}
	return false
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func stringifyAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return strings.TrimSpace(toString(v))
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
