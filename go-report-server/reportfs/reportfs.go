// Package reportfs is the public Report Server filesystem API used by the
// Designer storage adapter. It delegates to the existing report store, report
// key rules, catalog metadata extractors, and file ownership helper.
//
// The Designer module cannot import Report Server internal packages. This
// package is the boundary that keeps catalog parsing and writing in one place.
package reportfs

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/acugis/acugis-report-server-go/internal/config"
	"github.com/acugis/acugis-report-server-go/internal/fileown"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

// Sentinel errors from the report store. They do not include filesystem paths.
var (
	ErrNotFound   = reports.ErrNotFound
	ErrInvalidKey = reports.ErrInvalidKey
)

// Store is the Report Server report catalog and JRXML directory.
type Store struct {
	inner *reports.Store
}

// Definition is one {key}.report.json document.
type Definition struct {
	Key   string
	Raw   map[string]any
	inner *reports.Definition
}

// NewStore constructs a filesystem-backed report store.
func NewStore(jsonDir, thumbnailDir, jrxmlDir string) *Store {
	return &Store{inner: reports.NewStore(jsonDir, thumbnailDir, jrxmlDir)}
}

// SetFileOwner configures optional chown for files the store writes.
func (s *Store) SetFileOwner(owner, group string) {
	if s == nil || s.inner == nil {
		return
	}
	s.inner.SetFileOwner(owner, group)
}

// EnsureDirs creates the JSON, JRXML, and thumbnail directories when missing.
func (s *Store) EnsureDirs() error {
	if s == nil || s.inner == nil {
		return errors.New("report store not configured")
	}
	return s.inner.EnsureDirs()
}

// SaveResourceBase writes {key}.resource-base. slashFolderPath is a catalog
// path such as "/finance/sales-report" or "/sales-report". An empty path is
// rejected by the caller when the report must stay isolated from Home.
func (s *Store) SaveResourceBase(key, slashFolderPath string) error {
	if s == nil || s.inner == nil {
		return errors.New("report store not configured")
	}
	return s.inner.SaveResourceBase(key, slashFolderPath)
}

// ReadResourceBase returns the catalog folder stored in {key}.resource-base.
func (s *Store) ReadResourceBase(key string) (string, error) {
	if s == nil || s.inner == nil {
		return "", errors.New("report store not configured")
	}
	return s.inner.ReadResourceBase(key)
}

// JRXMLDir returns the configured JRXML directory.
func (s *Store) JRXMLDir() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.JRXMLDir()
}

// Exists reports whether {key}.report.json is present.
func (s *Store) Exists(key string) bool {
	if s == nil || s.inner == nil {
		return false
	}
	return s.inner.Exists(key)
}

// Get loads one report definition.
func (s *Store) Get(key string) (*Definition, error) {
	if s == nil || s.inner == nil {
		return nil, errors.New("report store not configured")
	}
	def, err := s.inner.Get(key)
	if err != nil {
		return nil, err
	}
	return &Definition{Key: def.Key, Raw: def.Raw, inner: def}, nil
}

// SaveJSON writes the report definition with the store's existing JSON format.
func (s *Store) SaveJSON(key string, raw map[string]any) error {
	if s == nil || s.inner == nil {
		return errors.New("report store not configured")
	}
	return s.inner.SaveJSON(key, raw)
}

// DisplayName returns the catalog title.
func (d *Definition) DisplayName() string {
	if d == nil || d.inner == nil {
		return ""
	}
	return d.inner.DisplayName()
}

// Datasource returns the catalog datasource name.
func (d *Definition) Datasource() string {
	if d == nil || d.inner == nil {
		return ""
	}
	return d.inner.Datasource()
}

// Type returns jasper or map.
func (d *Definition) Type() string {
	if d == nil || d.inner == nil {
		return ""
	}
	return d.inner.Type()
}

// QuerySQL returns the catalog query string.
func (d *Definition) QuerySQL() string {
	if d == nil || d.inner == nil {
		return ""
	}
	return d.inner.QuerySQL()
}

// Parameters returns catalog parameter maps.
func (d *Definition) Parameters() []map[string]any {
	if d == nil || d.inner == nil {
		return nil
	}
	return d.inner.Parameters()
}

// NormalizeReportKey canonicalizes a report key the same way the server does.
func NormalizeReportKey(value string) string {
	return jasper.NormalizeReportKey(value)
}

// IsValidReportKey reports whether value matches the server key pattern.
func IsValidReportKey(value string) bool {
	return jasper.IsValidReportKey(value)
}

// NormalizeReportType maps a report type to jasper or map.
func NormalizeReportType(value string) string {
	return reports.NormalizeReportType(value)
}

// CatalogParametersFromJRXML extracts publish-style parameter metadata.
func CatalogParametersFromJRXML(data []byte) []map[string]any {
	return reports.CatalogParametersFromJRXML(data)
}

// ExtractCollectionQueryFromJRXML reads the report query embedded in JRXML.
func ExtractCollectionQueryFromJRXML(data []byte) string {
	return reports.ExtractCollectionQueryFromJRXML(data)
}

// ApplyFileOwner sets ownership using the Report Server file helper.
// On Windows this is a no-op. On Unix it matches JRXML and Jasper uploads.
func ApplyFileOwner(path, owner, group string) error {
	return fileown.Apply(path, owner, group)
}

// ExecResult is the outcome of the existing Jasper gateway execution path.
type ExecResult struct {
	StatusCode int
	Body       []byte
	Code       string
	Message    string
}

// ExecuteReport addresses a report through the existing Jasper gateway.
// reportExists is the gateway's catalog check ({jsonDir}/{key}.report.json).
// The HTTP call uses reportURL. This does not add a Designer execution endpoint.
func ExecuteReport(ctx context.Context, jsonDir, reportURL, imageURL, reportName, format, rawQuery string) (ExecResult, error) {
	gw := jasper.NewGateway(config.JasperConfig{
		ReportURL: reportURL,
		ImageURL:  imageURL,
		Timeout:   5 * time.Second,
	}, jasper.Options{ReportJSONDir: jsonDir})
	resp, err := gw.ExecuteReport(ctx, jasper.ExecuteRequest{
		ReportName: reportName,
		Format:     format,
		RawQuery:   rawQuery,
	})
	if err != nil {
		out := ExecResult{Message: err.Error()}
		var ge *jasper.Error
		if errors.As(err, &ge) && ge != nil {
			out.Code = string(ge.Code)
			if ge.Message != "" {
				out.Message = ge.Message
			}
		}
		return out, err
	}
	defer resp.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return ExecResult{StatusCode: resp.StatusCode}, readErr
	}
	return ExecResult{StatusCode: resp.StatusCode, Body: body}, nil
}
