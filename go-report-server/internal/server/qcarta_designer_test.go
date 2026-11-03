package server

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

const designerOrdinaryJRXML = `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="parcel_card" language="java" pageWidth="595" pageHeight="842">
  <parameter name="parcel_id" class="java.lang.String"/>
  <queryString><![CDATA[SELECT * FROM parcels WHERE id = $P{parcel_id}]]></queryString>
  <field name="id" class="java.lang.String"/>
  <detail>
    <band height="40">
      <element kind="textField">
        <expression><![CDATA[$F{id}]]></expression>
      </element>
    </band>
  </detail>
</jasperReport>
`

const designerMapJRXML = `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport name="parcel_map" language="java" pageWidth="595" pageHeight="842">
  <parameter name="parcel_id" class="java.lang.String"/>
  <queryString><![CDATA[SELECT * FROM parcels WHERE id = $P{parcel_id}]]></queryString>
  <field name="id" class="java.lang.String"/>
  <detail>
    <band height="320">
      <element kind="image" uuid="designer-map-1" x="0" y="10" width="400" height="300">
        <expression><![CDATA[com.acugis.map.MapRenderer.render((java.sql.Connection)$P{REPORT_CONNECTION}, ("SELECT * FROM \"public\".\"parcels\"") + " WHERE id = " + $P{parcel_id}, 400, 300, null, "none", "")]]></expression>
      </element>
    </band>
  </detail>
</jasperReport>
`

func TestDesignerJRXMLParameterImport(t *testing.T) {
	params := spatial.PublishParametersFromJRXML([]byte(designerOrdinaryJRXML))
	if len(params) == 0 {
		t.Fatal("expected Designer parameter import")
	}
	found := false
	for _, p := range params {
		if stringify(p["name"]) == "parcel_id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("parcel_id missing: %+v", params)
	}
}

func TestDesignerMapJRXMLUsesFillTimeRenderer(t *testing.T) {
	if !strings.Contains(designerMapJRXML, "com.acugis.map.MapRenderer.render") {
		t.Fatal("fixture is not a Designer map report")
	}
	if strings.Contains(designerMapJRXML, "P_MAP_IMAGE") {
		t.Fatal("Designer map JRXML must not use Embedded Maps P_MAP_IMAGE")
	}
	params := spatial.PublishParametersFromJRXML([]byte(designerMapJRXML))
	found := false
	for _, p := range params {
		if stringify(p["name"]) == "parcel_id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("map report must still import parameters: %+v", params)
	}
}
