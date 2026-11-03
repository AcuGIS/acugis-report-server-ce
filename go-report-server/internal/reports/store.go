package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/fileown"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

// Summary is a catalog card projection of a report JSON file.
type Summary struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Datasource   string `json:"datasource"`
	Type         string `json:"type"`
	Format       string `json:"format"`
	HasThumbnail bool   `json:"has_thumbnail"`
}

// Definition is the full report JSON document plus resolved key.
type Definition struct {
	Key  string
	Raw  map[string]any
	Path string
}

// Store reads and writes report definitions under the PHP filesystem layout.
type Store struct {
	jsonDir      string
	thumbnailDir string
	jrxmlDir     string
	fileOwner    string
	fileGroup    string
}

// NewStore constructs a filesystem-backed report store.
// jrxmlDir may be empty when only JSON reads are needed (tests).
func NewStore(jsonDir, thumbnailDir, jrxmlDir string) *Store {
	return &Store{
		jsonDir:      strings.TrimSpace(jsonDir),
		thumbnailDir: strings.TrimSpace(thumbnailDir),
		jrxmlDir:     strings.TrimSpace(jrxmlDir),
	}
}

// SetFileOwner configures optional chown for files written under the reports dir.
func (s *Store) SetFileOwner(owner, group string) {
	if s == nil {
		return
	}
	s.fileOwner = strings.TrimSpace(owner)
	s.fileGroup = strings.TrimSpace(group)
}

func (s *Store) own(path string) {
	if s == nil {
		return
	}
	_ = fileown.Apply(path, s.fileOwner, s.fileGroup)
}

// JSONDir returns the configured JSON directory.
func (s *Store) JSONDir() string {
	if s == nil {
		return ""
	}
	return s.jsonDir
}

// JRXMLDir returns the configured JRXML / .jasper directory.
func (s *Store) JRXMLDir() string {
	if s == nil {
		return ""
	}
	return s.jrxmlDir
}

// NormalizeResourceBase canonicalizes the folder path stored in
// {key}.resource-base. An empty result means Home (no resource-base file).
// The value is a slash-separated catalog folder such as "finance" or
// "finance/q1", never an absolute filesystem path.
func NormalizeResourceBase(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "", nil
	}
	if strings.ContainsRune(raw, 0) || strings.Contains(raw, ":") {
		return "", fmt.Errorf("invalid resource base")
	}
	parts := strings.Split(raw, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid resource base")
		}
	}
	return strings.Join(parts, "/"), nil
}

// SaveResourceBase writes {key}.resource-base next to the JRXML so Tomcat can
// resolve image paths relative to the report's catalog folder (e.g. "finance").
// slashFolderPath is "/finance" or "" for Home.
func (s *Store) SaveResourceBase(key, slashFolderPath string) error {
	if s == nil || s.jrxmlDir == "" {
		return fmt.Errorf("jrxml dir not configured")
	}
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return fmt.Errorf("invalid report key")
	}
	base, err := NormalizeResourceBase(slashFolderPath)
	if err != nil {
		return err
	}
	path := filepath.Join(s.jrxmlDir, key+".resource-base")
	if base == "" {
		_ = os.Remove(path)
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(base+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.own(path)
	return nil
}

// ReadResourceBase reads {key}.resource-base using the same format
// SaveResourceBase writes and Tomcat's ReportsFileRepositoryService reads.
// A missing file is ErrNotFound (Home, or a report that has no resource folder).
func (s *Store) ReadResourceBase(key string) (string, error) {
	if s == nil || s.jrxmlDir == "" {
		return "", fmt.Errorf("jrxml dir not configured")
	}
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return "", ErrInvalidKey
	}
	data, err := os.ReadFile(filepath.Join(s.jrxmlDir, key+".resource-base"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	base, err := NormalizeResourceBase(string(data))
	if err != nil || base == "" {
		if err != nil {
			return "", err
		}
		return "", ErrNotFound
	}
	return base, nil
}

// ThumbnailDir returns the thumbnail directory.
func (s *Store) ThumbnailDir() string {
	if s == nil {
		return ""
	}
	return s.thumbnailDir
}

// Exists reports whether a report JSON file is present.
func (s *Store) Exists(key string) bool {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) || s == nil || s.jsonDir == "" {
		return false
	}
	_, err := os.Stat(s.jsonPath(key))
	return err == nil
}

// List reads all *.report.json files (unsorted ACL). Sorted by display name.
func (s *Store) List() ([]Summary, error) {
	if s == nil || s.jsonDir == "" {
		return nil, fmt.Errorf("report json directory not configured")
	}
	entries, err := filepath.Glob(filepath.Join(s.jsonDir, "*.report.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(entries))
	for _, path := range entries {
		def, err := s.readPath(path)
		if err != nil || def == nil {
			continue
		}
		out = append(out, summaryFromDefinition(def, s.hasThumbnail(def.Key)))
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Get loads one report definition by key.
func (s *Store) Get(key string) (*Definition, error) {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return nil, ErrInvalidKey
	}
	if s == nil || s.jsonDir == "" {
		return nil, fmt.Errorf("report json directory not configured")
	}
	path := s.jsonPath(key)
	def, err := s.readPath(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if def == nil {
		return nil, ErrNotFound
	}
	return def, nil
}

// EnsureDirs creates JSON / JRXML / thumbnail directories when missing (PHP jrs_ensure_dir).
func (s *Store) EnsureDirs() error {
	if s == nil {
		return fmt.Errorf("report store not configured")
	}
	for _, dir := range []string{s.jsonDir, s.jrxmlDir, s.thumbnailDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o775); err != nil {
			return err
		}
	}
	return nil
}

// SaveJSON writes the report definition with PHP-style pretty JSON.
func (s *Store) SaveJSON(key string, raw map[string]any) error {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return ErrInvalidKey
	}
	if s == nil || s.jsonDir == "" {
		return fmt.Errorf("report json directory not configured")
	}
	if raw == nil {
		raw = map[string]any{}
	}
	raw["report"] = key
	if err := os.MkdirAll(s.jsonDir, 0o775); err != nil {
		return err
	}
	data, err := json.MarshalIndent(raw, "", "    ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := s.jsonPath(key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	s.own(path)
	return nil
}

// SaveJRXML writes uploaded JRXML bytes for the report key.
func (s *Store) SaveJRXML(key string, data []byte) error {
	return s.writeReportFile(key, ".jrxml", data)
}

// SaveJasper writes an uploaded compiled .jasper file.
func (s *Store) SaveJasper(key string, data []byte) error {
	return s.writeReportFile(key, ".jasper", data)
}

// SaveThumbnail writes a thumbnail as {key}.png (PHP always uses .png).
func (s *Store) SaveThumbnail(key string, data []byte) error {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return ErrInvalidKey
	}
	if s == nil || s.thumbnailDir == "" {
		return fmt.Errorf("thumbnail directory not configured")
	}
	if err := os.MkdirAll(s.thumbnailDir, 0o775); err != nil {
		return err
	}
	path := filepath.Join(s.thumbnailDir, key+".png")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CompileJRXML runs `jasperstarter compile` like PHP publish_report.php.
// Returns a human-readable note; compile failure is non-fatal for publish.
func (s *Store) CompileJRXML(key string) string {
	return s.CompileJRXMLResult(key).Message
}

// CompileResult is the outcome of a local jasperstarter compile attempt.
type CompileResult struct {
	OK      bool
	Message string
	Detail  string
}

// CompileJRXMLResult runs jasperstarter and returns structured diagnostics.
func (s *Store) CompileJRXMLResult(key string) CompileResult {
	fail := func(detail string) CompileResult {
		msg := "JRXML uploaded, but compile to .jasper failed. Upload a compiled .jasper file or install jasperstarter on the server."
		if detail != "" {
			msg = msg + " (" + detail + ")"
		}
		return CompileResult{OK: false, Message: msg, Detail: detail}
	}
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) || s == nil || s.jrxmlDir == "" {
		return fail("report store not configured")
	}
	jrxml := filepath.Join(s.jrxmlDir, key+".jrxml")
	if _, err := os.Stat(jrxml); err != nil {
		return fail("JRXML not found")
	}
	if _, err := exec.LookPath("jasperstarter"); err != nil {
		return fail("jasperstarter not found on PATH")
	}
	jasperOut := filepath.Join(s.jrxmlDir, key+".jasper")
	cmd := exec.Command("jasperstarter", "compile", jrxml, "-o", jasperOut)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return CompileResult{OK: true, Message: "JRXML compiled to .jasper successfully."}
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	if len(detail) > 240 {
		detail = detail[:240]
	}
	return fail(detail)
}

// RemoveJasper deletes the compiled .jasper for key (ignored if missing).
// Used when JRXML changed but local compile is unavailable so Tomcat can
// compile the JRXML at request time instead of serving a stale .jasper.
func (s *Store) RemoveJasper(key string) error {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) || s == nil || s.jrxmlDir == "" {
		return nil
	}
	path := filepath.Join(s.jrxmlDir, key+".jasper")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// TouchJRXML bumps JRXML mtime so Tomcat treats it as newer than .jasper and recompiles.
func (s *Store) TouchJRXML(key string) error {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) || s == nil || s.jrxmlDir == "" {
		return nil
	}
	path := filepath.Join(s.jrxmlDir, key+".jrxml")
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		return err
	}
	s.own(path)
	return nil
}

// Delete removes JSON, JRXML, .jasper, and thumbnail files (PHP delete_report.php).
// Missing files are ignored. ACL resources are not deleted (PHP parity).
func (s *Store) Delete(key string) error {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return ErrInvalidKey
	}
	if s == nil {
		return fmt.Errorf("report store not configured")
	}
	paths := []string{s.jsonPath(key)}
	if s.jrxmlDir != "" {
		paths = append(paths,
			filepath.Join(s.jrxmlDir, key+".jrxml"),
			filepath.Join(s.jrxmlDir, key+".jasper"),
		)
	}
	if s.thumbnailDir != "" {
		paths = append(paths, filepath.Join(s.thumbnailDir, key+".png"))
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// JRXMLWritable reports whether the JRXML directory exists and is writable.
func (s *Store) JRXMLWritable() bool {
	if s == nil || s.jrxmlDir == "" {
		return false
	}
	info, err := os.Stat(s.jrxmlDir)
	if err != nil || !info.IsDir() {
		return false
	}
	tmp, err := os.CreateTemp(s.jrxmlDir, ".write-probe-*")
	if err != nil {
		return false
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	return true
}

func (s *Store) writeReportFile(key, ext string, data []byte) error {
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return ErrInvalidKey
	}
	if s == nil || s.jrxmlDir == "" {
		return fmt.Errorf("jrxml directory not configured")
	}
	if err := os.MkdirAll(s.jrxmlDir, 0o775); err != nil {
		return err
	}
	path := filepath.Join(s.jrxmlDir, key+ext)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	s.own(s.jrxmlDir)
	s.own(path)
	return nil
}

func (s *Store) jsonPath(key string) string {
	return filepath.Join(s.jsonDir, key+".report.json")
}

func (s *Store) hasThumbnail(key string) bool {
	if s.thumbnailDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.thumbnailDir, key+".png"))
	return err == nil
}

// ThumbnailPath returns the on-disk PNG path for a report key (may not exist).
func (s *Store) ThumbnailPath(key string) string {
	key = jasper.NormalizeReportKey(key)
	if s == nil || s.thumbnailDir == "" || key == "" {
		return ""
	}
	return filepath.Join(s.thumbnailDir, key+".png")
}

func (s *Store) readPath(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	key := stringField(raw, "report")
	if key == "" {
		base := filepath.Base(path)
		key = strings.TrimSuffix(base, ".report.json")
	}
	key = jasper.NormalizeReportKey(key)
	if !jasper.IsValidReportKey(key) {
		return nil, ErrInvalidKey
	}
	raw["report"] = key
	return &Definition{Key: key, Raw: raw, Path: path}, nil
}

func summaryFromDefinition(def *Definition, hasThumb bool) Summary {
	name := stringField(def.Raw, "name")
	if name == "" {
		name = def.Key
	}
	desc := stringField(def.Raw, "description")
	if desc == "" {
		desc = stringField(def.Raw, "summary")
	}
	ds := stringField(def.Raw, "datasource")
	if ds == "" {
		ds = stringField(def.Raw, "datasourceName")
	}
	typ := stringField(def.Raw, "type")
	if typ == "" {
		typ = stringField(def.Raw, "reportType")
	}
	if typ == "" {
		typ = "jasper"
	}
	format := stringField(def.Raw, "format")
	if format == "" {
		format = "html"
	}
	return Summary{
		Key:          def.Key,
		Name:         name,
		Description:  desc,
		Datasource:   ds,
		Type:         normalizeType(typ),
		Format:       format,
		HasThumbnail: hasThumb,
	}
}

func normalizeType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "map", "map_report", "map-report":
		return "map"
	default:
		return "jasper"
	}
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// Parameters extracts parameter definitions for the viewer form.
func (d *Definition) Parameters() []map[string]any {
	if d == nil || d.Raw == nil {
		return nil
	}
	for _, key := range []string{"parameters", "params"} {
		v, ok := d.Raw[key]
		if !ok {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// DisplayName returns the catalog title.
func (d *Definition) DisplayName() string {
	if d == nil {
		return ""
	}
	if n := stringField(d.Raw, "name"); n != "" {
		return n
	}
	return d.Key
}

// Datasource returns the default datasource name.
func (d *Definition) Datasource() string {
	if d == nil {
		return ""
	}
	ds := stringField(d.Raw, "datasource")
	if ds == "" {
		ds = stringField(d.Raw, "datasourceName")
	}
	return ds
}

// QuerySQL returns the report dataset query (CQL/SQL) from report JSON.
// For Cassandra, Designer embeds CQL in JRXML (language=cassandra); runtime
// prefers that. Catalog JSON "query" is a copy/fallback (also from project.json).
func (d *Definition) QuerySQL() string {
	if d == nil || d.Raw == nil {
		return ""
	}
	if s := extractQuerySQL(d.Raw["query"]); s != "" {
		return s
	}
	if s := stringField(d.Raw, "cql"); s != "" {
		return s
	}
	if s := stringField(d.Raw, "manualSql"); s != "" {
		return s
	}
	if s := stringField(d.Raw, "queryString"); s != "" {
		return s
	}
	return ""
}

func extractQuerySQL(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		// Designer QueryDefinition / AuthoritativeSQL-like nesting.
		if s := stringField(t, "manualSql"); s != "" {
			mode := strings.ToLower(stringField(t, "mode"))
			if mode == "" || mode == "sql" {
				return s
			}
		}
		if s := stringField(t, "generatedSql"); s != "" {
			return s
		}
		if s := stringField(t, "manualSql"); s != "" {
			return s
		}
		if s := stringField(t, "queryString"); s != "" {
			return s
		}
		if s := stringField(t, "cql"); s != "" {
			return s
		}
		if s := stringField(t, "sql"); s != "" {
			return s
		}
		return ""
	default:
		return ""
	}
}

// ExtractQueryFromProjectJSON reads query.manualSql (or nested query) from a Designer project.json.
func ExtractQueryFromProjectJSON(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}
	def := &Definition{Raw: raw}
	return def.QuerySQL()
}

// Type returns jasper or map.
func (d *Definition) Type() string {
	if d == nil {
		return "jasper"
	}
	typ := stringField(d.Raw, "type")
	if typ == "" {
		typ = stringField(d.Raw, "reportType")
	}
	return normalizeType(typ)
}

// Format returns the default export format.
func (d *Definition) Format() string {
	if d == nil {
		return "html"
	}
	f := stringField(d.Raw, "format")
	if f == "" {
		return "html"
	}
	return f
}

// NormalizeReportType mirrors PHP jrs_normalize_report_type.
func NormalizeReportType(value string) string {
	return normalizeType(value)
}
