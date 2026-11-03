package schedules

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SystemdOptions configures systemd integration.
type SystemdOptions struct {
	UnitDir      string
	SystemDir    string
	SystemctlBin string
	UseSudo      bool
	RunnerScript string
	PHPBinary    string
	ServiceUser  string
	JasperHome   string
	// GlobalEnvFile is an optional extra systemd EnvironmentFile (native packages).
	// Defaults to /etc/acugis-report-server/env so schedule units inherit JRS_DB_*.
	GlobalEnvFile string
}

// DefaultSystemdOptions reads defaults matching PHP config.php.
func DefaultSystemdOptions(appRoot string) SystemdOptions {
	unitDir := strings.TrimSpace(os.Getenv("JRS_JASPER_SYSTEMD_UNIT_DIR"))
	if unitDir == "" {
		unitDir = "/opt/acugis-report-server/systemd"
	}
	systemDir := strings.TrimSpace(os.Getenv("JRS_JASPER_SYSTEMD_SYSTEM_DIR"))
	if systemDir == "" {
		systemDir = "/etc/systemd/system"
	}
	systemctl := strings.TrimSpace(os.Getenv("JRS_JASPER_SYSTEMCTL"))
	if systemctl == "" {
		systemctl = "/bin/systemctl"
	}
	runner := filepath.Join(appRoot, "scripts", "run_schedule.php")
	globalEnv := strings.TrimSpace(os.Getenv("JRS_SYSTEMD_ENV_FILE"))
	if globalEnv == "" {
		globalEnv = "/etc/acugis-report-server/env"
	}
	return SystemdOptions{
		UnitDir:       unitDir,
		SystemDir:     systemDir,
		SystemctlBin:  systemctl,
		UseSudo:       envUseSudo(),
		RunnerScript:  runner,
		PHPBinary:     findPHPBinary(),
		ServiceUser:   ServiceUser(),
		JasperHome:    appRoot,
		GlobalEnvFile: globalEnv,
	}
}

// SystemdManager writes env/unit files and manages timers.
type SystemdManager struct {
	paths   Paths
	opts    SystemdOptions
	repo    *Repository
	lastErr string
}

// NewSystemdManager constructs a systemd helper.
func NewSystemdManager(paths Paths, opts SystemdOptions, repo *Repository) *SystemdManager {
	if strings.TrimSpace(opts.ServiceUser) == "" {
		opts.ServiceUser = ServiceUser()
	}
	return &SystemdManager{paths: paths, opts: opts, repo: repo}
}

// LastError returns the most recent systemd error message.
func (m *SystemdManager) LastError() string {
	return m.lastErr
}

func (m *SystemdManager) setError(msg string) {
	m.lastErr = msg
}

// SyncAfterSave persists env and installs/removes systemd units (PHP jrs_schedule_sync_systemd_after_save).
func (m *SystemdManager) SyncAfterSave(ctx context.Context, scheduleID int, schedule *Schedule) SyncResult {
	m.lastErr = ""
	if schedule == nil && m.repo != nil {
		fromDB, err := m.repo.Get(ctx, scheduleID)
		if err != nil {
			return SyncResult{OK: false, Error: "Schedule not found after save"}
		}
		schedule = fromDB
	}
	if schedule == nil {
		return SyncResult{OK: false, Error: "Schedule not found after save"}
	}

	prepared := PrepareForSystemd(*schedule)
	if err := m.persistEnvFromDB(ctx, scheduleID, prepared); err != nil {
		return SyncResult{
			OK:            false,
			TimerRequired: NeedsSystemd(prepared),
			Error:         err.Error(),
		}
	}

	timerRequired := NeedsSystemd(prepared)
	// Container / Kubernetes dispatcher runtime: keep native systemd as the
	// default. When JRS_SCHEDULE_RUNTIME=dispatcher, env files are still
	// written (same as systemd EnvironmentFile) but units are not installed.
	if ScheduleRuntime() == "dispatcher" {
		return SyncResult{OK: true, TimerRequired: timerRequired}
	}
	if !timerRequired {
		m.removeBestEffort(scheduleID)
		return SyncResult{OK: true, TimerRequired: false}
	}

	if !IsEnabled(prepared) {
		return SyncResult{
			OK:            false,
			TimerRequired: true,
			Error:         "Schedule is disabled (enabled=false). Enable the schedule and save again.",
		}
	}

	if runtime.GOOS != "linux" {
		if err := m.writeUnitFiles(scheduleID, prepared); err != nil {
			m.setError(err.Error())
		} else {
			m.setError("systemd timers are Linux-only; env file written")
		}
		return SyncResult{OK: true, TimerRequired: true, Error: m.lastErr}
	}

	if !m.systemctlAvailable() {
		if err := m.writeUnitFiles(scheduleID, prepared); err != nil {
			return SyncResult{OK: false, TimerRequired: true, Error: err.Error()}
		}
		m.setError("systemctl not available; unit files written but timer not enabled")
		return SyncResult{OK: true, TimerRequired: true, Error: m.lastErr}
	}

	if err := m.installSystemd(scheduleID, prepared); err != nil {
		return SyncResult{OK: false, TimerRequired: true, Error: err.Error()}
	}
	return SyncResult{OK: true, TimerRequired: true}
}

// Remove uninstalls systemd units and env file for a schedule.
func (m *SystemdManager) Remove(ctx context.Context, scheduleID int) error {
	m.lastErr = ""
	if ScheduleRuntime() == "dispatcher" {
		RemoveEnvFile(m.paths, scheduleID)
		return nil
	}
	if runtime.GOOS == "linux" && m.systemctlAvailable() {
		if err := m.removeSystemd(scheduleID); err != nil {
			return err
		}
	} else {
		m.removeBestEffort(scheduleID)
	}
	RemoveEnvFile(m.paths, scheduleID)
	return nil
}

func (m *SystemdManager) persistEnvFromDB(ctx context.Context, scheduleID int, schedule Schedule) error {
	if m.repo != nil {
		fromDB, err := m.repo.Get(ctx, scheduleID)
		if err == nil {
			schedule = PrepareForSystemd(*fromDB)
		}
	}
	if err := BuildEnvFile(m.paths, scheduleID, schedule, m.opts.JasperHome); err != nil {
		m.setError(err.Error())
		return err
	}
	return nil
}

func (m *SystemdManager) writeUnitFiles(scheduleID int, schedule Schedule) error {
	unitDir := m.opts.UnitDir
	if err := Mkdir(unitDir); err != nil {
		return fmt.Errorf("unit directory does not exist and could not be created: %s", unitDir)
	}
	if !isWritableDir(unitDir) {
		return fmt.Errorf("unit directory is not writable: %s", unitDir)
	}

	serviceName := ServiceName(scheduleID)
	timerName := TimerName(scheduleID)
	envFile := m.paths.EnvPath(scheduleID)
	logFile := m.paths.LogPath(scheduleID)
	phpBin := m.opts.PHPBinary
	runner := m.opts.RunnerScript

	globalEnv := strings.TrimSpace(m.opts.GlobalEnvFile)
	if globalEnv == "" {
		globalEnv = "/etc/acugis-report-server/env"
	}
	service := fmt.Sprintf(`[Unit]
Description=Jasper Report Services schedule #%d
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
EnvironmentFile=-%s
EnvironmentFile=-%s
ExecStart=%s %s %d
User=%s
Group=%s
StandardOutput=append:%s
StandardError=append:%s

`, scheduleID, globalEnv, envFile, phpBin, runner, scheduleID, m.opts.ServiceUser, m.opts.ServiceUser, logFile, logFile)

	spec := BuildTimerSpec(schedule)
	if spec.Type == "none" {
		msg := spec.Error
		if msg == "" {
			msg = EffectiveMode(schedule)
		}
		return fmt.Errorf("invalid or empty schedule timer spec: %s", msg)
	}

	var timerBody string
	if spec.Type == "interval" {
		timerBody = fmt.Sprintf("OnUnitActiveSec=%s\nOnBootSec=%s\nPersistent=true\nUnit=%s", spec.Interval, spec.Interval, serviceName)
	} else {
		if strings.TrimSpace(spec.Calendar) == "" {
			return fmt.Errorf("invalid or empty schedule calendar")
		}
		timerBody = fmt.Sprintf("OnCalendar=%s\nPersistent=true\nUnit=%s", spec.Calendar, serviceName)
	}

	timer := fmt.Sprintf(`[Unit]
Description=Timer for Jasper schedule #%d

[Timer]
%s

[Install]
WantedBy=timers.target

`, scheduleID, timerBody)

	servicePath := filepath.Join(unitDir, serviceName)
	timerPath := filepath.Join(unitDir, timerName)
	if err := os.WriteFile(servicePath, []byte(service), 0o644); err != nil {
		return fmt.Errorf("failed to write service unit: %s", servicePath)
	}
	if err := os.WriteFile(timerPath, []byte(timer), 0o644); err != nil {
		return fmt.Errorf("failed to write timer unit: %s", timerPath)
	}
	return nil
}

func (m *SystemdManager) installSystemd(scheduleID int, schedule Schedule) error {
	if err := BuildEnvFile(m.paths, scheduleID, schedule, m.opts.JasperHome); err != nil {
		return err
	}
	if err := m.writeUnitFiles(scheduleID, schedule); err != nil {
		return err
	}
	if err := m.linkUnits(scheduleID); err != nil {
		return err
	}
	if _, err := m.systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload failed: %w", err)
	}
	timer := TimerName(scheduleID)
	if _, err := m.systemctl("enable", "--now", timer); err != nil {
		return fmt.Errorf("enable --now failed for %s: %w", timer, err)
	}
	return nil
}

func (m *SystemdManager) removeSystemd(scheduleID int) error {
	timer := TimerName(scheduleID)
	_, _ = m.systemctl("disable", "--now", timer)
	_, _ = m.systemctl("stop", timer)
	_, _ = m.systemctl("reset-failed", timer)
	if err := m.unlinkUnits(scheduleID); err != nil {
		return err
	}
	for _, name := range UnitNames(scheduleID) {
		_ = os.Remove(filepath.Join(m.opts.UnitDir, name))
	}
	_, _ = m.systemctl("daemon-reload")
	return nil
}

func (m *SystemdManager) removeBestEffort(scheduleID int) {
	timer := TimerName(scheduleID)
	_, _ = m.systemctl("disable", "--now", timer)
	_, _ = m.systemctl("stop", timer)
	_, _ = m.systemctl("reset-failed", timer)
	_ = m.unlinkUnits(scheduleID)
	for _, name := range UnitNames(scheduleID) {
		_ = os.Remove(filepath.Join(m.opts.UnitDir, name))
	}
	_, _ = m.systemctl("daemon-reload")
}

func (m *SystemdManager) linkUnits(scheduleID int) error {
	for _, name := range UnitNames(scheduleID) {
		source := filepath.Join(m.opts.UnitDir, name)
		link := filepath.Join(m.opts.SystemDir, name)
		// Must honor UseSudo. Direct os.Symlink to /etc/systemd/system fails as www-data.
		if err := m.installSymlink(source, link); err != nil {
			return err
		}
	}
	return nil
}

func (m *SystemdManager) unlinkUnits(scheduleID int) error {
	for _, name := range UnitNames(scheduleID) {
		link := filepath.Join(m.opts.SystemDir, name)
		if err := m.removePath(link); err != nil {
			return err
		}
	}
	return nil
}

func (m *SystemdManager) installSymlink(source, link string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("unit file does not exist: %s", source)
	}
	if m.usePrivileged() {
		if out, err := m.runPrivileged("rm", "-f", link); err != nil {
			return fmt.Errorf("failed to remove %s: %s", link, errOutput(out, err))
		}
		if out, err := m.runPrivileged("ln", "-sf", source, link); err != nil {
			return fmt.Errorf("failed to create symlink %s -> %s: %s", link, source, errOutput(out, err))
		}
		return nil
	}
	return symlink(source, link)
}

func (m *SystemdManager) removePath(path string) error {
	if m.usePrivileged() {
		if out, err := m.runPrivileged("rm", "-f", path); err != nil {
			return fmt.Errorf("failed to remove %s: %s", path, errOutput(out, err))
		}
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove %s: %w", path, err)
	}
	return nil
}

func (m *SystemdManager) systemctl(args ...string) (string, error) {
	return m.runPrivileged(m.opts.SystemctlBin, args...)
}

// usePrivileged reports whether ln/rm/systemctl must run via sudo -n.
// Native .deb runs as www-data: /etc/systemd/system is never writable, even
// when JRS_JASPER_SYSTEMD_USE_SUDO was left false by an older install.
func (m *SystemdManager) usePrivileged() bool {
	if m.opts.UseSudo {
		return true
	}
	if runtime.GOOS == "windows" {
		return false
	}
	dir := strings.TrimSpace(m.opts.SystemDir)
	if dir == "" {
		return false
	}
	if os.Geteuid() == 0 {
		return false
	}
	return !isWritableDir(dir)
}

func (m *SystemdManager) runPrivileged(name string, args ...string) (string, error) {
	var cmd *exec.Cmd
	if m.usePrivileged() {
		name = privilegedName(name)
		all := append([]string{name}, args...)
		cmd = exec.Command("sudo", append([]string{"-n"}, all...)...)
	} else {
		cmd = exec.Command(name, args...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *SystemdManager) systemctlAvailable() bool {
	bin := m.opts.SystemctlBin
	if bin == "" {
		return false
	}
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir()
}

func symlink(source, link string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("unit file does not exist: %s", source)
	}
	_ = os.Remove(link)
	return os.Symlink(source, link)
}

func isWritableDir(dir string) bool {
	test := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(test, []byte("ok"), 0o644); err != nil {
		return false
	}
	_ = os.Remove(test)
	return true
}

// privilegedName resolves ln/rm/systemctl to the absolute paths listed in
// /etc/sudoers.d/acugis-report-server so sudo -n matches the NOPASSWD rules.
func privilegedName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	// Unix sudoers paths are always /usr/bin/... even when this code is
	// compiled/tested on Windows (filepath.IsAbs("/bin/x") is false there).
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return name
	}
	base := filepath.Base(name)
	for _, dir := range []string{"/usr/bin", "/bin"} {
		p := filepath.Join(dir, base)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func envUseSudo() bool {
	v := strings.TrimSpace(os.Getenv("JRS_JASPER_SYSTEMD_USE_SUDO"))
	if v == "" {
		// Match PHP config.php jasper_systemd_use_sudo default.
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func errOutput(out string, err error) string {
	msg := strings.TrimSpace(out)
	if msg != "" {
		return msg
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

func findPHPBinary() string {
	for _, candidate := range []string{"/usr/bin/php", "/bin/php"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "php"
}
