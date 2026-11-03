package server

import (
	_ "embed"
	"html/template"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/embeddedmap"
)

//go:embed publish.html
var publishHTML string

//go:embed edit.html
var editHTML string

const materialPageCSS = `
:root {
    --content-bg: #f8f9fa;
    --header-color: #202124;
    --subtle-text: #5f6368;
}
html, body { background: var(--content-bg); color: #263238; min-height: 100vh; }
body { padding-bottom: 0; }
.main-content { padding: 32px 40px; min-height: 100vh; box-sizing: border-box; }
.page-header { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 28px; }
.page-header .page-title { font-size: 2.2rem; font-weight: 500; color: var(--header-color); margin: 0 0 6px; line-height: 1.2; }
.page-header .page-subtitle { margin: 0; color: var(--subtle-text); font-size: 0.95rem; }
.main-content .card { border-radius: 6px; box-shadow: 0 1px 3px rgba(0,0,0,0.06), 0 1px 2px rgba(0,0,0,0.04); margin: 0 0 24px; }
.main-content .card .card-content { padding: 28px; }
.main-content .card .card-title { font-size: 1.15rem; font-weight: 500; color: var(--header-color); margin-bottom: 18px; display: block; }
.main-content .card .input-field { margin-top: 1rem; }
.btn, .btn-small, .btn-flat { text-transform: none; font-weight: 500; letter-spacing: 0.2px; }
.header-actions, .card-actions-row, .form-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.form-actions { justify-content: flex-end; }
.parameter-row { border: 1px solid #cfd8dc; border-radius: 6px; padding: 18px; margin-bottom: 16px; background: #ffffff; }
.parameter-row .remove-parameter-btn { margin-top: 22px; }
.section-divider { border: 0; border-top: 1px solid #e0e0e0; margin: 28px 0; }
.is-hidden, .d-none { display: none !important; }
.muted-small { color: var(--subtle-text); font-size: 0.9rem; }
.cassandra-query-readonly {
    margin: 0 0 6px;
    padding: 12px 14px;
    background: #fafafa;
    border: 1px solid #cfd8dc;
    border-radius: 4px;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.85rem;
    line-height: 1.45;
    max-height: 220px;
    overflow: auto;
    color: #263238;
}
.card-panel ul { margin-bottom: 0; }
#reportTabs { margin-bottom: 20px; background: transparent; }
.report-tab-pane { padding-top: 12px; }
.jrs-field-label { display: block; font-size: 0.8rem; color: #9e9e9e; margin-bottom: 6px; }
.jrs-panel { border: 1px solid #cfd8dc; border-radius: 6px; padding: 16px; margin-bottom: 16px; background: #fafafa; }
.jrs-check-row { margin: 8px 0; }
.jrs-inline-checks { display: flex; flex-wrap: wrap; gap: 8px 20px; margin: 8px 0 16px; }
input[type="color"] { height: 2.5rem; padding: 0; border: none; background: transparent; width: 100%; }
.mb-4 { margin-bottom: 24px; }
.mb-2 { margin-bottom: 12px; }
.d-flex { display: flex; }
.align-items-end { align-items: flex-end; }
@media only screen and (max-width: 992px) {
    .main-content { padding: 72px 20px 32px; }
    .page-header .page-title { font-size: 1.8rem; }
}
`

type materialChromeData struct {
	Title            string
	ProductName      string
	BrandHome        string
	ShowSidebarBrand bool
	FederationApp    string
	ShellMount       template.JS
	PHPMount         template.JS
	AssetsBase       template.JS
	AssetsURL        string
	CSS              template.CSS
}

type publishPageData struct {
	materialChromeData
	DataSources          []string
	CassandraDatasources map[string]bool
	BigQueryDatasources  map[string]bool
	DuckDBDatasources    map[string]bool
	ArcGISDatasources    map[string]bool
	AccessGroups         []string
	Message              string
	MessageType          string
	PublishNotes         []string
	RunReportURL         string
	ViewJSONURL          string
	HasDatasources       bool
	AllowMapReports      bool
}

type paramRowData struct {
	Index      int
	Name       string
	Type       string
	Label      string
	Required   bool
	Multiple   bool
	SourceType string
	SQL        string
	Options    string
}

type mapCategoryRow struct {
	Index        int
	Value        string
	Label        string
	FillColor    string
	OutlineColor string
}

type reportFieldOpt struct {
	Name  string
	Label string
}

type editPageData struct {
	materialChromeData
	ReportKey            string
	CatalogURL           string
	DataSources          []string
	CassandraDatasources map[string]bool
	BigQueryDatasources  map[string]bool
	DuckDBDatasources    map[string]bool
	ArcGISDatasources    map[string]bool
	QuerySQL             string
	QuerySource          string // e.g. "Report definition" when taken from JRXML
	HasReportQuery       bool   // true when query is already known from the report
	AccessGroups         []string
	SelectedGroups       map[string]bool
	FormErrors           []string
	Name                 string
	Description          string
	Datasource           string
	ReportType           string
	AllowMapReports      bool
	ShowMapTab           bool
	Params               []paramRowData
	ParamCount           int
	Map                  map[string]any
	MapEnabled           bool
	Basemaps             []struct {
		Key   string
		Label string
	}
	InitialZooms []struct {
		Key   string
		Label string
	}
	ColorRamps      []string
	GeomColumns     []string
	ReportFields    []reportFieldOpt
	LayerError      string
	LayerTable      string
	Categories      []mapCategoryRow
	CategoryCount   int
	PopupSelected   map[string]bool
	TooltipSelected map[string]bool
	Renderer        string
	Classification  string
	SingleSymbol    map[string]any
	Labels          map[string]any
	PopupsEnabled   bool
	TooltipsEnabled bool
	Sync            map[string]any
	// Embedded Map (PostGIS / MySQL / GeoServer PNG → P_MAP_IMAGE)
	Embedded                   map[string]any
	EmbeddedEnabled            bool
	EmbeddedMapTypes           []struct{ Key, Label string }
	EmbeddedMapReady           map[string]bool
	EmbeddedPointShapes        []struct{ Key, Label string }
	EmbeddedBands              []struct{ Key, Label string }
	EmbeddedClassifs           []struct{ Key, Label string }
	EmbeddedRampIDs            []string
	JRXMLGroups                []string
	LayerGeometryKind          string
	NumericFields              []reportFieldOpt
	GeoServerConnections       []string
	QGISConnections            []string
	EmbeddedCaps               edition.MapProviderFlags
	EmbeddedFilterMappingsJSON template.JS
	EmbeddedBasemaps           []struct{ Key, Label string }
}

var publishTemplate = template.Must(template.New("publish").Parse(publishHTML))
var editTemplate = template.Must(template.New("edit").Funcs(template.FuncMap{
	"eq": func(a, b any) bool {
		return stringify(a) == stringify(b)
	},
	"asMap": func(v any) map[string]any {
		if m, ok := v.(map[string]any); ok {
			return m
		}
		return nil
	},
	"embBasemapOn": func(optionKey, stored string) bool {
		return embeddedmap.SameBasemap(optionKey, stored)
	},
	"mapStr": func(m any, key string) string {
		mm, _ := m.(map[string]any)
		if mm == nil {
			return ""
		}
		return stringify(mm[key])
	},
	"mapBool": func(m any, key string) bool {
		mm, _ := m.(map[string]any)
		if mm == nil {
			return false
		}
		return truthyAny(mm[key])
	},
	"mapFloat": func(m any, key string) string {
		mm, _ := m.(map[string]any)
		if mm == nil {
			return ""
		}
		return stringify(mm[key])
	},
	"mapInt": func(m any, key string, fallback int) int {
		mm, _ := m.(map[string]any)
		if mm == nil {
			return fallback
		}
		switch t := mm[key].(type) {
		case int:
			return t
		case float64:
			return int(t)
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(t))
			if err == nil {
				return n
			}
		}
		return fallback
	},
}).Parse(editHTML))
