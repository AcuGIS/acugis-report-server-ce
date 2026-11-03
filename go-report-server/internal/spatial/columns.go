package spatial

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// ColumnMeta is a non-geometry table column (PHP jrs_list_table_columns).
type ColumnMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ColumnsResult mirrors PHP map_columns.php JSON.
type ColumnsResult struct {
	Schema          string           `json:"schema"`
	Table           string           `json:"table"`
	GeometryColumn  string           `json:"geometry_column"`
	GeometryColumns []string         `json:"geometry_columns"`
	Columns         []ColumnMeta     `json:"columns"`
	ReportFields    []map[string]any `json:"report_fields"`
	SRID            int              `json:"srid"`
	Error           string           `json:"error,omitempty"`
}

// ListTableColumns lists non-geometry columns (PHP jrs_list_table_columns).
func ListTableColumns(ctx context.Context, conn datasources.QueryConn, d SpatialDialect, schema, table string) ([]ColumnMeta, error) {
	if d == nil {
		d = pgSpatial{}
	}
	if !IsSafeSQLIdent(table) || (schema != "" && !IsSafeSQLIdent(schema)) {
		return nil, ErrLayerUnresolved
	}
	sql, argsFn := d.ListAttributeColumnsSQL()
	rows, err := conn.Query(ctx, sql, argsFn(schema, table)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ColumnMeta
	for rows.Next() {
		var name, udt string
		if err := rows.Scan(&name, &udt); err != nil {
			continue
		}
		if name != "" && IsSafeSQLIdent(name) && !isGeometryName(name) {
			out = append(out, ColumnMeta{Name: name, Type: udt})
		}
	}
	return out, rows.Err()
}

// ReportDatasetFields merges JRXML <field> metadata with SELECT-list names
// (PHP jrs_report_dataset_fields).
func ReportDatasetFields(jrxmlPath string) []map[string]any {
	byName := map[string]map[string]any{}
	for _, f := range jrxmlDatasetFields(jrxmlPath) {
		name := stringField(f, "name")
		if name == "" {
			continue
		}
		byName[name] = f
	}
	sql, _ := JRXMLSQLQuery(jrxmlPath)
	for _, f := range sqlSelectFields(sql) {
		name := stringField(f, "name")
		if name == "" {
			continue
		}
		if existing, ok := byName[name]; ok {
			label := stringField(f, "label")
			if label != "" && strings.Contains(label, ".") {
				existing["label"] = label
			}
		} else {
			byName[name] = f
		}
	}
	out := make([]map[string]any, 0, len(byName))
	for _, f := range byName {
		out = append(out, f)
	}
	return out
}

func jrxmlDatasetFields(jrxmlPath string) []map[string]any {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		return nil
	}
	var doc jrxmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return extractFieldsByRegex(string(data))
	}
	fields := collectJRXMLFields(doc.Nodes)
	if len(fields) == 0 {
		return extractFieldsByRegex(string(data))
	}
	return fields
}

func collectJRXMLFields(nodes []jrxmlNode) []map[string]any {
	var out []map[string]any
	for _, n := range nodes {
		if localName(n.XMLName.Local) == "field" {
			name := strings.TrimSpace(n.Name)
			if name != "" && IsSafeFieldKey(name) {
				class := strings.TrimSpace(n.Class)
				if class == "" {
					class = "java.lang.String"
				}
				out = append(out, map[string]any{
					"name":  name,
					"label": name,
					"type":  javaClassToFieldType(class),
					"class": class,
				})
			}
		}
		out = append(out, collectJRXMLFields(n.Children)...)
	}
	return out
}

var reFieldAttr = regexp.MustCompile(`(?i)<field\b[^>]*\bname\s*=\s*"([^"]+)"[^>]*(?:\bclass\s*=\s*"([^"]*)")?`)

func extractFieldsByRegex(raw string) []map[string]any {
	matches := reFieldAttr.FindAllStringSubmatch(raw, -1)
	var out []map[string]any
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name == "" || !IsSafeFieldKey(name) {
			continue
		}
		class := "java.lang.String"
		if len(m) > 2 && strings.TrimSpace(m[2]) != "" {
			class = strings.TrimSpace(m[2])
		}
		out = append(out, map[string]any{
			"name":  name,
			"label": name,
			"type":  javaClassToFieldType(class),
			"class": class,
		})
	}
	return out
}

func javaClassToFieldType(class string) string {
	c := strings.ToLower(class)
	switch {
	case strings.Contains(c, "int"), strings.Contains(c, "long"), strings.Contains(c, "short"),
		strings.Contains(c, "double"), strings.Contains(c, "float"), strings.Contains(c, "decimal"),
		strings.Contains(c, "number"), strings.Contains(c, "bigdecimal"):
		return "number"
	case strings.Contains(c, "date"), strings.Contains(c, "time"):
		return "date"
	case strings.Contains(c, "bool"):
		return "boolean"
	default:
		return "string"
	}
}

var (
	reSelectFrom = regexp.MustCompile(`(?is)^\s*SELECT\s+(.*?)\s+FROM\s+`)
	reAsAlias    = regexp.MustCompile(`(?is)^(.*)\s+AS\s+(?:"([^"]+)"|([A-Za-z_][\w]*))\s*$`)
	reQualAlias  = regexp.MustCompile(`(?i)^([A-Za-z_][\w]*)\s*\.\s*([A-Za-z_][\w]*)\s+(?:"([^"]+)"|([A-Za-z_][\w]*))$`)
	reQualCol    = regexp.MustCompile(`^([A-Za-z_][\w]*)\s*\.\s*([A-Za-z_][\w]*)$`)
	reBareCol    = regexp.MustCompile(`^([A-Za-z_][\w]*)$`)
)

func sqlSelectFields(sql string) []map[string]any {
	normalized := NeutralizeJasperSQL(sql)
	m := reSelectFrom.FindStringSubmatch(normalized)
	if len(m) < 2 {
		return nil
	}
	var fields []map[string]any
	for _, part := range splitSelectList(m[1]) {
		part = strings.TrimSpace(part)
		if part == "" || part == "*" {
			continue
		}
		var name, label, source string
		if am := reAsAlias.FindStringSubmatch(part); am != nil {
			source = strings.TrimSpace(am[1])
			if am[2] != "" {
				name = am[2]
			} else {
				name = am[3]
			}
		} else if am := reQualAlias.FindStringSubmatch(part); am != nil {
			source = am[1] + "." + am[2]
			if am[3] != "" {
				name = am[3]
			} else {
				name = am[4]
			}
		} else if cm := reQualCol.FindStringSubmatch(part); cm != nil {
			name = cm[2]
			label = cm[1] + "." + cm[2]
			source = cm[1] + "." + cm[2]
		} else if cm := reBareCol.FindStringSubmatch(part); cm != nil {
			name = cm[1]
			label = cm[1]
			source = cm[1]
		} else {
			continue
		}
		if name == "" || !IsSafeFieldKey(name) {
			continue
		}
		if label == "" {
			if cm := reQualCol.FindStringSubmatch(strings.TrimSpace(source)); cm != nil {
				label = cm[1] + "." + cm[2]
			} else {
				label = name
			}
		}
		fields = append(fields, map[string]any{
			"name":   name,
			"label":  label,
			"type":   "string",
			"source": source,
		})
	}
	return fields
}

func splitSelectList(selectList string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range selectList {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, selectList[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, selectList[start:])
	return parts
}

// Columns resolves map column metadata for the edit UI (PHP map_columns.php).
func (s *Resolver) Columns(ctx context.Context, def *reports.Definition) (*ColumnsResult, int, error) {
	out := &ColumnsResult{
		Columns:         []ColumnMeta{},
		ReportFields:    []map[string]any{},
		GeometryColumns: []string{},
	}
	if def == nil {
		out.Error = "invalid report"
		return out, 400, ErrInvalidReport
	}

	jrxmlPath := filepath.Join(s.JRXMLDir, def.Key+".jrxml")
	out.ReportFields = ReportDatasetFields(jrxmlPath)

	probe := *def
	if probe.Raw == nil {
		probe.Raw = map[string]any{}
	} else {
		rawCopy := make(map[string]any, len(def.Raw)+1)
		for k, v := range def.Raw {
			rawCopy[k] = v
		}
		probe.Raw = rawCopy
	}
	probe.Raw["type"] = "map"
	if _, ok := probe.Raw["mapDefinition"]; !ok {
		if _, ok2 := probe.Raw["map_definition"]; !ok2 {
			probe.Raw["mapDefinition"] = map[string]any{}
		}
	}

	layer, _ := s.ResolveLayer(ctx, &probe)
	out.Schema = layer.Schema
	out.Table = layer.Table
	out.GeometryColumn = layer.GeometryColumn
	out.GeometryColumns = layer.GeometryColumns
	out.SRID = layer.SRID
	out.Error = layer.Error
	if layer.ReportFields != nil {
		out.ReportFields = layer.ReportFields
	}

	if layer.Schema != "" && layer.Table != "" && s.Datasources != nil && s.Connector != nil {
		dsName := def.Datasource()
		if ds, err := s.Datasources.Find(ctx, dsName); err == nil && ds.IsSpatialCapable() {
			if conn, dialect, err := s.OpenSpatial(ctx, ds); err == nil {
				cols, cerr := ListTableColumns(ctx, conn, dialect, layer.Schema, layer.Table)
				_ = conn.Close(ctx)
				if cerr == nil {
					out.Columns = cols
				}
			}
		}
	}

	status := 200
	if out.Error != "" && out.Table == "" {
		status = 422
	}
	return out, status, nil
}
