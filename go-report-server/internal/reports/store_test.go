package reports_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestStoreListAndGet(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "alpha.report.json", `{
		"name": "Alpha Report",
		"report": "alpha",
		"datasource": "DS1",
		"description": "First",
		"type": "jasper",
		"format": "html",
		"parameters": [{"name": "p_x", "type": "text", "label": "X"}]
	}`)
	writeReport(t, dir, "beta.report.json", `{
		"name": "Beta Report",
		"report": "",
		"datasourceName": "DS2",
		"summary": "Second",
		"type": "map_report"
	}`)

	store := reports.NewStore(dir, "", "")
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	// Sorted by name: Alpha, Beta
	if list[0].Key != "alpha" || list[0].Name != "Alpha Report" {
		t.Fatalf("first=%+v", list[0])
	}
	if list[1].Key != "beta" || list[1].Type != "map" || list[1].Datasource != "DS2" {
		t.Fatalf("second=%+v", list[1])
	}

	def, err := store.Get("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if def.DisplayName() != "Alpha Report" || len(def.Parameters()) != 1 {
		t.Fatalf("def=%+v params=%v", def, def.Parameters())
	}
	if !store.Exists("alpha") {
		t.Fatal("exists")
	}
	if store.Exists("missing") {
		t.Fatal("missing should not exist")
	}
	_, err = store.Get("!!!")
	if err != reports.ErrInvalidKey {
		t.Fatalf("err=%v", err)
	}
	_, err = store.Get("nope")
	if err != reports.ErrNotFound {
		t.Fatalf("err=%v", err)
	}
}

func writeReport(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
