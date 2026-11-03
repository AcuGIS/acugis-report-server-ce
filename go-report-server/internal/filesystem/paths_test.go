package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

func TestValidateRootsExistingAndMissing(t *testing.T) {
	root := t.TempDir()
	jsonDir := filepath.Join(root, "reports", "json")
	jrxmlDir := filepath.Join(root, "docker", "report-server", "reports")
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := filepath.Join(root, "context.xml")
	if err := os.WriteFile(ctx, []byte("<Context/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New(config.PathConfig{
		AppRoot:            root,
		ReportJSONDir:      jsonDir,
		JRXMLDir:           jrxmlDir,
		ThumbnailDir:       filepath.Join(root, "missing-thumbs"),
		ScheduleDir:        filepath.Join(root, "schedules"),
		ScheduleOutputDir:  filepath.Join(root, "schedules", "output"),
		GeneratedReportDir: filepath.Join(root, "schedules", "output"),
		EmailTmplDir:       filepath.Join(root, "email_tmpl"),
		JasperContextPath:  ctx,
	})

	statuses := p.ValidateRoots()
	byName := map[string]PathStatus{}
	for _, s := range statuses {
		byName[s.Name] = s
	}
	if !byName["report_json_dir"].Exists || !byName["jrxml_dir"].Exists {
		t.Fatalf("expected required dirs present: %+v", byName)
	}
	if byName["thumbnail_dir"].Exists {
		t.Fatal("thumbnail should be missing")
	}
	if !byName["jasper_context_path"].Exists || !byName["jasper_context_path"].IsFile {
		t.Fatal("context.xml should exist as file")
	}
	if !p.AllRequiredPresent() {
		t.Fatal("required paths should be present")
	}

	p2 := New(config.PathConfig{
		ReportJSONDir: filepath.Join(root, "nope"),
		JRXMLDir:      filepath.Join(root, "also-nope"),
	})
	if p2.AllRequiredPresent() {
		t.Fatal("missing required dirs should fail")
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "opt", "acugis-report-server", "reports", "json")

	ok, err := SafeJoin(root, "demo.report.json")
	if err != nil {
		t.Fatalf("expected ok join: %v", err)
	}
	if filepath.Base(ok) != "demo.report.json" {
		t.Fatalf("unexpected join: %s", ok)
	}

	if _, err := SafeJoin(root, "../etc/passwd"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := SafeJoin(root, ".."); err == nil {
		t.Fatal("expected .. rejection")
	}
	absChild := string(filepath.Separator) + filepath.Join("etc", "passwd")
	if _, err := SafeJoin(root, absChild); err == nil {
		t.Fatalf("expected absolute child rejection for %q", absChild)
	}
	if _, err := SafeJoin(root, "/etc/passwd"); err == nil {
		t.Fatal("expected unix-style absolute child rejection")
	}
}

func TestSafeJoinRelativeAndAbsoluteRoot(t *testing.T) {
	t.Parallel()
	relRoot := filepath.Join("reports", "json")
	joined, err := SafeJoin(relRoot, "a.report.json")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(filepath.Join(relRoot, "a.report.json"))
	if joined != want {
		t.Fatalf("got %s want %s", joined, want)
	}

	absRoot := t.TempDir()
	joined, err = SafeJoin(absRoot, "nested"+string(filepath.Separator)+"file.json")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(joined) {
		t.Fatalf("expected absolute result, got %s", joined)
	}
}
