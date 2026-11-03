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
	"github.com/xuri/excelize/v2"
)

func TestPrepareExcelCollectionScheduledResolvesPublishedResource(t *testing.T) {
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
	xlsxPath := filepath.Join(pathsDir, "sales", "data", "sales.xlsx")
	if err := os.MkdirAll(filepath.Dir(xlsxPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f := excelize.NewFile()
	_, _ = f.NewSheet("Orders")
	_ = f.SetCellValue("Orders", "A1", "Customer")
	_ = f.SetCellValue("Orders", "B1", "Region")
	_ = f.SetCellValue("Orders", "C1", "Quantity")
	_ = f.SetCellValue("Orders", "A2", "Ada")
	_ = f.SetCellValue("Orders", "B2", "North")
	_ = f.SetCellValue("Orders", "C2", 4)
	_ = f.SetCellValue("Orders", "A3", "Bob")
	_ = f.SetCellValue("Orders", "B3", "South")
	_ = f.SetCellValue("Orders", "C3", 7)
	if err := f.SaveAs(xlsxPath); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="excel"/>
  <property name="acugis.datasource.resource" value="data/sales.xlsx"/>
  <property name="acugis.datasource.excel.sheet" value="Orders"/>
  <query language="collection"><![CDATA[SELECT "Customer", "Quantity" FROM "data"."Orders" WHERE "Region" = $P{region}]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "orders.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "orders.resource-base"), []byte("sales\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"name":"Orders","report":"orders","datasource":"","type":"jasper"}`
	if err := os.WriteFile(filepath.Join(jsonDir, "orders.report.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{
		Reports:       reports.NewStore(jsonDir, "", jrxmlDir),
		ResourcePaths: pathsDir,
	}
	ok, method, _, ct, rdr, err := runner.prepareExcelCollection(context.Background(), "orders", "", "region=North&_repFormat=pdf&_dataSource=")
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
	if len(rows) != 1 || rows[0]["Customer"] != "Ada" || rows[0]["Quantity"] != float64(4) {
		t.Fatalf("rows %+v", rows)
	}
}
