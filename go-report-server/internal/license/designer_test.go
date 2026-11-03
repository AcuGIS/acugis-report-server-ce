package license

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDesignerWebEntitlementMatrix(t *testing.T) {
	pub, priv := testKey(t)
	install := testInstallID()
	now := fixedNow()
	dir := t.TempDir()
	writeInstallID(t, dir, install)

	valid := signPayload(t, priv, testPayload(ProductReportDesignerWeb, install, now.Add(-time.Hour), now.Add(24*time.Hour)))
	if err := os.WriteFile(filepath.Join(dir, DesignerLicenseFileName), []byte(valid+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	mgr := newDesignerManager(t, dir, pub, now)
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if !mgr.DesignerEntitled() || mgr.ProfessionalEntitled() {
		t.Fatalf("valid web token: designer=%v professional=%v", mgr.DesignerEntitled(), mgr.ProfessionalEntitled())
	}
	if st := mgr.Status(); !st.LicenseValid || st.Status != StatusActive || st.EffectiveEdition == "professional" {
		t.Fatalf("valid status: %+v", st)
	}

	// A professional token in the designer file must not entitle Web Designer.
	wrong := signPayload(t, priv, testPayload(ProductReportServerProfessional, install, now.Add(-time.Hour), now.Add(24*time.Hour)))
	if err := os.WriteFile(filepath.Join(dir, DesignerLicenseFileName), []byte(wrong+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.DesignerEntitled() || mgr.Status().Status != StatusWrongProduct {
		t.Fatalf("wrong product: entitled=%v status=%s", mgr.DesignerEntitled(), mgr.Status().Status)
	}

	other := signPayload(t, priv, testPayload(ProductReportDesignerWeb, otherInstallID(), now.Add(-time.Hour), now.Add(24*time.Hour)))
	if err := os.WriteFile(filepath.Join(dir, DesignerLicenseFileName), []byte(other+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.DesignerEntitled() || mgr.Status().Status != StatusInstallationMismatch {
		t.Fatalf("wrong installation: entitled=%v status=%s", mgr.DesignerEntitled(), mgr.Status().Status)
	}

	bad := valid[:len(valid)-4] + "aaaa"
	if err := os.WriteFile(filepath.Join(dir, DesignerLicenseFileName), []byte(bad+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.DesignerEntitled() || mgr.Status().Status != StatusInvalid {
		t.Fatalf("bad signature: entitled=%v status=%s", mgr.DesignerEntitled(), mgr.Status().Status)
	}

	expired := signPayload(t, priv, testPayload(ProductReportDesignerWeb, install, now.Add(-48*time.Hour), now.Add(-time.Hour)))
	if err := os.WriteFile(filepath.Join(dir, DesignerLicenseFileName), []byte(expired+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.DesignerEntitled() || mgr.Status().Status != StatusExpired {
		t.Fatalf("expired: entitled=%v status=%s", mgr.DesignerEntitled(), mgr.Status().Status)
	}

	if err := os.Remove(filepath.Join(dir, DesignerLicenseFileName)); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if mgr.DesignerEntitled() || mgr.Status().Status != StatusNotActivated {
		t.Fatalf("missing: entitled=%v status=%s", mgr.DesignerEntitled(), mgr.Status().Status)
	}
}

func TestDesignerTokenDoesNotChangeProfessionalEdition(t *testing.T) {
	pub, priv := testKey(t)
	install := testInstallID()
	now := fixedNow()
	dir := t.TempDir()
	writeInstallID(t, dir, install)
	web := signPayload(t, priv, testPayload(ProductReportDesignerWeb, install, now.Add(-time.Hour), now.Add(24*time.Hour)))
	if err := os.WriteFile(filepath.Join(dir, licenseFileName), []byte(web+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	pro, err := NewManager(Options{StateDir: dir, PublicKey: pub, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if err := pro.Load(); err != nil {
		t.Fatal(err)
	}
	if pro.ProfessionalEntitled() || pro.DesignerEntitled() {
		t.Fatal("web token stored as license.lic enabled an entitlement")
	}
	if pro.Status().Status != StatusWrongProduct {
		t.Fatalf("professional status=%s", pro.Status().Status)
	}
	if pro.Status().EffectiveEdition != "community" {
		t.Fatalf("edition=%s", pro.Status().EffectiveEdition)
	}
}

func TestDesignerDoesNotCreateInstallationID(t *testing.T) {
	pub, _ := testKey(t)
	dir := t.TempDir()
	mgr := newDesignerManager(t, dir, pub, fixedNow())
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, installationFileName)); !os.IsNotExist(err) {
		t.Fatalf("designer created an installation id: %v", err)
	}
	if mgr.DesignerEntitled() {
		t.Fatal("missing installation id was entitled")
	}
}

func TestDesignerEntitledWhileLicenseServerUnreachable(t *testing.T) {
	pub, priv := testKey(t)
	install := testInstallID()
	now := fixedNow()
	dir := t.TempDir()
	writeInstallID(t, dir, install)
	token := signPayload(t, priv, testPayload(ProductReportDesignerWeb, install, now.Add(-time.Hour), now.Add(24*time.Hour)))
	if err := SaveVerifiedProductLicense(dir, DesignerLicenseFileName, token, testVerifier(pub, now), install, ProductReportDesignerWeb); err != nil {
		t.Fatal(err)
	}
	mgr := newDesignerManager(t, dir, pub, now)
	mgr.client.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("entitlement check contacted the license server")
		return nil, nil
	})}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	if !mgr.DesignerEntitled() {
		t.Fatal("locally valid token was not entitled while the license server was unreachable")
	}
	if mgr.ProfessionalEntitled() {
		t.Fatal("web token enabled professional")
	}
}

func TestDesignerActivationPersistsOnlyACL1(t *testing.T) {
	pub, priv := testKey(t)
	install := testInstallID()
	now := fixedNow()
	dir := t.TempDir()
	writeInstallID(t, dir, install)
	const key = "ACUGIS-WEB-TEST-KEY"
	mgr := newDesignerManager(t, dir, pub, now)
	mgr.client.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, tokenResponse{
			License:     signPayload(t, priv, testPayload(ProductReportDesignerWeb, install, now.Add(-time.Hour), now.Add(24*time.Hour))),
			LicenseID:   "lic_web",
			ProductCode: ProductReportDesignerWeb,
			ExpiresAt:   now.Add(24 * time.Hour).Format(time.RFC3339),
		}), nil
	})}
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Activate(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !st.LicenseValid || !mgr.DesignerEntitled() || mgr.ProfessionalEntitled() {
		t.Fatalf("activate status=%+v designer=%v professional=%v", st, mgr.DesignerEntitled(), mgr.ProfessionalEntitled())
	}
	raw, err := os.ReadFile(filepath.Join(dir, DesignerLicenseFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) == "" || strings.Contains(string(raw), key) {
		t.Fatal("activation key was persisted or token was empty")
	}
	if _, err := os.Stat(filepath.Join(dir, licenseFileName)); !os.IsNotExist(err) {
		t.Fatal("designer activation wrote license.lic")
	}
}

func newDesignerManager(t *testing.T, dir string, pub ed25519.PublicKey, now time.Time) *Manager {
	t.Helper()
	mgr, err := NewManager(Options{
		StateDir:                    dir,
		PublicKey:                   pub,
		ProductCode:                 ProductReportDesignerWeb,
		LicenseFile:                 DesignerLicenseFileName,
		RequireExistingInstallation: true,
		Now:                         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func writeInstallID(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, installationFileName), []byte(id+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(t *testing.T, v any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
}
