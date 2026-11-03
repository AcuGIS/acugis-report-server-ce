//go:build oss

package license

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOSSIgnoresValidProfessionalToken(t *testing.T) {
	if packageCapable {
		t.Fatal("oss build must not be package capable")
	}
	pub, priv := testKey(t)
	dir := t.TempDir()
	now := fixedNow()
	mgr, err := NewManager(Options{StateDir: dir, PublicKey: pub, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	install := mgr.Status().InstallationID
	token := signPayload(t, priv, testPayload(ProductReportServerProfessional, install, now.Add(-time.Hour), now.Add(24*time.Hour)))
	if err := os.WriteFile(filepath.Join(dir, licenseFileName), []byte(token+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.ProfessionalEntitled() || mgr.Status().EffectiveEdition != "community" || mgr.Status().LicenseValid {
		t.Fatalf("oss unlocked: %+v", mgr.Status())
	}
	t.Setenv("ACUGIS_EDITION", "professional")
	if mgr.ProfessionalEntitled() {
		t.Fatal("env must not unlock the oss package")
	}
}
