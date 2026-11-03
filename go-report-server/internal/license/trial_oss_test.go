//go:build oss

package license

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCommunityIgnoresProfessionalTrial(t *testing.T) {
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
	p := testPayload(ProductReportServerProfessional, install, now.Add(-time.Hour), now.Add(14*24*time.Hour))
	p.LicenseType = LicenseTypeTrial
	if err := os.WriteFile(filepath.Join(dir, licenseFileName), []byte(signPayload(t, priv, p)+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.ProfessionalEntitled() || mgr.Status().EffectiveEdition != "community" || mgr.Status().LicenseValid {
		t.Fatalf("community unlocked by professional trial: %+v", mgr.Status())
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("community package called the trial endpoint")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	mgr.client.TrialEndpoint = upstream.URL
	if _, err := mgr.StartTrial(context.Background()); err == nil {
		t.Fatal("community package started a professional trial")
	}
}

func TestCommunityDesignerTrialStillEntitlesDesigner(t *testing.T) {
	pub, priv := testKey(t)
	dir := t.TempDir()
	now := fixedNow()
	if err := os.WriteFile(filepath.Join(dir, installationFileName), []byte(testInstallID()+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	p := testPayload(ProductReportDesignerWeb, testInstallID(), now.Add(-time.Hour), now.Add(14*24*time.Hour))
	p.LicenseType = LicenseTypeTrial
	if err := os.WriteFile(filepath.Join(dir, DesignerLicenseFileName), []byte(signPayload(t, priv, p)+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	des, err := NewManager(Options{
		StateDir:                    dir,
		PublicKey:                   pub,
		Now:                         func() time.Time { return now },
		ProductCode:                 ProductReportDesignerWeb,
		LicenseFile:                 DesignerLicenseFileName,
		RequireExistingInstallation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := des.Load(); err != nil {
		t.Fatal(err)
	}
	if !des.DesignerEntitled() || des.ProfessionalEntitled() || des.Status().LicenseType != LicenseTypeTrial {
		t.Fatalf("community designer trial %+v", des.Status())
	}
	pro, err := NewManager(Options{StateDir: dir, PublicKey: pub, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := pro.Load(); err != nil {
		t.Fatal(err)
	}
	if pro.ProfessionalEntitled() || pro.Status().EffectiveEdition != "community" {
		t.Fatalf("designer trial changed community edition: %+v", pro.Status())
	}
}
