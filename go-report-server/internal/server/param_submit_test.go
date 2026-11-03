package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReportEntryPointsSubmitBlankTextParameters(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	repo := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	pages := []string{
		filepath.Join(repo, "go-report-server", "internal", "server", "viewer.html"),
		filepath.Join(repo, "go-report-server", "internal", "server", "report_runner.html"),
		filepath.Join(repo, "reports", "view.php"),
		filepath.Join(repo, "reports", "report_runner.php"),
	}
	for _, path := range pages {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, want := range []string{
			`../assets/js/report-param-submit.js`,
			`JrsReportParams.scalarQueryValue`,
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
		if strings.Contains(text, "if (val === '')") || strings.Contains(text, "if (value === '') {\n        return;") {
			t.Fatalf("%s still drops every blank parameter", path)
		}
	}

	for _, path := range []string{
		filepath.Join(repo, "go-report-server", "internal", "server", "viewer.html"),
		filepath.Join(repo, "reports", "view.php"),
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "JrsReportParams.appendScalarToUrl") {
			t.Fatalf("%s does not append a present empty string", path)
		}
	}
}
