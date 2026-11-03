package arcgis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConnectionsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "arcgis-connections.properties")
	err := WriteConnectionsFile(path, []Connection{
		{Name: "NeighborhoodsFS", URL: "https://services.example.com/FeatureServer/", Username: "user", Password: "s3cret"},
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
		"NeighborhoodsFS.url=https://services.example.com/FeatureServer",
		"NeighborhoodsFS.username=user",
		"NeighborhoodsFS.password=s3cret",
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
