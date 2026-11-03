package datasources

import "testing"

func TestQCartaDBTypeAllowList(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"postgresql", "mysql", "sqlserver", "sqlite", "oracle"} {
		if !AllowedDBType(key, QCartaDBTypeKeys) {
			t.Fatalf("expected %s to be allowed", key)
		}
	}
	for _, key := range []string{"duckdb", "snowflake", "cassandra", "bigquery", "db2", "arcgis", "geoserver"} {
		if AllowedDBType(key, QCartaDBTypeKeys) {
			t.Fatalf("did not expect %s in the QCarta Phase 1 set", key)
		}
	}
	metas := DBTypeMetasFor(QCartaDBTypeKeys)
	if len(metas) != 5 {
		t.Fatalf("QCarta metas=%d want 5", len(metas))
	}
	if !AllowedDBType("postgresql", nil) || !AllowedDBType("duckdb", nil) {
		t.Fatal("empty allow-list must keep standalone engines")
	}
}
