package embeddedmap

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/beevik/etree"
)

const postGISMapRendererPrefix = "com.acugis.map.MapRenderer.render("

var reGroupFieldRef = regexp.MustCompile(`\$F\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// groupedSpatialMap reports whether the embedded map is a PostGIS/MySQL map
// associated with a Jasper group. Go pre-renders one PNG per group value;
// JRXML picks the file with $F{group} and evaluationTime=Group.
func groupedSpatialMap(emb Definition) bool {
	if normalizeBand(emb.Band) != "groupHeader" {
		return false
	}
	switch NormalizeRenderer(emb.Renderer) {
	case RendererPostGIS, RendererMySQL, "":
		return true
	default:
		return false
	}
}

func jrxmlUsesPostGISMapRenderer(jrxmlPath string) bool {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), postGISMapRendererPrefix)
}

func jrxmlUsesGroupedFileMap(jrxmlPath string) bool {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "new java.io.File($P{") &&
		strings.Contains(s, "String.valueOf($F{") &&
		strings.Contains(s, `evaluationTime="Group"`)
}

func usesPostGISMapRenderer(expr string) bool {
	return strings.Contains(expr, postGISMapRendererPrefix)
}

func hasPostGISMapRenderer(doc *etree.Document) bool {
	return findPostGISMapImage(doc) != nil
}

func findPostGISMapImage(doc *etree.Document) *etree.Element {
	for _, el := range mapImageElements(doc) {
		if usesPostGISMapRenderer(elementFullText(el)) {
			return el
		}
	}
	return nil
}

func mapImageElements(doc *etree.Document) []*etree.Element {
	var out []*etree.Element
	for _, el := range doc.FindElements("//element") {
		if el.SelectAttrValue("kind", "") != "image" {
			continue
		}
		out = append(out, el)
	}
	for _, el := range doc.FindElements("//image") {
		out = append(out, el)
	}
	return out
}

func fileMapExpression(paramName string) string {
	return fmt.Sprintf(
		`($P{%s} == null || $P{%s}.length() == 0) ? null : new java.io.File($P{%s})`,
		paramName, paramName, paramName,
	)
}

func groupedFileMapExpression(paramName, field string) string {
	return fmt.Sprintf(
		`($P{%s} == null || $P{%s}.length() == 0) ? null : new java.io.File($P{%s}, java.lang.String.valueOf($F{%s}) + ".png")`,
		paramName, paramName, paramName, field,
	)
}

func quoteJavaString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\r\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\n`)
	escaped = strings.ReplaceAll(escaped, "\t", `\t`)
	return `"` + escaped + `"`
}

func quotePGIdent(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return `""`
	}
	parts := strings.Split(name, ".")
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(p, `"`, `""`)+`"`)
	}
	if len(quoted) == 0 {
		return `""`
	}
	return strings.Join(quoted, ".")
}

func quoteMySQLIdent(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "``"
	}
	parts := strings.Split(name, ".")
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), "`")
		if p == "" {
			continue
		}
		quoted = append(quoted, "`"+strings.ReplaceAll(p, "`", "``")+"`")
	}
	if len(quoted) == 0 {
		return "``"
	}
	return strings.Join(quoted, ".")
}

func firstGroupName(root *etree.Element) string {
	if root == nil {
		return ""
	}
	for _, g := range findAllByLocalName(root, "group") {
		if name := strings.TrimSpace(g.SelectAttrValue("name", "")); name != "" {
			return name
		}
	}
	return ""
}

func groupValueField(root *etree.Element, groupName string) string {
	if root == nil {
		return ""
	}
	groupName = strings.TrimSpace(groupName)
	var fallback string
	for _, g := range findAllByLocalName(root, "group") {
		name := g.SelectAttrValue("name", "")
		expr := ""
		if e := findChild(g, "expression"); e != nil {
			expr = elementFullText(e)
		} else if e := findChild(g, "groupExpression"); e != nil {
			expr = elementFullText(e)
		}
		m := reGroupFieldRef.FindStringSubmatch(expr)
		if len(m) < 2 {
			continue
		}
		field := strings.TrimSpace(m[1])
		if field == "" {
			continue
		}
		if groupName == "" || name == groupName {
			return field
		}
		if fallback == "" {
			fallback = field
		}
	}
	if isSafeFieldKey(groupName) {
		return groupName
	}
	return fallback
}

func groupFieldFromJRXML(jrxmlPath, groupName string) string {
	groupName = strings.TrimSpace(groupName)
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		return groupName
	}
	_, root, err := parseJRXML(data)
	if err != nil || root == nil {
		return groupName
	}
	if field := groupValueField(root, groupName); field != "" {
		return field
	}
	return groupName
}

func applyGroupEvaluation(el *etree.Element, groupName string) {
	if el == nil || strings.TrimSpace(groupName) == "" {
		return
	}
	el.CreateAttr("evaluationTime", "Group")
	el.CreateAttr("evaluationGroup", groupName)
}

func enclosingGroupName(el *etree.Element) string {
	for p := el.Parent(); p != nil; p = p.Parent() {
		if localName(p) == "group" {
			return p.SelectAttrValue("name", "")
		}
	}
	return ""
}

func inGroupBand(el *etree.Element) bool {
	for p := el.Parent(); p != nil; p = p.Parent() {
		switch localName(p) {
		case "groupHeader", "groupFooter":
			return true
		}
	}
	return false
}

func setImageExpression(el *etree.Element, java string) {
	expr := findChild(el, "expression")
	if expr == nil {
		expr = el.CreateElement("expression")
	}
	expr.Child = nil
	expr.AddChild(etree.NewCData(java))
}

// ensurePostGISGroupFilter restores MapSql.eq($F{groupField}) + Group evaluation
// on Designer fill-time MapRenderer images sitting in a group band.
func ensurePostGISGroupFilter(doc *etree.Document, root *etree.Element) bool {
	changed := false
	for _, img := range mapImageElements(doc) {
		expr := strings.TrimSpace(elementFullText(img))
		if !usesPostGISMapRenderer(expr) {
			continue
		}
		if !inGroupBand(img) {
			continue
		}
		groupName := enclosingGroupName(img)
		field := groupValueField(root, groupName)
		if field == "" || !isSafeFieldKey(field) {
			continue
		}
		if img.SelectAttrValue("evaluationTime", "") != "Group" ||
			img.SelectAttrValue("evaluationGroup", "") != groupName {
			applyGroupEvaluation(img, groupName)
			changed = true
		}
		if strings.Contains(expr, "MapSql.eq") && strings.Contains(expr, "$F{"+field+"}") {
			continue
		}
		connector := " WHERE "
		if strings.Contains(strings.ToUpper(expr), "WHERE") || strings.Contains(expr, "MapSql.in") {
			connector = " AND "
		}
		col := quotePGIdent(field)
		if strings.Contains(expr, "`") {
			col = quoteMySQLIdent(field)
		}
		wrapped := wrapMapRendererSQL(expr, connector, col, field)
		if wrapped != expr {
			setImageExpression(img, wrapped)
			changed = true
		}
	}
	return changed
}

func wrapMapRendererSQL(expr, connector, col, field string) string {
	idx := strings.Index(expr, postGISMapRendererPrefix)
	if idx < 0 {
		return expr
	}
	start := idx + len(postGISMapRendererPrefix)
	args, consumed := splitJavaCallArgs(expr[start:])
	if len(args) < 2 || consumed == 0 {
		return expr
	}
	sqlArg := strings.TrimSpace(args[1])
	args[1] = fmt.Sprintf("(%s) + %s + com.acugis.map.MapSql.eq(%s, $F{%s})",
		sqlArg, quoteJavaString(connector), quoteJavaString(col), field)
	return expr[:start] + strings.Join(args, ", ") + expr[start+consumed:]
}

func splitJavaCallArgs(s string) (args []string, consumed int) {
	depth := 1
	inString := false
	escape := false
	argStart := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[argStart:i]))
				return args, i + 1
			}
		case ',':
			if depth == 1 {
				args = append(args, strings.TrimSpace(s[argStart:i]))
				argStart = i + 1
			}
		}
	}
	return nil, 0
}

func groupMapExpressionCurrent(img *etree.Element, root *etree.Element, groupName string, width, height int) bool {
	if img == nil {
		return false
	}
	text := elementFullText(img)
	field := groupValueField(root, groupName)
	if field == "" || !strings.Contains(text, "$F{"+field+"}") {
		return false
	}
	if img.SelectAttrValue("evaluationTime", "") != "Group" {
		return false
	}
	if strings.TrimSpace(groupName) != "" && img.SelectAttrValue("evaluationGroup", "") != groupName {
		return false
	}
	if usesPostGISMapRenderer(text) {
		if !strings.Contains(text, "MapSql.eq") {
			return false
		}
	} else if !strings.Contains(text, "new java.io.File($P{") || !strings.Contains(text, "String.valueOf($F{"+field+"})") {
		return false
	}
	return attrInt(img, "width") == width && attrInt(img, "height") == height
}

func featureGroupKey(f map[string]any, field string) string {
	props := featureProps(f)
	if props == nil {
		return "null"
	}
	v, ok := lookupFeatureProp(props, field)
	if !ok || v == nil {
		return "null"
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return "null"
	}
	return s
}

func lookupFeatureProp(props map[string]any, field string) (any, bool) {
	field = strings.TrimSpace(field)
	if field == "" {
		return nil, false
	}
	if v, ok := props[field]; ok {
		return v, true
	}
	for k, v := range props {
		if strings.EqualFold(k, field) {
			return v, true
		}
		if i := strings.LastIndex(k, "."); i >= 0 && strings.EqualFold(k[i+1:], field) {
			return v, true
		}
	}
	if i := strings.LastIndex(field, "."); i >= 0 {
		return lookupFeatureProp(props, field[i+1:])
	}
	return nil, false
}

func splitFeaturesByGroup(features []map[string]any, field string) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for _, f := range features {
		key := featureGroupKey(f, field)
		out[key] = append(out[key], f)
	}
	return out
}
