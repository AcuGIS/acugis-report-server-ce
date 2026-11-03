// Package filesystem provides read-only diagnostics for existing AcuGIS paths.
// It never creates, alters, or removes report files.
package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/config"
)

// PathStatus describes a configured filesystem location.
type PathStatus struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	IsDir     bool   `json:"is_dir,omitempty"`
	IsFile    bool   `json:"is_file,omitempty"`
	Readable  bool   `json:"readable"`
	Writable  bool   `json:"writable"`
	Required  bool   `json:"required"`
	Error     string `json:"error,omitempty"`
}

// Paths holds the configured roots from the PHP install.
type Paths struct {
	cfg config.PathConfig
}

// New creates a Paths helper from configuration.
func New(cfg config.PathConfig) *Paths {
	return &Paths{cfg: cfg}
}

// Config returns the underlying path configuration.
func (p *Paths) Config() config.PathConfig {
	return p.cfg
}

// ValidateRoots reports status for each configured location without mutating the filesystem.
func (p *Paths) ValidateRoots() []PathStatus {
	checks := []struct {
		name     string
		path     string
		required bool
		wantDir  bool
		wantFile bool
	}{
		{"report_json_dir", p.cfg.ReportJSONDir, true, true, false},
		{"jrxml_dir", p.cfg.JRXMLDir, true, true, false},
		{"thumbnail_dir", p.cfg.ThumbnailDir, false, true, false},
		{"schedule_dir", p.cfg.ScheduleDir, false, true, false},
		{"schedule_output_dir", p.cfg.ScheduleOutputDir, false, true, false},
		{"generated_report_dir", p.cfg.GeneratedReportDir, false, true, false},
		{"email_tmpl_dir", p.cfg.EmailTmplDir, false, true, false},
		{"jasper_context_path", p.cfg.JasperContextPath, false, false, true},
		{"resources_dir", p.cfg.ResourcesDir, false, true, false},
		{"resource_thumbnails_dir", p.cfg.ResourceThumbnailsDir, false, true, false},
		{"resource_paths_dir", p.cfg.ResourcePathsDir, false, true, false},
	}

	out := make([]PathStatus, 0, len(checks))
	for _, c := range checks {
		out = append(out, inspectPath(c.name, c.path, c.required, c.wantDir, c.wantFile))
	}
	return out
}

// AllRequiredPresent reports whether every required path exists.
func (p *Paths) AllRequiredPresent() bool {
	for _, s := range p.ValidateRoots() {
		if s.Required && !s.Exists {
			return false
		}
	}
	return true
}

// SafeJoin joins root and a relative child, rejecting path traversal.
func SafeJoin(root, child string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("root path is empty")
	}
	cleanRoot := filepath.Clean(root)
	if child == "" {
		return cleanRoot, nil
	}
	// Reject absolute children and volume/root-relative paths (Windows `\foo`, Unix `/foo`).
	if filepath.IsAbs(child) || strings.HasPrefix(child, string(os.PathSeparator)) || strings.HasPrefix(child, "/") {
		return "", fmt.Errorf("absolute child path rejected")
	}
	cleanedChild := filepath.Clean(child)
	if cleanedChild == ".." || strings.HasPrefix(cleanedChild, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal rejected")
	}
	if filepath.IsAbs(cleanedChild) {
		return "", fmt.Errorf("absolute child path rejected")
	}
	joined := filepath.Join(cleanRoot, cleanedChild)
	rel, err := filepath.Rel(cleanRoot, joined)
	if err != nil {
		return "", fmt.Errorf("resolve relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal rejected")
	}
	return joined, nil
}

func inspectPath(name, path string, required, wantDir, wantFile bool) PathStatus {
	st := PathStatus{
		Name:     name,
		Path:     path,
		Required: required,
	}
	if path == "" {
		st.Error = "path not configured"
		return st
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			st.Exists = false
			if required {
				st.Error = "path does not exist"
			}
			return st
		}
		st.Error = err.Error()
		return st
	}
	st.Exists = true
	st.IsDir = info.IsDir()
	st.IsFile = !info.IsDir()
	if wantDir && !info.IsDir() {
		st.Error = "expected directory"
	}
	if wantFile && info.IsDir() {
		st.Error = "expected file"
	}
	st.Readable = isReadable(path)
	// Writable is inferred from mode bits only — never create or touch files.
	st.Writable = info.Mode().Perm()&0200 != 0
	return st
}

func isReadable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
