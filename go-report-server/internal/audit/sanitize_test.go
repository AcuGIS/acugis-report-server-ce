package audit_test

import (
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/audit"
)

func TestIsSecretParamName(t *testing.T) {
	secrets := []string{
		"password", "Password", "db_passwd", "api_key", "apiKey", "apikey",
		"secret", "token", "authorization", "credential", "private_key",
		"privateKey", "bearer_token", "session_cookie",
	}
	for _, name := range secrets {
		if !audit.IsSecretParamName(name) {
			t.Errorf("expected %q to be treated as secret", name)
		}
	}
	safe := []string{"p_country", "year", "format", "report", "user_id"}
	for _, name := range safe {
		if audit.IsSecretParamName(name) {
			t.Errorf("expected %q NOT to be secret", name)
		}
	}
}

func TestSanitizeParamsRedactsSecrets(t *testing.T) {
	in := map[string]any{
		"p_country":     []any{"USA"},
		"p_year":        2026,
		"password":      "s3cret",
		"api_key":       "key-123",
		"authorization": "Bearer abc",
		"token":         "tok",
		"private_key":   "-----BEGIN PRIVATE KEY-----",
		"nested": map[string]any{
			"secret": "hidden",
			"ok":     "visible",
		},
	}
	out := audit.SanitizeParams(in)
	if out["password"] != "[redacted]" {
		t.Fatalf("password not redacted: %v", out["password"])
	}
	if out["api_key"] != "[redacted]" {
		t.Fatalf("api_key not redacted")
	}
	if out["authorization"] != "[redacted]" {
		t.Fatalf("authorization not redacted")
	}
	if out["token"] != "[redacted]" {
		t.Fatalf("token not redacted")
	}
	if out["private_key"] != "[redacted]" {
		t.Fatalf("private_key not redacted")
	}
	if out["p_year"] != 2026 {
		t.Fatalf("safe param altered: %v", out["p_year"])
	}
	nested, ok := out["nested"].(map[string]any)
	if !ok {
		t.Fatal("nested map missing")
	}
	if nested["secret"] != "[redacted]" {
		t.Fatalf("nested secret not redacted: %v", nested["secret"])
	}
	if nested["ok"] != "visible" {
		t.Fatalf("nested safe value altered")
	}
}

func TestSanitizeErrorNoStack(t *testing.T) {
	msg := audit.SanitizeErrorMessage("boom\n\tat main.go:1\n\tat runtime.go:2")
	if len(msg) == 0 {
		t.Fatal("empty")
	}
	if containsNewline(msg) {
		t.Fatalf("newlines remain: %q", msg)
	}
}

func TestSanitizeErrorMessageRedactsURLCredentials(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			in:   `connect failed: https://dbuser:s3cret@db.example.com:5432/app`,
			want: `connect failed: https://[redacted]@db.example.com:5432/app`,
		},
		{
			in:   `postgres://admin:p%40ss@localhost/reports`,
			want: `postgres://[redacted]@localhost/reports`,
		},
		{
			in:   `http://onlyuser@host.example/path`,
			want: `http://[redacted]@host.example/path`,
		},
		{
			in:   `no credentials https://example.com/ok`,
			want: `no credentials https://example.com/ok`,
		},
	}
	for _, tc := range cases {
		got := audit.SanitizeErrorMessage(tc.in)
		if got != tc.want {
			t.Errorf("SanitizeErrorMessage(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
		if strings.Contains(got, "s3cret") || strings.Contains(got, "p%40ss") || strings.Contains(got, "dbuser:") {
			t.Errorf("credential leaked in %q", got)
		}
	}
}

func containsNewline(s string) bool {
	for _, c := range s {
		if c == '\n' || c == '\r' {
			return true
		}
	}
	return false
}

func TestExportFormatAction(t *testing.T) {
	if audit.ExportFormatAction("pdf") != audit.ActionReportExport {
		t.Fatal("pdf should be export")
	}
	if audit.ExportFormatAction("html") != audit.ActionReportExecute {
		t.Fatal("html should be execute")
	}
}
