package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// handleParameterLookupPHP serves GET|POST /reports/parameter_lookup.php?report=&parameter=
func (s *Server) handleParameterLookupPHP(w http.ResponseWriter, r *http.Request) {
	key := jasper.NormalizeReportKey(r.URL.Query().Get("report"))
	name := strings.TrimSpace(r.URL.Query().Get("parameter"))
	if name == "" {
		name = strings.TrimSpace(r.URL.Query().Get("name"))
	}
	s.writeParameterLookup(w, r, key, name)
}

// handleAPIParameterLookup serves GET|POST /api/v1/reports/{key}/parameters/{name}/lookup
func (s *Server) handleAPIParameterLookup(w http.ResponseWriter, r *http.Request) {
	key := jasper.NormalizeReportKey(chi.URLParam(r, "key"))
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	s.writeParameterLookup(w, r, key, name)
}

func (s *Server) writeParameterLookup(w http.ResponseWriter, r *http.Request, key, paramName string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !jasper.IsValidReportKey(key) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid report"})
		return
	}
	if err := s.enforceEmbedContentScope(r, key); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}
	paramName = strings.TrimSpace(paramName)
	if paramName == "" || !mapreport.IsSafeSQLIdent(paramName) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid parameter"})
		return
	}

	user := auth.FromContext(r.Context())
	def, err := s.loadAuthorizedReport(r, user, key)
	if err != nil {
		s.writeAuthzJSON(w, err)
		return
	}

	lookups, jrxmlParams, err := s.loadReportLookups(def)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read report JRXML"})
		return
	}
	lookup, ok := lookups[paramName]
	if !ok || !lookup.HasLookup() {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "parameter lookup not found"})
		return
	}

	finder, connector := s.datasourceAccess()
	if finder == nil || connector == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "datasource access unavailable"})
		return
	}

	dsName := mapreport.ResolveLookupDatasource(lookup.Datasource, def.Datasource())
	if dsName == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "report has no datasource for lookup"})
		return
	}
	ds, err := finder.Find(r.Context(), dsName)
	if err != nil || ds == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lookup datasource not found"})
		return
	}

	parentValues := overlayEmbedBound(s.collectLookupParentValues(r, def, jrxmlParams), embedParameters(r))
	dialect, err := mapreport.DialectFor(ds)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported lookup datasource"})
		return
	}
	bound, err := mapreport.BindJasperSQLDialect(lookup.SQL, parentValues, jrxmlParams, dialect)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	opts, err := datasources.RunBoundParameterLookup(
		r.Context(), ds, connector, bound.SQL, bound.Params, lookup.DisplayField, lookup.ValueField,
	)
	if err != nil {
		s.logger.Error("parameter lookup", "report", key, "parameter", paramName, "error", err)
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lookup query failed"})
		return
	}
	if opts == nil {
		opts = []map[string]any{}
	}
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, map[string]any{"options": opts})
}

func (s *Server) loadReportLookups(def *reports.Definition) (map[string]mapreport.ParameterLookupMeta, map[string]mapreport.ParamMeta, error) {
	if def == nil || s.deps.Reports == nil {
		return map[string]mapreport.ParameterLookupMeta{}, map[string]mapreport.ParamMeta{}, nil
	}
	jrxmlPath := filepath.Join(s.deps.Reports.JRXMLDir(), def.Key+".jrxml")
	lookups, err := mapreport.JRXMLParameterLookups(jrxmlPath)
	if err != nil {
		return nil, nil, err
	}
	params, err := mapreport.JRXMLParameters(jrxmlPath)
	if err != nil {
		return nil, nil, err
	}
	return lookups, params, nil
}

func (s *Server) collectLookupParentValues(r *http.Request, def *reports.Definition, jrxmlParams map[string]mapreport.ParamMeta) map[string]any {
	rawQuery := r.URL.RawQuery
	if r.Method == http.MethodPost {
		ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
		if strings.Contains(ct, "application/json") {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body != nil {
				if nested, ok := body["params"].(map[string]any); ok {
					return coerceLookupParentMap(nested, jrxmlParams, def)
				}
				if nested, ok := body["parameters"].(map[string]any); ok {
					return coerceLookupParentMap(nested, jrxmlParams, def)
				}
				return coerceLookupParentMap(body, jrxmlParams, def)
			}
		}
		_ = r.ParseForm()
		if len(r.PostForm) > 0 {
			values := map[string]any{}
			for k, vals := range r.PostForm {
				if len(vals) == 1 {
					values[k] = vals[0]
				} else if len(vals) > 1 {
					cp := make([]string, len(vals))
					copy(cp, vals)
					values[k] = cp
				}
			}
			return coerceLookupParentMap(values, jrxmlParams, def)
		}
	}
	query := mapreport.ParseQueryMulti(rawQuery)
	delete(query, "report")
	delete(query, "parameter")
	delete(query, "name")
	reportParams := reportParamMaps(def)
	return mapreport.CollectFilterContext(query, jrxmlParams, reportParams)
}

func coerceLookupParentMap(raw map[string]any, jrxmlParams map[string]mapreport.ParamMeta, def *reports.Definition) map[string]any {
	// Reuse CollectFilterContext by normalizing into ParseQueryMulti-shaped values.
	query := map[string]any{}
	for k, v := range raw {
		k = strings.TrimSpace(k)
		if k == "" || strings.HasPrefix(k, "_") {
			continue
		}
		switch strings.ToLower(k) {
		case "report", "parameter", "name", "params", "parameters":
			continue
		}
		query[k] = v
	}
	return mapreport.CollectFilterContext(query, jrxmlParams, reportParamMaps(def))
}

func reportParamMaps(def *reports.Definition) []map[string]any {
	if def == nil {
		return nil
	}
	return def.Parameters()
}

// enrichConfigWithJRXMLLookups merges JRXML cascading lookup metadata and
// defaultValueExpression values onto report config parameters.
// Defaults set initial selection only; they never filter available options.
func enrichConfigWithJRXMLLookups(payload map[string]any, lookups map[string]mapreport.ParameterLookupMeta, jrxmlParams map[string]mapreport.ParamMeta) map[string]any {
	if payload == nil {
		return payload
	}

	payload = enrichConfigWithJRXMLDefaults(payload, jrxmlParams)

	if len(lookups) == 0 {
		return payload
	}

	params, key := configParameterList(payload)
	if params == nil {
		params = []any{}
		key = "parameters"
	}

	byName := map[string]int{}
	for i, raw := range params {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringify(m["name"]))
		if name != "" {
			byName[name] = i
		}
	}

	// Ensure dependency parents exist as promptable fields.
	for _, lookup := range lookups {
		if !lookup.HasLookup() {
			continue
		}
		for _, dep := range lookup.Depends {
			if _, ok := byName[dep]; ok {
				continue
			}
			entry := synthesizeParamFromJRXML(dep, jrxmlParams[dep], nil)
			params = append(params, entry)
			byName[dep] = len(params) - 1
		}
	}

	for name, lookup := range lookups {
		if !lookup.HasLookup() {
			continue
		}
		lookupObj := map[string]any{
			"enabled":      true,
			"datasource":   lookup.Datasource,
			"depends":      append([]string{}, lookup.Depends...),
			"displayField": lookup.DisplayField,
			"valueField":   lookup.ValueField,
			"sql":          lookup.SQL,
		}
		if i, ok := byName[name]; ok {
			m, ok := params[i].(map[string]any)
			if !ok {
				m = map[string]any{"name": name}
			}
			m = cloneParamMap(m)
			m["lookup"] = lookupObj
			m["type"] = "select"
			if m["options"] == nil {
				m["options"] = []any{}
			}
			if meta, ok := jrxmlParams[name]; ok && meta.Kind == "collection" {
				m["multiple"] = true
			}
			applyJRXMLDefault(m, jrxmlParams[name])
			// Cascading options come from the on-demand lookup API, not static expansion.
			delete(m, "source")
			params[i] = m
			continue
		}
		entry := synthesizeParamFromJRXML(name, jrxmlParams[name], lookupObj)
		params = append(params, entry)
		byName[name] = len(params) - 1
	}

	out := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		out[k] = v
	}
	out["parameters"] = params
	if key == "params" {
		delete(out, "params")
	}
	return out
}

// enrichConfigWithJRXMLDefaults copies parsed JRXML defaultValueExpression onto
// matching config parameters as "default". Does not alter options lists.
func enrichConfigWithJRXMLDefaults(payload map[string]any, jrxmlParams map[string]mapreport.ParamMeta) map[string]any {
	if payload == nil || len(jrxmlParams) == 0 {
		return payload
	}
	params, key := configParameterList(payload)
	if params == nil {
		return payload
	}
	changed := false
	for i, raw := range params {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringify(m["name"]))
		meta, ok := jrxmlParams[name]
		if !ok || meta.Default == nil {
			continue
		}
		m = cloneParamMap(m)
		applyJRXMLDefault(m, meta)
		params[i] = m
		changed = true
	}
	if !changed {
		return payload
	}
	out := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		out[k] = v
	}
	out["parameters"] = params
	if key == "params" {
		delete(out, "params")
	}
	return out
}

func applyJRXMLDefault(m map[string]any, meta mapreport.ParamMeta) {
	if m == nil || meta.Default == nil {
		return
	}
	m["default"] = meta.Default
	if meta.Kind == "collection" {
		m["multiple"] = true
	}
}

func synthesizeParamFromJRXML(name string, meta mapreport.ParamMeta, lookupObj map[string]any) map[string]any {
	entry := map[string]any{
		"name":  name,
		"label": name,
	}
	if lookupObj != nil {
		entry["type"] = "select"
		entry["lookup"] = lookupObj
		entry["options"] = []any{}
		if meta.Kind == "collection" {
			entry["multiple"] = true
		}
		applyJRXMLDefault(entry, meta)
		return entry
	}
	switch meta.Kind {
	case "integer", "double", "boolean", "date":
		entry["type"] = meta.Kind
	case "collection":
		entry["type"] = "select"
		entry["multiple"] = true
		entry["options"] = []any{}
		entry["required"] = true
	default:
		entry["type"] = "string"
		entry["required"] = true
	}
	applyJRXMLDefault(entry, meta)
	return entry
}

func cloneParamMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+4)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func configParameterList(definition map[string]any) ([]any, string) {
	for _, key := range []string{"parameters", "params"} {
		v, ok := definition[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case []any:
			out := make([]any, len(t))
			copy(out, t)
			return out, key
		case []map[string]any:
			out := make([]any, len(t))
			for i, m := range t {
				out[i] = m
			}
			return out, key
		}
	}
	return nil, ""
}
