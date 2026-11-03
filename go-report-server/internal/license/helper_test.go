package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func testInstallID() string {
	return "11111111-1111-4111-8111-111111111111"
}

func otherInstallID() string {
	return "22222222-2222-4222-8222-222222222222"
}

func fixedNow() time.Time {
	return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
}

func testPayload(product, install string, issued, expires time.Time) Payload {
	return Payload{
		SchemaVersion:  SchemaVersion,
		LicenseID:      "lic_test_1",
		InstallationID: install,
		ProductCode:    product,
		IssuedAt:       issued.UTC().Format(time.RFC3339),
		ExpiresAt:      expires.UTC().Format(time.RFC3339),
	}
}

func signPayload(t *testing.T, priv ed25519.PrivateKey, p Payload) string {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	msg := []byte(TokenMarker + "." + b64)
	sig := ed25519.Sign(priv, msg)
	return TokenMarker + "." + b64 + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func testVerifier(pub ed25519.PublicKey, now time.Time) *Verifier {
	return &Verifier{PublicKey: pub, Now: func() time.Time { return now }}
}
