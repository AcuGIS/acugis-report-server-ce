package mapreport

import (
	"context"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/reports"
	"github.com/acugis/acugis-report-server-go/internal/spatial"
)

// Compatibility aliases: shared spatial/JRXML helpers live in internal/spatial.
// Callers may keep importing mapreport; OSS embedded maps import spatial directly.

const (
	EnginePostgreSQL              = spatial.EnginePostgreSQL
	EngineMySQL                   = spatial.EngineMySQL
	EngineSQLServer               = spatial.EngineSQLServer
	EngineOracle                  = spatial.EngineOracle
	EngineDb2                     = spatial.EngineDb2
	LookupDatasourceCurrentReport = spatial.LookupDatasourceCurrentReport
)

var (
	ErrNotMapReport    = spatial.ErrNotMapReport
	ErrInvalidReport   = spatial.ErrInvalidReport
	ErrLayerUnresolved = spatial.ErrLayerUnresolved
	ErrQueryFailed     = spatial.ErrQueryFailed

	DialectFor                           = spatial.DialectFor
	DiscoverGeometry                     = spatial.DiscoverGeometry
	ListGeometryColumns                  = spatial.ListGeometryColumns
	ListTableColumns                     = spatial.ListTableColumns
	ReportDatasetFields                  = spatial.ReportDatasetFields
	LayerGeoJSONFromReport               = spatial.LayerGeoJSONFromReport
	LayerGeoJSON                         = spatial.LayerGeoJSON
	TransformExtentToWKT                 = spatial.TransformExtentToWKT
	TransformGeoJSONToWKT                = spatial.TransformGeoJSONToWKT
	GeoJSONToWKT                         = spatial.GeoJSONToWKT
	WKTToGeoJSON                         = spatial.WKTToGeoJSON
	SQLPrimaryTable                      = spatial.SQLPrimaryTable
	SQLTableAliases                      = spatial.SQLTableAliases
	SQLRelationList                      = spatial.SQLRelationList
	SQLGeometryRef                       = spatial.SQLGeometryRef
	SQLGeometryRefDialect                = spatial.SQLGeometryRefDialect
	IsSafeSQLIdent                       = spatial.IsSafeSQLIdent
	IsSafeFieldKey                       = spatial.IsSafeFieldKey
	IsSafeJasperColumnRef                = spatial.IsSafeJasperColumnRef
	QuoteSQLIdent                        = spatial.QuoteSQLIdent
	NeutralizeJasperSQL                  = spatial.NeutralizeJasperSQL
	JRXMLSQLQuery                        = spatial.JRXMLSQLQuery
	JRXMLParameters                      = spatial.JRXMLParameters
	JRXMLParameterLookups                = spatial.JRXMLParameterLookups
	JRXMLHasSpatialParameters            = spatial.JRXMLHasSpatialParameters
	ResolveLookupDatasource              = spatial.ResolveLookupDatasource
	JavaClassToParamKind                 = spatial.JavaClassToParamKind
	ParseQueryMulti                      = spatial.ParseQueryMulti
	CollectFilterContext                 = spatial.CollectFilterContext
	CoerceFilterScalar                   = spatial.CoerceFilterScalar
	ReportParamIsMultiple                = spatial.ReportParamIsMultiple
	BindJasperSQL                        = spatial.BindJasperSQL
	BindJasperSQLDialect                 = spatial.BindJasperSQLDialect
	PromoteCollectionEqualityToXin       = spatial.PromoteCollectionEqualityToXin
	ParseDefaultValueExpression          = spatial.ParseDefaultValueExpression
	PublishParametersFromJRXML           = spatial.PublishParametersFromJRXML
	IsMapDrivenParameter                 = spatial.IsMapDrivenParameter
	IsPromptExcludedParameter            = spatial.IsPromptExcludedParameter
	DefaultMapReportBasemap              = spatial.DefaultMapReportBasemap
	DefaultMapDefinition                 = spatial.DefaultMapDefinition
	NormalizeMapReportBasemap            = spatial.NormalizeMapReportBasemap
	NormalizeMapDefinition               = spatial.NormalizeMapDefinition
	MapReportViewerCSP                   = spatial.MapReportViewerCSP
	ReportMapDefinition                  = spatial.ReportMapDefinition
	IsMapReport                          = spatial.IsMapReport
	StripInactiveSpatialFilterForTest    = spatial.StripInactiveSpatialFilterForTest
	StripGeometryColumnFromSelectForTest = spatial.StripGeometryColumnFromSelectForTest
)

type (
	SpatialDialect      = spatial.SpatialDialect
	LayerMeta           = spatial.LayerMeta
	FeatureCollection   = spatial.FeatureCollection
	TableRef            = spatial.TableRef
	ParamMeta           = spatial.ParamMeta
	ParameterLookupMeta = spatial.ParameterLookupMeta
	BindResult          = spatial.BindResult
	ColumnMeta          = spatial.ColumnMeta
	ColumnsResult       = spatial.ColumnsResult
)

func (s *Service) resolver() *spatial.Resolver {
	if s == nil {
		return &spatial.Resolver{}
	}
	return &spatial.Resolver{
		Datasources: s.Datasources,
		Connector:   s.Connector,
		JRXMLDir:    s.JRXMLDir,
	}
}

// ResolveLayer delegates to the shared spatial resolver.
func (s *Service) ResolveLayer(ctx context.Context, def *reports.Definition) (LayerMeta, error) {
	return s.resolver().ResolveLayer(ctx, def)
}

// Columns delegates to the shared spatial resolver.
func (s *Service) Columns(ctx context.Context, def *reports.Definition) (*ColumnsResult, int, error) {
	return s.resolver().Columns(ctx, def)
}

func (s *Service) openSpatial(ctx context.Context, ds *datasources.Datasource) (datasources.QueryConn, SpatialDialect, error) {
	return s.resolver().OpenSpatial(ctx, ds)
}
