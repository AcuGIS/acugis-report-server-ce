package cassandra

import (
	"fmt"
	"log"
	"sort"
	"strings"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
)

var systemKeyspaces = map[string]bool{
	"system":                true,
	"system_schema":         true,
	"system_auth":           true,
	"system_distributed":    true,
	"system_traces":         true,
	"system_views":          true,
	"system_virtual_schema": true,
	"dse_system":            true,
	"dse_system_local":      true,
	"solr_admin":            true,
}

func isSystemKeyspace(name string) bool {
	return systemKeyspaces[strings.ToLower(strings.TrimSpace(name))]
}

func listKeyspaces(session *gocql.Session, showSystem bool) ([]SchemaInfo, error) {
	iter := session.Query(`SELECT keyspace_name FROM system_schema.keyspaces`).Iter()
	var name string
	var out []SchemaInfo
	for iter.Scan(&name) {
		if !showSystem && isSystemKeyspace(name) {
			continue
		}
		out = append(out, SchemaInfo{Name: name})
	}
	if err := iter.Close(); err != nil {
		return nil, friendlyQueryErr(err)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	log.Printf("[acugis-cassandra] listed %d keyspaces (showSystem=%v)", len(out), showSystem)
	return out, nil
}

func listTables(session *gocql.Session, keyspace string) ([]TableInfo, error) {
	keyspace = strings.TrimSpace(keyspace)
	if keyspace == "" {
		return nil, fmt.Errorf("keyspace is required")
	}
	iter := session.Query(`
		SELECT table_name FROM system_schema.tables
		WHERE keyspace_name = ?`, keyspace).Iter()
	var name string
	var out []TableInfo
	for iter.Scan(&name) {
		out = append(out, TableInfo{
			Schema: keyspace,
			Name:   name,
			Type:   "table",
		})
	}
	if err := iter.Close(); err != nil {
		return nil, friendlyQueryErr(err)
	}
	// Also list views as type "view".
	vIter := session.Query(`
		SELECT view_name FROM system_schema.views
		WHERE keyspace_name = ?`, keyspace).Iter()
	var vname string
	for vIter.Scan(&vname) {
		out = append(out, TableInfo{
			Schema: keyspace,
			Name:   vname,
			Type:   "view",
		})
	}
	_ = vIter.Close() // views metadata may be unavailable on older clusters

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	log.Printf("[acugis-cassandra] keyspace=%s tables=%d", keyspace, len(out))
	return out, nil
}

type columnMeta struct {
	Name         string
	Type         string
	Kind         string // partition_key | clustering | regular | static
	Position     int
	IsPrimaryKey bool
}

func listColumnMeta(session *gocql.Session, keyspace, table string) ([]columnMeta, error) {
	keyspace = strings.TrimSpace(keyspace)
	table = strings.TrimSpace(table)
	if keyspace == "" || table == "" {
		return nil, fmt.Errorf("keyspace and table are required")
	}
	iter := session.Query(`
		SELECT column_name, type, kind, position
		FROM system_schema.columns
		WHERE keyspace_name = ? AND table_name = ?`, keyspace, table).Iter()
	var name, typ, kind string
	var pos int
	var raw []columnMeta
	for iter.Scan(&name, &typ, &kind, &pos) {
		k := strings.ToLower(strings.TrimSpace(kind))
		raw = append(raw, columnMeta{
			Name:         name,
			Type:         typ,
			Kind:         k,
			Position:     pos,
			IsPrimaryKey: k == "partition_key" || k == "clustering",
		})
	}
	if err := iter.Close(); err != nil {
		return nil, friendlyQueryErr(err)
	}
	sort.SliceStable(raw, func(i, j int) bool {
		ki, kj := kindOrder(raw[i].Kind), kindOrder(raw[j].Kind)
		if ki != kj {
			return ki < kj
		}
		if raw[i].Position != raw[j].Position {
			return raw[i].Position < raw[j].Position
		}
		return strings.ToLower(raw[i].Name) < strings.ToLower(raw[j].Name)
	})
	return raw, nil
}

func kindOrder(kind string) int {
	switch kind {
	case "partition_key":
		return 0
	case "clustering":
		return 1
	case "static":
		return 2
	default:
		return 3
	}
}

func kindLabel(kind string) string {
	switch kind {
	case "partition_key":
		return "partition key"
	case "clustering":
		return "clustering"
	case "static":
		return "static"
	default:
		return "regular"
	}
}

func toColumnInfos(meta []columnMeta) []ColumnInfo {
	out := make([]ColumnInfo, 0, len(meta))
	for _, c := range meta {
		label := kindLabel(c.Kind)
		udt := c.Type
		if label != "regular" {
			udt = c.Type + " · " + label
		}
		out = append(out, ColumnInfo{
			Name:         c.Name,
			DataType:     c.Type,
			UDTName:      udt,
			IsNullable:   c.Kind != "partition_key",
			IsPrimaryKey: c.Kind == "partition_key", // PK badge = partition key
		})
	}
	return out
}

func listPrimaryKeys(session *gocql.Session, keyspace, table string) ([]string, error) {
	meta, err := listColumnMeta(session, keyspace, table)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range meta {
		if c.Kind == "partition_key" || c.Kind == "clustering" {
			out = append(out, c.Name)
		}
	}
	return out, nil
}

func countTablesInKeyspaces(session *gocql.Session, keyspaces []SchemaInfo) int {
	total := 0
	for _, ks := range keyspaces {
		tables, err := listTables(session, ks.Name)
		if err != nil {
			continue
		}
		total += len(tables)
	}
	return total
}

func releaseVersion(session *gocql.Session) string {
	var release string
	if err := session.Query(`SELECT release_version FROM system.local`).Scan(&release); err != nil {
		return ""
	}
	return release
}
