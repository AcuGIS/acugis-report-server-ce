package datasources

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/acugis/acugis-report-server-go/excelfile"
	"github.com/acugis/acugis-report-server-go/jsonurl"
	"github.com/acugis/acugis-report-server-go/xmlfile"
)

const (
	documentDir      = "datasources"
	documentIncoming = "incoming"
	// DefaultDocumentRoot is the Designer Web catalog directory.
	// Uploaded CSV, JSON, Excel, and XML files live under datasources/<id>/.
	DefaultDocumentRoot = "/var/lib/acugis-report-designer-web"
)

// DefaultDatasourceFilesDir is the shared catalog root for document files.
// JRS_DATASOURCE_FILES_DIR overrides WEB_DESIGNER_DATA_DIR.
func DefaultDatasourceFilesDir() string {
	if v := strings.TrimSpace(os.Getenv("JRS_DATASOURCE_FILES_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("WEB_DESIGNER_DATA_DIR")); v != "" {
		return v
	}
	return DefaultDocumentRoot
}

// DocumentInput is one Add Datasource upload or an edit that keeps the stored file.
type DocumentInput struct {
	Root        string
	Existing    *Datasource
	UploadName  string
	Upload      []byte
	ClaimedPath string
}

// Discovery lists worksheets or record collections in a document.
type Discovery struct {
	Sheets      []string
	Collections []string
}

// PrepareDocument stores a file-backed document or validates a JSON document URL.
// The resulting datasource uses the Designer option keys.
func PrepareDocument(ds *Datasource, in DocumentInput) error {
	if ds == nil || !ds.IsDocument() {
		return fmt.Errorf("document datasource is required")
	}
	clearDocumentJDBC(ds)
	if ds.IsJSONURL() {
		if err := prepareJSONURL(ds, in); err != nil {
			return err
		}
		return NormalizeDocumentRecord(ds)
	}
	if err := prepareDocumentFile(ds, in); err != nil {
		return err
	}
	return NormalizeDocumentRecord(ds)
}

// NormalizeDocumentRecord checks a document row before it is written to jasper_datasource.
func NormalizeDocumentRecord(ds *Datasource) error {
	if ds == nil || !ds.IsDocument() {
		return fmt.Errorf("document datasource is required")
	}
	clearDocumentJDBC(ds)
	if ds.Options == nil {
		ds.Options = map[string]string{}
	}
	if ds.IsJSON() && strings.EqualFold(ds.Option(OptionJSONSource), JSONSourceURL) {
		raw := ds.Option(OptionJSONURL)
		if err := jsonurl.Validate(raw); err != nil {
			return err
		}
		ds.Options[OptionJSONSource] = JSONSourceURL
		ds.Options[OptionJSONURL] = raw
		delete(ds.Options, OptionResourceFile)
		ds.DBName = ""
		if c := ds.Option(OptionCollection); c != "" {
			if err := validateCollection(c); err != nil {
				return err
			}
			ds.Options[OptionCollection] = c
		} else {
			delete(ds.Options, OptionCollection)
		}
		delete(ds.Options, OptionDelimiter)
		delete(ds.Options, OptionSheet)
		return nil
	}
	logical := firstNonEmpty(ds.Option(OptionResourceFile), ds.DBName)
	owner, filename, ok := parseOwned(logical)
	if !ok || !allowedDocumentExt(ds.DBType, filename) {
		return fmt.Errorf("document path is not accepted")
	}
	logical = documentDir + "/" + owner + "/" + filename
	ds.DBName = logical
	ds.Options[OptionResourceFile] = logical
	switch NormalizeDBType(ds.DBType) {
	case "csv":
		name := ds.Option(OptionDelimiter)
		if name == "" {
			name = defaultDelimiter(filename)
		}
		if _, _, err := collectionfile.ParseDelimiter(name); err != nil {
			return err
		}
		ds.Options[OptionDelimiter] = collectionfile.DelimiterName(mustDelim(name))
		delete(ds.Options, OptionJSONSource)
		delete(ds.Options, OptionJSONURL)
		delete(ds.Options, OptionCollection)
		delete(ds.Options, OptionSheet)
	case "json":
		ds.Options[OptionJSONSource] = JSONSourceFile
		delete(ds.Options, OptionJSONURL)
		delete(ds.Options, OptionDelimiter)
		delete(ds.Options, OptionSheet)
		if c := ds.Option(OptionCollection); c != "" {
			if err := validateCollection(c); err != nil {
				return err
			}
			ds.Options[OptionCollection] = c
		}
	case "excel":
		sheet := ds.Option(OptionSheet)
		if sheet == "" || strings.Contains(sheet, "..") || strings.ContainsAny(sheet, `/\`) {
			return fmt.Errorf("worksheet is required")
		}
		ds.Options[OptionSheet] = sheet
		delete(ds.Options, OptionJSONSource)
		delete(ds.Options, OptionJSONURL)
		delete(ds.Options, OptionDelimiter)
		delete(ds.Options, OptionCollection)
	case "xml":
		c := ds.Option(OptionCollection)
		if err := validateCollection(c); err != nil {
			return err
		}
		if c == "" {
			return fmt.Errorf("record collection is required")
		}
		ds.Options[OptionCollection] = c
		delete(ds.Options, OptionJSONSource)
		delete(ds.Options, OptionJSONURL)
		delete(ds.Options, OptionDelimiter)
		delete(ds.Options, OptionSheet)
	}
	return nil
}

// DiscoverBytes lists worksheets or collections in an uploaded document.
func DiscoverBytes(dbType, filename string, body []byte) (Discovery, error) {
	if err := validateUpload(dbType, filename, body); err != nil {
		return Discovery{}, err
	}
	return discoverBody(dbType, filename, body)
}

// DiscoverStored lists worksheets or collections in a catalog file.
func DiscoverStored(root string, ds Datasource) (Discovery, error) {
	body, filename, err := readStored(root, ds)
	if err != nil {
		return Discovery{}, err
	}
	return discoverBody(ds.DBType, filename, body)
}

// TestDocumentBytes checks an uploaded document with the existing parsers.
func TestDocumentBytes(ctx context.Context, ds Datasource, filename string, body []byte) (string, error) {
	if ds.IsJSONURL() {
		return testJSONURL(ctx, ds)
	}
	if err := validateUpload(ds.DBType, filename, body); err != nil {
		return "", err
	}
	return testBody(ds, filename, body)
}

// TestDocumentStored checks the catalog file already saved for ds.
func TestDocumentStored(ctx context.Context, root string, ds Datasource) (string, error) {
	if ds.IsJSONURL() {
		return testJSONURL(ctx, ds)
	}
	body, filename, err := readStored(root, ds)
	if err != nil {
		return "", err
	}
	return testBody(ds, filename, body)
}

// RemoveDocument deletes the catalog directory for a managed document path.
func RemoveDocument(root, logical string) {
	owner, _, ok := parseOwned(logical)
	if !ok || strings.TrimSpace(root) == "" {
		return
	}
	dir, err := resolveInside(root, documentDir+"/"+owner)
	if err != nil {
		return
	}
	_ = os.RemoveAll(dir)
}

func prepareJSONURL(ds *Datasource, in DocumentInput) error {
	if ds.Options == nil {
		ds.Options = map[string]string{}
	}
	raw := ds.Option(OptionJSONURL)
	if err := jsonurl.Validate(raw); err != nil {
		return err
	}
	if in.Existing != nil {
		prev := firstNonEmpty(in.Existing.Option(OptionResourceFile), in.Existing.DBName)
		RemoveDocument(in.Root, prev)
	}
	ds.Options[OptionJSONSource] = JSONSourceURL
	ds.Options[OptionJSONURL] = raw
	delete(ds.Options, OptionResourceFile)
	ds.DBName = ""
	return nil
}

func prepareDocumentFile(ds *Datasource, in DocumentInput) error {
	if strings.TrimSpace(in.Root) == "" {
		return fmt.Errorf("document storage is not configured")
	}
	claimed := strings.TrimSpace(in.ClaimedPath)
	if claimed != "" {
		if strings.Contains(claimed, "..") || strings.ContainsAny(claimed, `\`) {
			return fmt.Errorf("document path is not accepted")
		}
		if _, _, ok := parseOwned(claimed); !ok {
			return fmt.Errorf("document path is not accepted")
		}
	}
	existingLogical := ""
	if in.Existing != nil && in.Existing.IsDocument() && !in.Existing.IsJSONURL() {
		existingLogical = firstNonEmpty(in.Existing.Option(OptionResourceFile), in.Existing.DBName)
		if _, _, ok := parseOwned(existingLogical); !ok {
			existingLogical = ""
		}
	}
	if claimed != "" && existingLogical != "" && path.Clean(claimed) != path.Clean(existingLogical) {
		return fmt.Errorf("document path is not accepted")
	}
	if claimed != "" && existingLogical == "" {
		return fmt.Errorf("document path is not accepted")
	}

	var filename string
	var body []byte
	var err error
	if len(in.Upload) > 0 || strings.TrimSpace(in.UploadName) != "" {
		filename, err = cleanUploadName(in.UploadName)
		if err != nil {
			return err
		}
		if err := validateUpload(ds.DBType, filename, in.Upload); err != nil {
			return err
		}
		body = in.Upload
	} else if existingLogical != "" {
		body, filename, err = readLogical(in.Root, existingLogical)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("a document file is required")
	}

	owner := ""
	if existingLogical != "" {
		owner, _, _ = parseOwned(existingLogical)
	}
	if owner == "" {
		owner, err = newOwner()
		if err != nil {
			return err
		}
	}
	logical := documentDir + "/" + owner + "/" + filename
	if len(in.Upload) > 0 {
		if err := writeLogical(in.Root, logical, body); err != nil {
			return err
		}
	}
	if ds.Options == nil {
		ds.Options = map[string]string{}
	}
	ds.DBName = logical
	ds.Options[OptionResourceFile] = logical
	return bindDiscovered(ds, filename, body)
}

func bindDiscovered(ds *Datasource, filename string, body []byte) error {
	switch NormalizeDBType(ds.DBType) {
	case "csv":
		name := ds.Option(OptionDelimiter)
		if name == "" {
			name = defaultDelimiter(filename)
		}
		if _, _, err := collectionfile.ParseDelimiter(name); err != nil {
			return err
		}
		delim, auto := mustDelim(name)
		table, err := collectionfile.ParseCSV(bytes.NewReader(body), collectionfile.Options{Delimiter: delim, Auto: auto})
		if err != nil {
			return err
		}
		if table == nil || len(table.Columns) == 0 {
			return fmt.Errorf("CSV document has no columns")
		}
		ds.Options[OptionDelimiter] = collectionfile.DelimiterName(delim, auto)
		return nil
	case "json":
		doc, err := collectionfile.ParseJSON(bytes.NewReader(body))
		if err != nil {
			return err
		}
		selected, err := doc.Select(ds.Option(OptionCollection), "")
		if err != nil {
			return err
		}
		ds.Options[OptionJSONSource] = JSONSourceFile
		ds.Options[OptionCollection] = selected
		return nil
	case "excel":
		wb, err := excelfile.Open(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return err
		}
		defer wb.Close()
		sheets, err := wb.Sheets()
		if err != nil {
			return err
		}
		selected, err := excelfile.Select(sheets, ds.Option(OptionSheet), "")
		if err != nil {
			return err
		}
		if _, err := wb.Table(selected); err != nil {
			return err
		}
		ds.Options[OptionSheet] = selected
		return nil
	case "xml":
		doc, err := xmlfile.Parse(bytes.NewReader(body))
		if err != nil {
			return err
		}
		selected, err := doc.Select(ds.Option(OptionCollection), "")
		if err != nil {
			return err
		}
		ds.Options[OptionCollection] = selected
		return nil
	default:
		return fmt.Errorf("document datasource is required")
	}
}

func discoverBody(dbType, filename string, body []byte) (Discovery, error) {
	switch NormalizeDBType(dbType) {
	case "csv":
		return Discovery{}, nil
	case "json":
		doc, err := collectionfile.ParseJSON(bytes.NewReader(body))
		if err != nil {
			return Discovery{}, err
		}
		var names []string
		for _, c := range doc.Collections() {
			names = append(names, c.Path)
		}
		return Discovery{Collections: names}, nil
	case "excel":
		wb, err := excelfile.Open(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return Discovery{}, err
		}
		defer wb.Close()
		sheets, err := wb.Sheets()
		if err != nil {
			return Discovery{}, err
		}
		return Discovery{Sheets: sheets}, nil
	case "xml":
		doc, err := xmlfile.Parse(bytes.NewReader(body))
		if err != nil {
			return Discovery{}, err
		}
		var names []string
		for _, c := range doc.Collections() {
			names = append(names, c.Path)
		}
		return Discovery{Collections: names}, nil
	default:
		return Discovery{}, fmt.Errorf("document datasource is required")
	}
}

func testBody(ds Datasource, filename string, body []byte) (string, error) {
	switch NormalizeDBType(ds.DBType) {
	case "csv":
		name := ds.Option(OptionDelimiter)
		if name == "" {
			name = defaultDelimiter(filename)
		}
		delim, auto := mustDelim(name)
		table, err := collectionfile.ParseCSV(bytes.NewReader(body), collectionfile.Options{Delimiter: delim, Auto: auto})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Read %d columns from %s", len(table.Columns), filename), nil
	case "json":
		doc, err := collectionfile.ParseJSON(bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		selected, err := doc.Select(ds.Option(OptionCollection), "")
		if err != nil {
			return "", err
		}
		table, err := doc.Table(selected)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Read %d columns from %s", len(table.Columns), selected), nil
	case "excel":
		wb, err := excelfile.Open(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return "", err
		}
		defer wb.Close()
		sheets, err := wb.Sheets()
		if err != nil {
			return "", err
		}
		selected, err := excelfile.Select(sheets, ds.Option(OptionSheet), "")
		if err != nil {
			return "", err
		}
		table, err := wb.Table(selected)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Read %d columns from %s", len(table.Columns), selected), nil
	case "xml":
		doc, err := xmlfile.Parse(bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		selected, err := doc.Select(ds.Option(OptionCollection), "")
		if err != nil {
			return "", err
		}
		table, err := doc.Table(selected)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Read %d columns from %s", len(table.Columns), selected), nil
	default:
		return "", fmt.Errorf("document datasource is required")
	}
}

func testJSONURL(ctx context.Context, ds Datasource) (string, error) {
	raw := ds.Option(OptionJSONURL)
	if err := jsonurl.Validate(raw); err != nil {
		return "", err
	}
	body, err := jsonurl.Get(ctx, raw)
	if err != nil {
		return "", err
	}
	doc, err := collectionfile.ParseJSON(bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	selected, err := doc.Select(ds.Option(OptionCollection), "")
	if err != nil {
		return "", err
	}
	table, err := doc.Table(selected)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Read %d columns from %s", len(table.Columns), selected), nil
}

func clearDocumentJDBC(ds *Datasource) {
	ds.DBType = NormalizeDBType(ds.DBType)
	ds.Host = ""
	ds.Port = 0
	ds.Username = ""
	ds.Password = ""
	ds.SchemaName = ""
	ds.AuthType = AuthPassword
	ds.PublicKeyResourceUUID = ""
	ds.PrivateKeyResourceUUID = ""
}

func validateUpload(dbType, filename string, body []byte) error {
	name, err := cleanUploadName(filename)
	if err != nil {
		return err
	}
	if !allowedDocumentExt(dbType, name) {
		return fmt.Errorf("%s documents must use %s", MetaForDBType(dbType).Label, extList(dbType))
	}
	if len(body) == 0 {
		return fmt.Errorf("a document file is required")
	}
	limit := collectionfile.MaxBytes
	if NormalizeDBType(dbType) == "excel" && int(excelfile.MaxFileBytes) < limit {
		limit = int(excelfile.MaxFileBytes)
	}
	if len(body) > limit {
		return fmt.Errorf("document exceeds the 32 MB limit")
	}
	return nil
}

func allowedDocumentExt(dbType, filename string) bool {
	ext := strings.ToLower(path.Ext(filename))
	for _, want := range documentExts(dbType) {
		if ext == want {
			return true
		}
	}
	return false
}

func documentExts(dbType string) []string {
	switch NormalizeDBType(dbType) {
	case "csv":
		return []string{".csv", ".tsv", ".txt"}
	case "json":
		return []string{".json"}
	case "excel":
		return []string{".xlsx"}
	case "xml":
		return []string{".xml"}
	default:
		return nil
	}
}

func extList(dbType string) string {
	return strings.Join(documentExts(dbType), ", ")
}

func defaultDelimiter(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".tsv":
		return "tab"
	case ".txt":
		return "auto"
	default:
		return "comma"
	}
}

func mustDelim(name string) (rune, bool) {
	delim, auto, err := collectionfile.ParseDelimiter(name)
	if err != nil {
		return ',', false
	}
	return delim, auto
}

func validateCollection(c string) error {
	if strings.Contains(c, "..") || strings.ContainsAny(c, `\`) {
		return fmt.Errorf("record collection is not accepted")
	}
	return nil
}

func cleanUploadName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid document name")
	}
	if !safeFilename(name) {
		return "", fmt.Errorf("invalid document name")
	}
	return name, nil
}

func readStored(root string, ds Datasource) ([]byte, string, error) {
	logical := firstNonEmpty(ds.Option(OptionResourceFile), ds.DBName)
	return readLogical(root, logical)
}

func readLogical(root, logical string) ([]byte, string, error) {
	_, filename, ok := parseOwned(logical)
	if !ok {
		return nil, "", fmt.Errorf("document path is not accepted")
	}
	abs, err := resolveInside(root, logical)
	if err != nil {
		return nil, "", fmt.Errorf("document path is not accepted")
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return nil, "", fmt.Errorf("document is unavailable: %s", filename)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		return nil, "", fmt.Errorf("document path is not accepted")
	}
	if st.Size() > int64(collectionfile.MaxBytes) {
		return nil, "", fmt.Errorf("document exceeds the 32 MB limit")
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", fmt.Errorf("document is unavailable: %s", filename)
	}
	return body, filename, nil
}

func writeLogical(root, logical string, data []byte) error {
	full, err := resolveInside(root, logical)
	if err != nil {
		return fmt.Errorf("document path is not accepted")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("document path is not accepted")
	}
	tmp := full + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("document path is not accepted")
	}
	_, copyErr := io.Copy(out, bytes.NewReader(data))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("document path is not accepted")
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("document path is not accepted")
	}
	return nil
}

func resolveInside(root, logical string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" || !safeRelative(logical) {
		return "", fmt.Errorf("invalid document path")
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	slash := path.Clean(strings.ReplaceAll(logical, `\`, "/"))
	full := filepath.Join(base, filepath.FromSlash(slash))
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid document path")
	}
	return full, nil
}

func parseOwned(raw string) (owner, filename string, ok bool) {
	slash := path.Clean(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"))
	parts := strings.Split(slash, "/")
	if len(parts) != 3 || parts[0] != documentDir || parts[1] == documentIncoming {
		return "", "", false
	}
	if !safeSegment(parts[1]) || !safeFilename(parts[2]) {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func newOwner() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("document path is not accepted")
	}
	return hex.EncodeToString(buf), nil
}

func safeRelative(raw string) bool {
	p := strings.TrimSpace(raw)
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "file:") {
		return false
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return false
	}
	if len(p) >= 2 && p[1] == ':' {
		return false
	}
	slash := strings.ReplaceAll(p, `\`, "/")
	cleaned := path.Clean(slash)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return false
	}
	return !strings.Contains(cleaned, "..")
}

func safeSegment(raw string) bool {
	if raw == "" || raw == "." || raw == ".." || raw == documentIncoming || len(raw) > 80 {
		return false
	}
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func safeFilename(raw string) bool {
	if raw == "" || raw == "." || raw == ".." || strings.ContainsAny(raw, `/\`) || len(raw) > 120 {
		return false
	}
	ext := path.Ext(raw)
	if ext == "" || ext == "." || len(ext) > 16 {
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
