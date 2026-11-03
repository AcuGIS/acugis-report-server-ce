package cassandra

import (
	"fmt"
	"log"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
)

// executePreview runs CQL and returns a tabular preview (string cells).
func executePreview(session *gocql.Session, cql string, params map[string]any, limit int, applyPreviewLimit bool) (*PreviewResult, error) {
	if err := ensureReadOnly(cql); err != nil {
		return nil, err
	}
	execCQL := cql
	if applyPreviewLimit {
		execCQL = wrapLimitForPreview(cql, limit)
	}
	bound, err := bindParameters(execCQL, params)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	log.Printf("[acugis-cassandra] query start limit=%d", limit)

	q := session.Query(bound.CQL, bound.Values...)
	if limit > 0 {
		q = q.PageSize(limit)
	}
	iter := q.Iter()
	cols := iter.Columns()
	if len(cols) == 0 {
		if err := iter.Close(); err != nil {
			return nil, friendlyQueryErr(err)
		}
		log.Printf("[acugis-cassandra] query done duration=%s rows=0", time.Since(start))
		return &PreviewResult{Columns: nil, Rows: nil}, nil
	}

	result := &PreviewResult{
		Columns: make([]string, len(cols)),
	}
	targets := make([]any, len(cols))
	for i, c := range cols {
		result.Columns[i] = c.Name
		targets[i] = scanTarget(c.TypeInfo)
	}

	for {
		if limit > 0 && len(result.Rows) >= limit {
			result.Truncated = true
			break
		}
		if !iter.Scan(targets...) {
			break
		}
		row := make([]string, len(cols))
		for i := range cols {
			row[i] = formatCell(derefScan(targets[i]))
			// Reset pointer targets for next scan where needed.
			targets[i] = scanTarget(cols[i].TypeInfo)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := iter.Close(); err != nil {
		return nil, friendlyQueryErr(err)
	}
	log.Printf("[acugis-cassandra] query done duration=%s rows=%d truncated=%v",
		time.Since(start), len(result.Rows), result.Truncated)
	return result, nil
}

// executeTypedRows runs CQL and returns row maps with JSON-friendly typed values for Jasper fill.
func executeTypedRows(session *gocql.Session, cql string, params map[string]any, limit int) ([]map[string]any, error) {
	if err := ensureReadOnly(cql); err != nil {
		return nil, err
	}
	bound, err := bindParameters(cql, params)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	log.Printf("[acugis-cassandra] fill query start limit=%d", limit)

	q := session.Query(bound.CQL, bound.Values...)
	if limit > 0 {
		q = q.PageSize(limit)
	}
	iter := q.Iter()
	cols := iter.Columns()
	if len(cols) == 0 {
		if err := iter.Close(); err != nil {
			return nil, friendlyQueryErr(err)
		}
		return nil, nil
	}

	targets := make([]any, len(cols))
	for i, c := range cols {
		targets[i] = scanTarget(c.TypeInfo)
	}

	var out []map[string]any
	for {
		if limit > 0 && len(out) >= limit {
			break
		}
		if !iter.Scan(targets...) {
			break
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c.Name] = jasperValue(derefScan(targets[i]))
			targets[i] = scanTarget(c.TypeInfo)
		}
		out = append(out, m)
	}
	if err := iter.Close(); err != nil {
		return nil, friendlyQueryErr(err)
	}
	log.Printf("[acugis-cassandra] fill query done duration=%s rows=%d", time.Since(start), len(out))
	return out, nil
}

// discoverColumnsFromCQL runs a limited SELECT and returns column name/type metadata.
func discoverColumnsFromCQL(session *gocql.Session, cql string, params map[string]any) ([]ColumnInfo, error) {
	if err := ensureReadOnly(cql); err != nil {
		return nil, err
	}
	execCQL := wrapLimitForPreview(cql, 1)
	bound, err := bindParameters(execCQL, params)
	if err != nil {
		return nil, err
	}
	iter := session.Query(bound.CQL, bound.Values...).PageSize(1).Iter()
	cols := iter.Columns()
	// Drain at most one row so metadata is populated.
	if len(cols) > 0 {
		targets := make([]any, len(cols))
		for i, c := range cols {
			targets[i] = scanTarget(c.TypeInfo)
		}
		_ = iter.Scan(targets...)
	}
	if err := iter.Close(); err != nil {
		// Some drivers need a successful page for Columns(); surface the error.
		if len(cols) == 0 {
			return nil, friendlyQueryErr(err)
		}
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("CQL returned no columns")
	}
	out := make([]ColumnInfo, 0, len(cols))
	for _, c := range cols {
		typ := typeNameFromInfo(c.TypeInfo)
		out = append(out, ColumnInfo{
			Name:       c.Name,
			DataType:   typ,
			UDTName:    typ,
			IsNullable: true,
		})
	}
	return out, nil
}

// BuildSelectCQL builds a simple read-only SELECT for a table (manual CQL mode).
func BuildSelectCQL(keyspace, table string, columns []string) string {
	ks := strings.TrimSpace(keyspace)
	tbl := strings.TrimSpace(table)
	var colList string
	if len(columns) == 0 {
		colList = "*"
	} else {
		parts := make([]string, 0, len(columns))
		for _, c := range columns {
			c = strings.TrimSpace(c)
			if c != "" {
				parts = append(parts, quoteIdent(c))
			}
		}
		if len(parts) == 0 {
			colList = "*"
		} else {
			colList = strings.Join(parts, ", ")
		}
	}
	if ks != "" {
		return fmt.Sprintf("SELECT %s\nFROM %s.%s", colList, quoteIdent(ks), quoteIdent(tbl))
	}
	return fmt.Sprintf("SELECT %s\nFROM %s", colList, quoteIdent(tbl))
}

func quoteIdent(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	// Quote when not a simple unquoted identifier.
	if isSimpleIdent(name) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func isSimpleIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
