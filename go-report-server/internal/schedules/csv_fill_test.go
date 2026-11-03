package schedules

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestPrepareCSVCollectionScheduledResolvesPublishedResource(t *testing.T) {
	dir := t.TempDir()
	jrxmlDir := filepath.Join(dir, "jrxml")
	jsonDir := filepath.Join(dir, "json")
	pathsDir := filepath.Join(dir, "resource-paths")
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	csvPath := filepath.Join(pathsDir, "sales", "data", "customers.csv")
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "customer,region,quantity,amount,active,order_date\nAda,North,2,19.50,true,2024-03-01\nBea,South,3,4.00,false,2024-03-02\n"
	if err := os.WriteFile(csvPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="csv"/>
  <property name="acugis.datasource.resource" value="data/customers.csv"/>
  <property name="acugis.datasource.delimiter" value="comma"/>
  <query language="collection"><![CDATA[SELECT "customer", "quantity" FROM "data"."customers" WHERE "region" = $P{region}]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "customers.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "customers.resource-base"), []byte("sales\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"name":"Customers","report":"customers","datasource":"","type":"jasper"}`
	if err := os.WriteFile(filepath.Join(jsonDir, "customers.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{
		Reports:       reports.NewStore(jsonDir, "", jrxmlDir),
		ResourcePaths: pathsDir,
	}
	ok, method, _, ct, rdr, err := runner.prepareCSVCollection(context.Background(), "customers", "", "region=North&_repFormat=pdf&_dataSource=")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if method != "POST" || !strings.Contains(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("method %s ct %s", method, ct)
	}
	raw, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("_dataSource") != "" {
		t.Fatalf("datasource leaked %q", vals.Get("_dataSource"))
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(vals.Get("_collectionData")), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["customer"] != "Ada" || rows[0]["quantity"] != float64(2) {
		t.Fatalf("rows %+v", rows)
	}
	if strings.Contains(string(raw), csvPath) {
		t.Fatal("physical path in scheduled body")
	}
}
