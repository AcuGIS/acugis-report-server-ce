package contentresource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeType(t *testing.T) {
	got, err := NormalizeType(" Public_Key ")
	if err != nil || got != TypePublicKey {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizeType("  "); err != ErrBadType {
		t.Fatalf("expected ErrBadType, got %v", err)
	}
	// Future types allowed without schema change.
	got, err = NormalizeType("geojson")
	if err != nil || got != "geojson" {
		t.Fatalf("future type: %q %v", got, err)
	}
}

func TestStorageFilenameAndExtension(t *testing.T) {
	if got := StorageFilename("abc-123", ".pem"); got != "abc-123.pem" {
		t.Fatalf("storage=%q", got)
	}
	if got := StorageFilename("abc-123", "pem"); got != "abc-123.pem" {
		t.Fatalf("storage without dot=%q", got)
	}
	if got := ExtensionFromFilename("rsa_key.pub"); got != ".pub" {
		t.Fatalf("ext=%q", got)
	}
	if got := ExtensionFromFilename("noext"); got != "" {
		t.Fatalf("noext=%q", got)
	}
	if got := DefaultExtensionForType(TypePrivateKey); got != ".pem" {
		t.Fatalf("default=%q", got)
	}
}

func TestFileStoreWriteReadHash(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	name := StorageFilename("41c2d2d4-97b2-4fd3-86c7-f86d9e4a24b2", ".pem")
	data := []byte("-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n")
	sha, err := store.Write(name, data)
	if err != nil {
		t.Fatal(err)
	}
	if sha != HashBytes(data) {
		t.Fatalf("sha mismatch %s vs %s", sha, HashBytes(data))
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("read mismatch")
	}
	// Duplicate filenames never conflict: same original name, different UUID storage.
	name2 := StorageFilename("8a21fa91-1d9d-4d98-b82a-b0d2b10db87a", ".pem")
	if _, err := store.Write(name2, data); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(name); err != ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestTypeLabelAndPaste(t *testing.T) {
	if !SupportsPaste(TypePublicKey) {
		t.Fatal("public key should support paste")
	}
	if SupportsPaste(TypeGenericFile) {
		t.Fatal("generic file should not prefer paste")
	}
	if !strings.Contains(TypeLabel(TypeCACertificate), "CA") {
		t.Fatal(TypeLabel(TypeCACertificate))
	}
}
