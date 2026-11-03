package spatial

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func TestGeoJSONSelectAliasOracleNoLeadingUnderscore(t *testing.T) {
	d, err := DialectFor(&datasources.Datasource{DBType: "oracle"})
	if err != nil {
		t.Fatal(err)
	}
	alias := geoJSONSelectAlias(d)
	if strings.HasPrefix(alias, "_") {
		t.Fatalf("oracle alias %q must not start with underscore (ORA-00911)", alias)
	}
	if alias != "jrs_geojson" {
		t.Fatalf("alias=%q", alias)
	}

	sql := "SELECT " + d.AsGeoJSON4326(`t."GEOM"`) + " AS " + alias +
		` FROM "HR"."CITIES" t WHERE t."GEOM" IS NOT NULL`
	if strings.Contains(sql, " AS _") {
		t.Fatalf("sql still has underscore alias: %s", sql)
	}
	// Position of former bug: unquoted _jrs_geojson around char 48.
	if idx := strings.Index(sql, " AS "); idx >= 0 {
		rest := sql[idx+4:]
		if strings.HasPrefix(rest, "_") {
			t.Fatalf("column alias starts with _: %s", rest)
		}
	}
}

func TestIsGeoJSONAliasCol(t *testing.T) {
	for _, name := range []string{"_jrs_geojson", "JRS_GEOJSON", "jrs_geojson"} {
		if !isGeoJSONAliasCol(name) {
			t.Errorf("%q should be recognized", name)
		}
	}
}
