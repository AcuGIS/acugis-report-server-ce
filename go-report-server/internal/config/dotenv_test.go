package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

func TestLoadDotEnvFilesDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("JRS_SMTP_HOST=from-file\nJRS_SMTP_FROM=a@b.c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JRS_SMTP_HOST", "already-set")
	_ = os.Unsetenv("JRS_SMTP_FROM")

	config.LoadDotEnvFiles(dir)

	if got := os.Getenv("JRS_SMTP_HOST"); got != "already-set" {
		t.Fatalf("host=%q want already-set", got)
	}
	if got := os.Getenv("JRS_SMTP_FROM"); got != "a@b.c" {
		t.Fatalf("from=%q", got)
	}
}

func TestLoadDotEnvFilesReadsSystemdEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "native.env")
	if err := os.WriteFile(envPath, []byte("JRS_JASPER_SYSTEMD_USE_SUDO=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JRS_SYSTEMD_ENV_FILE", envPath)
	_ = os.Unsetenv("JRS_JASPER_SYSTEMD_USE_SUDO")

	config.LoadDotEnvFiles(dir)

	if got := os.Getenv("JRS_JASPER_SYSTEMD_USE_SUDO"); got != "true" {
		t.Fatalf("JRS_JASPER_SYSTEMD_USE_SUDO=%q want true", got)
	}
}
