package sfdriver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

func TestSnowflakeAccountID(t *testing.T) {
	cases := map[string]string{
		"xy12345.us-east-1":                                "xy12345.us-east-1",
		"xy12345.us-east-1.snowflakecomputing.com":         "xy12345.us-east-1",
		"https://xy12345.us-east-1.snowflakecomputing.com": "xy12345.us-east-1",
		" org-account ":                                    "org-account",
	}
	for in, want := range cases {
		if got := snowflakeAccountID(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestParseSnowflakePrivateKey_PKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	parsed, err := parseSnowflakePrivateKey(pemBytes, "")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.N.Cmp(key.N) != 0 {
		t.Fatal("key mismatch")
	}
}

func TestParseSnowflakePrivateKey_Invalid(t *testing.T) {
	if _, err := parseSnowflakePrivateKey([]byte("not-a-pem"), ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestSnowflakeConfig_KeyPairRequiresPEM(t *testing.T) {
	_, err := snowflakeConfig(&datasources.Datasource{
		DBType:                 "snowflake",
		Host:                   "acct",
		Username:               "user",
		DBName:                 "DB",
		AuthType:               datasources.AuthKeyPair,
		PrivateKeyResourceUUID: "11111111-1111-1111-1111-111111111111",
	})
	if err == nil {
		t.Fatal("expected missing PEM error")
	}
}
