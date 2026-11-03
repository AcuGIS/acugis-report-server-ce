package schedules

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Paths resolves schedule filesystem locations (PHP lib/schedule_admin.php).
type Paths struct {
	ScheduleDir       string
	ScheduleOutputDir string
	LogDir            string
	EmailTmplDir      string
	AppRoot           string
}

// NewPaths builds paths from config.Paths-style values.
func NewPaths(scheduleDir, scheduleOutputDir, emailTmplDir, appRoot string) Paths {
	scheduleDir = strings.TrimRight(strings.TrimSpace(scheduleDir), `/\`)
	if scheduleDir == "" && appRoot != "" {
		scheduleDir = filepath.Join(appRoot, "schedules")
	}
	outputDir := strings.TrimSpace(scheduleOutputDir)
	if outputDir == "" {
		outputDir = filepath.Join(scheduleDir, "output")
	}
	emailDir := strings.TrimSpace(emailTmplDir)
	if emailDir == "" && appRoot != "" {
		emailDir = filepath.Join(appRoot, "email_tmpl")
	}
	return Paths{
		ScheduleDir:       scheduleDir,
		ScheduleOutputDir: outputDir,
		LogDir:            filepath.Join(scheduleDir, "logs"),
		EmailTmplDir:      emailDir,
		AppRoot:           strings.TrimSpace(appRoot),
	}
}

// EnvPath returns {schedule_dir}/{id}_env.sh.
func (p Paths) EnvPath(scheduleID int) string {
	return filepath.Join(p.ScheduleDir, formatInt(scheduleID)+"_env.sh")
}

// OutputDirForSchedule returns schedules/output/{id}.
func (p Paths) OutputDirForSchedule(scheduleID int) string {
	return filepath.Join(p.ScheduleOutputDir, formatInt(scheduleID))
}

// LogPath returns schedules/logs/schedule-{id}.log.
func (p Paths) LogPath(scheduleID int) string {
	return filepath.Join(p.LogDir, "schedule-"+formatInt(scheduleID)+".log")
}

// ServiceName returns jrs-schedule-{id}.service.
func ServiceName(scheduleID int) string {
	return unitPrefix() + formatInt(scheduleID) + ".service"
}

// TimerName returns jrs-schedule-{id}.timer.
func TimerName(scheduleID int) string {
	return unitPrefix() + formatInt(scheduleID) + ".timer"
}

func unitPrefix() string {
	return "jrs-schedule-"
}

// UnitNames returns service and timer unit filenames.
func UnitNames(scheduleID int) []string {
	return []string{ServiceName(scheduleID), TimerName(scheduleID)}
}

// Mkdir creates a directory with 0775 permissions.
func Mkdir(dir string) error {
	if dir == "" {
		return os.ErrInvalid
	}
	info, err := os.Stat(dir)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		return os.ErrExist
	}
	return os.MkdirAll(dir, 0o775)
}

// RequireWritableDir creates dir and verifies writability.
func RequireWritableDir(dir, label string) error {
	if err := Mkdir(dir); err != nil {
		return err
	}
	return checkWritable(dir, label)
}

// PrepareOutputDir creates schedules/output/{id} for a run.
func (p Paths) PrepareOutputDir(scheduleID int) (string, error) {
	if err := RequireWritableDir(p.ScheduleDir, "Schedule directory"); err != nil {
		return "", err
	}
	if err := RequireWritableDir(p.ScheduleOutputDir, "Schedule output directory"); err != nil {
		return "", err
	}
	out := p.OutputDirForSchedule(scheduleID)
	if err := RequireWritableDir(out, "Output directory"); err != nil {
		return "", err
	}
	return out, nil
}

func checkWritable(dir, label string) error {
	testFile := filepath.Join(dir, ".write_test_"+formatInt(os.Getpid()))
	if err := os.WriteFile(testFile, []byte("ok"), 0o664); err != nil {
		return &PathError{Dir: dir, Label: label, Err: err}
	}
	_ = os.Remove(testFile)
	return nil
}

// PathError describes a non-writable schedule directory.
type PathError struct {
	Dir   string
	Label string
	Err   error
}

func (e *PathError) Error() string {
	user := ServiceUser()
	return e.Label + " is not writable:\n" + e.Dir + "\n\n" +
		"Current owner/group mismatch.\n" +
		"Expected writable by " + user + ".\n" +
		"Run:\n\n" +
		"chown -R " + user + ":" + user + " " + e.Dir + "\n" +
		"chmod 775 " + e.Dir
}

func (e *PathError) Unwrap() error { return e.Err }

// ServiceUser is the Unix user for systemd units (PHP jrs_schedule_service_user).
func ServiceUser() string {
	return "www-data"
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}
