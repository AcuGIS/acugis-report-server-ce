package schedules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var envLineSafeRe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,.\/-]+$`)

// EnvLine formats a systemd EnvironmentFile line (PHP jrs_schedule_env_line).
func EnvLine(key, value string) string {
	if value == "" || envLineSafeRe.MatchString(value) {
		return key + "=" + value
	}
	return fmt.Sprintf("%s=%s", key, shellQuote(value))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// BuildEnvFile writes {id}_env.sh for a schedule row.
func BuildEnvFile(paths Paths, scheduleID int, schedule Schedule, jasperHome string) error {
	if strings.TrimSpace(schedule.ReportName) == "" {
		return fmt.Errorf("cannot build EnvironmentFile: report_name is empty")
	}
	if strings.TrimSpace(schedule.DatasourceName) == "" {
		return fmt.Errorf("cannot build EnvironmentFile: datasource_name is empty")
	}
	if err := Mkdir(paths.ScheduleDir); err != nil {
		return fmt.Errorf("could not create schedule directory: %w", err)
	}

	lines := []string{
		"JRS_SCHEDULE_ID=" + formatInt(scheduleID),
		EnvLine("JRS_REPORT_NAME", schedule.ReportName),
		EnvLine("JRS_DATASOURCE", schedule.DatasourceName),
		EnvLine("JRS_FORMAT", defaultString(schedule.Format, "pdf")),
		EnvLine("JRS_FILENAME", schedule.Filename),
		EnvLine("JRS_EMAIL", schedule.Email),
		EnvLine("JRS_EMAIL_SUBJECT", schedule.EmailSubject),
		EnvLine("JRS_EMAIL_BODY", schedule.EmailBody),
		EnvLine("JRS_EMAIL_TEMPLATE", schedule.EmailTemplate),
		"JRS_NOEMAIL=" + boolTo01(schedule.NoEmail),
		EnvLine("JRS_URL_OPT_PARAMS", schedule.URLOptParams),
		EnvLine("JRS_JASPER_HOME", jasperHome),
	}
	path := paths.EnvPath(scheduleID)
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}
	return nil
}

// RemoveEnvFile deletes the schedule env file if present.
func RemoveEnvFile(paths Paths, scheduleID int) {
	path := paths.EnvPath(scheduleID)
	_ = os.Remove(path)
}

// ReadEnvFile parses a systemd EnvironmentFile into key/value pairs.
func ReadEnvFile(paths Paths, scheduleID int) map[string]string {
	path := paths.EnvPath(scheduleID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	vars := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := ParseEnvValue(strings.TrimSpace(line[eq+1:]))
		if key != "" {
			vars[key] = value
		}
	}
	return vars
}

// ParseEnvValue unquotes shell-style env values (PHP jrs_schedule_parse_env_value).
func ParseEnvValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		inner := raw[1 : len(raw)-1]
		return strings.ReplaceAll(inner, "'\\''", "'")
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		unquoted := raw[1 : len(raw)-1]
		return unescapeDoubleQuoted(unquoted)
	}
	return raw
}

func unescapeDoubleQuoted(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\', '"':
				b.WriteByte(s[i])
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// EnvBool parses env boolean values.
func EnvBool(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "1" || v == "true" || v == "t" || v == "yes" || v == "on"
}

// EnvOverridePresent is true when an env value should override DB fields.
func EnvOverridePresent(value string) bool {
	return strings.TrimSpace(value) != ""
}

// RuntimeEnv reads env file plus process environment overrides.
func RuntimeEnv(paths Paths, scheduleID int) map[string]string {
	vars := map[string]string{}
	for k, v := range ReadEnvFile(paths, scheduleID) {
		if EnvOverridePresent(v) {
			vars[k] = v
		}
	}
	for _, key := range []string{
		"JRS_SCHEDULE_ID", "JRS_REPORT_NAME", "JRS_DATASOURCE", "JRS_FORMAT", "JRS_FILENAME",
		"JRS_EMAIL", "JRS_EMAIL_SUBJECT", "JRS_EMAIL_BODY", "JRS_EMAIL_TEMPLATE",
		"JRS_NOEMAIL", "JRS_URL_OPT_PARAMS",
	} {
		if v, ok := os.LookupEnv(key); ok && EnvOverridePresent(v) {
			vars[key] = v
		}
	}
	return vars
}

// ApplyRuntimeEnv overlays non-empty env values onto a schedule row.
func ApplyRuntimeEnv(schedule Schedule, env map[string]string) Schedule {
	if len(env) == 0 {
		return schedule
	}
	fieldMap := map[string]*string{
		"JRS_REPORT_NAME":    &schedule.ReportName,
		"JRS_DATASOURCE":     &schedule.DatasourceName,
		"JRS_FORMAT":         &schedule.Format,
		"JRS_FILENAME":       &schedule.Filename,
		"JRS_EMAIL":          &schedule.Email,
		"JRS_EMAIL_SUBJECT":  &schedule.EmailSubject,
		"JRS_EMAIL_BODY":     &schedule.EmailBody,
		"JRS_EMAIL_TEMPLATE": &schedule.EmailTemplate,
		"JRS_URL_OPT_PARAMS": &schedule.URLOptParams,
	}
	for envKey, field := range fieldMap {
		if v, ok := env[envKey]; ok && EnvOverridePresent(v) {
			*field = v
		}
	}
	if v, ok := env["JRS_NOEMAIL"]; ok && EnvOverridePresent(v) {
		schedule.NoEmail = EnvBool(v)
	}
	return schedule
}

// EnvSnapshotUsable reports whether env file contents look valid.
func EnvSnapshotUsable(contents string) bool {
	if strings.TrimSpace(contents) == "" {
		return false
	}
	matched, _ := regexp.MatchString(`(?m)^JRS_REPORT_NAME=\S`, contents)
	return matched
}

func boolTo01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// ResolveEnvPath returns absolute env path for logging.
func ResolveEnvPath(paths Paths, scheduleID int) string {
	p := paths.EnvPath(scheduleID)
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
