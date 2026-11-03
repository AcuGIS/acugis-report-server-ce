package schedules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSendRequiresSMTPOrSendmail(t *testing.T) {
	dir := t.TempDir()
	attach := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(attach, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Ensure no accidental sendmail in PATH for this assertion: empty host + no binary → clear error.
	t.Setenv("JRS_SENDMAIL_PATH", filepath.Join(dir, "missing-sendmail"))
	m := NewMailer(MailConfig{})
	res := m.Send(context.Background(), []string{"a@b.c"}, "subj", "body", attach, "out.pdf", false)
	if res.OK {
		t.Fatal("expected failure without SMTP/sendmail")
	}
	if res.Message == "" || res.Message == "SMTP not configured" {
		// Accept either legacy or expanded message.
		if res.Message == "" {
			t.Fatal("empty message")
		}
	}
}

func TestHeloNameFromAddress(t *testing.T) {
	cases := map[string]string{
		"reports@webgis1.com": "webgis1.com",
		"  a@b.co.uk ":        "b.co.uk",
		"nodomain":            "",
		"x@localhost":         "",
		"":                    "",
	}
	for in, want := range cases {
		if got := heloNameFromAddress(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestMailConfigFromEnvHelo(t *testing.T) {
	t.Setenv("JRS_SMTP_HOST", "smtp.example.com")
	t.Setenv("JRS_SMTP_HELO", "mail.webgis1.com")
	t.Setenv("JRS_SMTP_FROM", "reports@webgis1.com")
	cfg := MailConfigFromEnv()
	if cfg.Helo != "mail.webgis1.com" {
		t.Fatalf("helo=%q", cfg.Helo)
	}
}
