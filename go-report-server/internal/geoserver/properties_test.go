package geoserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConnectionsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geoserver-connections.properties")
	err := WriteConnectionsFile(path, []Connection{
		{Name: "ProdGS", URL: "https://gs.example/geoserver/", Username: "admin", Password: "s3cret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"ProdGS.url=https://gs.example/geoserver",
		"ProdGS.username=admin",
		"ProdGS.password=s3cret",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

func TestPropEscape(t *testing.T) {
	got := propEscape("a\\b\nc")
	want := `a\\b\nc`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
