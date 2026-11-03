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

func TestPrepareJSONCollectionScheduledResolvesPublishedResource(t *testing.T) {
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
	docPath := filepath.Join(pathsDir, "sales", "data", "customers.json")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `[{"customer":"Ada","region":"North","quantity":4},{"customer":"Bob","region":"South","quantity":7}]`
	if err := os.WriteFile(docPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="json"/>
  <property name="acugis.datasource.json.source" value="file"/>
  <property name="acugis.datasource.resource" value="data/customers.json"/>
  <property name="acugis.datasource.json.collection" value="root"/>
  <query language="collection"><![CDATA[SELECT "customer", "quantity" FROM "data"."root" WHERE "region" = $P{region}]]></query>
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
	ok, method, _, ct, rdr, err := runner.prepareJSONCollection(context.Background(), "customers", "", "region=North&_repFormat=pdf&_dataSource=")
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
	if len(rows) != 1 || rows[0]["customer"] != "Ada" || rows[0]["quantity"] != float64(4) {
		t.Fatalf("rows %+v", rows)
	}
	if strings.Contains(string(raw), docPath) {
		t.Fatal("physical path in scheduled body")
	}
}
