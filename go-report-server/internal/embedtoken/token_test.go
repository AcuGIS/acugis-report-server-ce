package embedtoken

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIssueAndValidate(t *testing.T) {
	secret := "test-secret-value"
	tok, exp, err := Issue(secret, Claims{
		UserID:      7,
		Username:    "alice",
		ContentType: ContentReport,
		ContentID:   "Invoice",
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, Prefix) {
		t.Fatalf("prefix: %s", tok)
	}
	if time.Until(exp) > time.Minute+time.Second || time.Until(exp) <= 0 {
		t.Fatalf("unexpected expiry %v", exp)
	}
	claims, err := ParseAndValidate(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 7 || claims.Username != "alice" || claims.ContentID != "Invoice" {
		t.Fatalf("claims: %+v", claims)
	}
	if len(claims.Parameters) != 0 {
		t.Fatalf("token without bound parameters must keep Parameters empty: %+v", claims.Parameters)
	}
	if err := MatchContent(claims, ContentReport, "Invoice"); err != nil {
		t.Fatal(err)
	}
	if err := MatchContent(claims, ContentReport, "Other"); err == nil {
		t.Fatal("expected content mismatch")
	}
}

func TestRejectTamperAndExpire(t *testing.T) {
	secret := "test-secret-value"
	tok, _, err := Issue(secret, Claims{
		UserID:      1,
		Username:    "bob",
		ContentType: ContentDashboard,
		ContentID:   "bob/sales",
	}, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	tampered := tok + "x"
	if _, err := ParseAndValidate(secret, tampered); err == nil {
		t.Fatal("expected invalid tampered token")
	}
	if _, err := ParseAndValidate("other-secret", tok); err == nil {
		t.Fatal("expected invalid with wrong secret")
	}
	time.Sleep(60 * time.Millisecond)
	if _, err := ParseAndValidate(secret, tok); err != ErrExpiredToken {
		t.Fatalf("got %v, want expired", err)
	}
}

// TestMintFromInstalledSecretFile checks a secret written by the installer.
// JRS_TEST_EMBED_SECRET_FILE is the protected env file path. The secret and
// the minted token are not logged.
func TestMintFromInstalledSecretFile(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("JRS_TEST_EMBED_SECRET_FILE"))
	if path == "" {
		t.Skip("JRS_TEST_EMBED_SECRET_FILE not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		switch {
		case strings.HasPrefix(line, "ACUGIS_EMBED_TOKEN_SECRET="):
			secret = strings.TrimSpace(strings.TrimPrefix(line, "ACUGIS_EMBED_TOKEN_SECRET="))
		case secret == "" && strings.HasPrefix(line, "JRS_EMBED_TOKEN_SECRET="):
			secret = strings.TrimSpace(strings.TrimPrefix(line, "JRS_EMBED_TOKEN_SECRET="))
		}
	}
	if secret == "" {
		t.Fatal("protected env file has no embed signing secret")
	}
	tok, _, err := Issue(secret, Claims{
		UserID:      1,
		Username:    "install-check",
		ContentType: ContentReport,
		ContentID:   "install-report",
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseAndValidate(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ContentID != "install-report" || claims.Username != "install-check" {
		t.Fatal("minted claims did not round-trip")
	}
}

func TestRejectPATPrefix(t *testing.T) {
	if IsEmbedToken("jrs_abc") {
		t.Fatal("PAT must not look like embed token")
	}
	if !IsEmbedToken("emb_abc.def") {
		t.Fatal("expected embed token")
	}
}

func TestBoundParametersRoundTrip(t *testing.T) {
	secret := "test-secret-value"
	raw := []byte(`{"account_id":"smith-residential","year":2026,"active":true,"region":["east","west"]}`)
	params, err := ParseMintParameters(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := Issue(secret, Claims{
		UserID:      3,
		Username:    "host",
		ContentType: ContentReport,
		ContentID:   "portfolio-performance",
		Parameters:  params,
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseAndValidate(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Parameters["account_id"][0] != "smith-residential" {
		t.Fatalf("account_id: %+v", claims.Parameters)
	}
	if claims.Parameters["year"][0] != "2026" || claims.Parameters["active"][0] != "true" {
		t.Fatalf("scalars: %+v", claims.Parameters)
	}
	if len(claims.Parameters["region"]) != 2 || claims.Parameters["region"][0] != "east" || claims.Parameters["region"][1] != "west" {
		t.Fatalf("region: %+v", claims.Parameters["region"])
	}
	if claims.UserID != 3 || claims.ContentID != "portfolio-performance" {
		t.Fatalf("other claims changed: %+v", claims)
	}
}

func TestBoundParametersRejectSystemNames(t *testing.T) {
	for _, name := range []string{"_repName", "_repFormat", "_dataSource", "datasource", "report", "et", "P_MAP_IMAGE", "REPORT_DIR", "SORT_FIELDS"} {
		_, err := ParseMintParameters(json.RawMessage(`{"` + name + `":"x"}`))
		if err == nil {
			t.Fatalf("expected rejection for %s", name)
		}
	}
	if _, err := ParseMintParameters(json.RawMessage(`{"account_id":{"nested":true}}`)); err == nil {
		t.Fatal("expected object value rejection")
	}
	if _, err := ParseMintParameters(json.RawMessage(`{"account_id":null}`)); err == nil {
		t.Fatal("expected null rejection")
	}
	if _, _, err := Issue("secret", Claims{
		ContentType: ContentReport,
		ContentID:   "invoice",
		Parameters:  map[string][]string{"_repName": {"other"}},
	}, time.Minute); err == nil {
		t.Fatal("Issue must reject system bound parameters")
	}
}

func TestRequireSecret(t *testing.T) {
	if _, _, err := Issue("", Claims{ContentType: ContentReport, ContentID: "x"}, time.Minute); err != ErrEmptySecret {
		t.Fatalf("got %v", err)
	}
}
