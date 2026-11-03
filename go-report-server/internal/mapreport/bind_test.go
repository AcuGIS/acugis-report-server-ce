package mapreport_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestSQLPrimaryTable(t *testing.T) {
	cases := []struct {
		sql    string
		schema string
		table  string
		alias  string
	}{
		{`SELECT * FROM public.parcels p WHERE 1=1`, "public", "parcels", "p"},
		{`SELECT a FROM "public"."usapgservice" "u"`, "public", "usapgservice", "u"},
		{`SELECT * FROM buildings WHERE $P{x} IS NOT NULL`, "", "buildings", ""},
		{`SELECT c.id FROM ` + "`auroras_classicmodels`.`customers`" + ` ` + "`c`" + `
INNER JOIN ` + "`auroras_classicmodels`.`customer_locations`" + ` ` + "`c1`" + `
    ON c.customerNumber = c1.customerNumber`, "auroras_classicmodels", "customers", "c"},
		{`SELECT * FROM [dbo].[parcels] [p]`, "dbo", "parcels", "p"},
		{``, "", "", ""},
	}
	for _, tc := range cases {
		got := mapreport.SQLPrimaryTable(tc.sql)
		if got.Schema != tc.schema || got.Table != tc.table {
			t.Fatalf("sql=%q got schema=%q table=%q want %q.%q", tc.sql, got.Schema, got.Table, tc.schema, tc.table)
		}
		if tc.alias != "" && got.Alias != tc.alias {
			t.Fatalf("sql=%q alias=%q want %q", tc.sql, got.Alias, tc.alias)
		}
	}
}

func TestSQLRelationListIncludesJoins(t *testing.T) {
	sql := `SELECT c.id FROM ` + "`db`.`customers`" + ` ` + "`c`" + `
INNER JOIN ` + "`db`.`customer_locations`" + ` ` + "`c1`" + ` ON c.id = c1.id`
	rels := mapreport.SQLRelationList(sql)
	if len(rels) < 2 {
		t.Fatalf("rels=%v", rels)
	}
	if rels[0].Table != "customers" || rels[1].Table != "customer_locations" {
		t.Fatalf("rels=%v", rels)
	}
}

func TestSQLGeometryRefDialect_JoinedMySQL(t *testing.T) {
	d, _ := mapreport.DialectFor(&datasources.Datasource{DBType: "mysql"})
	sql := `SELECT c.id FROM ` + "`db`.`customers`" + ` c INNER JOIN ` + "`db`.`customer_locations`" + ` c1 ON c.id = c1.id`
	ref := mapreport.SQLGeometryRefDialect(sql, "db", "customer_locations", "location", d)
	if ref != "`c1`.`location`" {
		t.Fatalf("ref=%q", ref)
	}
}

func TestIsSafeJasperColumnRef_Backticks(t *testing.T) {
	if !mapreport.IsSafeJasperColumnRef("`c`.`country`") {
		t.Fatal("expected backtick column ref to be safe")
	}
}

func TestSQLGeometryRefPrefersAlias(t *testing.T) {
	sql := `SELECT u.id FROM public.parcels u JOIN public.other o ON u.id = o.id`
	ref := mapreport.SQLGeometryRef(sql, "public", "parcels", "geom")
	if ref != `"u"."geom"` {
		t.Fatalf("ref=%q", ref)
	}
}

func TestBindJasperSQL_IN(t *testing.T) {
	sql := `SELECT * FROM t WHERE $X{IN, industry, p_industry} AND status = $P{p_status}`
	bound, err := mapreport.BindJasperSQL(sql, map[string]any{
		"p_industry": []any{"a", "b"},
		"p_status":   "open",
	}, map[string]mapreport.ParamMeta{
		"p_status": {Name: "p_status", Class: "java.lang.String", Kind: "string"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT * FROM t WHERE industry IN ($1, $2) AND status = $3`; bound.SQL != want {
		t.Fatalf("sql=%q", bound.SQL)
	}
	if len(bound.Params) != 3 {
		t.Fatalf("params=%v", bound.Params)
	}
}

func TestBindJasperSQL_PromotesCollectionEqualityToXin(t *testing.T) {
	sql := `SELECT DISTINCT state FROM t WHERE region = $P{p_region} ORDER BY state`
	bound, err := mapreport.BindJasperSQL(sql, map[string]any{
		"p_region": []any{"Mid Atl", "E S Cen"},
	}, map[string]mapreport.ParamMeta{
		"p_region": {Name: "p_region", Class: "java.util.Collection", Kind: "collection"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT DISTINCT state FROM t WHERE region IN ($1, $2) ORDER BY state`; bound.SQL != want {
		t.Fatalf("sql=%q", bound.SQL)
	}
	if len(bound.Params) != 2 || bound.Params[0] != "Mid Atl" || bound.Params[1] != "E S Cen" {
		t.Fatalf("params=%v", bound.Params)
	}
}

func TestPromoteCollectionEqualityToXin(t *testing.T) {
	meta := map[string]mapreport.ParamMeta{
		"p_region": {Name: "p_region", Kind: "collection", Class: "java.util.Collection"},
		"p_note":   {Name: "p_note", Kind: "string", Class: "java.lang.String"},
	}
	ctx := map[string]any{"p_region": []any{"a", "b"}}

	got := mapreport.PromoteCollectionEqualityToXin(
		`SELECT city FROM t WHERE "u"."sub_region" = $P{p_region} AND note = $P{p_note}`,
		ctx, meta,
	)
	want := `SELECT city FROM t WHERE $X{IN, "u"."sub_region", p_region} AND note = $P{p_note}`
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}

	got = mapreport.PromoteCollectionEqualityToXin(
		`SELECT city FROM t WHERE $P{p_region} = region`,
		ctx, meta,
	)
	want = `SELECT city FROM t WHERE $X{IN, region, p_region}`
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}

	got = mapreport.PromoteCollectionEqualityToXin(
		`SELECT city FROM t WHERE region IN ($P{p_region})`,
		ctx, meta,
	)
	want = `SELECT city FROM t WHERE $X{IN, region, p_region}`
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}

	// Scalar params must not be rewritten.
	got = mapreport.PromoteCollectionEqualityToXin(
		`SELECT city FROM t WHERE note = $P{p_note}`,
		map[string]any{"p_note": "x"}, meta,
	)
	want = `SELECT city FROM t WHERE note = $P{p_note}`
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestBindJasperSQL_EmptyIN(t *testing.T) {
	sql := `SELECT * FROM t WHERE $X{IN, col, p_x}`
	bound, err := mapreport.BindJasperSQL(sql, map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bound.SQL != `SELECT * FROM t WHERE 0 = 0` {
		t.Fatalf("sql=%q", bound.SQL)
	}
}

func TestCollectFilterContext_Multi(t *testing.T) {
	q := mapreport.ParseQueryMulti("report=demo&p_a=1&p_a=2&_skip=x&p_srid=2263")
	ctx := mapreport.CollectFilterContext(q, map[string]mapreport.ParamMeta{
		"p_srid": {Name: "p_srid", Kind: "integer", Class: "java.lang.Integer"},
	}, nil)
	if _, ok := ctx["report"]; ok {
		t.Fatal("report should be reserved")
	}
	if _, ok := ctx["_skip"]; ok {
		t.Fatal("_skip should be ignored")
	}
	list, ok := ctx["p_a"].([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("p_a=%v", ctx["p_a"])
	}
	if ctx["p_srid"] != 2263 {
		t.Fatalf("p_srid=%v", ctx["p_srid"])
	}
}

func TestCollectFilterContext_EmptyCollection(t *testing.T) {
	q := mapreport.ParseQueryMulti("p_country=USA&p_city=")
	ctx := mapreport.CollectFilterContext(q, map[string]mapreport.ParamMeta{
		"p_country": {Name: "p_country", Kind: "collection", Class: "java.util.Collection"},
		"p_city":    {Name: "p_city", Kind: "collection", Class: "java.util.Collection"},
	}, nil)
	country, ok := ctx["p_country"].([]any)
	if !ok || len(country) != 1 || country[0] != "USA" {
		t.Fatalf("p_country=%v", ctx["p_country"])
	}
	// Blank key= is omitted (JRXML defaults may fill later); not an explicit clear.
	if _, ok := ctx["p_city"]; ok {
		t.Fatalf("blank p_city= should be omitted, got %#v", ctx["p_city"])
	}
	// Explicit clear sentinel keeps empty Collection.
	qClear := mapreport.ParseQueryMulti("p_city=__JRS_CLEAR__")
	ctxClear := mapreport.CollectFilterContext(qClear, map[string]mapreport.ParamMeta{
		"p_city": {Name: "p_city", Kind: "collection", Class: "java.util.Collection", Default: []string{"Austin"}},
	}, nil)
	cityClear, ok := ctxClear["p_city"].([]any)
	if !ok || len(cityClear) != 0 {
		t.Fatalf("expected explicit empty p_city, got %#v", ctxClear["p_city"])
	}
	// Empty scalar still omitted.
	q2 := mapreport.ParseQueryMulti("p_note=")
	ctx2 := mapreport.CollectFilterContext(q2, map[string]mapreport.ParamMeta{
		"p_note": {Name: "p_note", Kind: "string", Class: "java.lang.String"},
	}, nil)
	if _, ok := ctx2["p_note"]; ok {
		t.Fatalf("empty scalar should be omitted, got %#v", ctx2["p_note"])
	}
}

func TestCollectFilterContext_AppliesMissingJRXMLDefaults(t *testing.T) {
	q := mapreport.ParseQueryMulti("")
	ctx := mapreport.CollectFilterContext(q, map[string]mapreport.ParamMeta{
		"p_city": {
			Name: "p_city", Kind: "collection", Class: "java.util.Collection",
			Default: []string{"Austin"},
		},
		"p_country": {
			Name: "p_country", Kind: "string", Class: "java.lang.String",
			Default: "USA",
		},
	}, nil)
	city, ok := ctx["p_city"].([]any)
	if !ok || len(city) != 1 || city[0] != "Austin" {
		t.Fatalf("p_city default=%#v", ctx["p_city"])
	}
	if ctx["p_country"] != "USA" {
		t.Fatalf("p_country default=%#v", ctx["p_country"])
	}
	// Blank key= is omitted, so JRXML default still applies.
	q2 := mapreport.ParseQueryMulti("p_city=")
	ctx2 := mapreport.CollectFilterContext(q2, map[string]mapreport.ParamMeta{
		"p_city": {
			Name: "p_city", Kind: "collection", Class: "java.util.Collection",
			Default: []string{"Austin"},
		},
	}, nil)
	city2, ok := ctx2["p_city"].([]any)
	if !ok || len(city2) != 1 || city2[0] != "Austin" {
		t.Fatalf("blank should fall back to default: %#v", ctx2["p_city"])
	}
}

func TestJRXMLSQLAndParams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.jrxml")
	body := `<?xml version="1.0"?>
<jasperReport>
  <parameter name="p_status" class="java.lang.String"/>
  <parameter name="p_geometry" class="java.lang.String"/>
  <parameter name="p_srid" class="java.lang.Integer"/>
  <queryString><![CDATA[SELECT id FROM public.parcels WHERE status = $P{p_status}]]></queryString>
</jasperReport>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sql, err := mapreport.JRXMLSQLQuery(path)
	if err != nil {
		t.Fatal(err)
	}
	if sql != "SELECT id FROM public.parcels WHERE status = $P{p_status}" {
		t.Fatalf("sql=%q", sql)
	}
	params, err := mapreport.JRXMLParameters(path)
	if err != nil {
		t.Fatal(err)
	}
	if params["p_srid"].Kind != "integer" {
		t.Fatalf("params=%v", params)
	}
	if !mapreport.JRXMLHasSpatialParameters(path) {
		t.Fatal("expected spatial params")
	}
}

func TestReportMapDefinition(t *testing.T) {
	def := &reports.Definition{
		Key: "m1",
		Raw: map[string]any{
			"type": "map",
			"mapDefinition": map[string]any{
				"basemap":        "Carto Dark",
				"geometryColumn": "shape",
			},
		},
	}
	rawMap := def.Raw["mapDefinition"].(map[string]any)
	md := mapreport.ReportMapDefinition(def)
	if md == nil {
		t.Fatal("expected map definition")
	}
	if md["basemap"] != "ofm_dark" {
		t.Fatalf("basemap=%v", md["basemap"])
	}
	if rawMap["basemap"] != "Carto Dark" {
		t.Fatalf("stored basemap was rewritten: %v", rawMap["basemap"])
	}
	if md["geometryColumn"] != "shape" {
		t.Fatalf("geom=%v", md["geometryColumn"])
	}
	jasper := &reports.Definition{Key: "j", Raw: map[string]any{"type": "jasper"}}
	if mapreport.ReportMapDefinition(jasper) != nil {
		t.Fatal("jasper should not have map definition")
	}
	jasperMap := &reports.Definition{
		Key: "jm",
		Raw: map[string]any{
			"type":          "jasper",
			"mapDefinition": map[string]any{"enabled": true, "geometryColumn": "geom"},
		},
	}
	if mapreport.ReportMapDefinition(jasperMap) == nil {
		t.Fatal("enabled mapDefinition on jasper should resolve")
	}
	if mapreport.IsMapReport(jasperMap) {
		t.Fatal("PHP jrs_is_map_report is type===map only; jasper+mapDefinition must not open map viewer")
	}
	if !mapreport.IsMapReport(def) {
		t.Fatal("type map should be IsMapReport")
	}
}

func TestNeutralizeJasperSQL(t *testing.T) {
	in := `SELECT * FROM t WHERE $X{IN,a,p} AND x=$P{p} AND y=$P!{q}`
	out := mapreport.NeutralizeJasperSQL(in)
	if out != `SELECT * FROM t WHERE TRUE AND x=NULL AND y=NULL` {
		t.Fatalf("out=%q", out)
	}
}
