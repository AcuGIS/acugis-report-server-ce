package license

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivateAndRefreshHTTP(t *testing.T) {
	pub, priv := testKey(t)
	good := signPayload(t, priv, professionalPayload())
	renewedP := professionalPayload()
	renewedP.ExpiresAt = fixedNow().Add(60 * 24 * time.Hour).Format(time.RFC3339)
	renewed := signPayload(t, priv, renewedP)
	wrongProduct := signPayload(t, priv, testPayload(ProductReportDesignerWeb, testInstallID(), fixedNow().Add(-time.Hour), fixedNow().Add(24*time.Hour)))
	wrongInstall := signPayload(t, priv, testPayload(ProductReportServerProfessional, otherInstallID(), fixedNow().Add(-time.Hour), fixedNow().Add(24*time.Hour)))
	_, other := testKey(t)
	badSig := signPayload(t, other, professionalPayload())

	var mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "ok-activate":
			if r.URL.Path != "/v1/activate" || r.Method != http.MethodPost {
				t.Errorf("activate path %s %s", r.Method, r.URL.Path)
			}
			var req tokenRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.ActivationKey == "" || req.InstallationID != testInstallID() || req.ProductCode != ProductReportServerProfessional {
				t.Errorf("activate body %+v", req)
			}
			_ = json.NewEncoder(w).Encode(tokenResponse{License: good, LicenseID: "lic_test_1", ProductCode: ProductReportServerProfessional, ExpiresAt: renewedP.ExpiresAt})
		case "bad-key":
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "invalid_activation_key", Message: "nope"})
		case "wrong-product":
			_ = json.NewEncoder(w).Encode(tokenResponse{License: wrongProduct, LicenseID: "lic_x", ProductCode: ProductReportDesignerWeb})
		case "wrong-install":
			_ = json.NewEncoder(w).Encode(tokenResponse{License: wrongInstall, LicenseID: "lic_x", ProductCode: ProductReportServerProfessional})
		case "bad-sig":
			_ = json.NewEncoder(w).Encode(tokenResponse{License: badSig, LicenseID: "lic_x"})
		case "refresh-ok":
			if r.URL.Path != "/v1/refresh" {
				t.Errorf("refresh path %s", r.URL.Path)
			}
			var req tokenRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.License == "" || req.ProductCode != ProductReportServerProfessional {
				t.Errorf("refresh body %+v", req)
			}
			_ = json.NewEncoder(w).Encode(tokenResponse{License: renewed, LicenseID: "lic_test_1", ProductCode: ProductReportServerProfessional, ExpiresAt: renewedP.ExpiresAt})
		case "suspended":
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "license_inactive", Message: "license is not active"})
		case "revoked":
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "refresh_denied", Message: "refresh denied"})
		case "boom":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("<html>secret token ACL1.secret</html>"))
		case "malformed":
			_, _ = w.Write([]byte("{"))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	c := &Client{ActivateEndpoint: srv.URL + "/v1/activate", RefreshEndpoint: srv.URL + "/v1/refresh"}
	ctx := context.Background()

	mode = "ok-activate"
	res, err := c.Activate(ctx, "ACUGIS-TEST-TEST-TEST-TEST-TEST", testInstallID(), "1.0.0")
	if err != nil || res.License != good {
		t.Fatalf("activate: %v %+v", err, res)
	}
	if _, err := ParseAndVerify(res.License, testVerifier(pub, fixedNow()), testInstallID()); err != nil {
		t.Fatal(err)
	}

	mode = "bad-key"
	_, err = c.Activate(ctx, "ACUGIS-TEST-TEST-TEST-TEST-TEST", testInstallID(), "")
	if !IsAuthoritative(err) {
		t.Fatalf("bad key: %v", err)
	}

	mode = "wrong-product"
	res, err = c.Activate(ctx, "ACUGIS-TEST-TEST-TEST-TEST-TEST", testInstallID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, verr := ParseAndVerify(res.License, testVerifier(pub, fixedNow()), testInstallID()); verr == nil {
		t.Fatal("wrong product must fail local verify")
	}

	mode = "wrong-install"
	res, err = c.Activate(ctx, "k", testInstallID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, verr := ParseAndVerify(res.License, testVerifier(pub, fixedNow()), testInstallID()); verr == nil {
		t.Fatal("wrong installation must fail local verify")
	}

	mode = "bad-sig"
	res, err = c.Activate(ctx, "k", testInstallID(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, verr := ParseAndVerify(res.License, testVerifier(pub, fixedNow()), testInstallID()); verr == nil {
		t.Fatal("bad signature must fail local verify")
	}

	mode = "refresh-ok"
	res, err = c.Refresh(ctx, good, testInstallID())
	if err != nil || res.License != renewed {
		t.Fatalf("refresh: %v", err)
	}

	mode = "suspended"
	_, err = c.Refresh(ctx, good, testInstallID())
	api, ok := err.(*APIError)
	if !ok || api.Code != "license_inactive" || !api.Authoritative || strings.Contains(api.Error(), "ACL1.") {
		t.Fatalf("suspended: %v", err)
	}

	mode = "revoked"
	_, err = c.Refresh(ctx, good, testInstallID())
	api, ok = err.(*APIError)
	if !ok || api.Code != "refresh_denied" || !api.Authoritative {
		t.Fatalf("revoked: %v", err)
	}

	mode = "boom"
	_, err = c.Refresh(ctx, good, testInstallID())
	if !IsTransport(err) || strings.Contains(err.Error(), "ACL1.") {
		t.Fatalf("5xx: %v", err)
	}

	mode = "malformed"
	_, err = c.Activate(ctx, "k", testInstallID(), "")
	if err == nil || !IsAuthoritative(err) {
		t.Fatalf("malformed: %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &Client{
		ActivateEndpoint: srv.URL,
		HTTP:             &http.Client{Timeout: 20 * time.Millisecond},
	}
	_, err := c.Activate(context.Background(), "k", testInstallID(), "")
	if !IsTransport(err) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestHTTPSRequiredForDefaultServer(t *testing.T) {
	c := &Client{ServerURL: "http://license.example"}
	if _, err := c.Activate(context.Background(), "k", testInstallID(), ""); err == nil {
		t.Fatal("http server URL must be rejected")
	}
}
