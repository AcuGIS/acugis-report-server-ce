package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

func professionalPayload() Payload {
	return testPayload(ProductReportServerProfessional, testInstallID(),
		fixedNow().Add(-24*time.Hour), fixedNow().Add(30*24*time.Hour))
}

func TestVerifyValidProfessional(t *testing.T) {
	pub, priv := testKey(t)
	token := signPayload(t, priv, professionalPayload())
	lic, err := ParseAndVerify(token, testVerifier(pub, fixedNow()), testInstallID())
	if err != nil {
		t.Fatal(err)
	}
	if lic.ProductCode != ProductReportServerProfessional || lic.InstallationID != testInstallID() {
		t.Fatalf("%+v", lic)
	}
}

func TestVerifyRejects(t *testing.T) {
	pub, priv := testKey(t)
	otherPub, _ := testKey(t)
	good := signPayload(t, priv, professionalPayload())
	cases := []struct {
		name    string
		token   string
		pub     ed25519.PublicKey
		install string
		code    string
	}{
		{name: "bad signature", token: good, pub: otherPub, install: testInstallID(), code: CodeSignature},
		{name: "wrong prefix", token: "GJL1.aa.bb", pub: pub, install: testInstallID(), code: CodeInvalid},
		{name: "missing segment", token: "ACL1.only", pub: pub, install: testInstallID(), code: CodeInvalid},
		{name: "extra segment", token: good + ".extra", pub: pub, install: testInstallID(), code: CodeInvalid},
		{name: "bad base64", token: "ACL1.!!!!.!!!!", pub: pub, install: testInstallID(), code: CodeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAndVerify(tc.token, testVerifier(tc.pub, fixedNow()), tc.install)
			assertCode(t, err, tc.code)
		})
	}
}

func TestVerifyPayloadRules(t *testing.T) {
	pub, priv := testKey(t)
	v := testVerifier(pub, fixedNow())

	badSchema := professionalPayload()
	badSchema.SchemaVersion = 2
	_, err := ParseAndVerify(signPayload(t, priv, badSchema), v, testInstallID())
	assertCode(t, err, CodeInvalid)

	wrongProduct := professionalPayload()
	wrongProduct.ProductCode = ProductReportDesignerWeb
	_, err = ParseAndVerify(signPayload(t, priv, wrongProduct), v, testInstallID())
	assertCode(t, err, CodeWrongProduct)

	desktop := professionalPayload()
	desktop.ProductCode = ProductReportDesignerDesktop
	_, err = ParseAndVerify(signPayload(t, priv, desktop), v, testInstallID())
	assertCode(t, err, CodeWrongProduct)

	_, err = ParseAndVerify(signPayload(t, priv, professionalPayload()), v, otherInstallID())
	assertCode(t, err, CodeMismatch)

	expired := professionalPayload()
	expired.ExpiresAt = fixedNow().Add(-time.Hour).Format(time.RFC3339)
	_, err = ParseAndVerify(signPayload(t, priv, expired), v, testInstallID())
	assertCode(t, err, CodeExpired)

	future := professionalPayload()
	future.IssuedAt = fixedNow().Add(time.Hour).Format(time.RFC3339)
	future.ExpiresAt = fixedNow().Add(48 * time.Hour).Format(time.RFC3339)
	_, err = ParseAndVerify(signPayload(t, priv, future), v, testInstallID())
	assertCode(t, err, CodeInvalid)

	badTime := professionalPayload()
	badTime.IssuedAt = "yesterday"
	_, err = ParseAndVerify(signPayload(t, priv, badTime), v, testInstallID())
	assertCode(t, err, CodeInvalid)

	badID := professionalPayload()
	badID.InstallationID = "not-a-uuid"
	_, err = ParseAndVerify(signPayload(t, priv, badID), v, testInstallID())
	assertCode(t, err, CodeInvalid)
}

func TestInvalidJSONPayload(t *testing.T) {
	pub, priv := testKey(t)
	raw := []byte("{")
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	msg := []byte(TokenMarker + "." + b64)
	sig := ed25519.Sign(priv, msg)
	token := TokenMarker + "." + b64 + "." + base64.RawURLEncoding.EncodeToString(sig)
	_, err := ParseAndVerify(token, testVerifier(pub, fixedNow()), testInstallID())
	assertCode(t, err, CodeInvalid)
}

func TestExpiredStillInsideRefreshGrace(t *testing.T) {
	pub, priv := testKey(t)
	p := professionalPayload()
	p.IssuedAt = fixedNow().Add(-48 * time.Hour).Format(time.RFC3339)
	p.ExpiresAt = fixedNow().Add(-time.Hour).Format(time.RFC3339)
	token := signPayload(t, priv, p)
	_, err := ParseAndVerify(token, testVerifier(pub, fixedNow()), testInstallID())
	assertCode(t, err, CodeExpired)
	ident, err := ParseIdentity(token, testVerifier(pub, fixedNow()), testInstallID(), ProductReportServerProfessional)
	if err != nil {
		t.Fatal(err)
	}
	if !WithinRefreshGrace(ident, fixedNow()) {
		t.Fatal("expected refresh grace")
	}
	if WithinRefreshGrace(ident, ident.ExpiresAt.Add(RefreshGracePeriod+time.Second)) {
		t.Fatal("grace must end")
	}
}

func TestProductionPublicKeyParses(t *testing.T) {
	pub, err := ProductionPublicKey()
	if err != nil || len(pub) != 32 {
		t.Fatalf("production public key: %v len=%d", err, len(pub))
	}
	_, priv := testKey(t)
	token := signPayload(t, priv, professionalPayload())
	if _, err := ParseAndVerify(token, &Verifier{PublicKey: pub, Now: func() time.Time { return fixedNow() }}, testInstallID()); err == nil {
		t.Fatal("test key must not verify with the production public key")
	}
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	e, ok := As(err)
	if !ok || e.Code != code {
		t.Fatalf("err=%v want %s", err, code)
	}
}
