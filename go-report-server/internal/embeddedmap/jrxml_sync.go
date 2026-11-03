package embeddedmap

import (
	"bytes"
	"os"
	"regexp"
	"strings"

	"github.com/beevik/etree"
)

// SyncResult is the outcome of JRXML embedded-map sync.
type SyncResult struct {
	OK           bool
	Message      string
	JRXMLChanged bool
}

var reGroupNames = regexp.MustCompile(`(?is)<group\b[^>]*\bname\s*=\s*"([^"]+)"`)

// SyncJRXML injects or removes the embedded map image + parameter in a JRXML file.
// Uses XML DOM (PHP parity): remove → ensure band → insert → refit/shrink all layout bands → fit target.
func SyncJRXML(jrxmlPath string, emb Definition) SyncResult {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		return SyncResult{OK: false, Message: "JRXML file not found."}
	}
	original := string(data)
	paramName := emb.ParameterName
	if paramName == "" {
		paramName = ParamName
	}

	doc, root, err := parseJRXML(data)
	if err != nil {
		return SyncResult{OK: false, Message: "Failed to parse JRXML."}
	}

	padding := BandPadding
	hadMap := domHasEmbeddedMap(doc, paramName)
	origInvalid := hasInvalidSectionHeight(root)

	if !emb.Enabled && !hadMap {
		if !hasPostGISMapRenderer(doc) {
			return SyncResult{OK: true, Message: "No embedded map configured.", JRXMLChanged: false}
		}
		// Designer-published maps: restore the group filter without injecting P_MAP_IMAGE.
		healSectionContainers(root)
		if !ensurePostGISGroupFilter(doc, root) && !origInvalid {
			return SyncResult{OK: true, Message: "No embedded map configured.", JRXMLChanged: false}
		}
		return writeSyncResult(jrxmlPath, doc, original,
			"Restored PostGIS group filter on embedded map.",
			"No embedded map configured.")
	}

	band := normalizeBand(emb.Band)
	width, height, x, y := emb.Width, emb.Height, emb.X, emb.Y

	healSectionContainers(root)

	// Designer-published MapRenderer images must keep fill-time rendering.
	// Replacing them with a static P_MAP_IMAGE is the unfiltered-USA regression.
	if emb.Enabled && hasPostGISMapRenderer(doc) {
		filterChanged := ensurePostGISGroupFilter(doc, root)
		needGrow := false
		if img := findPostGISMapImage(doc); img != nil {
			if parent := imageParentBand(img); parent != nil && attrInt(parent, "height") < y+height+padding {
				needGrow = true
			}
		}
		needShrink := layoutBandsNeedShrink(doc, root, padding)
		if !filterChanged && !needGrow && !needShrink && !origInvalid {
			return SyncResult{
				OK: true, Message: "Embedded map already present in JRXML (" + band + ").", JRXMLChanged: false,
			}
		}
		refitLayoutBands(doc, root, padding)
		if img := findPostGISMapImage(doc); img != nil {
			if parent := imageParentBand(img); parent != nil {
				bandFitContent(parent, padding)
			}
		}
		msg := "Healed embedded map band height in JRXML."
		if filterChanged {
			msg = "Restored PostGIS group filter on embedded map."
		}
		return writeSyncResult(jrxmlPath, doc, original, msg,
			"Embedded map already present in JRXML ("+band+").")
	}

	layoutOK := emb.Enabled && embedLayoutCurrentDOM(doc, root, emb, paramName, band, emb.GroupName, x, y, width, height)
	needGrow := false
	if layoutOK {
		if img := findMapImage(doc); img != nil {
			if parent := imageParentBand(img); parent != nil && attrInt(parent, "height") < y+height+padding {
				needGrow = true
			}
		}
	}
	needShrink := layoutBandsNeedShrink(doc, root, padding)

	if layoutOK && !needGrow && !needShrink && !origInvalid {
		return SyncResult{
			OK: true, Message: "Embedded map already present in JRXML (" + band + ").", JRXMLChanged: false,
		}
	}

	// Heal-only path: map already in the right place; just fit/refit heights or section markup.
	if layoutOK {
		refitLayoutBands(doc, root, padding)
		if img := findMapImage(doc); img != nil {
			if parent := imageParentBand(img); parent != nil {
				bandFitContent(parent, padding)
			}
		}
		return writeSyncResult(jrxmlPath, doc, original,
			"Healed embedded map band height in JRXML.",
			"Embedded map already present in JRXML ("+band+").")
	}

	touched := domRemoveEmbeddedMap(doc, root, paramName)
	for _, b := range touched {
		bandFitContent(b, padding)
	}

	if !emb.Enabled {
		refitLayoutBands(doc, root, padding)
		return writeSyncResult(jrxmlPath, doc, original,
			"Embedded map removed from JRXML.",
			"Embedded map already absent from JRXML.")
	}

	target := domEnsureBand(doc, root, band, emb.GroupName, 0)
	if target == nil {
		msg := "Could not locate or create target band: " + band
		if band == "groupHeader" {
			msg = "Could not find a group header band for the embedded map. Add a group in the JRXML or choose Summary/Detail."
		}
		return SyncResult{OK: false, Message: msg}
	}

	ensureParameter(doc, root, ReportDirParam)
	ensureParameter(doc, root, paramName)

	target.AddChild(createMapImageElement(root, emb, paramName, x, y, width, height))
	refitLayoutBands(doc, root, padding)
	bandFitContent(target, padding)

	return writeSyncResult(jrxmlPath, doc, original,
		"Embedded map image inserted into JRXML ("+band+").",
		"Embedded map already present in JRXML ("+band+").")
}

func writeSyncResult(jrxmlPath string, doc *etree.Document, original, changedMsg, sameMsg string) SyncResult {
	updated, err := writeJRXML(doc)
	if err != nil {
		return SyncResult{OK: false, Message: "Failed to serialize JRXML."}
	}
	if updated == original {
		return SyncResult{OK: true, Message: sameMsg, JRXMLChanged: false}
	}
	if err := os.WriteFile(jrxmlPath, []byte(updated), 0o644); err != nil {
		return SyncResult{OK: false, Message: "Failed to write JRXML."}
	}
	_ = os.Chmod(jrxmlPath, 0o644)
	return SyncResult{OK: true, Message: changedMsg, JRXMLChanged: true}
}

// GroupNames returns JRXML group names for the Group Header dropdown.
func GroupNames(jrxmlPath string) []string {
	data, err := os.ReadFile(jrxmlPath)
	if err != nil {
		return nil
	}
	matches := reGroupNames.FindAllStringSubmatch(string(data), -1)
	var names []string
	seen := map[string]bool{}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		n := strings.TrimSpace(m[1])
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	return names
}

// HasMarkers reports whether JRXML already contains embed param/image.
func HasMarkers(jrxmlText, paramName string) (hasParam, hasImage bool) {
	if paramName == "" {
		paramName = ParamName
	}
	hasParam = strings.Contains(jrxmlText, `name="`+paramName+`"`)
	hasImage = strings.Contains(jrxmlText, ElementUUID) || strings.Contains(jrxmlText, "$P{"+paramName+"}")
	return
}

// imageInTargetBand reports whether the embedded-map element sits in the requested band.
func imageInTargetBand(xml, band, groupName string) bool {
	doc, _, err := parseJRXML([]byte(xml))
	if err != nil {
		return false
	}
	return imageInBandDOM(doc, band, groupName)
}

// enclosingBandHeight returns the height of the band/summary that contains the map image.
func enclosingBandHeight(xml string) int {
	doc, _, err := parseJRXML([]byte(xml))
	if err != nil {
		return 0
	}
	return enclosingBandHeightDOM(doc)
}

// nestSectionHeightIntoBand rewrites invalid <section height="N">…</section>
// into <section><band height="N">…</band></section> for JRDesignSection containers.
func nestSectionHeightIntoBand(xml, section string) string {
	wrapped := wrapFragment(xml)
	doc, root, err := parseJRXML([]byte(wrapped))
	if err != nil {
		return xml
	}
	for _, sec := range findAllByLocalName(root, section) {
		nestSectionHeight(sec)
	}
	if strings.Contains(xml, "<jasperReport") {
		out, err := writeJRXML(doc)
		if err != nil {
			return xml
		}
		return out
	}
	return extractChildrenXML(root)
}

func wrapFragment(xml string) string {
	trim := strings.TrimSpace(xml)
	if strings.Contains(trim, "<jasperReport") {
		return xml
	}
	return `<?xml version="1.0" encoding="UTF-8"?><jasperReport name="frag">` + xml + `</jasperReport>`
}

func extractChildrenXML(root *etree.Element) string {
	wrap := etree.NewDocument()
	wrap.SetRoot(etree.NewElement("wrap"))
	for _, c := range childElements(root) {
		wrap.Root().AddChild(c.Copy())
	}
	wrap.Indent(0)
	s, err := wrap.WriteToString()
	if err != nil {
		return ""
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<wrap>")
	s = strings.TrimSuffix(strings.TrimSpace(s), "</wrap>")
	return strings.TrimSpace(s)
}

// CompileOK reports whether CompileJRXML note indicates success.
func CompileOK(note string) bool {
	return strings.Contains(note, "compiled to .jasper successfully")
}

// PrettyXML is unused helper kept for tests.
func PrettyXML(s string) string {
	return string(bytes.TrimSpace([]byte(s)))
}
