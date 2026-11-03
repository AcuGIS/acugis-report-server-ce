package embeddedmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

const usaLikeJRXML = `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="usa">
<query language="SQL"><![CDATA[SELECT "u"."state_name", "u"."sub_region"
FROM "public"."usapgservice" "u"
WHERE $X{IN, "u"."sub_region", p_region}]]></query>
<field name="state_name" class="java.lang.String"></field>
<field name="sub_region" class="java.lang.String"></field>
<group name="sub_region">
<expression><![CDATA[$F{sub_region}]]></expression>
<groupHeader>
<band height="20"></band>
</groupHeader>
</group>
<summary height="40"></summary>
</jasperReport>`

func assertGroupedFileMapExpression(t *testing.T, text, field string) {
	t.Helper()
	if !strings.Contains(text, "new java.io.File($P{P_MAP_IMAGE}, java.lang.String.valueOf($F{"+field+"}) + \".png\")") &&
		!strings.Contains(text, `new java.io.File($P{P_MAP_IMAGE}, java.lang.String.valueOf($F{`+field+`}) + ".png")`) {
		t.Fatalf("expected grouped File($P{P_MAP_IMAGE}, $F{%s}.png) expression:\n%s", field, text)
	}
	if !strings.Contains(text, "$F{"+field+"}") {
		t.Fatalf("expected $F{%s} group value:\n%s", field, text)
	}
	if !strings.Contains(text, `evaluationTime="Group"`) {
		t.Fatalf("expected evaluationTime=Group:\n%s", text)
	}
	if !strings.Contains(text, `evaluationGroup="`) {
		t.Fatalf("expected evaluationGroup:\n%s", text)
	}
	if strings.Contains(text, postGISMapRendererPrefix) {
		t.Fatalf("Go grouped map must keep P_MAP_IMAGE, not MapRenderer:\n%s", text)
	}
}

func TestSyncJRXMLGroupHeaderEmitsPostGISGroupFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usa.jrxml")
	if err := os.WriteFile(path, []byte(usaLikeJRXML), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := Default()
	emb.Enabled = true
	emb.Band = "groupHeader"
	emb.GroupName = "sub_region"
	emb.Width = 400
	emb.Height = 300
	res := SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("group header insert: %+v", res)
	}
	text := string(mustRead(t, path))
	assertGroupedFileMapExpression(t, text, "sub_region")
	if !strings.Contains(text, `name="P_MAP_IMAGE"`) {
		t.Fatalf("grouped PostGIS map must keep P_MAP_IMAGE parameter:\n%s", text)
	}

	res = SyncJRXML(path, emb)
	if !res.OK || res.JRXMLChanged {
		t.Fatalf("second grouped sync should be no-op: %+v", res)
	}
}

func TestSyncJRXMLPreservesDesignerMapRenderer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "designer.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="designer">
<query language="SQL"><![CDATA[SELECT * FROM "public"."usapgservice"]]></query>
<field name="sub_region" class="java.lang.String"></field>
<group name="sub_region">
<expression><![CDATA[$F{sub_region}]]></expression>
<groupHeader>
<band height="320">
<element kind="image" uuid="designer-map-1" x="0" y="10" width="400" height="300" onErrorType="Blank" evaluationTime="Group" evaluationGroup="sub_region">
<expression><![CDATA[com.acugis.map.MapRenderer.render((java.sql.Connection)$P{REPORT_CONNECTION}, ("SELECT * FROM \"public\".\"usapgservice\"") + " WHERE " + com.acugis.map.MapSql.eq("\"sub_region\"", $F{sub_region}), 400, 300, null, "none", "")]]></expression>
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
	emb.GroupName = "sub_region"
	emb.X = 0
	emb.Y = 10
	emb.Width = 400
	emb.Height = 300
	res := SyncJRXML(path, emb)
	if !res.OK {
		t.Fatalf("preserve designer map: %+v", res)
	}
	text := string(mustRead(t, path))
	if strings.Contains(text, "new java.io.File($P{P_MAP_IMAGE})") || strings.Contains(text, `name="P_MAP_IMAGE"`) {
		t.Fatalf("must not replace Designer MapRenderer with P_MAP_IMAGE:\n%s", text)
	}
	if !strings.Contains(text, "MapSql.eq") || !strings.Contains(text, "$F{sub_region}") {
		t.Fatalf("must keep Designer group filter:\n%s", text)
	}
	if strings.Count(text, postGISMapRendererPrefix) != 1 {
		t.Fatalf("expected a single MapRenderer image:\n%s", text)
	}
}

func TestSyncJRXMLAddsEqToDesignerMapRenderer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-eq.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="missing">
<query language="SQL"><![CDATA[SELECT * FROM "public"."usapgservice"]]></query>
<field name="sub_region" class="java.lang.String"></field>
<group name="sub_region">
<expression><![CDATA[$F{sub_region}]]></expression>
<groupHeader>
<band height="320">
<element kind="image" x="0" y="10" width="400" height="300" onErrorType="Blank">
<expression><![CDATA[com.acugis.map.MapRenderer.render((java.sql.Connection)$P{REPORT_CONNECTION}, "SELECT * FROM \"public\".\"usapgservice\"", 400, 300, null, "none", "")]]></expression>
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
	emb.GroupName = "sub_region"
	emb.X = 0
	emb.Y = 10
	emb.Width = 400
	emb.Height = 300
	res := SyncJRXML(path, emb)
	if !res.OK || !res.JRXMLChanged {
		t.Fatalf("expected group filter restore: %+v", res)
	}
	text := string(mustRead(t, path))
	if !strings.Contains(text, "MapSql.eq") || !strings.Contains(text, "$F{sub_region}") {
		t.Fatalf("expected MapSql.eq restore:\n%s", text)
	}
	if !strings.Contains(text, `evaluationTime="Group"`) {
		t.Fatalf("expected evaluationTime=Group:\n%s", text)
	}
}

func TestSyncJRXMLSummaryStillUsesFileParameter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="summary">
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
		t.Fatalf("summary insert: %+v", res)
	}
	text := string(mustRead(t, path))
	if !strings.Contains(text, "new java.io.File($P{P_MAP_IMAGE})") {
		t.Fatalf("summary map must keep P_MAP_IMAGE file expression:\n%s", text)
	}
	if strings.Contains(text, postGISMapRendererPrefix) {
		t.Fatalf("summary map must not use fill-time MapRenderer:\n%s", text)
	}
}

func TestAppendToQuerySkipsPMapImageWhenDesignerMapRenderer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usa.jrxml")
	base := `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="usa">
<field name="sub_region" class="java.lang.String"></field>
<group name="sub_region">
<expression><![CDATA[$F{sub_region}]]></expression>
<groupHeader>
<band height="320">
<element kind="image" x="0" y="10" width="400" height="300">
<expression><![CDATA[com.acugis.map.MapRenderer.render((java.sql.Connection)$P{REPORT_CONNECTION}, "SELECT * FROM usapgservice", 400, 300, null, "none", "")]]></expression>
</element>
</band>
</groupHeader>
</group>
</jasperReport>`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		Paths:   Paths{JRXMLDir: dir},
		Spatial: &spatial.Resolver{JRXMLDir: dir},
	}
	def := &reports.Definition{
		Key: "usa",
		Raw: map[string]any{
			"embeddedMap": map[string]any{
				"enabled":   true,
				"band":      "groupHeader",
				"groupName": "sub_region",
				"renderer":  "postgis",
			},
		},
	}
	res := svc.AppendToQuery(t.Context(), def, "a=1", map[string]any{"a": "1"})
	if res.Query != "a=1" || res.Error != "" || res.Param != "" {
		t.Fatalf("Designer MapRenderer execute must not append P_MAP_IMAGE: %+v", res)
	}
	text := string(mustRead(t, path))
	if !strings.Contains(text, "MapSql.eq") || !strings.Contains(text, "$F{sub_region}") {
		t.Fatalf("execute must restore MapSql.eq group filter:\n%s", text)
	}
	if !strings.Contains(text, `evaluationTime="Group"`) {
		t.Fatalf("execute must set evaluationTime=Group:\n%s", text)
	}
}

func TestSplitFeaturesByGroup(t *testing.T) {
	feats := []map[string]any{
		{"properties": map[string]any{"sub_region": "E S Cen", "name": "KY"}},
		{"properties": map[string]any{"sub_region": "E S Cen", "name": "TN"}},
		{"properties": map[string]any{"SUB_REGION": "Pacific", "name": "CA"}},
	}
	got := splitFeaturesByGroup(feats, "sub_region")
	if len(got["E S Cen"]) != 2 {
		t.Fatalf("E S Cen: got %d, want 2", len(got["E S Cen"]))
	}
	if len(got["Pacific"]) != 1 {
		t.Fatalf("Pacific: got %d, want 1", len(got["Pacific"]))
	}
}
