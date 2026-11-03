package cassandra

// SchemaInfo is a Cassandra keyspace (schema) name.
type SchemaInfo struct {
	Name string
}

// TableInfo describes a table or view in a keyspace.
type TableInfo struct {
	Schema string
	Name   string
	Type   string // "table" or "view"
}

// ColumnInfo describes a column for browse / fill metadata.
type ColumnInfo struct {
	Name         string
	DataType     string
	UDTName      string
	IsNullable   bool
	IsPrimaryKey bool
}

// PreviewResult is a tabular preview of query results (string cells).
type PreviewResult struct {
	Columns   []string
	Rows      [][]string
	Truncated bool
}
