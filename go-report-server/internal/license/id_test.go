package license

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallationIDGenerateFormatPersist(t *testing.T) {
	dir := t.TempDir()
	id, err := LoadOrCreateInstallationID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidInstallationID(id) {
		t.Fatalf("id %q is not lowercase UUIDv4", id)
	}
	if id != strings.ToLower(id) {
		t.Fatalf("id not lowercase: %s", id)
	}
	again, err := LoadOrCreateInstallationID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again != id {
		t.Fatalf("reload changed id %s -> %s", id, again)
	}
	raw, err := os.ReadFile(filepath.Join(dir, installationFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != id {
		t.Fatalf("file %q", raw)
	}
}

func TestInstallationIDMalformedNotReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, installationFileName)
	if err := os.WriteFile(path, []byte("not-a-uuid\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateInstallationID(dir); err == nil {
		t.Fatal("expected malformed identity error")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "not-a-uuid\n" {
		t.Fatalf("malformed identity was replaced: %q", raw)
	}
}

func TestNewInstallationIDNotDerived(t *testing.T) {
	a, err := NewInstallationID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewInstallationID()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !ValidInstallationID(a) || !ValidInstallationID(b) {
		t.Fatalf("a=%s b=%s", a, b)
	}
}
