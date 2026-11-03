package schedules

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var emailTmplExtRe = regexp.MustCompile(`(?i)\.(html?|txt)$`)

// EmailTemplateStore lists and loads HTML templates from email_tmpl dir.
type EmailTemplateStore struct {
	dir string
}

// NewEmailTemplateStore constructs a template store.
func NewEmailTemplateStore(dir string) *EmailTemplateStore {
	return &EmailTemplateStore{dir: strings.TrimSpace(dir)}
}

// List returns template filenames for the schedule dropdown.
func (s *EmailTemplateStore) List() []string {
	if s == nil || s.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !emailTmplExtRe.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}

// ResolvePath resolves a template name to a readable file path (no traversal).
func (s *EmailTemplateStore) ResolvePath(name string) string {
	if s == nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = filepath.Base(strings.ReplaceAll(strings.ReplaceAll(name, `\`, ""), `/`, ""))
	if name == "" {
		return ""
	}
	path := filepath.Join(s.dir, name)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	if !strings.Contains(name, ".") {
		for _, ext := range []string{".html", ".htm", ".txt"} {
			try := filepath.Join(s.dir, name+ext)
			if info, err := os.Stat(try); err == nil && !info.IsDir() {
				return try
			}
		}
	}
	return ""
}

// Load returns template contents or false when missing.
func (s *EmailTemplateStore) Load(name string) (string, bool) {
	path := s.ResolvePath(name)
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "", false
	}
	return string(data), true
}

// IsHTML reports whether template content should be sent as HTML.
func (s *EmailTemplateStore) IsHTML(name, contents string) bool {
	if matched, _ := regexp.MatchString(`(?i)\.html?$`, name); matched {
		return true
	}
	trim := strings.TrimLeft(contents, " \t\r\n")
	lower := strings.ToLower(trim)
	return strings.HasPrefix(lower, "<!doctype") ||
		strings.HasPrefix(lower, "<html") ||
		strings.HasPrefix(lower, "<body")
}

// ApplyEmailPlaceholders replaces {{key}} placeholders.
func ApplyEmailPlaceholders(text string, vars map[string]string) string {
	for key, value := range vars {
		text = strings.ReplaceAll(text, "{{"+key+"}}", value)
	}
	return text
}
