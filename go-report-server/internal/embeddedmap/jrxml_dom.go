package embeddedmap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// Studio-7 sections that carry height on the section itself (no nested <band>).
var studioBandSections = map[string]bool{
	"summary": true, "title": true, "pageHeader": true, "pageFooter": true,
	"columnHeader": true, "columnFooter": true, "lastPageFooter": true,
	"background": true, "noData": true,
}

func parseJRXML(data []byte) (*etree.Document, *etree.Element, error) {
	doc := etree.NewDocument()
	doc.ReadSettings.CharsetReader = nil
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, nil, err
	}
	root := doc.Root()
	if root == nil {
		return nil, nil, fmt.Errorf("invalid JRXML root")
	}
	return doc, root, nil
}

func writeJRXML(doc *etree.Document) (string, error) {
	doc.Indent(2)
	return doc.WriteToString()
}

func localName(el *etree.Element) string {
	if el == nil {
		return ""
	}
	name := el.Tag
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func attrInt(el *etree.Element, name string) int {
	v := el.SelectAttrValue(name, "")
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func setAttrInt(el *etree.Element, name string, n int) {
	el.CreateAttr(name, strconv.Itoa(n))
}

func childElements(el *etree.Element) []*etree.Element {
	out := make([]*etree.Element, 0, len(el.Child))
	for _, c := range el.Child {
		if e, ok := c.(*etree.Element); ok {
			out = append(out, e)
		}
	}
	return out
}

func findChild(el *etree.Element, name string) *etree.Element {
	for _, c := range childElements(el) {
		if localName(c) == name {
			return c
		}
	}
	return nil
}

func findRootChild(root *etree.Element, name string) *etree.Element {
	return findChild(root, name)
}

func isBandContainer(el *etree.Element) bool {
	n := localName(el)
	if n == "band" {
		return true
	}
	return studioBandSections[n]
}

// elementBottom is y+height for Studio-7 <element> or classic <image>/<reportElement>.
func elementBottom(el *etree.Element) int {
	if el.SelectAttr("y") != nil || el.SelectAttr("height") != nil {
		return max(0, attrInt(el, "y")) + max(0, attrInt(el, "height"))
	}
	if re := findChild(el, "reportElement"); re != nil {
		return max(0, attrInt(re, "y")) + max(0, attrInt(re, "height"))
	}
	return 0
}

func bandRequiredHeight(band *etree.Element, padding int) int {
	maxBottom := 0
	for _, c := range childElements(band) {
		switch localName(c) {
		case "printWhenExpression", "property", "propertyExpression", "break":
			continue
		}
		if b := elementBottom(c); b > maxBottom {
			maxBottom = b
		}
	}
	if maxBottom <= 0 {
		return 0
	}
	if padding < 0 {
		padding = 0
	}
	return maxBottom + padding
}

// bandFitContent sets height exactly to content (grow or shrink).
func bandFitContent(band *etree.Element, padding int) {
	setAttrInt(band, "height", bandRequiredHeight(band, padding))
}

// refitLayoutBands shrinks oversized bands to content; does not grow tight bands.
func refitLayoutBands(doc *etree.Document, root *etree.Element, padding int) {
	seen := map[*etree.Element]bool{}
	fitShrink := func(el *etree.Element) {
		if el == nil || seen[el] {
			return
		}
		seen[el] = true
		required := bandRequiredHeight(el, padding)
		current := attrInt(el, "height")
		if current > required {
			setAttrInt(el, "height", required)
		}
	}

	for _, band := range doc.FindElements("//band") {
		fitShrink(band)
	}
	for _, child := range childElements(root) {
		if !isBandContainer(child) {
			continue
		}
		hasNested := false
		for _, inner := range childElements(child) {
			if localName(inner) == "band" {
				hasNested = true
				break
			}
		}
		if !hasNested {
			fitShrink(child)
		}
	}
}

func layoutBandsNeedShrink(doc *etree.Document, root *etree.Element, padding int) bool {
	check := func(el *etree.Element) bool {
		if el == nil {
			return false
		}
		required := bandRequiredHeight(el, padding)
		return attrInt(el, "height") > required
	}
	for _, band := range doc.FindElements("//band") {
		if check(band) {
			return true
		}
	}
	for _, child := range childElements(root) {
		if !isBandContainer(child) {
			continue
		}
		hasNested := false
		for _, inner := range childElements(child) {
			if localName(inner) == "band" {
				hasNested = true
				break
			}
		}
		if !hasNested && check(child) {
			return true
		}
	}
	return false
}

func domHasEmbeddedMap(doc *etree.Document, paramName string) bool {
	for _, p := range doc.FindElements("//parameter") {
		if p.SelectAttrValue("name", "") == paramName {
			return true
		}
	}
	needle := "$P{" + paramName + "}"
	for _, el := range doc.FindElements("//element") {
		if el.SelectAttrValue("uuid", "") == ElementUUID {
			return true
		}
		if el.SelectAttrValue("kind", "") == "image" && strings.Contains(elementFullText(el), needle) {
			return true
		}
	}
	for _, el := range doc.FindElements("//image") {
		if strings.Contains(elementFullText(el), needle) {
			return true
		}
	}
	return false
}

// domRemoveEmbeddedMap removes map param/images; returns parent band containers that lost children.
func domRemoveEmbeddedMap(doc *etree.Document, root *etree.Element, paramName string) []*etree.Element {
	needle := "$P{" + paramName + "}"
	var toRemove []*etree.Element

	for _, p := range doc.FindElements("//parameter") {
		if p.SelectAttrValue("name", "") == paramName {
			toRemove = append(toRemove, p)
		}
	}
	for _, el := range doc.FindElements("//element") {
		if el.SelectAttrValue("uuid", "") == ElementUUID ||
			(el.SelectAttrValue("kind", "") == "image" && strings.Contains(elementFullText(el), needle)) {
			toRemove = append(toRemove, el)
		}
	}
	for _, el := range doc.FindElements("//image") {
		if strings.Contains(elementFullText(el), needle) {
			toRemove = append(toRemove, el)
		}
	}

	var touched []*etree.Element
	seen := map[*etree.Element]bool{}
	for _, node := range toRemove {
		parent := node.Parent()
		if parent != nil && isBandContainer(parent) && !seen[parent] {
			seen[parent] = true
			touched = append(touched, parent)
		}
		if parent != nil {
			parent.RemoveChild(node)
		}
	}
	return touched
}

func hasNestedBand(el *etree.Element) bool {
	return findChild(el, "band") != nil
}

// healSectionContainers nests invalid height on detail/groupHeader into <band>,
// and flattens invalid nested <band> under Studio summary-style sections.
func healSectionContainers(root *etree.Element) {
	// Flatten <summary><band>…</band></summary> → Studio flat summary.
	for _, secName := range []string{"summary"} {
		for _, sec := range findAllByLocalName(root, secName) {
			flattenStudioSection(sec)
		}
	}
	// Nest height from detail / groupHeader into <band>.
	for _, secName := range []string{"detail", "groupHeader"} {
		for _, sec := range findAllByLocalName(root, secName) {
			nestSectionHeight(sec)
		}
	}
}

func findAllByLocalName(root *etree.Element, name string) []*etree.Element {
	var out []*etree.Element
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if localName(el) == name {
			out = append(out, el)
		}
		for _, c := range childElements(el) {
			walk(c)
		}
	}
	walk(root)
	return out
}

func flattenStudioSection(sec *etree.Element) {
	band := findChild(sec, "band")
	if band == nil {
		return
	}
	// Move height from nested band onto section if missing.
	if sec.SelectAttr("height") == nil && band.SelectAttr("height") != nil {
		setAttrInt(sec, "height", attrInt(band, "height"))
	}
	// Move children of band onto section, then remove band.
	for _, c := range append([]*etree.Element{}, childElements(band)...) {
		band.RemoveChild(c)
		sec.AddChild(c)
	}
	sec.RemoveChild(band)
}

func nestSectionHeight(sec *etree.Element) {
	hAttr := sec.SelectAttr("height")
	band := findChild(sec, "band")
	if band != nil {
		if hAttr != nil {
			sec.RemoveAttr("height")
		}
		return
	}
	h := 0
	if hAttr != nil {
		h = attrInt(sec, "height")
		sec.RemoveAttr("height")
	}
	band = sec.CreateElement("band")
	setAttrInt(band, "height", h)
	// Move existing children into the new band (except we just created band as last child).
	for _, c := range append([]*etree.Element{}, childElements(sec)...) {
		if c == band {
			continue
		}
		sec.RemoveChild(c)
		band.AddChild(c)
	}
}

// domEnsureBand returns the insert target: nested <band> for detail/groupHeader,
// or the Studio section itself for summary (never nest <band> under summary).
func domEnsureBand(doc *etree.Document, root *etree.Element, band, groupName string, initialHeight int) *etree.Element {
	if initialHeight < 0 {
		initialHeight = 0
	}
	switch band {
	case "groupHeader":
		var groupEl *etree.Element
		for _, g := range childElements(root) {
			if localName(g) != "group" {
				continue
			}
			if groupName == "" || g.SelectAttrValue("name", "") == groupName {
				groupEl = g
				break
			}
		}
		if groupEl == nil {
			return nil
		}
		gh := findChild(groupEl, "groupHeader")
		if gh == nil {
			gh = groupEl.CreateElement("groupHeader")
		}
		nestSectionHeight(gh) // strip any height onto band
		bandEl := findChild(gh, "band")
		if bandEl == nil {
			bandEl = gh.CreateElement("band")
			setAttrInt(bandEl, "height", initialHeight)
		}
		return bandEl

	case "detail":
		detail := findRootChild(root, "detail")
		if detail == nil {
			detail = root.CreateElement("detail")
		}
		nestSectionHeight(detail)
		bandEl := findChild(detail, "band")
		if bandEl == nil {
			bandEl = detail.CreateElement("band")
			setAttrInt(bandEl, "height", initialHeight)
		}
		return bandEl

	default: // summary
		summary := findRootChild(root, "summary")
		if summary == nil {
			summary = root.CreateElement("summary")
			setAttrInt(summary, "height", initialHeight)
			return summary
		}
		flattenStudioSection(summary)
		if summary.SelectAttr("height") == nil {
			setAttrInt(summary, "height", initialHeight)
		}
		return summary
	}
}

func ensureParameter(doc *etree.Document, root *etree.Element, name string) {
	for _, p := range doc.FindElements("//parameter") {
		if p.SelectAttrValue("name", "") == name {
			return
		}
	}
	param := etree.NewElement("parameter")
	param.CreateAttr("name", name)
	param.CreateAttr("class", "java.lang.String")
	param.CreateAttr("forPrompting", "false")
	def := param.CreateElement("defaultValueExpression")
	def.AddChild(etree.NewCData(`""`))

	insertBefore := firstLayoutSibling(root)
	if insertBefore != nil {
		root.InsertChildAt(insertBefore.Index(), param)
	} else {
		root.AddChild(param)
	}
}

func firstLayoutSibling(root *etree.Element) *etree.Element {
	markers := map[string]bool{
		"query": true, "queryString": true, "field": true, "variable": true, "group": true,
		"background": true, "title": true, "pageHeader": true, "columnHeader": true,
		"detail": true, "columnFooter": true, "pageFooter": true, "summary": true, "lastPageFooter": true,
	}
	for _, c := range childElements(root) {
		if markers[localName(c)] {
			return c
		}
	}
	return nil
}

func createMapImageElement(root *etree.Element, emb Definition, paramName string, x, y, width, height int) *etree.Element {
	el := etree.NewElement("element")
	el.CreateAttr("kind", "image")
	el.CreateAttr("uuid", ElementUUID)
	el.CreateAttr("x", strconv.Itoa(x))
	el.CreateAttr("y", strconv.Itoa(y))
	el.CreateAttr("width", strconv.Itoa(width))
	el.CreateAttr("height", strconv.Itoa(height))
	el.CreateAttr("onErrorType", "Blank")
	expr := el.CreateElement("expression")
	if groupedSpatialMap(emb) {
		groupName := strings.TrimSpace(emb.GroupName)
		if groupName == "" {
			groupName = firstGroupName(root)
		}
		field := groupValueField(root, groupName)
		if field == "" {
			field = groupName
		}
		if isSafeFieldKey(field) && groupName != "" {
			applyGroupEvaluation(el, groupName)
			expr.AddChild(etree.NewCData(groupedFileMapExpression(paramName, field)))
			return el
		}
	}
	expr.AddChild(etree.NewCData(fileMapExpression(paramName)))
	return el
}

func findMapImage(doc *etree.Document) *etree.Element {
	for _, el := range doc.FindElements("//element") {
		if el.SelectAttrValue("uuid", "") == ElementUUID {
			return el
		}
	}
	return nil
}

func imageParentBand(el *etree.Element) *etree.Element {
	for p := el.Parent(); p != nil; p = p.Parent() {
		n := localName(p)
		if n == "band" || studioBandSections[n] {
			return p
		}
	}
	return nil
}

func imageInBandDOM(doc *etree.Document, band, groupName string) bool {
	img := findMapImage(doc)
	if img == nil {
		return false
	}
	band = normalizeBand(band)
	switch band {
	case "detail":
		for p := img.Parent(); p != nil; p = p.Parent() {
			if localName(p) == "detail" {
				return true
			}
			if localName(p) == "summary" || localName(p) == "groupHeader" {
				return false
			}
		}
		return false
	case "groupHeader":
		inGH := false
		var groupEl *etree.Element
		for p := img.Parent(); p != nil; p = p.Parent() {
			if localName(p) == "groupHeader" {
				inGH = true
			}
			if localName(p) == "group" {
				groupEl = p
			}
		}
		if !inGH {
			return false
		}
		if strings.TrimSpace(groupName) == "" || groupEl == nil {
			return true
		}
		return groupEl.SelectAttrValue("name", "") == groupName
	default:
		for p := img.Parent(); p != nil; p = p.Parent() {
			if localName(p) == "summary" {
				return true
			}
		}
		return false
	}
}

func enclosingBandHeightDOM(doc *etree.Document) int {
	img := findMapImage(doc)
	if img == nil {
		return 0
	}
	b := imageParentBand(img)
	if b == nil {
		return 0
	}
	return attrInt(b, "height")
}

func hasInvalidSectionHeight(root *etree.Element) bool {
	for _, name := range []string{"detail", "groupHeader"} {
		for _, sec := range findAllByLocalName(root, name) {
			if sec.SelectAttr("height") != nil {
				return true
			}
		}
	}
	// Nested band under summary is invalid for JR 7.
	if sum := findRootChild(root, "summary"); sum != nil && hasNestedBand(sum) {
		return true
	}
	return false
}

func hasParameterDOM(doc *etree.Document, name string) bool {
	for _, p := range doc.FindElements("//parameter") {
		if p.SelectAttrValue("name", "") == name {
			return true
		}
	}
	return false
}

func elementFullText(el *etree.Element) string {
	if el == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(e *etree.Element) {
		for _, c := range e.Child {
			switch n := c.(type) {
			case *etree.CharData:
				b.WriteString(n.Data)
			case *etree.Element:
				walk(n)
			}
		}
	}
	walk(el)
	return b.String()
}

func embedLayoutCurrentDOM(doc *etree.Document, root *etree.Element, emb Definition, paramName, band, groupName string, x, y, width, height int) bool {
	if hasInvalidSectionHeight(root) {
		return false
	}
	img := findMapImage(doc)
	if img == nil && groupedSpatialMap(emb) {
		img = findPostGISMapImage(doc)
	}
	if img == nil {
		return false
	}
	if attrInt(img, "x") != x || attrInt(img, "y") != y ||
		attrInt(img, "width") != width || attrInt(img, "height") != height {
		return false
	}
	if groupedSpatialMap(emb) {
		if !hasParameterDOM(doc, paramName) {
			return false
		}
		if !groupMapExpressionCurrent(img, root, groupName, width, height) {
			return false
		}
	} else {
		if !hasParameterDOM(doc, ReportDirParam) || !hasParameterDOM(doc, paramName) {
			return false
		}
		fileExpr := fmt.Sprintf("new java.io.File($P{%s})", paramName)
		if !strings.Contains(elementFullText(img), fileExpr) {
			return false
		}
	}
	if enclosingBandHeightDOM(doc) < y+height {
		return false
	}
	return imageInBandDOM(doc, band, groupName)
}
