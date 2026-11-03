package duckdb

import (
	"strings"
	"testing"
)

func TestNormalizeAuthType(t *testing.T) {
	cases := map[string]string{
		"":               AuthNone,
		"none":           AuthNone,
		"basic":          AuthBasic,
		"Basic Auth":     AuthNone, // unknown -> none (UI sends "basic")
		"bearer":         AuthBearer,
		"bearer_token":   AuthBearer,
		"headers":        AuthHeaders,
		"custom_headers": AuthHeaders,
	}
	for in, want := range cases {
		if got := NormalizeAuthType(in); got != want {
			t.Fatalf("NormalizeAuthType(%q)=%q want %q", in, got, want)
		}
	}
}
func TestRedactRemoteURL(t *testing.T) {
	in := "https://user:pass@example.com/private/data.parquet?X-Amz-Signature=secret&x=1"
	got := RedactRemoteURL(in)
	if strings.Contains(got, "pass") || strings.Contains(got, "secret") || strings.Contains(got, "user:") {
		t.Fatalf("leaked credentials: %s", got)
	}
	if !strings.Contains(got, "example.com/private/data.parquet") {
		t.Fatalf("expected path preserved: %s", got)
	}
	if !strings.HasSuffix(got, "?…") {
		t.Fatalf("expected query redaction: %s", got)
	}
}
func TestMergeHeaderSecretJSON(t *testing.T) {
	existing := `{"X-API-Key":"old","Authorization":"Bearer oldtok"}`
	updates := `{"X-API-Key":"new"}`
	keys := []string{"X-API-Key", "X-Custom"}
	merged := MergeHeaderSecretJSON(existing, updates, keys)
	m := ParseHeaderSecretValues(merged)
	if m["X-API-Key"] != "new" {
		t.Fatalf("expected updated key: %#v", m)
	}
	if _, ok := m["Authorization"]; ok {
		t.Fatalf("dropped key should be gone: %#v", m)
	}
	// Unchanged blank update keeps existing for X-Custom if it existed — it didn't.
	if _, ok := m["X-Custom"]; ok {
		t.Fatalf("empty new key should not appear: %#v", m)
	}
	// Blank updates keep existing for kept keys.
	merged2 := MergeHeaderSecretJSON(existing, "{}", []string{"Authorization"})
	m2 := ParseHeaderSecretValues(merged2)
	if m2["Authorization"] != "Bearer oldtok" {
		t.Fatalf("expected preserved auth: %#v", m2)
	}
}
func TestHTTPAuthHeadersBasicBearer(t *testing.T) {
	basic, err := HTTPAuthHeaders(Config{
		Username: "alice",
		Password: "s3cret",
		Options:  map[string]string{OptionAuthType: AuthBasic},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(basic["Authorization"], "Basic ") {
		t.Fatalf("%#v", basic)
	}
	if strings.Contains(basic["Authorization"], "s3cret") {
		t.Fatal("raw password must not appear; expect base64 only")
	}
	bearer, err := HTTPAuthHeaders(Config{
		Password: "tok_abc",
		Options:  map[string]string{OptionAuthType: AuthBearer},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bearer["Authorization"] != "Bearer tok_abc" {
		t.Fatalf("%#v", bearer)
	}
}
func TestBuildSecretSQLDoesNotUseURLCredentials(t *testing.T) {
	sql, err := buildBearerSecretSQL("my-token", "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "BEARER_TOKEN") || !strings.Contains(sql, "TYPE HTTP") {
		t.Fatalf("unexpected SQL shape: %s", sql)
	}
	if strings.Contains(sql, "my-token@") || strings.Contains(sql, "://my-token") {
		t.Fatal("must not embed token in URL form")
	}
	hsql, err := buildHeadersSecretSQL(map[string]string{
		"X-API-Key":     "k",
		"Authorization": "Bearer t",
	}, "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hsql, "EXTRA_HTTP_HEADERS") {
		t.Fatalf("%s", hsql)
	}
}
func TestFormatHTTPAuthError(t *testing.T) {
	err := FormatHTTPAuthError(fmtError("HTTP 401 Unauthorized for https://x/?token=abc"), "abc")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "abc") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("leaked secret: %v", err)
	}
}
func fmtError(s string) error {
	return &simpleErr{s}
}

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
func TestSanitizeErrorMessageRedactsURLQuery(t *testing.T) {
	msg := `failed to read https://host/file.parquet?X-Amz-Signature=VERYSECRET`
	got := SanitizeErrorMessage(msg)
	if strings.Contains(got, "VERYSECRET") {
		t.Fatalf("query not redacted: %s", got)
	}
}
func TestAuthNoneSkipsSecret(t *testing.T) {
	h, err := HTTPAuthHeaders(Config{
		Options: map[string]string{OptionAuthType: AuthNone},
	})
	if err != nil || h != nil {
		t.Fatalf("got %#v err=%v", h, err)
	}
}
