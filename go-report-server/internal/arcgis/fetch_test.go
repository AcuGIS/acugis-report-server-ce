package arcgis

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type arcServer struct {
	features   []map[string]any
	maxPage    int
	failMsg    string
	rejectOnce bool

	mu        sync.Mutex
	tokenN    int
	rejected  bool
	wheres    []string
	offsets   []string
	counts    []string
	fields    []string
	tokens    []string
	passwords []string
}

func (s *arcServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	if r.Method == http.MethodPost && (strings.Contains(path, "tokens") || strings.Contains(path, "generateToken")) {
		_ = r.ParseForm()
		s.mu.Lock()
		s.tokenN++
		s.passwords = append(s.passwords, r.Form.Get("password"))
		n := s.tokenN
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"token": fmt.Sprintf("tok-%d", n)})
		return
	}
	if strings.Contains(path, "/query") {
		if s.failMsg != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 400, "message": s.failMsg}})
			return
		}
		s.mu.Lock()
		if s.rejectOnce && !s.rejected {
			s.rejected = true
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 498, "message": "Invalid token."}})
			return
		}
		q := r.URL.Query()
		s.wheres = append(s.wheres, q.Get("where"))
		s.offsets = append(s.offsets, q.Get("resultOffset"))
		s.counts = append(s.counts, q.Get("resultRecordCount"))
		s.fields = append(s.fields, q.Get("outFields"))
		s.tokens = append(s.tokens, q.Get("token"))
		s.mu.Unlock()
		offset, _ := strconv.Atoi(q.Get("resultOffset"))
		count, _ := strconv.Atoi(q.Get("resultRecordCount"))
		if offset < 0 {
			offset = 0
		}
		if offset > len(s.features) {
			offset = len(s.features)
		}
		end := len(s.features)
		if count > 0 && offset+count < end {
			end = offset + count
		}
		feats := make([]any, 0, end-offset)
		for _, attrs := range s.features[offset:end] {
			feats = append(feats, map[string]any{"attributes": attrs})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"features":              feats,
			"exceededTransferLimit": end < len(s.features),
		})
		return
	}
	if strings.HasSuffix(strings.TrimRight(path, "/"), "/0") {
		maxPage := s.maxPage
		if maxPage <= 0 {
			maxPage = 1000
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Temperatuur", "id": 0, "supportsQuery": true, "type": "Feature Layer",
			"maxRecordCount": maxPage,
			"fields": []any{
				map[string]any{"name": "stationcode", "type": "esriFieldTypeInteger"},
				map[string]any{"name": "datum", "type": "esriFieldTypeDate"},
				map[string]any{"name": "zicht", "type": "esriFieldTypeString"},
				map[string]any{"name": "lat", "type": "esriFieldTypeDouble"},
				map[string]any{"name": "lon", "type": "esriFieldTypeDouble"},
				map[string]any{"name": "SHAPE", "type": "esriFieldTypeGeometry"},
			},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"layers": []any{map[string]any{"id": 0, "name": "Temperatuur", "supportsQuery": true}},
	})
}

func (s *arcServer) snapshot() (wheres, offsets, fields, tokens []string, tokenN int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.wheres...), append([]string{}, s.offsets...), append([]string{}, s.fields...), append([]string{}, s.tokens...), s.tokenN
}

func startArc(t *testing.T, s *arcServer) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}

func featureURL(srv *httptest.Server) string {
	return srv.URL + "/arcgis/rest/services/Weer/FeatureServer"
}

func weatherSQL() string {
	return `SELECT "t"."stationcode", "t"."datum", "t"."zicht", "t"."lat", "t"."lon" FROM "layers"."Temperatuur" "t"`
}

func TestCleanServiceURLStripsLayer(t *testing.T) {
	in := "https://services8.arcgis.com/GAWPLGpr6obR6MJB/arcgis/rest/services/neighborhoods/FeatureServer/0?f=pjson"
	got := CleanServiceURL(in)
	want := "https://services8.arcgis.com/GAWPLGpr6obR6MJB/arcgis/rest/services/neighborhoods/FeatureServer"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveConnectionNamePrefersJRXML(t *testing.T) {
	got := ResolveConnectionName("NeighborhoodsFS", "_dataSource=Other&datasource=Third", "Catalog")
	if got != "NeighborhoodsFS" {
		t.Fatalf("got %q", got)
	}
	if got := ResolveConnectionName("", "_dataSource=Viewer", "Catalog"); got != "Viewer" {
		t.Fatalf("viewer=%q", got)
	}
	if got := ResolveConnectionName("", "", "Catalog"); got != "Catalog" {
		t.Fatalf("catalog=%q", got)
	}
}

func TestRewriteFiltersKeepMultipleValues(t *testing.T) {
	params := map[string]any{"p_div": []string{"Mtn", "W N Cen"}}
	in := rewriteWhereForArcGIS(`$X{IN, "t"."division", p_div}`, params)
	if in != `"division" IN ('Mtn', 'W N Cen')` {
		t.Fatalf("IN=%q", in)
	}
	if strings.Contains(in, "Mtn, W N Cen") && !strings.Contains(in, "'Mtn', 'W N Cen'") {
		t.Fatalf("flattened IN: %s", in)
	}
	notIn := rewriteWhereForArcGIS(`$X{NOTIN, "division", p_div}`, params)
	if notIn != `"division" NOT IN ('Mtn', 'W N Cen')` {
		t.Fatalf("NOTIN=%q", notIn)
	}
	eq := rewriteWhereForArcGIS(`"t"."name" = $P{p_name}`, map[string]any{"p_name": "East"})
	if eq != `"name" = 'East'` {
		t.Fatalf("eq=%q", eq)
	}
	promoted := rewriteWhereForArcGIS(`"division" = $P{p_div}`, params)
	if promoted != `"division" IN ('Mtn', 'W N Cen')` {
		t.Fatalf("promoted=%q", promoted)
	}
}

func TestFillParamsCollectionDefaultAndRepeatedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "divisions.jrxml")
	body := `<?xml version="1.0"?>
<jasperReport>
  <parameter name="p_div" class="java.util.Collection">
    <defaultValueExpression><![CDATA[java.util.Arrays.asList("Mtn", "W N Cen")]]></defaultValueExpression>
  </parameter>
  <query language="collection"><![CDATA[SELECT "division" FROM "layers"."Temperatuur"]]></query>
</jasperReport>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	omitted := FillParams("_repName=weather&_dataSource=WeatherFS", path, nil)
	vals, ok := omitted["p_div"].([]any)
	if !ok || len(vals) != 2 || vals[0] != "Mtn" || vals[1] != "W N Cen" {
		t.Fatalf("default=%#v", omitted["p_div"])
	}
	if _, isString := omitted["p_div"].(string); isString {
		t.Fatal("collection default was flattened to a string")
	}
	repeated := FillParams("p_div=Pac&p_div=W+S+Cen", path, nil)
	got, ok := repeated["p_div"].([]any)
	if !ok || len(got) != 2 || got[0] != "Pac" || got[1] != "W S Cen" {
		t.Fatalf("repeated=%#v", repeated["p_div"])
	}
}

func sampleFeatures() []map[string]any {
	return []map[string]any{
		{"stationcode": 6275, "datum": int64(1792605000000), "zicht": "100000", "lat": 52.07, "lon": 5.88},
		{"stationcode": 6249, "datum": nil, "zicht": "43200", "lat": 52.65, "lon": 4.98},
		{"stationcode": 6260, "datum": "1.792605e+12", "zicht": "36200", "lat": 52.1, "lon": 5.18},
	}
}

func TestFetchTypedRowsTypesNullsAndEpoch(t *testing.T) {
	mock := &arcServer{features: sampleFeatures()}
	srv := startArc(t, mock)
	rows, err := FetchTypedRows(Credentials{URL: featureURL(srv)}, weatherSQL(), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0]["stationcode"] != int64(6275) {
		t.Fatalf("integer %#v", rows[0]["stationcode"])
	}
	if _, isTime := rows[0]["stationcode"].(time.Time); isTime {
		t.Fatal("integer was interpreted as a date")
	}
	if rows[0]["datum"] != int64(1792605000000) {
		t.Fatalf("epoch %#v", rows[0]["datum"])
	}
	if _, isTime := rows[0]["datum"].(time.Time); isTime {
		t.Fatal("epoch was converted before the field type is known")
	}
	if rows[0]["zicht"] != "100000" {
		t.Fatalf("string %#v", rows[0]["zicht"])
	}
	if rows[0]["lat"] != 52.07 || rows[0]["lon"] != 5.88 {
		t.Fatalf("double lat=%#v lon=%#v", rows[0]["lat"], rows[0]["lon"])
	}
	if rows[1]["datum"] != nil {
		t.Fatalf("null %#v", rows[1]["datum"])
	}
	if rows[2]["datum"] != "1.792605e+12" {
		t.Fatalf("scientific string rewritten: %#v", rows[2]["datum"])
	}
	raw, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"datum":1792605000000`) || strings.Contains(string(raw), "e+") {
		t.Fatalf("collection JSON = %s", raw)
	}
	_, _, fields, tokens, tokenN := mock.snapshot()
	if tokenN != 0 {
		t.Fatalf("anonymous request generated %d tokens", tokenN)
	}
	if len(tokens) == 0 || tokens[0] != "" {
		t.Fatalf("anonymous token=%v", tokens)
	}
	if len(fields) == 0 || !strings.Contains(fields[0], "stationcode") || strings.Contains(fields[0], "SHAPE") {
		t.Fatalf("outFields=%v", fields)
	}
}

func TestFetchTypedRowsAnonymousDoesNotUsePreviewCap(t *testing.T) {
	feats := make([]map[string]any, 120)
	for i := range feats {
		feats[i] = map[string]any{"stationcode": i + 1, "zicht": "ok", "lat": 1.5}
	}
	mock := &arcServer{features: feats, maxPage: 50}
	srv := startArc(t, mock)
	sql := `SELECT "t"."stationcode", "t"."zicht", "t"."lat" FROM "layers"."Temperatuur" "t"`
	rows, err := FetchTypedRows(Credentials{URL: featureURL(srv)}, sql, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 120 {
		t.Fatalf("rows=%d, report fill must not stop at the 100-row preview cap", len(rows))
	}
	_, offsets, _, _, _ := mock.snapshot()
	if len(offsets) < 3 || offsets[0] != "0" || offsets[1] != "50" || offsets[2] != "100" {
		t.Fatalf("pages=%v", offsets)
	}
	if rows[119]["stationcode"] != int64(120) || rows[50]["lat"] != 1.5 {
		t.Fatalf("last=%#v mid=%#v", rows[119], rows[50])
	}
}

func TestFetchTypedRowsShortPageEndsRetrieval(t *testing.T) {
	feats := make([]map[string]any, 5)
	for i := range feats {
		feats[i] = map[string]any{"stationcode": i + 1}
	}
	mock := &arcServer{features: feats, maxPage: 2}
	srv := startArc(t, mock)
	sql := `SELECT "stationcode" FROM "layers"."Temperatuur"`
	rows, err := FetchTypedRows(Credentials{URL: featureURL(srv)}, sql, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("rows=%d", len(rows))
	}
	_, offsets, _, _, _ := mock.snapshot()
	if strings.Join(offsets, ",") != "0,2,4" {
		t.Fatalf("offsets=%v", offsets)
	}
}

func TestFetchTypedRowsParameterFilters(t *testing.T) {
	mock := &arcServer{features: sampleFeatures()}
	srv := startArc(t, mock)
	sql := `SELECT "t"."stationcode", "t"."zicht" FROM "layers"."Temperatuur" "t" WHERE "t"."zicht" = $P{p_zicht} AND $X{IN, "t"."division", p_div} AND $X{NOTIN, "t"."region", p_region}`
	params := map[string]any{
		"p_zicht":  "100000",
		"p_div":    []string{"Mtn", "W N Cen"},
		"p_region": []any{"Pac", "NE"},
	}
	rows, err := FetchTypedRows(Credentials{URL: featureURL(srv)}, sql, params, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	wheres, _, fields, _, _ := mock.snapshot()
	if len(wheres) != 1 {
		t.Fatalf("queries=%v", wheres)
	}
	where := wheres[0]
	for _, want := range []string{`= 100000`, `'Mtn'`, `'W N Cen'`, `'Pac'`, `'NE'`, " NOT IN ", " IN "} {
		if !strings.Contains(where, want) {
			t.Fatalf("where %q missing %q", where, want)
		}
	}
	if strings.Contains(where, "'Mtn, W N Cen'") || strings.Contains(where, "'Pac, NE'") {
		t.Fatalf("flattened where: %s", where)
	}
	if len(fields) == 0 || strings.Contains(fields[0], "lat") {
		t.Fatalf("selected fields=%v", fields)
	}
}

func TestFetchTypedRowsEmptyResult(t *testing.T) {
	mock := &arcServer{features: nil}
	srv := startArc(t, mock)
	rows, err := FetchTypedRows(Credentials{URL: featureURL(srv)}, `SELECT "stationcode" FROM "layers"."Temperatuur"`, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if rows == nil || len(rows) != 0 {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestFetchTypedRowsRESTError(t *testing.T) {
	mock := &arcServer{features: sampleFeatures(), failMsg: "Unable to complete operation."}
	srv := startArc(t, mock)
	_, err := FetchTypedRows(Credentials{URL: featureURL(srv)}, weatherSQL(), nil, 10)
	if err == nil || !strings.Contains(err.Error(), "Unable to complete operation.") {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchTypedRowsAuthenticatedTokenRefresh(t *testing.T) {
	mock := &arcServer{features: sampleFeatures(), rejectOnce: true}
	srv := startArc(t, mock)
	rows, err := FetchTypedRows(Credentials{URL: featureURL(srv), Username: "user", Password: "s3cret"}, weatherSQL(), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0]["stationcode"] != int64(6275) {
		t.Fatalf("rows=%#v", rows)
	}
	_, _, _, tokens, tokenN := mock.snapshot()
	if tokenN < 2 {
		t.Fatalf("token posts=%d", tokenN)
	}
	if len(tokens) == 0 || tokens[len(tokens)-1] == "" {
		t.Fatalf("query token=%v", tokens)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	for _, pw := range mock.passwords {
		if pw != "s3cret" {
			t.Fatalf("token form password=%q", pw)
		}
	}
	for _, tok := range tokens {
		if strings.Contains(tok, "s3cret") {
			t.Fatal("password leaked into the feature query")
		}
	}
}

func TestDefaultFillLimitConstant(t *testing.T) {
	if DefaultFillLimit != 5000 {
		t.Fatalf("limit=%d", DefaultFillLimit)
	}
}

func TestQuoteDoesNotDropSpacedToken(t *testing.T) {
	got := paramValues(map[string]any{"p_div": []string{"Mtn", "W N Cen"}}, "p_div")
	if len(got) != 2 || got[0] != "Mtn" || got[1] != "W N Cen" {
		t.Fatalf("%#v", got)
	}
}
