package duckdb

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// FileEntries parses registered remote analytical files from connection options.
func FileEntries(cfg Config) []FileEntry {
	if cfg.Options == nil {
		return nil
	}
	raw := strings.TrimSpace(cfg.Options[OptionFiles])
	if raw == "" {
		raw = strings.TrimSpace(cfg.Options[OptionFileAliases])
	}
	if raw == "" {
		return nil
	}
	var entries []FileEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		parts := strings.Split(raw, ";")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			entries = append(entries, FileEntry{
				Alias:  defaultAlias(p),
				Source: p,
				Format: FormatAuto,
			})
		}
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		src := e.RemoteSource()
		if src == "" {
			continue
		}
		alias := strings.TrimSpace(e.Alias)
		if alias == "" {
			alias = defaultAlias(src)
		}
		out = append(out, FileEntry{
			Alias:  sanitizeAlias(alias),
			Source: src,
			Format: NormalizeFormat(e.Format),
		})
	}
	return out
}

// NormalizeFormat maps user/format strings to auto|parquet|csv|json.
func NormalizeFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", FormatAuto, "detect", "auto_detect", "autodetect":
		return FormatAuto
	case FormatParquet, "parq", "pq":
		return FormatParquet
	case FormatCSV, "tsv", "txt", "text":
		return FormatCSV
	case FormatJSON, "jsonl", "ndjson":
		return FormatJSON
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}

// IsRemoteHTTPSource reports whether s is an http:// or https:// URL.
func IsRemoteHTTPSource(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// ValidateRemoteSource ensures Data Files sources are HTTP(S) only for this phase.
func ValidateRemoteSource(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("remote source URL is required")
	}
	if !IsRemoteHTTPSource(source) {
		return fmt.Errorf("Data Files requires an HTTP or HTTPS URL (local paths are not supported in this phase): %s", RedactRemoteURL(source))
	}
	u, err := url.Parse(source)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid remote source URL: %s", RedactRemoteURL(source))
	}
	return nil
}

func defaultAlias(source string) string {
	base := sourceBaseName(source)
	name := stripDataFileExt(base)
	return sanitizeAlias(name)
}

func sourceBaseName(source string) string {
	s := strings.TrimSpace(source)
	if s == "" {
		return ""
	}
	if IsRemoteHTTPSource(s) {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			base := path.Base(u.Path)
			if base != "" && base != "." && base != "/" {
				return base
			}
		}
	}
	s = strings.ReplaceAll(s, `\`, `/`)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func stripDataFileExt(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{
		".csv.gz", ".tsv.gz", ".txt.gz", ".json.gz", ".jsonl.gz", ".ndjson.gz",
		".parquet", ".parq",
		".csv", ".tsv", ".txt",
		".jsonl", ".ndjson", ".json",
	} {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[:i]
	}
	return name
}

func sanitizeAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "data"
	}
	var b strings.Builder
	for i, r := range alias {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
		if i == 0 && r >= '0' && r <= '9' {
			b.WriteByte('_')
		}
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" {
		return "data"
	}
	return s
}

// EncodeFileEntries serializes file entries for connection options.
func EncodeFileEntries(entries []FileEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	normalized := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		src := e.RemoteSource()
		if src == "" {
			continue
		}
		alias := strings.TrimSpace(e.Alias)
		if alias == "" {
			alias = defaultAlias(src)
		}
		normalized = append(normalized, FileEntry{
			Alias:  sanitizeAlias(alias),
			Source: src,
			Format: NormalizeFormat(e.Format),
		})
	}
	if len(normalized) == 0 {
		return "[]"
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// LogicalFileNames returns portable aliases.
func LogicalFileNames(cfg Config) []string {
	entries := FileEntries(cfg)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Alias)
	}
	return out
}

// FindFileEntry returns the FileEntry matching a logical table/alias name.
func FindFileEntry(cfg Config, alias string) (FileEntry, bool) {
	alias = strings.TrimSpace(alias)
	for _, e := range FileEntries(cfg) {
		if e.Alias == alias || strings.EqualFold(e.Alias, alias) {
			return e, true
		}
	}
	return FileEntry{}, false
}

// DefaultAliasFromSource returns a sanitized alias derived from a remote URL.
func DefaultAliasFromSource(source string) string {
	return defaultAlias(source)
}
