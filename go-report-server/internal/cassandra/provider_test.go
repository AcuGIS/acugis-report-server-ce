package cassandra

import (
	"strings"
	"testing"
)

func TestContactPoints(t *testing.T) {
	cfg := Config{Host: "a.example.com, b.example.com;c.example.com", Port: 9042}
	hosts := ContactPoints(cfg)
	if len(hosts) != 3 {
		t.Fatalf("got %v", hosts)
	}
	if hosts[0] != "a.example.com" || hosts[2] != "c.example.com" {
		t.Fatalf("unexpected hosts: %v", hosts)
	}
}

func TestContactPointsDedupAndPortStrip(t *testing.T) {
	cfg := Config{Host: "host1:9042, host1, HOST1", Port: 9042}
	hosts := ContactPoints(cfg)
	if len(hosts) != 1 || hosts[0] != "host1" {
		t.Fatalf("got %v", hosts)
	}
}

func TestMapJavaClass(t *testing.T) {
	cases := map[string]string{
		"text":               "java.lang.String",
		"ascii":              "java.lang.String",
		"varchar":            "java.lang.String",
		"tinyint":            "java.lang.Byte",
		"smallint":           "java.lang.Short",
		"int":                "java.lang.Integer",
		"bigint":             "java.lang.Long",
		"counter":            "java.lang.Long",
		"varint":             "java.math.BigInteger",
		"decimal":            "java.math.BigDecimal",
		"float":              "java.lang.Float",
		"double":             "java.lang.Double",
		"boolean":            "java.lang.Boolean",
		"date":               "java.time.LocalDate",
		"timestamp":          "java.sql.Timestamp",
		"time":               "java.lang.Long",
		"uuid":               "java.util.UUID",
		"timeuuid":           "java.util.UUID",
		"inet":               "java.lang.String",
		"blob":               "[B",
		"duration":           "java.lang.String",
		"list<text>":         "java.lang.String",
		"set<int>":           "java.lang.String",
		"map<text, int>":     "java.lang.String",
		"frozen<list<text>>": "java.lang.String",
	}
	for in, want := range cases {
		if got := MapJavaClass(in); got != want {
			t.Errorf("MapJavaClass(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEnsureReadOnly(t *testing.T) {
	if err := ensureReadOnly("SELECT * FROM ks.t"); err != nil {
		t.Fatal(err)
	}
	if err := ensureReadOnly("  select a from t"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		"INSERT INTO t (a) VALUES (1)",
		"UPDATE t SET a=1",
		"DELETE FROM t WHERE a=1",
		"TRUNCATE t",
		"DROP TABLE t",
		"ALTER TABLE t ADD x int",
		"CREATE TABLE t (a int PRIMARY KEY)",
	} {
		if err := ensureReadOnly(bad); err == nil {
			t.Fatalf("expected reject for %q", bad)
		}
	}
}

func TestBindParametersSimple(t *testing.T) {
	bound, err := bindParameters(
		"SELECT customer_id FROM sales.customers_by_state WHERE state = $P{p_state}",
		map[string]any{"p_state": "Texas"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bound.CQL, "?") || strings.Contains(bound.CQL, "$P{") {
		t.Fatalf("CQL not rewritten: %s", bound.CQL)
	}
	if len(bound.Values) != 1 || bound.Values[0] != "Texas" {
		t.Fatalf("values=%v", bound.Values)
	}
}

func TestBindParametersMultiValueIN(t *testing.T) {
	bound, err := bindParameters(
		"SELECT * FROM t WHERE state IN ($P{p_states})",
		map[string]any{"p_states": []any{"Texas", "Florida"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM t WHERE state IN (?, ?)"; bound.CQL != want {
		t.Fatalf("got %q", bound.CQL)
	}
	if len(bound.Values) != 2 {
		t.Fatalf("values=%v", bound.Values)
	}
}

func TestBindParametersXin(t *testing.T) {
	bound, err := bindParameters(
		"SELECT * FROM t WHERE $X{IN, state, p_states}",
		map[string]any{"p_states": []string{"Texas", "Florida"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM t WHERE state IN (?, ?)"; bound.CQL != want {
		t.Fatalf("got %q", bound.CQL)
	}
}

func TestWrapLimitForPreview(t *testing.T) {
	got := wrapLimitForPreview("SELECT * FROM t", 100)
	if got != "SELECT * FROM t LIMIT 100" {
		t.Fatalf("got %q", got)
	}
	unchanged := wrapLimitForPreview("SELECT * FROM t LIMIT 10", 100)
	if unchanged != "SELECT * FROM t LIMIT 10" {
		t.Fatalf("should not modify user LIMIT: %q", unchanged)
	}
}

func TestBuildSelectCQL(t *testing.T) {
	got := BuildSelectCQL("sales", "customers", []string{"customer_id", "state"})
	if got != "SELECT customer_id, state\nFROM sales.customers" {
		t.Fatalf("got %q", got)
	}
}

func TestFriendlyErrMessages(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"authentication failed: bad credentials", "authentication failed"},
		{"no hosts available in the pool", "no hosts available"},
		{"lookup cassandra.bad: no such host", "unable to contact Cassandra host"},
		{"i/o timeout dialing", "connection timed out"},
		{"Keyspace 'foo' does not exist", "keyspace does not exist"},
	}
	for _, tc := range cases {
		err := friendlyErr(errString(tc.in))
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
			t.Errorf("friendlyErr(%q)=%v want contain %q", tc.in, err, tc.want)
		}
	}
}

func TestIsSystemKeyspace(t *testing.T) {
	for _, ks := range []string{"system", "system_schema", "system_auth", "system_traces", "system_views"} {
		if !isSystemKeyspace(ks) {
			t.Errorf("%s should be system", ks)
		}
	}
	if isSystemKeyspace("sales") {
		t.Fatal("sales should not be system")
	}
}

func TestKindLabels(t *testing.T) {
	meta := []columnMeta{
		{Name: "pk", Type: "uuid", Kind: "partition_key", Position: 0},
		{Name: "ck", Type: "text", Kind: "clustering", Position: 0},
		{Name: "s", Type: "text", Kind: "static", Position: 0},
		{Name: "r", Type: "int", Kind: "regular", Position: 0},
	}
	cols := toColumnInfos(meta)
	if !cols[0].IsPrimaryKey {
		t.Fatal("partition key should set IsPrimaryKey")
	}
	if !strings.Contains(cols[1].UDTName, "clustering") {
		t.Fatalf("clustering label missing: %s", cols[1].UDTName)
	}
	if !strings.Contains(cols[2].UDTName, "static") {
		t.Fatalf("static label missing: %s", cols[2].UDTName)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
