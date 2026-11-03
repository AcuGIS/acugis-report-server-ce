package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnvFiles loads KEY=VALUE pairs from optional .env files without
// overriding variables already present in the process environment.
// Searches go-report-server/.env (cwd variants) and the PHP install root.
func LoadDotEnvFiles(appRoot string) {
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, ".env"),
			filepath.Join(wd, "go-report-server", ".env"),
		)
		if filepath.Base(wd) == "go-report-server" {
			candidates = append(candidates, filepath.Join(filepath.Dir(wd), ".env"))
		}
	}
	if appRoot != "" {
		candidates = append(candidates,
			filepath.Join(appRoot, ".env"),
			filepath.Join(appRoot, "go-report-server", ".env"),
		)
	}
	if v := strings.TrimSpace(os.Getenv("JRS_SYSTEMD_ENV_FILE")); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates, "/etc/acugis-report-server/env")
	seen := map[string]struct{}{}
	for _, path := range candidates {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		_ = loadDotEnvFile(path)
	}
}

func loadDotEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		val = unquoteEnv(val)
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}

func unquoteEnv(val string) string {
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			return val[1 : len(val)-1]
		}
	}
	return val
}
