package resourcefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestLogicalPathsAndConfinement(t *testing.T) {
	st, paths := newResourceFixture(t, "report-a", "folder-a")
	secret := filepath.Join(paths, "folder-a", "credentials", "key.pem")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("TEST_PRIVATE_KEY_DO_NOT_EXPOSE"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := []string{
		"../secret",
		"images/../../secret",
		"/data/file",
		`C:\secret`,
		`\\server\share`,
		"file:///etc/passwd",
		"images/logo.png\x00.png",
		"credentials/key.pem",
		"secrets/token",
		"resources/11111111-1111-1111-1111-111111111111",
	}
	for _, logical := range bad {
		if _, err := st.Resolve("report-a", logical); err == nil {
			t.Fatalf("accepted %q", logical)
		}
	}
	if _, err := CleanLogical("images/company/logo.png"); err != nil {
		t.Fatal(err)
	}
	info, err := st.Put("report-a", "images/company/logo.png", []byte("png"), true)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != "images/company/logo.png" || strings.Contains(info.Path, `\`) {
		t.Fatalf("logical path %q", info.Path)
	}
	full, err := st.Resolve("report-a", "images/company/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(full), "/folder-a/images/company/logo.png") {
		t.Fatalf("resolved outside resource base: %s", full)
	}
	list, err := st.List("report-a", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if strings.Contains(item.Path, "credentials") || strings.Contains(item.Path, "TEST_PRIVATE") {
			t.Fatalf("listed forbidden resource %#v", item)
		}
		if filepath.IsAbs(item.Path) {
			t.Fatalf("physical path in list %q", item.Path)
		}
	}
	if _, _, err := st.Open("report-a", "credentials/key.pem"); err == nil {
		t.Fatal("opened credential path")
	}
}

func TestSymlinkRejected(t *testing.T) {
	st, paths := newResourceFixture(t, "report-a", "folder-a")
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(paths, "folder-a", "images")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "escape.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not permitted: %v", err)
	}
	if _, err := st.Resolve("report-a", "images/escape.png"); err == nil {
		t.Fatal("followed symlink")
	}
	list, err := st.List("report-a", "images")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.Name == "escape.png" {
			t.Fatal("listed symlink")
		}
	}
}

func TestCrossReportRoots(t *testing.T) {
	jrxml := t.TempDir()
	paths := t.TempDir()
	writeBase(t, jrxml, "report-a", "folder-a")
	writeBase(t, jrxml, "report-b", "folder-b")
	st := NewStore(jrxml, paths)
	if _, err := st.Put("report-b", "images/logo.png", []byte("bee"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put("report-a", "images/logo.png", []byte("aaa"), false); err != nil {
		t.Fatal(err)
	}
	f, info, err := st.Open("report-a", "images/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, _ := f.Read(buf)
	f.Close()
	if string(buf[:n]) != "aaa" {
		t.Fatalf("report A saw %q", buf[:n])
	}
	if info.Path != "images/logo.png" {
		t.Fatal(info.Path)
	}
	b, _, err := st.Open("report-b", "images/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	n, _ = b.Read(buf)
	b.Close()
	if string(buf[:n]) != "bee" {
		t.Fatalf("report B changed to %q", buf[:n])
	}
}

func TestPutConflictAndReplace(t *testing.T) {
	st, _ := newResourceFixture(t, "report-a", "folder-a")
	if _, err := st.Put("report-a", "data/cities.sqlite", []byte("one"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put("report-a", "data/cities.sqlite", []byte("two"), false); err != ErrConflict {
		t.Fatalf("overwrite false: %v", err)
	}
	info, err := st.Put("report-a", "data/cities.sqlite", []byte("two"), true)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 3 {
		t.Fatalf("size %d", info.Size)
	}
}

func TestRelocateIsolatedMovesReportDirectory(t *testing.T) {
	jrxml := t.TempDir()
	paths := t.TempDir()
	st := NewStore(jrxml, paths)
	from := filepath.Join(paths, "DavidG", "testingone", "images")
	if err := os.MkdirAll(from, 0o755); err != nil {
		t.Fatal(err)
	}
	logo := filepath.Join(from, "logo.png")
	if err := os.WriteFile(logo, []byte("per-report"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.RelocateIsolated("DavidG/testingone", "Finance/testingone"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(paths, "Finance", "testingone", "images", "logo.png")
	if string(mustReadFile(t, moved)) != "per-report" {
		t.Fatal("file was not moved")
	}
	if _, err := os.Stat(logo); !os.IsNotExist(err) {
		t.Fatal("source file remained")
	}
	if err := st.RelocateIsolated("missing/testingone", "Other/testingone"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths, "Taken", "testingone"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.RelocateIsolated("Finance/testingone", "Taken/testingone"); err != ErrConflict {
		t.Fatalf("conflict: %v", err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMissingBaseUntouched(t *testing.T) {
	jrxml := t.TempDir()
	paths := t.TempDir()
	st := NewStore(jrxml, paths)
	if err := reports.NewStore("", "", jrxml).SaveResourceBase("report-a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ResourceBase("report-a"); err != ErrNoBase {
		t.Fatalf("home base: %v", err)
	}
	if _, err := st.List("missing", ""); err != ErrNoBase {
		t.Fatalf("missing: %v", err)
	}
}

func newResourceFixture(t *testing.T, key, base string) (*Store, string) {
	t.Helper()
	jrxml := t.TempDir()
	paths := t.TempDir()
	writeBase(t, jrxml, key, base)
	return NewStore(jrxml, paths), paths
}

func writeBase(t *testing.T, jrxml, key, base string) {
	t.Helper()
	if err := reports.NewStore("", "", jrxml).SaveResourceBase(key, base); err != nil {
		t.Fatal(err)
	}
}
