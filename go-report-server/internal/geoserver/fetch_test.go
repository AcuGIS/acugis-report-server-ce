package geoserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const describeRoads = `<?xml version="1.0"?>
<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema">
  <xsd:complexType name="roadsType">
    <xsd:sequence>
      <xsd:element name="the_geom" type="gml:MultiLineStringPropertyType"/>
      <xsd:element name="name" type="xsd:string" minOccurs="0" nillable="true"/>
      <xsd:element name="class" type="xsd:string"/>
      <xsd:element name="lanes" type="xsd:int"/>
      <xsd:element name="length" type="xsd:double"/>
      <xsd:element name="toll" type="xsd:boolean"/>
      <xsd:element name="built" type="xsd:date"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`

func writeFeatures(w http.ResponseWriter, features []map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":     "FeatureCollection",
		"features": features,
	})
}

func feature(id string, props map[string]any) map[string]any {
	return map[string]any{"type": "Feature", "id": id, "properties": props}
}

func TestFetchTypedRowsTypesNullAndAuth(t *testing.T) {
	var auth string
	var cql string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		switch strings.ToLower(r.URL.Query().Get("request")) {
		case "describefeaturetype":
			_, _ = io.WriteString(w, describeRoads)
		default:
			cql = r.URL.Query().Get("CQL_FILTER")
			writeFeatures(w, []map[string]any{
				feature("roads.1", map[string]any{
					"name": "Main", "class": "highway", "lanes": 4, "length": 12.5,
					"toll": true, "built": "2020-01-02",
				}),
				feature("roads.2", map[string]any{
					"name": nil, "class": "local", "lanes": nil, "length": nil,
					"toll": false, "built": nil,
				}),
			})
		}
	}))
	defer srv.Close()

	rows, err := FetchTypedRows(Credentials{
		Name: "ProdGeoServer", URL: srv.URL + "/geoserver", Username: "admin", Password: "secret",
	}, `SELECT "r"."name", "r"."lanes", "r"."length", "r"."toll", "r"."built" FROM "transport"."roads" "r" WHERE "r"."lanes" > $P{minlanes} AND "r"."name" = $P{road}`,
		map[string]any{"minlanes": 2, "road": "Main"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	if auth != wantAuth {
		t.Fatalf("auth=%q", auth)
	}
	if strings.Contains(cql, "'2'") || !strings.Contains(cql, "lanes > 2") || !strings.Contains(cql, "name = 'Main'") {
		t.Fatalf("cql=%s", cql)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0]["name"] != "Main" || rows[0]["lanes"] != int64(4) || rows[0]["length"] != 12.5 || rows[0]["toll"] != true {
		t.Fatalf("row0=%#v", rows[0])
	}
	built, ok := rows[0]["built"].(time.Time)
	if !ok || built.Format("2006-01-02") != "2020-01-02" {
		t.Fatalf("built=%#v", rows[0]["built"])
	}
	if rows[1]["name"] != nil || rows[1]["lanes"] != nil {
		t.Fatalf("row1=%#v", rows[1])
	}
	if _, geom := rows[0]["the_geom"]; geom {
		t.Fatal("geometry must not be copied into the tabular row")
	}
}

func TestFetchAnonymousOmitsAuthorization(t *testing.T) {
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, describeRoads)
			return
		}
		writeFeatures(w, []map[string]any{feature("roads.1", map[string]any{"name": "Open"})})
	}))
	defer srv.Close()
	rows, err := FetchTypedRows(Credentials{URL: srv.URL + "/geoserver/wfs"}, `SELECT "name" FROM "transport"."roads"`, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Open" {
		t.Fatalf("rows=%#v", rows)
	}
	for _, h := range auth {
		if h != "" {
			t.Fatalf("anonymous sent %q", h)
		}
	}
}

func TestFetchAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, describeRoads)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()
	_, err := FetchTypedRows(Credentials{URL: srv.URL, Username: "admin", Password: "secret"}, `SELECT "name" FROM "roads"`, nil, 5)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("password leaked: %v", err)
	}
}

func TestFetchParametersInAndNotIn(t *testing.T) {
	var filters []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, describeRoads)
			return
		}
		filters = append(filters, r.URL.Query().Get("CQL_FILTER"))
		writeFeatures(w, []map[string]any{feature("roads.1", map[string]any{"name": "Main"})})
	}))
	defer srv.Close()
	cfg := Credentials{URL: srv.URL + "/geoserver"}
	sqlIn := `SELECT "r"."name" FROM "transport"."roads" "r" WHERE $X{IN, "r"."class", classes}`
	if _, err := FetchTypedRows(cfg, sqlIn, map[string]any{"classes": []any{"highway", "local"}}, 5); err != nil {
		t.Fatal(err)
	}
	sqlNot := `SELECT "r"."name" FROM "transport"."roads" "r" WHERE $X{NOTIN, "r"."class", classes}`
	if _, err := FetchTypedRows(cfg, sqlNot, map[string]any{"classes": []any{int64(1), "local"}}, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := FetchTypedRows(cfg, sqlIn, map[string]any{"classes": []any{}}, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := FetchTypedRows(cfg, sqlNot, map[string]any{"classes": []string{}}, 5); err != nil {
		t.Fatal(err)
	}
	if len(filters) != 4 {
		t.Fatalf("filters=%v", filters)
	}
	if !strings.Contains(filters[0], "class IN (") || !strings.Contains(filters[0], "'highway'") || !strings.Contains(filters[0], "'local'") {
		t.Fatalf("in=%s", filters[0])
	}
	if !strings.Contains(filters[1], "NOT IN (1, 'local')") {
		t.Fatalf("notin=%s", filters[1])
	}
	if filters[2] != "1=0" || filters[3] != "1=1" {
		t.Fatalf("empty=%v", filters[2:])
	}
}

func TestFetchQuotedWorkspaceAndFeatureType(t *testing.T) {
	var typeNames []string
	var sortBy string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, `<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema"><xsd:complexType><xsd:sequence><xsd:element name="name" type="xsd:string"/><xsd:element name="length_km" type="xsd:decimal"/></xsd:sequence></xsd:complexType></xsd:schema>`)
			return
		}
		tn := r.URL.Query().Get("typeNames")
		if tn == "" {
			tn = r.URL.Query().Get("typeName")
		}
		typeNames = append(typeNames, tn)
		sortBy = r.URL.Query().Get("sortBy")
		writeFeatures(w, []map[string]any{feature("r.1", map[string]any{"name": "Waikato", "length_km": 12.5})})
	}))
	defer srv.Close()
	rows, err := FetchTypedRows(Credentials{URL: srv.URL}, `SELECT "r"."name", "r"."length_km" FROM "LINZ Roads"."River Name Lines - Pilot" "r" ORDER BY "r"."name" DESC`, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(typeNames) != 1 || typeNames[0] != "LINZ Roads:River Name Lines - Pilot" {
		t.Fatalf("typeNames=%v", typeNames)
	}
	if sortBy != "name+D" && sortBy != "name D" {
		t.Fatalf("sortBy=%q", sortBy)
	}
	if len(rows) != 1 || rows[0]["name"] != "Waikato" || rows[0]["length_km"] != 12.5 {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestFetchPagingAdvancesAndStops(t *testing.T) {
	var starts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, describeRoads)
			return
		}
		start := r.URL.Query().Get("startIndex")
		starts = append(starts, start)
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		if count == 0 {
			count, _ = strconv.Atoi(r.URL.Query().Get("maxFeatures"))
		}
		n := count
		base, _ := strconv.Atoi(start)
		if base >= 1000 {
			n = 3
		}
		feats := make([]map[string]any, 0, n)
		for i := 0; i < n; i++ {
			feats = append(feats, feature("roads."+strconv.Itoa(base+i), map[string]any{"name": "R"}))
		}
		writeFeatures(w, feats)
	}))
	defer srv.Close()
	rows, err := FetchTypedRows(Credentials{URL: srv.URL + "/geoserver"}, `SELECT "name" FROM "transport"."roads"`, nil, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1003 {
		t.Fatalf("rows=%d starts=%v", len(rows), starts)
	}
	if len(starts) != 2 || starts[0] != "0" || starts[1] != "1000" {
		t.Fatalf("starts=%v", starts)
	}
}

func TestFetchPagingStopsWhenServerIgnoresStartIndex(t *testing.T) {
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.URL.Query().Get("request"), "DescribeFeatureType") {
			_, _ = io.WriteString(w, describeRoads)
			return
		}
		pages++
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		if count == 0 {
			count = 1000
		}
		feats := make([]map[string]any, count)
		for i := range feats {
			feats[i] = feature("roads.1", map[string]any{"name": "Same"})
		}
		writeFeatures(w, feats)
	}))
	defer srv.Close()
	rows, err := FetchTypedRows(Credentials{URL: srv.URL}, `SELECT "name" FROM "transport"."roads"`, nil, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d pages=%d", len(rows), pages)
	}
	if pages != 2 {
		t.Fatalf("pages=%d", pages)
	}
}

func TestFetchRejectsUnsupportedSQLWithoutNetwork(t *testing.T) {
	cases := []string{
		`SELECT "a" FROM "s"."t" "a" JOIN "s"."u" "u" ON "a"."id" = "u"."id"`,
		`SELECT COUNT("a"."name") FROM "s"."t" "a"`,
		`SELECT "a"."name" FROM "s"."t" "a" GROUP BY "a"."name"`,
	}
	for _, sql := range cases {
		_, err := FetchTypedRows(Credentials{URL: "http://127.0.0.1:1/geoserver"}, sql, nil, 5)
		if err == nil {
			t.Fatalf("expected error for %s", sql)
		}
		if strings.Contains(err.Error(), "WFS request failed") {
			t.Fatalf("unsupported SQL reached the network: %v", err)
		}
	}
}
