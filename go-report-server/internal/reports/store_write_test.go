package reports_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestStoreSaveAndDelete(t *testing.T) {
	jsonDir := t.TempDir()
	jrxmlDir := t.TempDir()
	thumbDir := t.TempDir()
	store := reports.NewStore(jsonDir, thumbDir, jrxmlDir)

	raw := map[string]any{
		"name":       "Demo",
		"report":     "demo",
		"datasource": "DS",
		"type":       "jasper",
		"format":     "html",
		"parameters":  []any{},
	}
	if err := store.SaveJSON("demo", raw); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJRXML("demo", []byte(`<?xml version="1.0"?><jasperReport/>`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveJasper("demo", []byte("jasper-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveThumbnail("demo", []byte("png-bytes")); err != nil {
		t.Fatal(err)
	}

	def, err := store.Get("demo")
	if err != nil {
		t.Fatal(err)
	}
	if def.DisplayName() != "Demo" {
		t.Fatalf("name=%s", def.DisplayName())
	}
	jrxml, err := os.ReadFile(filepath.Join(jrxmlDir, "demo.jrxml"))
	if err != nil || !strings.Contains(string(jrxml), "jasperReport") {
		t.Fatalf("jrxml=%v err=%v", jrxml, err)
	}
	if !store.Exists("demo") {
		t.Fatal("expected exists")
	}

	note := store.CompileJRXML("demo")
	if note == "" {
		t.Fatal("expected compile note")
	}
	res := store.CompileJRXMLResult("demo")
	if res.Message == "" {
		t.Fatal("expected compile result message")
	}
	if err := store.RemoveJasper("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(jrxmlDir, "demo.jasper")); !os.IsNotExist(err) {
		t.Fatalf("expected .jasper removed, err=%v", err)
	}
	// Re-write jasper for delete coverage below.
	if err := store.SaveJasper("demo", []byte("jasper-bytes")); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete("demo"); err != nil {
		t.Fatal(err)
	}
	if store.Exists("demo") {
		t.Fatal("expected deleted")
	}
	for _, path := range []string{
		filepath.Join(jsonDir, "demo.report.json"),
		filepath.Join(jrxmlDir, "demo.jrxml"),
		filepath.Join(jrxmlDir, "demo.jasper"),
		filepath.Join(thumbDir, "demo.png"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected missing %s err=%v", path, err)
		}
	}
}
