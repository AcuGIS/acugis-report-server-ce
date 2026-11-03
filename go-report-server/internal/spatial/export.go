package spatial

// StripInactiveSpatialFilterForTest exposes stripInactiveSpatialFilter for tests.
func StripInactiveSpatialFilterForTest(sql string, filterContext map[string]any) string {
	return stripInactiveSpatialFilter(sql, filterContext)
}

// StripGeometryColumnFromSelectForTest exposes stripGeometryColumnFromSelect for tests.
func StripGeometryColumnFromSelectForTest(afterSelectKeyword, geometryColumn string, d SpatialDialect) string {
	return stripGeometryColumnFromSelect(afterSelectKeyword, geometryColumn, d)
}
