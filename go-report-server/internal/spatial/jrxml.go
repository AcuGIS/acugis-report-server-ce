package spatial

import (
	"encoding/xml"
	"os"
	"regexp"
	"strings"
)

var (
	reNeutralizeX  = regexp.MustCompile(`\$X\{[^}]+\}`)
	reNeutralizePB = regexp.MustCompile(`\$P!\{[^}]+\}`)
	reNeutralizeP  = regexp.MustCompile(`\$P\{[^}]+\}`)
)

// ParamMeta is JRXML parameter metadata used for filter coercion / binding.
type ParamMeta struct {
	Name    string
	Class   string
	Kind    string
	Default any // parsed defaultValueExpression: string, number, bool, or []string
}

// LookupDatasourceCurrentReport is the Designer-emitted datasource label meaning
// "use the published report's JDBC datasource".
const LookupDatasourceCurrentReport = "Current Report Dataset"

// ParameterLookupMeta is cascading prompt lookup config from JRXML custom properties
// (acugis.parameter.{name}.lookup.*).
type ParameterLookupMeta struct {
	Enabled      bool
	Datasource   string
	Depends      []string
	DisplayField string
	ValueField   string
	SQL          string
}

// HasLookup reports whether the meta has an enabled SQL lookup.
func (m ParameterLookupMeta) HasLookup() bool {
	return m.Enabled && strings.TrimSpace(m.SQL) != ""
}

// NeutralizeJasperSQL replaces Jasper placeholders so SQL can be parsed for metadata.
func NeutralizeJasperSQL(sql string) string {
	out := reNeutralizeX.ReplaceAllString(sql, "TRUE")
	out = reNeutralizePB.ReplaceAllString(out, "NULL")
	out = reNeutralizeP.ReplaceAllString(out, "NULL")
	return out
}

// JRXMLSQLQuery extracts the first query/queryString text from a JRXML file.
func JRXMLSQLQuery(jrxmlPath string) (string, error) {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return extractJRXMLSQL(data), nil
}

// JRXMLParameters returns parameter name → metadata from JRXML.
func JRXMLParameters(jrxmlPath string) (map[string]ParamMeta, error) {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]ParamMeta{}, nil
		}
		return nil, err
	}
	return extractJRXMLParameters(data), nil
}

// JRXMLParameterLookups returns parameter name → lookup metadata for parameters
// that declare acugis.parameter.*.lookup.* properties. Entries without enabled+sql
// are omitted.
func JRXMLParameterLookups(jrxmlPath string) (map[string]ParameterLookupMeta, error) {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]ParameterLookupMeta{}, nil
		}
		return nil, err
	}
	return extractJRXMLParameterLookups(data), nil
}

// JRXMLHasSpatialParameters reports p_geometry + p_srid presence.
func JRXMLHasSpatialParameters(jrxmlPath string) bool {
	params, err := JRXMLParameters(jrxmlPath)
	if err != nil || len(params) == 0 {
		return false
	}
	_, hasGeom := params["p_geometry"]
	_, hasSRID := params["p_srid"]
	return hasGeom && hasSRID
}

type jrxmlDoc struct {
	XMLName xml.Name
	Nodes   []jrxmlNode `xml:",any"`
}

type jrxmlNode struct {
	XMLName   xml.Name
	Name      string      `xml:"name,attr"`
	Class     string      `xml:"class,attr"`
	Value     string      `xml:"value,attr"`
	InnerText string      `xml:",chardata"`
	CDATA     string      `xml:",cdata"`
	Children  []jrxmlNode `xml:",any"`
}

func extractJRXMLSQL(data []byte) string {
	var doc jrxmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		// Fallback: local-name scan via regex on raw XML when namespaces confuse decoder.
		return extractSQLByRegex(string(data))
	}
	if sql := findSQLNodes(doc.Nodes); sql != "" {
		return sql
	}
	return extractSQLByRegex(string(data))
}

func findSQLNodes(nodes []jrxmlNode) string {
	for _, n := range nodes {
		local := localName(n.XMLName.Local)
		if local == "query" || local == "queryString" {
			text := strings.TrimSpace(n.InnerText)
			if text == "" {
				text = strings.TrimSpace(n.CDATA)
			}
			if text == "" {
				text = collectText(n.Children)
			}
			if text != "" {
				return text
			}
		}
		if sql := findSQLNodes(n.Children); sql != "" {
			return sql
		}
	}
	return ""
}

func extractJRXMLParameters(data []byte) map[string]ParamMeta {
	out := map[string]ParamMeta{}
	var doc jrxmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return extractParamsByRegex(string(data))
	}
	collectParams(doc.Nodes, out)
	if len(out) == 0 {
		return extractParamsByRegex(string(data))
	}
	return out
}

func collectParams(nodes []jrxmlNode, out map[string]ParamMeta) {
	for _, n := range nodes {
		if localName(n.XMLName.Local) == "parameter" {
			name := strings.TrimSpace(n.Name)
			if name != "" && IsSafeSQLIdent(name) {
				class := strings.TrimSpace(n.Class)
				if class == "" {
					class = "java.lang.String"
				}
				out[name] = ParamMeta{
					Name:    name,
					Class:   class,
					Kind:    JavaClassToParamKind(class),
					Default: defaultFromChildren(n.Children),
				}
			}
		}
		collectParams(n.Children, out)
	}
}

func defaultFromChildren(children []jrxmlNode) any {
	for _, child := range children {
		if localName(child.XMLName.Local) != "defaultValueExpression" {
			continue
		}
		text := strings.TrimSpace(child.InnerText)
		if text == "" {
			text = strings.TrimSpace(child.CDATA)
		}
		if text == "" {
			text = collectText(child.Children)
		}
		return ParseDefaultValueExpression(text)
	}
	return nil
}

func extractJRXMLParameterLookups(data []byte) map[string]ParameterLookupMeta {
	out := map[string]ParameterLookupMeta{}
	var doc jrxmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return extractLookupsByRegex(string(data))
	}
	collectLookups(doc.Nodes, out)
	if len(out) == 0 {
		return extractLookupsByRegex(string(data))
	}
	return out
}

func collectLookups(nodes []jrxmlNode, out map[string]ParameterLookupMeta) {
	for _, n := range nodes {
		if localName(n.XMLName.Local) == "parameter" {
			name := strings.TrimSpace(n.Name)
			if name != "" && IsSafeSQLIdent(name) {
				if meta, ok := lookupMetaFromProperties(name, n.Children); ok {
					out[name] = meta
				}
			}
		}
		collectLookups(n.Children, out)
	}
}

func lookupMetaFromProperties(paramName string, children []jrxmlNode) (ParameterLookupMeta, bool) {
	meta := ParameterLookupMeta{}
	found := false
	for _, child := range children {
		if localName(child.XMLName.Local) != "property" {
			continue
		}
		field, ok := lookupPropField(strings.TrimSpace(child.Name), paramName)
		if !ok {
			continue
		}
		found = true
		val := propertyNodeValue(child)
		switch field {
		case "enabled":
			meta.Enabled = strings.EqualFold(strings.TrimSpace(val), "true") || strings.TrimSpace(val) == "1"
		case "datasource":
			meta.Datasource = strings.TrimSpace(val)
		case "depends":
			meta.Depends = parseDependsCSV(val)
		case "displayField":
			meta.DisplayField = strings.TrimSpace(val)
		case "valueField":
			meta.ValueField = strings.TrimSpace(val)
		case "sql":
			meta.SQL = strings.TrimSpace(val)
		}
	}
	if !found {
		return ParameterLookupMeta{}, false
	}
	if meta.Datasource == "" {
		meta.Datasource = LookupDatasourceCurrentReport
	}
	meta.Depends = mergeLookupDepends(paramName, meta.Depends, meta.SQL)
	if !meta.HasLookup() {
		return ParameterLookupMeta{}, false
	}
	return meta, true
}

func lookupPropField(propName, paramName string) (string, bool) {
	prefix := "acugis.parameter." + paramName + ".lookup."
	if !strings.HasPrefix(propName, prefix) {
		return "", false
	}
	field := propName[len(prefix):]
	if field == "" || strings.Contains(field, ".") {
		return "", false
	}
	return field, true
}

func propertyNodeValue(n jrxmlNode) string {
	if v := strings.TrimSpace(n.Value); v != "" {
		return v
	}
	if t := strings.TrimSpace(n.InnerText); t != "" {
		return t
	}
	if t := strings.TrimSpace(n.CDATA); t != "" {
		return t
	}
	return collectText(n.Children)
}

func parseDependsCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" || !IsSafeSQLIdent(name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

var reLookupSQLParam = regexp.MustCompile(`\$P\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// mergeLookupDepends unions explicit depends with $P{name} refs in lookup SQL.
func mergeLookupDepends(paramName string, listed []string, sql string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(listed)+4)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || name == paramName || !IsSafeSQLIdent(name) || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, d := range listed {
		add(d)
	}
	for _, m := range reLookupSQLParam.FindAllStringSubmatch(sql, -1) {
		if len(m) >= 2 {
			add(m[1])
		}
	}
	return out
}

// ResolveLookupDatasource maps Designer "Current Report Dataset" to the report DS name.
func ResolveLookupDatasource(lookupDatasource, reportDatasource string) string {
	ds := strings.TrimSpace(lookupDatasource)
	if ds == "" || strings.EqualFold(ds, LookupDatasourceCurrentReport) {
		return strings.TrimSpace(reportDatasource)
	}
	return ds
}

func collectText(nodes []jrxmlNode) string {
	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		t := strings.TrimSpace(n.InnerText)
		if t == "" {
			t = strings.TrimSpace(n.CDATA)
		}
		if t != "" {
			parts = append(parts, t)
		}
		if child := collectText(n.Children); child != "" {
			parts = append(parts, child)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func localName(name string) string {
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[i+1:]
	}
	return name
}

var (
	reQueryTag      = regexp.MustCompile(`(?is)<(?:[\w-]+:)?(?:query|queryString)\b[^>]*>(.*?)</(?:[\w-]+:)?(?:query|queryString)>`)
	reParamTag      = regexp.MustCompile(`(?is)<(?:[\w-]+:)?parameter\b([^>]*)/?>`)
	reParamBlock    = regexp.MustCompile(`(?is)<(?:[\w-]+:)?parameter\b([^>]*)>(.*?)</(?:[\w-]+:)?parameter>`)
	rePropertyTag   = regexp.MustCompile(`(?is)<(?:[\w-]+:)?property\b([^>]*)(?:/>|>(.*?)</(?:[\w-]+:)?property>)`)
	reDefaultExpr   = regexp.MustCompile(`(?is)<(?:[\w-]+:)?defaultValueExpression\b[^>]*>(.*?)</(?:[\w-]+:)?defaultValueExpression>`)
	reAttrName      = regexp.MustCompile(`(?i)\bname\s*=\s*"([^"]+)"`)
	reAttrClass     = regexp.MustCompile(`(?i)\bclass\s*=\s*"([^"]+)"`)
	reAttrValue     = regexp.MustCompile(`(?i)\bvalue\s*=\s*"([^"]*)"`)
	reCDATA         = regexp.MustCompile(`(?is)<!\[CDATA\[(.*?)\]\]>`)
)

func extractSQLByRegex(raw string) string {
	m := reQueryTag.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	body := m[1]
	if cd := reCDATA.FindStringSubmatch(body); len(cd) == 2 {
		return strings.TrimSpace(cd[1])
	}
	return strings.TrimSpace(stripTags(body))
}

func extractParamsByRegex(raw string) map[string]ParamMeta {
	out := map[string]ParamMeta{}
	// Prefer full parameter blocks so defaultValueExpression bodies are visible.
	for _, m := range reParamBlock.FindAllStringSubmatch(raw, -1) {
		if len(m) < 3 {
			continue
		}
		attrs := m[1]
		body := m[2]
		nm := reAttrName.FindStringSubmatch(attrs)
		if len(nm) < 2 {
			continue
		}
		name := strings.TrimSpace(nm[1])
		if name == "" || !IsSafeSQLIdent(name) {
			continue
		}
		class := "java.lang.String"
		if cm := reAttrClass.FindStringSubmatch(attrs); len(cm) == 2 && strings.TrimSpace(cm[1]) != "" {
			class = strings.TrimSpace(cm[1])
		}
		out[name] = ParamMeta{
			Name:    name,
			Class:   class,
			Kind:    JavaClassToParamKind(class),
			Default: defaultFromExpressionBody(body),
		}
	}
	// Self-closing / attribute-only parameters not captured above.
	for _, m := range reParamTag.FindAllStringSubmatch(raw, -1) {
		if len(m) < 2 {
			continue
		}
		attrs := m[1]
		nm := reAttrName.FindStringSubmatch(attrs)
		if len(nm) < 2 {
			continue
		}
		name := strings.TrimSpace(nm[1])
		if name == "" || !IsSafeSQLIdent(name) {
			continue
		}
		if _, exists := out[name]; exists {
			continue
		}
		class := "java.lang.String"
		if cm := reAttrClass.FindStringSubmatch(attrs); len(cm) == 2 && strings.TrimSpace(cm[1]) != "" {
			class = strings.TrimSpace(cm[1])
		}
		out[name] = ParamMeta{Name: name, Class: class, Kind: JavaClassToParamKind(class)}
	}
	return out
}

func defaultFromExpressionBody(body string) any {
	m := reDefaultExpr.FindStringSubmatch(body)
	if len(m) < 2 {
		return nil
	}
	inner := m[1]
	if cd := reCDATA.FindStringSubmatch(inner); len(cd) == 2 {
		return ParseDefaultValueExpression(cd[1])
	}
	return ParseDefaultValueExpression(stripTags(inner))
}

func extractLookupsByRegex(raw string) map[string]ParameterLookupMeta {
	out := map[string]ParameterLookupMeta{}
	for _, m := range reParamBlock.FindAllStringSubmatch(raw, -1) {
		if len(m) < 3 {
			continue
		}
		attrs := m[1]
		body := m[2]
		nm := reAttrName.FindStringSubmatch(attrs)
		if len(nm) < 2 {
			continue
		}
		name := strings.TrimSpace(nm[1])
		if name == "" || !IsSafeSQLIdent(name) {
			continue
		}
		props := map[string]string{}
		for _, pm := range rePropertyTag.FindAllStringSubmatch(body, -1) {
			if len(pm) < 2 {
				continue
			}
			pattrs := pm[1]
			pnm := reAttrName.FindStringSubmatch(pattrs)
			if len(pnm) < 2 {
				continue
			}
			propName := strings.TrimSpace(pnm[1])
			field, ok := lookupPropField(propName, name)
			if !ok {
				continue
			}
			val := ""
			if vm := reAttrValue.FindStringSubmatch(pattrs); len(vm) == 2 {
				val = vm[1]
			}
			if len(pm) >= 3 && strings.TrimSpace(pm[2]) != "" {
				inner := pm[2]
				if cd := reCDATA.FindStringSubmatch(inner); len(cd) == 2 {
					val = cd[1]
				} else if strings.TrimSpace(stripTags(inner)) != "" {
					val = stripTags(inner)
				}
			}
			props[field] = strings.TrimSpace(val)
		}
		if len(props) == 0 {
			continue
		}
		meta := ParameterLookupMeta{
			Enabled:      strings.EqualFold(props["enabled"], "true") || props["enabled"] == "1",
			Datasource:   strings.TrimSpace(props["datasource"]),
			Depends:      parseDependsCSV(props["depends"]),
			DisplayField: strings.TrimSpace(props["displayField"]),
			ValueField:   strings.TrimSpace(props["valueField"]),
			SQL:          strings.TrimSpace(props["sql"]),
		}
		if meta.Datasource == "" {
			meta.Datasource = LookupDatasourceCurrentReport
		}
		meta.Depends = mergeLookupDepends(name, meta.Depends, meta.SQL)
		if meta.HasLookup() {
			out[name] = meta
		}
	}
	return out
}

func stripTags(s string) string {
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")
}

// JavaClassToParamKind mirrors PHP jrs_java_class_to_param_kind.
func JavaClassToParamKind(class string) string {
	c := strings.ToLower(class)
	switch {
	case strings.Contains(c, "collection"), strings.Contains(c, "list"), strings.Contains(c, "set"):
		return "collection"
	case strings.Contains(c, "integer"), strings.Contains(c, "long"), strings.Contains(c, "short"), strings.Contains(c, "biginteger"):
		return "integer"
	case strings.Contains(c, "double"), strings.Contains(c, "float"), strings.Contains(c, "decimal"),
		strings.Contains(c, "bigdecimal"), strings.Contains(c, "number"):
		return "double"
	case strings.Contains(c, "bool"):
		return "boolean"
	case strings.Contains(c, "date"), strings.Contains(c, "time"):
		return "date"
	default:
		return "string"
	}
}
