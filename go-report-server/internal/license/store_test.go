package license

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndReloadLicense(t *testing.T) {
	pub, priv := testKey(t)
	dir := t.TempDir()
	token := signPayload(t, priv, professionalPayload())
	v := testVerifier(pub, fixedNow())
	if err := SaveVerifiedLicense(dir, token, v, testInstallID()); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLicenseToken(dir)
	if err != nil || got != token {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := ParseAndVerify(got, v, testInstallID()); err != nil {
		t.Fatal(err)
	}
}

func TestUnverifiedReplacementNotPersisted(t *testing.T) {
	pub, priv := testKey(t)
	dir := t.TempDir()
	token := signPayload(t, priv, professionalPayload())
	v := testVerifier(pub, fixedNow())
	if err := SaveVerifiedLicense(dir, token, v, testInstallID()); err != nil {
		t.Fatal(err)
	}
	if err := SaveVerifiedLicense(dir, "ACL1.bad.sig", v, testInstallID()); err == nil {
		t.Fatal("expected reject")
	}
	got, err := ReadLicenseToken(dir)
	if err != nil || got != token {
		t.Fatalf("old token lost: %q %v", got, err)
	}
}

func TestFailedReplacementLeavesOldToken(t *testing.T) {
	pub, priv := testKey(t)
	dir := t.TempDir()
	old := signPayload(t, priv, professionalPayload())
	v := testVerifier(pub, fixedNow())
	if err := SaveVerifiedLicense(dir, old, v, testInstallID()); err != nil {
		t.Fatal(err)
	}
	newer := professionalPayload()
	newer.ExpiresAt = fixedNow().Add(90 * 24 * time.Hour).Format(time.RFC3339)
	next := signPayload(t, priv, newer)
	atomicWriteHook = func(step string) error {
		if step == "before_rename" {
			return os.ErrClosed
		}
		return nil
	}
	t.Cleanup(func() { atomicWriteHook = nil })
	if err := SaveVerifiedLicense(dir, next, v, testInstallID()); err == nil {
		t.Fatal("expected write failure")
	}
	got, err := os.ReadFile(filepath.Join(dir, licenseFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != old {
		t.Fatalf("token changed: %s", got)
	}
}
