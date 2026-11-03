package server

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

const maxUploadBytes = 64 << 20 // 64 MiB

func (s *Server) parseReportForm(r *http.Request) (url.Values, *multipart.Form, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			return nil, nil, err
		}
		if r.MultipartForm == nil {
			return url.Values{}, nil, nil
		}
		return r.MultipartForm.Value, r.MultipartForm, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, nil, err
	}
	return r.PostForm, nil, nil
}

func formAccessGroups(form url.Values) []string {
	vals := form["access_groups[]"]
	if len(vals) == 0 {
		vals = form["access_groups"]
	}
	var out []string
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func formAccessGroupsCSV(groups []string) string {
	return strings.Join(groups, ",")
}

// parseParametersFromForm mirrors publish/edit_report.php parameter POST handling.
func parseParametersFromForm(form url.Values) (params []map[string]any, errs []string) {
	indexes := collectFormIndexes(form, "params[")
	for _, idx := range indexes {
		prefix := "params[" + strconv.Itoa(idx) + "]"
		name := strings.TrimSpace(firstFormValue(form, prefix+"[name]", ""))
		if name == "" {
			errs = append(errs, "Parameter name is required.")
			continue
		}
		typ := strings.TrimSpace(firstFormValue(form, prefix+"[type]", "string"))
		switch typ {
		case "string", "number", "date", "select":
		default:
			typ = "string"
		}
		label := strings.TrimSpace(firstFormValue(form, prefix+"[label]", ""))
		if label == "" {
			label = name
		}
		param := map[string]any{
			"name":     name,
			"type":     typ,
			"label":    label,
			"required": formTruthy(form, prefix+"[required]"),
			"hidden":   false,
		}
		if typ == "select" {
			sourceType := strings.TrimSpace(firstFormValue(form, prefix+"[source_type]", "static"))
			if sourceType == "sql" {
				sql := strings.TrimSpace(firstFormValue(form, prefix+"[sql]", ""))
				if sql == "" {
					errs = append(errs, `SQL query is required for select parameter "`+name+`".`)
					continue
				}
				param["source"] = "sql:" + sql
			} else {
				optionsRaw := firstFormValue(form, prefix+"[options]", "")
				var options []string
				for _, line := range strings.Split(optionsRaw, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						options = append(options, line)
					}
				}
				if len(options) == 0 {
					errs = append(errs, `Select parameter "`+name+`" requires at least one option.`)
					continue
				}
				param["options"] = options
			}
			if formTruthy(form, prefix+"[multiple]") {
				param["multiple"] = true
			}
		}
		params = append(params, param)
	}
	return params, errs
}

func collectFormIndexes(form url.Values, prefix string) []int {
	seen := map[int]struct{}{}
	var indexes []int
	for key := range form {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(key, prefix)
		end := strings.IndexByte(rest, ']')
		if end <= 0 {
			continue
		}
		n, err := strconv.Atoi(rest[:end])
		if err != nil {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		indexes = append(indexes, n)
	}
	for i := 0; i < len(indexes); i++ {
		for j := i + 1; j < len(indexes); j++ {
			if indexes[j] < indexes[i] {
				indexes[i], indexes[j] = indexes[j], indexes[i]
			}
		}
	}
	return indexes
}

func firstFormValue(form url.Values, key, fallback string) string {
	vals, ok := form[key]
	if !ok || len(vals) == 0 {
		return fallback
	}
	v := strings.TrimSpace(vals[0])
	if v == "" {
		return fallback
	}
	return v
}

func formTruthy(form url.Values, key string) bool {
	vals, ok := form[key]
	if !ok || len(vals) == 0 {
		return false
	}
	v := strings.TrimSpace(vals[0])
	return v != "" && v != "0" && !strings.EqualFold(v, "false") && !strings.EqualFold(v, "off")
}

func readUpload(form *multipart.Form, field string) (filename string, data []byte, present bool, err error) {
	if form == nil {
		return "", nil, false, nil
	}
	files := form.File[field]
	if len(files) == 0 {
		return "", nil, false, nil
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		return fh.Filename, nil, true, err
	}
	defer f.Close()
	data, err = io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
	if err != nil {
		return fh.Filename, nil, true, err
	}
	if len(data) > maxUploadBytes {
		return fh.Filename, nil, true, errUploadTooLarge
	}
	return fh.Filename, data, true, nil
}

var errUploadTooLarge = &httpError{Status: http.StatusRequestEntityTooLarge, Message: "upload too large"}

func reportKeyFromFilename(name string) string {
	base := filepath.Base(name)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return jasper.NormalizeReportKey(stem)
}

func optionLines(options any) string {
	arr, ok := options.([]any)
	if !ok {
		if strs, ok := options.([]string); ok {
			return strings.Join(strs, "\n")
		}
		return ""
	}
	var lines []string
	for _, opt := range arr {
		switch t := opt.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				lines = append(lines, t)
			}
		case map[string]any:
			label := stringify(t["label"])
			if label == "" {
				label = stringify(t["value"])
			}
			if label != "" {
				lines = append(lines, label)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func stripLegacyReportFields(raw map[string]any) {
	for _, key := range []string{"params", "datasourceName", "outputFormat", "jrxmlPath", "summary", "reportType", "map_definition", "embedded_map"} {
		delete(raw, key)
	}
}

// resolveCassandraReportQuery picks CQL in Designer/publish priority order:
// 1) JRXML / Designer-embedded query (JR7 <query> or Digester <queryString>, language=cassandra|cql|collection)
// 2) Accompanying project.json / AcuGIS report metadata
// 3) Manual override (fallback for unusual imported reports only)
func resolveCassandraReportQuery(jrxmlQuery, projectQuery, manualQuery string) string {
	for _, q := range []string{jrxmlQuery, projectQuery, manualQuery} {
		if t := strings.TrimSpace(q); t != "" {
			return t
		}
	}
	return ""
}

// resolveReportQueryDisplay returns the CQL shown on Report Settings and its source label.
// Prefer JRXML (runtime authority) over catalog JSON so administrators see the Designer query.
func resolveReportQueryDisplay(def *reports.Definition, store *reports.Store) (query, source string) {
	if def == nil {
		return "", ""
	}
	if store != nil {
		path := filepath.Join(store.JRXMLDir(), def.Key+".jrxml")
		if q := reports.LoadCollectionQueryFromJRXMLFile(path); q != "" {
			return q, "Report definition"
		}
	}
	if q := strings.TrimSpace(def.QuerySQL()); q != "" {
		return q, "Report settings"
	}
	return "", ""
}

func buildPublishDefinition(key, name, description, datasource, reportType, groupsCSV string, parameters []map[string]any, querySQL string) map[string]any {
	raw := map[string]any{
		"name":          name,
		"description":   description,
		"report":        key,
		"type":          reportType,
		"datasource":    datasource,
		"format":        "html",
		"parameters":    parameters,
		"access_groups": groupsCSV,
	}
	if strings.TrimSpace(querySQL) != "" {
		raw["query"] = strings.TrimSpace(querySQL)
	}
	if reportType == "map" {
		raw["mapDefinition"] = mapreport.DefaultMapDefinition("")
	}
	return raw
}

func paramMapByName(params []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(params))
	for _, p := range params {
		name := stringify(p["name"])
		if name == "" {
			continue
		}
		if _, exists := out[name]; !exists {
			out[name] = p
		}
	}
	return out
}

// reconcileEditParameters keeps manually configured parameter metadata when a
// replacement JRXML is uploaded. Form values win for submitted fields; existing
// defaults/lookups/hidden flags are preserved when the form omitted them.
// Parameters present only in JRXML are appended. Parameters present only in
// current metadata are kept so an upload does not silently drop them.
func reconcileEditParameters(existing, formParams []map[string]any, jrxmlData []byte) []map[string]any {
	base := formParams
	if len(base) == 0 {
		base = existing
	}
	existingByName := paramMapByName(existing)
	byName := map[string]map[string]any{}
	var order []string
	add := func(p map[string]any) {
		name := stringify(p["name"])
		if name == "" || byName[name] != nil {
			return
		}
		merged := cloneParamMap(p)
		if prev := existingByName[name]; prev != nil {
			for _, key := range []string{"default", "lookup", "hidden", "source", "options"} {
				if _, ok := p[key]; !ok {
					if v, ok := prev[key]; ok {
						merged[key] = v
					}
				}
			}
			if stringify(merged["label"]) == "" || stringify(merged["label"]) == name {
				if label := stringify(prev["label"]); label != "" {
					merged["label"] = label
				}
			}
		}
		byName[name] = merged
		order = append(order, name)
	}
	for _, p := range base {
		add(p)
	}
	if len(jrxmlData) > 0 {
		for _, p := range mapreport.PublishParametersFromJRXML(jrxmlData) {
			name := stringify(p["name"])
			if name == "" {
				continue
			}
			if prev, ok := byName[name]; ok {
				if stringify(prev["type"]) == "" && stringify(p["type"]) != "" {
					prev["type"] = p["type"]
				}
				continue
			}
			add(p)
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func applyEditDefinition(existing map[string]any, key, name, description, datasource, reportType, groupsCSV string, parameters []map[string]any, mapDef map[string]any, embeddedMap map[string]any, querySQL string) map[string]any {
	raw := map[string]any{}
	for k, v := range existing {
		raw[k] = v
	}
	if name == "" {
		if prev := stringify(raw["name"]); prev != "" {
			name = prev
		} else {
			name = key
		}
	}
	raw["name"] = name
	raw["description"] = description
	raw["report"] = key
	raw["type"] = reportType
	raw["datasource"] = datasource
	raw["format"] = "html"
	raw["parameters"] = parameters
	raw["access_groups"] = groupsCSV
	raw["mapDefinition"] = mapDef
	if embeddedMap != nil {
		raw["embeddedMap"] = embeddedMap
	}
	if strings.TrimSpace(querySQL) != "" {
		raw["query"] = strings.TrimSpace(querySQL)
	} else {
		delete(raw, "query")
	}
	stripLegacyReportFields(raw)
	return raw
}

func (s *Server) validateMapDatasource(r *http.Request, datasource, reportType string) string {
	if reportType != "map" || datasource == "" {
		return ""
	}
	if s.arcgisDatasourceExists(r.Context(), datasource) {
		return "Map Report requires a spatial datasource (PostgreSQL/PostGIS, MySQL, SQL Server, or Oracle)."
	}
	if s.deps.Datasources == nil {
		return ""
	}
	ds, err := s.deps.Datasources.Find(r.Context(), datasource)
	if err != nil || ds == nil {
		return ""
	}
	if !ds.IsSpatialCapable() {
		return "Map Report requires a spatial datasource (PostgreSQL/PostGIS, MySQL, SQL Server, or Oracle)."
	}
	return ""
}

func paramViewModels(params []map[string]any) []paramRowData {
	out := make([]paramRowData, 0, len(params))
	for i, p := range params {
		source := stringify(p["source"])
		sourceType := "static"
		sql := ""
		if strings.HasPrefix(strings.ToLower(source), "sql:") {
			sourceType = "sql"
			sql = strings.TrimSpace(source[4:])
		}
		out = append(out, paramRowData{
			Index:      i,
			Name:       stringify(p["name"]),
			Type:       stringify(p["type"]),
			Label:      stringify(p["label"]),
			Required:   truthyAny(p["required"]),
			Multiple:   truthyAny(p["multiple"]),
			SourceType: sourceType,
			SQL:        sql,
			Options:    optionLines(p["options"]),
		})
	}
	return out
}

func truthyAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(t)
		return s != "" && s != "0" && !strings.EqualFold(s, "false")
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}
