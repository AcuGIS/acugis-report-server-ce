package mapreport_test

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

func TestDialectForEngines(t *testing.T) {
	cases := []struct {
		dbType string
		engine string
		ph1    string
	}{
		{"postgresql", mapreport.EnginePostgreSQL, "$1"},
		{"mysql", mapreport.EngineMySQL, "?"},
		{"sqlserver", mapreport.EngineSQLServer, "@p1"},
		{"oracle", mapreport.EngineOracle, ":1"},
	}
	for _, tc := range cases {
		d, err := mapreport.DialectFor(&datasources.Datasource{DBType: tc.dbType})
		if err != nil {
			t.Fatalf("%s: %v", tc.dbType, err)
		}
		if d.Engine() != tc.engine {
			t.Fatalf("%s engine=%q", tc.dbType, d.Engine())
		}
		if d.Placeholder(1) != tc.ph1 {
			t.Fatalf("%s placeholder=%q", tc.dbType, d.Placeholder(1))
		}
	}
	_, err := mapreport.DialectFor(&datasources.Datasource{DBType: "sqlite"})
	if err == nil {
		t.Fatal("sqlite should be unsupported")
	}
}

func TestBindJasperSQLDialect_MySQL(t *testing.T) {
	d, _ := mapreport.DialectFor(&datasources.Datasource{DBType: "mysql"})
	bound, err := mapreport.BindJasperSQLDialect(
		`SELECT * FROM t WHERE $X{IN, industry, p_industry} AND status = $P{p_status}`,
		map[string]any{"p_industry": []any{"a", "b"}, "p_status": "open"},
		map[string]mapreport.ParamMeta{"p_status": {Name: "p_status", Kind: "string"}},
		d,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT * FROM t WHERE industry IN (?, ?) AND status = ?`
	if bound.SQL != want {
		t.Fatalf("sql=%q", bound.SQL)
	}
}

func TestGeoJSONToWKT_Polygon(t *testing.T) {
	wkt, err := mapreport.GeoJSONToWKT(`{"type":"Polygon","coordinates":[[[-105,39],[-104,39],[-104,40],[-105,40],[-105,39]]]}`)
	if err != nil {
		t.Fatal(err)
	}
	if wkt != "POLYGON((-105 39,-104 39,-104 40,-105 40,-105 39))" {
		t.Fatalf("wkt=%q", wkt)
	}
}

func TestMySQLSalesTerritoriesFeatureSQL(t *testing.T) {
	d, _ := mapreport.DialectFor(&datasources.Datasource{DBType: "mysql"})
	reportSQL := "SELECT\n" +
		"    `s`.`id`,\n" +
		"    `s`.`territory_name`,\n" +
		"    `s`.`geom`\n" +
		"FROM `auroras_classicmodels`.`sales_territories` `s`\n" +
		"WHERE (\n" +
		"    $P{p_geometry} IS NULL\n" +
		"    OR ST_Intersects(\n" +
		"        `s`.`geom`,\n" +
		"        ST_GeomFromText($P{p_geometry}, $P{p_srid})\n" +
		"    )\n" +
		")"

	primary := mapreport.SQLPrimaryTable(reportSQL)
	if primary.Schema != "auroras_classicmodels" || primary.Table != "sales_territories" || primary.Alias != "s" {
		t.Fatalf("primary=%+v", primary)
	}
	ref := mapreport.SQLGeometryRefDialect(reportSQL, primary.Schema, primary.Table, "geom", d)
	if ref != "`s`.`geom`" {
		t.Fatalf("geomRef=%q", ref)
	}

	// Simulate LayerGeoJSONFromReport prep without a DB.
	stripped := mapreport.StripInactiveSpatialFilterForTest(reportSQL, nil)
	if strings.Contains(stripped, "ST_Intersects") || strings.Contains(stripped, "$P{p_geometry}") {
		t.Fatalf("spatial filter should be neutralized: %s", stripped)
	}
	bound, err := mapreport.BindJasperSQLDialect(stripped, nil, map[string]mapreport.ParamMeta{
		"p_geometry": {Name: "p_geometry", Kind: "string"},
		"p_srid":     {Name: "p_srid", Kind: "integer"},
	}, d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bound.SQL, "ST_GeomFromText") {
		t.Fatalf("bound SQL still has ST_GeomFromText: %s", bound.SQL)
	}
	body := mapreport.StripGeometryColumnFromSelectForTest(bound.SQL[strings.Index(strings.ToUpper(bound.SQL), "SELECT")+6:], "geom", d)
	injected := "SELECT " + d.AsGeoJSON4326(ref) + " AS _jrs_geojson," + body
	if !strings.Contains(injected, "ST_AsGeoJSON(`s`.`geom`)") {
		t.Fatalf("injected=%s", injected)
	}
	if strings.Contains(injected, "`s`.`geom`") && strings.Count(injected, "`s`.`geom`") != 1 {
		// only inside ST_AsGeoJSON
		t.Fatalf("raw geom should be stripped from select list: %s", injected)
	}
	if strings.Contains(injected, "territory_name") == false || strings.Contains(injected, "`s`.`id`") == false {
		t.Fatalf("attribute columns missing: %s", injected)
	}
}
