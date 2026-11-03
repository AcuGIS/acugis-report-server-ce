package embeddedmap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/edition"
	"github.com/acugis/acugis-report-server-go/internal/geoserver"
	"github.com/acugis/acugis-report-server-go/internal/qgis"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

// Service renders embedded maps (PostGIS/MySQL thematic PNG or GeoServer WMS) for Jasper output.
type Service struct {
	Spatial   *spatial.Resolver
	Paths     Paths
	Edition   *edition.Service // optional; when set, enforces embedded map provider entitlements
	GeoServer    geoserver.Store // optional; required when renderer=geoserver
	QGIS         qgis.Store      // optional; required when renderer=qgis
	QGISMapPrefix string         // JRS_QGIS_MAP_PREFIX; applied to MAP= filenames
}

// PrepareResult is a successful PNG cache write.
type PrepareResult struct {
	Param    string
	Value    string // Tomcat-visible path
	HostPath string
}

// PrepareImage renders the embedded map PNG and returns the Jasper parameter binding.
func (s *Service) PrepareImage(ctx context.Context, def *reports.Definition, queryParams map[string]any) (*PrepareResult, error) {
	emb := ReportEmbeddedMap(def)
	if emb == nil {
		return nil, fmt.Errorf("Embedded map is not enabled for this report")
	}
	if s == nil || s.Spatial == nil {
		return nil, fmt.Errorf("embedded map service is not configured")
	}

	dsName := def.Datasource()
	var ds *datasources.Datasource
	if s.Spatial.Datasources != nil && dsName != "" {
		if found, err := s.Spatial.Datasources.Find(ctx, dsName); err == nil {
			ds = found
		}
	}
	provider := ResolveProvider(emb.Renderer, ds)
	emb.Renderer = NormalizeRenderer(emb.Renderer)
	if s.Edition != nil {
		if err := s.Edition.RequireEmbeddedMapProvider(provider); err != nil {
			return nil, err
		}
	}

	cacheDir, err := s.Paths.CacheDir()
	if err != nil {
		return nil, fmt.Errorf("map cache directory: %w", err)
	}
	fp, _ := json.Marshal(cacheFingerprint(*emb, queryParams))
	sum := sha256.Sum256(append([]byte(def.Key+"|"), fp...))
	hash := hex.EncodeToString(sum[:8])

	if groupedSpatialMap(*emb) && provider != RendererGeoServer && provider != RendererQGIS {
		pngs, err := s.renderSpatialGroupPNGs(ctx, def, emb, ds, queryParams)
		if err != nil {
			return nil, err
		}
		hostDir := filepath.Join(cacheDir, def.Key+"_"+hash)
		if err := os.RemoveAll(hostDir); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to write embedded map PNG cache")
		}
		if err := os.MkdirAll(hostDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to write embedded map PNG cache")
		}
		for key, png := range pngs {
			if err := os.WriteFile(filepath.Join(hostDir, key+".png"), png, 0o644); err != nil {
				return nil, fmt.Errorf("failed to write embedded map PNG cache")
			}
		}
		s.Paths.Own(cacheDir)
		s.Paths.Own(hostDir)
		absHost, err := filepath.Abs(hostDir)
		if err != nil {
			absHost = hostDir
		}
		value := s.Paths.TomcatPath(absHost)
		if value == "" || !isAbsPath(value) {
			value = filepath.ToSlash(absHost)
		}
		return &PrepareResult{
			Param:    emb.ParameterName,
			Value:    value,
			HostPath: absHost,
		}, nil
	}

	var png []byte
	if provider == RendererGeoServer || emb.Renderer == RendererGeoServer {
		reportName := strings.TrimSpace(asString(def.Raw["name"]))
		if reportName == "" {
			reportName = def.Key
		}
		filterParams := map[string]any{}
		for k, v := range queryParams {
			filterParams[k] = v
		}
		delete(filterParams, emb.ParameterName)
		delete(filterParams, ParamName)
		png, err = s.renderGeoServerPNG(ctx, *emb, reportName, filterParams)
	} else if provider == RendererQGIS || emb.Renderer == RendererQGIS {
		reportName := strings.TrimSpace(asString(def.Raw["name"]))
		if reportName == "" {
			reportName = def.Key
		}
		png, err = s.renderQGISPNG(ctx, *emb, reportName)
	} else {
		png, err = s.renderSpatialPNG(ctx, def, emb, ds, queryParams)
	}
	if err != nil {
		return nil, err
	}

	hostPath := filepath.Join(cacheDir, def.Key+"_"+hash+".png")
	if err := os.WriteFile(hostPath, png, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write embedded map PNG cache")
	}
	// Ensure Tomcat (often a different OS user) can read the cache.
	s.Paths.Own(cacheDir)
	s.Paths.Own(hostPath)

	// Jasper resolves String image expressions as java.io.File against the JVM CWD
	// unless the value is absolute. Always pass an absolute path Tomcat can open.
	// Standalone: host path == Tomcat path. Docker: rewrite via JasperReportsContainerDir.
	absHost, err := filepath.Abs(hostPath)
	if err != nil {
		absHost = hostPath
	}
	value := s.Paths.TomcatPath(absHost)
	if value == "" || !isAbsPath(value) {
		value = filepath.ToSlash(absHost)
	}
	return &PrepareResult{
		Param:    emb.ParameterName,
		Value:    value,
		HostPath: absHost,
	}, nil
}

func (s *Service) loadSpatialFeatures(ctx context.Context, def *reports.Definition, emb *Definition, ds *datasources.Datasource, queryParams map[string]any) (features []map[string]any, contextFeats []map[string]any, err error) {
	probe := probeDefinition(def)
	layer, err := s.Spatial.ResolveLayer(ctx, probe)
	if err != nil || layer.Table == "" || layer.GeometryColumn == "" {
		msg := layer.Error
		if msg == "" {
			msg = "Map layer could not be resolved"
		}
		return nil, nil, fmt.Errorf("%s", msg)
	}

	dsName := def.Datasource()
	if ds == nil {
		var findErr error
		ds, findErr = s.Spatial.Datasources.Find(ctx, dsName)
		if findErr != nil || ds == nil {
			return nil, nil, fmt.Errorf("failed to connect to report datasource")
		}
	}
	if !ds.IsSpatialCapable() || (!ds.IsPostgreSQL() && !ds.IsMySQL()) {
		return nil, nil, fmt.Errorf("Embedded Map requires a PostgreSQL/PostGIS or MySQL datasource")
	}
	renderer := ResolveProvider(emb.Renderer, ds)
	if renderer == RendererPostGIS && !ds.IsPostgreSQL() {
		return nil, nil, fmt.Errorf("PostGIS Embedded Map requires a PostgreSQL/PostGIS datasource")
	}
	if renderer == RendererMySQL && !ds.IsMySQL() {
		return nil, nil, fmt.Errorf("MySQL Embedded Map requires a MySQL datasource")
	}

	conn, dialect, err := s.openSpatial(ctx, ds)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to report datasource")
	}
	defer conn.Close(ctx)

	jrxmlPath := filepath.Join(s.Spatial.JRXMLDir, def.Key+".jrxml")
	reportSQL, _ := spatial.JRXMLSQLQuery(jrxmlPath)
	jrxmlParams, _ := spatial.JRXMLParameters(jrxmlPath)

	filterContext := spatial.CollectFilterContext(queryParams, jrxmlParams, def.Parameters())
	delete(filterContext, emb.ParameterName)
	delete(filterContext, ParamName)

	geojson, err := spatial.LayerGeoJSONFromReport(
		ctx, conn, dialect, reportSQL, nullSchema(layer.Schema), layer.Table, layer.GeometryColumn,
		filterContext, jrxmlParams,
	)
	if err != nil || geojson == nil || geojson.Error != "" {
		msg := "failed to load map features"
		if geojson != nil && geojson.Error != "" {
			msg = geojson.Error
		} else if err != nil {
			msg = err.Error()
		}
		return nil, nil, fmt.Errorf("%s", msg)
	}

	wantsContext := (emb.GrayContext || emb.MapType == "overview_highlight") &&
		(emb.MapType == "locator" || emb.MapType == "overview_highlight" || emb.MapType == "selected_features") &&
		filterActive(filterContext)
	if wantsContext {
		ctxJSON, ctxErr := spatial.LayerGeoJSON(ctx, conn, dialect, layer.Schema, layer.Table, layer.GeometryColumn, nil)
		if ctxErr == nil && ctxJSON != nil && ctxJSON.Error == "" {
			contextFeats = ctxJSON.Features
		}
	}
	return geojson.Features, contextFeats, nil
}

func (s *Service) renderSpatialPNG(ctx context.Context, def *reports.Definition, emb *Definition, ds *datasources.Datasource, queryParams map[string]any) ([]byte, error) {
	features, contextFeats, err := s.loadSpatialFeatures(ctx, def, emb, ds, queryParams)
	if err != nil {
		return nil, err
	}
	reportName := strings.TrimSpace(asString(def.Raw["name"]))
	if reportName == "" {
		reportName = def.Key
	}
	thematic := PrepareThematic(features, contextFeats, *emb, reportName)
	return RenderPNGContext(ctx, thematic, *emb)
}

func (s *Service) renderSpatialGroupPNGs(ctx context.Context, def *reports.Definition, emb *Definition, ds *datasources.Datasource, queryParams map[string]any) (map[string][]byte, error) {
	features, _, err := s.loadSpatialFeatures(ctx, def, emb, ds, queryParams)
	if err != nil {
		return nil, err
	}
	jrxmlPath := s.jrxmlPathFor(def)
	field := groupFieldFromJRXML(jrxmlPath, emb.GroupName)
	if field == "" {
		field = strings.TrimSpace(emb.GroupName)
	}
	groups := splitFeaturesByGroup(features, field)
	reportName := strings.TrimSpace(asString(def.Raw["name"]))
	if reportName == "" {
		reportName = def.Key
	}
	gEmb := *emb
	gEmb.ZoomToFiltered = true
	gEmb.GrayContext = false
	out := make(map[string][]byte, len(groups))
	for key, feats := range groups {
		thematic := PrepareThematic(feats, nil, gEmb, reportName)
		png, err := RenderPNGContext(ctx, thematic, gEmb)
		if err != nil {
			return nil, err
		}
		out[key] = png
	}
	return out, nil
}

func (s *Service) openSpatial(ctx context.Context, ds *datasources.Datasource) (datasources.QueryConn, spatial.SpatialDialect, error) {
	d, err := spatial.DialectFor(ds)
	if err != nil {
		return nil, nil, err
	}
	if opener, ok := s.Spatial.Connector.(datasources.SpatialOpener); ok {
		conn, err := opener.ConnectSpatial(ctx, ds)
		if err != nil {
			return nil, nil, err
		}
		return conn, d, nil
	}
	if ds.IsPostgreSQL() {
		conn, err := s.Spatial.Connector.ConnectPG(ctx, ds)
		if err != nil {
			return nil, nil, err
		}
		return datasources.AdaptPG(conn), d, nil
	}
	return nil, nil, datasources.ErrNotSpatial
}

func isAbsPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return true
	}
	// Windows: C:\... or C:/...
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// AppendResult is the soft-fail query rewrite outcome.
type AppendResult struct {
	Query string
	Error string // empty on success or when embed disabled
	Path  string // absolute P_MAP_IMAGE value when render succeeded
	Param string
}

func (s *Service) jrxmlPathFor(def *reports.Definition) string {
	if s == nil || def == nil || strings.TrimSpace(def.Key) == "" {
		return ""
	}
	dir := strings.TrimSpace(s.Paths.JRXMLDir)
	if dir == "" && s.Spatial != nil {
		dir = strings.TrimSpace(s.Spatial.JRXMLDir)
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, def.Key+".jrxml")
}

// AppendToQuery renders the map (when enabled) and appends P_MAP_IMAGE to the query string.
func (s *Service) AppendToQuery(ctx context.Context, def *reports.Definition, queryString string, queryParams map[string]any) AppendResult {
	emb := ReportEmbeddedMap(def)
	if emb == nil {
		return AppendResult{Query: queryString}
	}
	jrxmlPath := s.jrxmlPathFor(def)
	// Designer-published MapRenderer images render at fill time; do not overlay P_MAP_IMAGE.
	// Heal MapSql.eq + evaluationTime=Group on execute so a .deb import works
	// without a second Save (direct Go deploys usually saved after the fix).
	if jrxmlPath != "" && jrxmlUsesPostGISMapRenderer(jrxmlPath) {
		if sync := SyncJRXML(jrxmlPath, *emb); sync.JRXMLChanged {
			_ = os.Remove(strings.TrimSuffix(jrxmlPath, filepath.Ext(jrxmlPath)) + ".jasper")
		}
		return AppendResult{Query: queryString}
	}
	// Existing grouped reports still have File($P{P_MAP_IMAGE}) with no $F{group}.
	// Upgrade the image expression once so each group header loads its PNG.
	if groupedSpatialMap(*emb) && jrxmlPath != "" && !jrxmlUsesGroupedFileMap(jrxmlPath) {
		if sync := SyncJRXML(jrxmlPath, *emb); sync.JRXMLChanged {
			_ = os.Remove(strings.TrimSuffix(jrxmlPath, filepath.Ext(jrxmlPath)) + ".jasper")
		}
	}
	prepared, err := s.PrepareImage(ctx, def, queryParams)
	if err != nil {
		return AppendResult{Query: queryString, Error: err.Error()}
	}
	pair := url.QueryEscape(prepared.Param) + "=" + url.QueryEscape(prepared.Value)
	if queryString == "" {
		return AppendResult{Query: pair, Path: prepared.Value, Param: prepared.Param}
	}
	var filtered []string
	for _, part := range strings.Split(queryString, "&") {
		if part == "" {
			continue
		}
		key, _, _ := strings.Cut(part, "=")
		if keyDecoded, err := url.QueryUnescape(key); err == nil {
			key = keyDecoded
		}
		if key == prepared.Param {
			continue
		}
		filtered = append(filtered, part)
	}
	filtered = append(filtered, pair)
	return AppendResult{Query: strings.Join(filtered, "&"), Path: prepared.Value, Param: prepared.Param}
}

func probeDefinition(def *reports.Definition) *reports.Definition {
	if def == nil {
		return nil
	}
	raw := map[string]any{}
	for k, v := range def.Raw {
		raw[k] = v
	}
	raw["type"] = "map"
	mapDef := spatial.NormalizeMapDefinition(asMap(raw["mapDefinition"]), "")
	if asMap(raw["map_definition"]) != nil && asMap(raw["mapDefinition"]) == nil {
		mapDef = spatial.NormalizeMapDefinition(asMap(raw["map_definition"]), "")
	}
	mapDef["enabled"] = true
	if strings.TrimSpace(asString(mapDef["geometryColumn"])) == "" {
		mapDef["geometryColumn"] = "geom"
	}
	raw["mapDefinition"] = mapDef
	return &reports.Definition{Key: def.Key, Raw: raw, Path: def.Path}
}

func nullSchema(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

func filterActive(fc map[string]any) bool {
	for _, v := range fc {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return true
			}
		case []string:
			if len(t) > 0 {
				return true
			}
		case []any:
			if len(t) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func cacheFingerprint(emb Definition, params map[string]any) map[string]any {
	return map[string]any{
		"source":                   emb.Renderer,
		"size":                     []int{emb.Width, emb.Height},
		"padding":                  emb.Padding,
		"background":               emb.Background,
		"dpi":                      emb.DPI,
		"mapType":                  emb.MapType,
		"field":                    emb.Field,
		"classification":           emb.Classification,
		"classes":                  emb.Classes,
		"colorRamp":                emb.ColorRamp,
		"legend":                   emb.IncludeLegend,
		"title":                    emb.ShowTitle,
		"highlight":                emb.HighlightSelected,
		"grayContext":              emb.GrayContext,
		"zoom":                     emb.ZoomToFiltered,
		"basemap":                  emb.Basemap,
		"pointSymbol":              emb.PointSymbol,
		"geoserverConnection":      emb.GeoServerConnection,
		"geoserverBaseUrl":         emb.GeoServerBaseURL,
		"geoserverWorkspace":       emb.GeoServerWorkspace,
		"geoserverLayer":           emb.GeoServerLayer,
		"geoserverStyle":           emb.GeoServerStyle,
		"geoserverFilterMappings":  emb.GeoServerFilterMappings,
		"qgisConnection":           emb.QGISConnection,
		"qgisBaseUrl":              emb.QGISBaseURL,
		"qgisProject":              emb.QGISProject,
		"qgisLayer":                emb.QGISLayer,
		"qgisStyle":                emb.QGISStyle,
		"groupName":                emb.GroupName,
		"band":                     emb.Band,
		"filters":                  params,
	}
}
