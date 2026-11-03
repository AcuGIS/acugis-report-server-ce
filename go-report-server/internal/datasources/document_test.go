package datasources

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/collectionfile"
)

func TestPrepareDocumentRejectsTraversalAndSize(t *testing.T) {
	root := t.TempDir()
	ds := Datasource{Name: "Orders", DBType: "csv", Options: map[string]string{OptionDelimiter: "comma"}}
	err := PrepareDocument(&ds, DocumentInput{
		Root:       root,
		UploadName: `..\..\evil.csv`,
		Upload:     []byte("name\nAda\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid document name") {
		t.Fatalf("traversal name: %v", err)
	}
	ds = Datasource{Name: "Orders", DBType: "csv"}
	err = PrepareDocument(&ds, DocumentInput{Root: root, ClaimedPath: `../../etc/passwd`})
	if err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Fatalf("claimed path: %v", err)
	}
	err = validateUpload("csv", "orders.csv", bytes.Repeat([]byte("a"), collectionfile.MaxBytes+1))
	if err == nil || !strings.Contains(err.Error(), "32 MB") {
		t.Fatalf("size: %v", err)
	}
	err = validateUpload("csv", "notes.exe", []byte("name\nAda\n"))
	if err == nil {
		t.Fatal("expected extension rejection")
	}
}

func TestPrepareCSVUsesManagedPath(t *testing.T) {
	root := t.TempDir()
	ds := Datasource{Name: "Orders", DBType: "csv", Options: map[string]string{OptionDelimiter: "comma"}}
	body := []byte("name,city\nAda,Paris\n")
	if err := PrepareDocument(&ds, DocumentInput{Root: root, UploadName: "orders.csv", Upload: body}); err != nil {
		t.Fatal(err)
	}
	if ds.Host != "" || ds.Username != "" || ds.Password != "" || ds.Port != 0 {
		t.Fatalf("jdbc fields leaked: %+v", ds)
	}
	if ds.DBName != ds.Option(OptionResourceFile) || !strings.HasPrefix(ds.DBName, "datasources/") || !strings.HasSuffix(ds.DBName, "/orders.csv") {
		t.Fatalf("path: %+v", ds)
	}
	if strings.Contains(ds.DBName, "..") {
		t.Fatal(ds.DBName)
	}
	abs := filepath.Join(root, filepath.FromSlash(ds.DBName))
	got, err := os.ReadFile(abs)
	if err != nil || string(got) != string(body) {
		t.Fatalf("stored %q %v", got, err)
	}
	if ds.Option(OptionDelimiter) != "comma" {
		t.Fatal(ds.Options)
	}
}

func TestJSONURLRecord(t *testing.T) {
	ds := Datasource{Name: "Remote", DBType: "json", Options: map[string]string{
		OptionJSONSource: JSONSourceURL,
		OptionJSONURL:    "https://example.com/orders.json",
		OptionCollection: "root",
	}}
	if err := PrepareDocument(&ds, DocumentInput{}); err != nil {
		t.Fatal(err)
	}
	if ds.DBName != "" || ds.Option(OptionResourceFile) != "" || ds.Option(OptionJSONSource) != JSONSourceURL {
		t.Fatalf("%+v", ds)
	}
	if ds.Option(OptionJSONURL) != "https://example.com/orders.json" || ds.Option(OptionCollection) != "root" {
		t.Fatal(ds.Options)
	}
	bad := Datasource{Name: "Local", DBType: "json", Options: map[string]string{
		OptionJSONSource: JSONSourceURL,
		OptionJSONURL:    "http://127.0.0.1/orders.json",
	}}
	if err := PrepareDocument(&bad, DocumentInput{}); err == nil {
		t.Fatal("loopback JSON URL was accepted")
	}
}
