package datasourcefs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func TestMapDatasourceKeepsSecretsOffJSON(t *testing.T) {
	ds := datasources.Datasource{
		ID:       17,
		Name:     "Production PostGIS",
		DBType:   "postgres",
		Host:     "db.internal",
		Port:     5432,
		DBName:   "gis",
		Username: "gis",
		Password: "TEST_PASSWORD_DO_NOT_LEAK",
		Options: map[string]string{
			"credentialsResourceUuid": "uuid-1",
			"mode":                    "files",
		},
	}
	rec := MapDatasource(ds, []byte("TEST_PRIVATE_KEY_DO_NOT_LEAK"), []byte("TEST_BIGQUERY_JSON_DO_NOT_LEAK"))
	if rec.DBType != "postgresql" {
		t.Fatalf("db type %s", rec.DBType)
	}
	if rec.Options["credentialsResourceUuid"] != "" {
		t.Fatal("credential resource uuid was copied into options")
	}
	if rec.Options["mode"] != "files" {
		t.Fatal("non-secret option was dropped")
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, secret := range []string{
		"TEST_PASSWORD_DO_NOT_LEAK",
		"TEST_PRIVATE_KEY_DO_NOT_LEAK",
		"TEST_BIGQUERY_JSON_DO_NOT_LEAK",
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("serialized record contains %s: %s", secret, body)
		}
	}
}

func TestMapOracleSIDAndDuckDBHeaders(t *testing.T) {
	ora := MapDatasource(datasources.Datasource{
		ID: 4, Name: "Ora", DBType: "oracle", Host: "ora", Port: 1521, DBName: "sid:ORCL", Username: "app", Password: "TEST_PASSWORD_DO_NOT_LEAK",
	}, nil, nil)
	if ora.Database != "sid:ORCL" || ora.DBType != "oracle" {
		t.Fatalf("oracle record %+v", ora)
	}
	duck := MapDatasource(datasources.Datasource{
		ID: 8, Name: "Remote", DBType: "duckdb", Password: `{"X-API-Key":"TEST_BEARER_TOKEN_DO_NOT_LEAK"}`,
		Options: map[string]string{
			"mode":           "files",
			"authType":       "headers",
			"httpHeaderKeys": `["X-API-Key"]`,
			"files":          `[{"alias":"t","source":"https://example.com/a.parquet","format":"parquet"}]`,
		},
	}, nil, nil)
	if strings.Contains(duck.Options["files"], "TEST_BEARER_TOKEN_DO_NOT_LEAK") {
		t.Fatal("header secret landed in options")
	}
	if !strings.Contains(duck.Password, "TEST_BEARER_TOKEN_DO_NOT_LEAK") {
		t.Fatal("header secret was dropped from the server-side password")
	}
	raw, _ := json.Marshal(duck)
	if strings.Contains(string(raw), "TEST_BEARER_TOKEN_DO_NOT_LEAK") {
		t.Fatal(string(raw))
	}
}

func TestMapConnections(t *testing.T) {
	gs := MapGeoServer(3, "GS", "https://gs.example.com/geoserver/", "admin", "TEST_PASSWORD_DO_NOT_LEAK")
	if gs.DBType != "jasper_geoserver" || gs.Host != "https://gs.example.com/geoserver" {
		t.Fatalf("%+v", gs)
	}
	q := MapQGIS(5, "Planning", "https://qgis.example.com/", "ada", "TEST_PASSWORD_DO_NOT_LEAK", "planning.qgz")
	if q.DBType != "jasper_qgis" || q.Options["project"] != "planning.qgz" {
		t.Fatalf("%+v", q)
	}
	a := MapArcGIS(9, "Features", "https://services.example.com/arcgis/rest/services/C/FeatureServer", "u", "TEST_PASSWORD_DO_NOT_LEAK")
	if a.DBType != "jasper_arcgis" || a.Registry != RegistryArcGIS {
		t.Fatalf("%+v", a)
	}
}

func TestOpenRequiresURL(t *testing.T) {
	if _, err := Open(t.Context(), Config{}); err != ErrUnavailable {
		t.Fatalf("err %v", err)
	}
}
