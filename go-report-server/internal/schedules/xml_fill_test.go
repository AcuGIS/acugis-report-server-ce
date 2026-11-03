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

func TestPrepareXMLCollectionScheduledResolvesPublishedResource(t *testing.T) {
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
	xmlPath := filepath.Join(pathsDir, "sales", "data", "customers.xml")
	if err := os.MkdirAll(filepath.Dir(xmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `<?xml version="1.0" encoding="UTF-8"?>
<customers>
  <customer><customer>Ada</customer><region>North</region><quantity>4</quantity></customer>
  <customer><customer>Bob</customer><region>South</region><quantity>7</quantity></customer>
</customers>`
	if err := os.WriteFile(xmlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="xml"/>
  <property name="acugis.datasource.resource" value="data/customers.xml"/>
  <property name="acugis.datasource.xml.collection" value="/customers/customer"/>
  <query language="collection"><![CDATA[SELECT "customer", "quantity" FROM "data"."/customers/customer" WHERE "region" = $P{region}]]></query>
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
	ok, method, _, ct, rdr, err := runner.prepareXMLCollection(context.Background(), "customers", "", "region=North&_repFormat=pdf&_dataSource=")
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
}
