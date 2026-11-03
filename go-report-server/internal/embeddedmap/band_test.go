package embeddedmap

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNormalizeBandPreservesPlacement(t *testing.T) {
	cases := map[string]string{
		"summary":      "summary",
		"detail":       "detail",
		"groupHeader":  "groupHeader",
		"groupheader":  "groupHeader",
		"group_header": "groupHeader",
		"Group Header": "groupHeader",
		"":             "summary",
		"bogus":        "summary",
	}
	for in, want := range cases {
		d := Normalize(map[string]any{"band": in})
		if d.Band != want {
			t.Errorf("Normalize band %q => %q, want %q", in, d.Band, want)
		}
	}
}

func TestFromFormBand(t *testing.T) {
	for _, band := range []string{"detail", "groupHeader"} {
		form := url.Values{}
		form.Set("embedded_map[enabled]", "1")
		form.Set("embedded_map[band]", band)
		d := FromForm(form)
		if d.Band != band {
			t.Fatalf("FromForm band %q => %q", band, d.Band)
		}
	}
}

func TestSyncJRXMLGroupHeaderFlatStudio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "group.jrxml")
	// Invalid flat height on groupHeader (JRDesignSection) — sync must nest into <band>.
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="group">
<group name="City">
<groupHeader height="20"></groupHeader>
</group>
<summary height="40"></summary>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "groupHeader"
	emb.GroupName = "City"
	emb.Width = 200
	emb.Height = 80
	res := SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("groupHeader sync: %+v", res)
	}
	text := string(mustRead(t, path))
	if regexp.MustCompile(`(?is)<groupHeader[^>]*\bheight\s*=`).MatchString(text) {
		t.Fatalf("groupHeader must not carry height attr:\n%s", text)
	}
	if !regexp.MustCompile(`(?is)<groupHeader[^>]*>\s*<band\b[^>]*height=`).MatchString(text) {
		t.Fatalf("expected nested band under groupHeader:\n%s", text)
	}
	if !imageInTargetBand(text, "groupHeader", "City") {
		t.Fatalf("expected map in groupHeader:\n%s", text)
	}
	assertGroupedFileMapExpression(t, text, "City")
}

func TestNestSectionHeightIntoBand(t *testing.T) {
	in := `<group name="G"><groupHeader height="25"><element kind="staticText"/></groupHeader></group>`
	out := nestSectionHeightIntoBand(in, "groupHeader")
	if regexp.MustCompile(`(?is)<groupHeader[^>]*\bheight\s*=`).MatchString(out) {
		t.Fatalf("height still on groupHeader:\n%s", out)
	}
	if !strings.Contains(out, `<band height="25">`) {
		t.Fatalf("expected nested band:\n%s", out)
	}
}

func TestSyncJRXMLMovesBand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="demo">
<detail>
<band height="40"></band>
</detail>
<summary height="40"></summary>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	emb := Default()
	emb.Enabled = true
	emb.Band = "summary"
	emb.Width = 200
	emb.Height = 100
	res := SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("insert summary: %+v", res)
	}
	if !imageInTargetBand(string(mustRead(t, path)), "summary", "") {
		t.Fatalf("expected map in summary")
	}

	emb.Band = "detail"
	res = SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("move to detail should rewrite JRXML: %+v", res)
	}
	text := string(mustRead(t, path))
	if !imageInTargetBand(text, "detail", "") {
		t.Fatalf("expected map in detail band:\n%s", text)
	}
	if regexp.MustCompile(`(?is)<detail[^>]*\bheight\s*=`).MatchString(text) {
		t.Fatalf("detail must not carry height attr:\n%s", text)
	}
	// Default y=10 height=100 → need band >= 110 (+ padding in sync).
	if !regexp.MustCompile(`(?is)<detail[^>]*>\s*<band\b[^>]*height="(1[1-9]\d|[2-9]\d{2,})"`).MatchString(text) {
		t.Fatalf("detail band height not bumped for map:\n%s", text)
	}
}

func TestSyncJRXMLBumpsShortDetailBand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short-detail.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="short">
<detail>
<band height="34"></band>
</detail>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "detail"
	emb.Y = 10
	emb.Height = 300
	emb.Width = 400
	res := SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("detail sync: %+v", res)
	}
	text := string(mustRead(t, path))
	need := emb.Y + emb.Height // Jasper: element bottom must fit
	h := enclosingBandHeight(text)
	if h < need {
		t.Fatalf("band height %d < need %d:\n%s", h, need, text)
	}
}

func TestSyncJRXMLBumpsShortGroupHeaderBand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short-group.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="short">
<group name="City">
<groupHeader>
<band height="30"></band>
</groupHeader>
</group>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "groupHeader"
	emb.GroupName = "City"
	emb.Y = 10
	emb.Height = 300
	emb.Width = 400
	res := SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("groupHeader sync: %+v", res)
	}
	text := string(mustRead(t, path))
	need := emb.Y + emb.Height
	h := enclosingBandHeight(text)
	if h < need {
		t.Fatalf("band height %d < need %d:\n%s", h, need, text)
	}
	if regexp.MustCompile(`(?is)<groupHeader[^>]*\bheight\s*=`).MatchString(text) {
		t.Fatalf("groupHeader must not carry height:\n%s", text)
	}
	assertGroupedFileMapExpression(t, text, "City")
}

func TestSyncJRXMLFitsExistingShortBandWithoutReinsert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stuck.jrxml")
	// Map already present but band too short — save must grow band (no Jasper overflow).
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="stuck">
<parameter name="REPORT_DIR" class="java.lang.String" forPrompting="false">
<defaultValueExpression><![CDATA[""]]></defaultValueExpression>
</parameter>
<parameter name="P_MAP_IMAGE" class="java.lang.String" forPrompting="false">
<defaultValueExpression><![CDATA[""]]></defaultValueExpression>
</parameter>
<group name="City">
<groupHeader>
<band height="30">
<element kind="image" uuid="` + ElementUUID + `" x="0" y="10" width="400" height="300" onErrorType="Blank">
<expression><![CDATA[($P{P_MAP_IMAGE} == null || $P{P_MAP_IMAGE}.length() == 0) ? null : new java.io.File($P{P_MAP_IMAGE})]]></expression>
</element>
</band>
</groupHeader>
</group>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "groupHeader"
	emb.GroupName = "City"
	emb.X = 0
	emb.Y = 10
	emb.Width = 400
	emb.Height = 300
	res := SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("expected band-height heal: %+v", res)
	}
	text := string(mustRead(t, path))
	need := emb.Y + emb.Height
	if h := enclosingBandHeight(text); h < need {
		t.Fatalf("band height %d < need %d:\n%s", h, need, text)
	}
	assertGroupedFileMapExpression(t, text, "City")
}

func TestSyncJRXMLGroupHeaderNoSilentSummaryFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nogroup.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="nogroup">
<summary height="40"></summary>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "groupHeader"
	res := SyncJRXML(path, emb)
	if res.OK {
		t.Fatalf("expected failure when no groupHeader exists, got %+v", res)
	}
}

func TestSyncJRXMLDetailToSummaryShrinksDetail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "move-shrink.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="move-shrink">
<detail>
<band height="34">
<element kind="staticText" x="0" y="0" width="100" height="20"/>
</band>
</detail>
<summary height="40"></summary>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	emb := Default()
	emb.Enabled = true
	emb.Band = "detail"
	emb.Y = 10
	emb.Height = 300
	emb.Width = 400
	res := SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("detail place: %+v", res)
	}
	text := string(mustRead(t, path))
	if !imageInTargetBand(text, "detail", "") {
		t.Fatalf("expected map in detail:\n%s", text)
	}
	if enclosingBandHeight(text) < emb.Y+emb.Height {
		t.Fatalf("detail band too short after place:\n%s", text)
	}

	emb.Band = "summary"
	res = SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("move to summary: %+v", res)
	}
	text = string(mustRead(t, path))
	if !imageInTargetBand(text, "summary", "") {
		t.Fatalf("expected map in summary:\n%s", text)
	}
	if regexp.MustCompile(`(?is)<detail[^>]*\bheight\s*=`).MatchString(text) {
		t.Fatalf("detail must not carry height:\n%s", text)
	}
	// Detail should shrink back around the staticText only (y+h=20 + padding), not ~318.
	detailH := detailBandHeight(text)
	if detailH <= 0 || detailH > 40 {
		t.Fatalf("detail band height %d not shrunk after move to summary:\n%s", detailH, text)
	}
	if enclosingBandHeight(text) < emb.Y+emb.Height {
		t.Fatalf("summary too short for map:\n%s", text)
	}
}

func detailBandHeight(xml string) int {
	doc, root, err := parseJRXML([]byte(xml))
	if err != nil {
		return -1
	}
	_ = doc
	detail := findRootChild(root, "detail")
	if detail == nil {
		return -1
	}
	if band := findChild(detail, "band"); band != nil {
		return attrInt(band, "height")
	}
	return attrInt(detail, "height")
}

func TestSyncJRXMLHealsInvalidGroupHeaderHeight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "heal.jrxml")
	// Map already in place but groupHeader incorrectly has height — must heal on sync.
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="heal">
<parameter name="REPORT_DIR" class="java.lang.String" forPrompting="false">
<defaultValueExpression><![CDATA[""]]></defaultValueExpression>
</parameter>
<parameter name="P_MAP_IMAGE" class="java.lang.String" forPrompting="false">
<defaultValueExpression><![CDATA[""]]></defaultValueExpression>
</parameter>
<group name="City">
<groupHeader height="100">
<element kind="image" uuid="` + ElementUUID + `" x="0" y="0" width="200" height="80" onErrorType="Blank">
<expression><![CDATA[($P{P_MAP_IMAGE} == null || $P{P_MAP_IMAGE}.length() == 0) ? null : new java.io.File($P{P_MAP_IMAGE})]]></expression>
</element>
</groupHeader>
</group>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "groupHeader"
	emb.GroupName = "City"
	emb.Width = 200
	emb.Height = 80
	emb.X = 0
	emb.Y = 0
	res := SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("expected heal rewrite: %+v", res)
	}
	text := string(mustRead(t, path))
	if regexp.MustCompile(`(?is)<groupHeader[^>]*\bheight\s*=`).MatchString(text) {
		t.Fatalf("groupHeader still has height:\n%s", text)
	}
	if !strings.Contains(text, `<band height="100">`) && enclosingBandHeight(text) < 80 {
		t.Fatalf("expected band tall enough for map:\n%s", text)
	}
	if enclosingBandHeight(text) < emb.Y+emb.Height {
		t.Fatalf("band height %d too small after heal:\n%s", enclosingBandHeight(text), text)
	}
	assertGroupedFileMapExpression(t, text, "City")
}
