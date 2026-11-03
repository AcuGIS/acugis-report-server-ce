package schedules

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnvUseSudoDefaultsTrue(t *testing.T) {
	t.Setenv("JRS_JASPER_SYSTEMD_USE_SUDO", "")
	if !envUseSudo() {
		t.Fatal("empty env should match PHP jasper_systemd_use_sudo default (true)")
	}
	opts := DefaultSystemdOptions("/opt/acugis-report-server")
	if !opts.UseSudo {
		t.Fatal("DefaultSystemdOptions UseSudo should default true")
	}
}

func TestEnvUseSudoFalse(t *testing.T) {
	for _, v := range []string{"false", "0", "no", "off", "FALSE"} {
		t.Setenv("JRS_JASPER_SYSTEMD_USE_SUDO", v)
		if envUseSudo() {
			t.Fatalf("JRS_JASPER_SYSTEMD_USE_SUDO=%q should disable sudo", v)
		}
	}
}

func TestEnvUseSudoTrue(t *testing.T) {
	for _, v := range []string{"true", "1", "yes", "TRUE"} {
		t.Setenv("JRS_JASPER_SYSTEMD_USE_SUDO", v)
		if !envUseSudo() {
			t.Fatalf("JRS_JASPER_SYSTEMD_USE_SUDO=%q should enable sudo", v)
		}
	}
}

func TestInstallSymlinkMissingSource(t *testing.T) {
	m := NewSystemdManager(Paths{}, SystemdOptions{UseSudo: false}, nil)
	err := m.installSymlink(filepath.Join(t.TempDir(), "missing.service"), filepath.Join(t.TempDir(), "link.service"))
	if err == nil || !strings.Contains(err.Error(), "unit file does not exist") {
		t.Fatalf("expected missing unit error, got %v", err)
	}
}

func TestInstallSymlinkWithoutSudo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink requires privilege on Windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "jrs-schedule-1.service")
	dst := filepath.Join(dir, "jrs-schedule-1.service.link")
	if err := os.WriteFile(src, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewSystemdManager(Paths{}, SystemdOptions{UseSudo: false}, nil)
	if err := m.installSymlink(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Fatalf("symlink target %q, want %q", got, src)
	}
}

func TestNewFromOptionsReReadsUseSudo(t *testing.T) {
	t.Setenv("JRS_JASPER_SYSTEMD_USE_SUDO", "true")
	_, _, _, _, mgr, _ := NewFromOptions(Options{
		AppRoot: "/opt/acugis-report-server",
		Systemd: SystemdOptions{}, // bool zero value is false
	})
	if !mgr.opts.UseSudo {
		t.Fatal("zero SystemdOptions must not disable sudo when env enables it")
	}
}

func TestPrivilegedNameKeepsAbsolute(t *testing.T) {
	if got := privilegedName("/bin/systemctl"); got != "/bin/systemctl" {
		t.Fatalf("got %q want /bin/systemctl", got)
	}
	if got := privilegedName(""); got != "" {
		t.Fatalf("empty name %q", got)
	}
}

func TestUsePrivilegedWhenSystemDirNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("geteuid/chmod semantics differ on Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	m := NewSystemdManager(Paths{}, SystemdOptions{UseSudo: false, SystemDir: dir}, nil)
	if !m.usePrivileged() {
		t.Fatal("non-writable SystemDir must use sudo even when UseSudo is false")
	}
}

func TestInstallSymlinkWithSudoDoesNotCallUnprivilegedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sudo path is Linux-only")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "jrs-schedule-1.service")
	dst := filepath.Join(dir, "jrs-schedule-1.service.link")
	if err := os.WriteFile(src, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewSystemdManager(Paths{}, SystemdOptions{UseSudo: true}, nil)
	err := m.installSymlink(src, dst)
	if err == nil {
		// NOPASSWD sudo succeeded in this environment; the privileged path was used.
		return
	}
	if _, lerr := os.Lstat(dst); lerr == nil {
		t.Fatal("UseSudo=true must not create an unprivileged symlink when sudo fails")
	}
}
