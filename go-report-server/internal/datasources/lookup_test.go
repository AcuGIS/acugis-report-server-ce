package datasources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/acugis/acugis-report-server-go/internal/mapreport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type lookupStubRows struct {
	cols []string
	data [][]any
	i    int
}

func (r *lookupStubRows) Close()                        {}
func (r *lookupStubRows) Err() error                    { return nil }
func (r *lookupStubRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *lookupStubRows) FieldDescriptions() []pgconn.FieldDescription {
	out := make([]pgconn.FieldDescription, len(r.cols))
	for i, c := range r.cols {
		out[i] = pgconn.FieldDescription{Name: c}
	}
	return out
}
func (r *lookupStubRows) Next() bool {
	if r.i >= len(r.data) {
		return false
	}
	r.i++
	return true
}
func (r *lookupStubRows) Scan(dest ...any) error {
	row := r.data[r.i-1]
	for i := range dest {
		if i >= len(row) {
			continue
		}
		switch p := dest[i].(type) {
		case *any:
			*p = row[i]
		}
	}
	return nil
}
func (r *lookupStubRows) Values() ([]any, error) {
	return r.data[r.i-1], nil
}
func (r *lookupStubRows) RawValues() [][]byte { return nil }
func (r *lookupStubRows) Conn() *pgx.Conn     { return nil }

type lookupStubConn struct {
	lastSQL  string
	lastArgs []any
	rows     *lookupStubRows
	err      error
}

func (c *lookupStubConn) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.lastSQL = sql
	c.lastArgs = append([]any{}, args...)
	if c.err != nil {
		return nil, c.err
	}
	c.rows.i = 0
	return c.rows, nil
}
func (c *lookupStubConn) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (c *lookupStubConn) Close(ctx context.Context) error                               { return nil }

func TestRunBoundParameterLookup_MapsFields(t *testing.T) {
	conn := &lookupStubConn{
		rows: &lookupStubRows{
			cols: []string{"city_name", "city_code"},
			data: [][]any{
				{"Austin", "AUS"},
				{"Dallas", "DAL"},
				{"Austin", "AUS"},
			},
		},
	}
	ds := &datasources.Datasource{Name: "sales", DBType: "postgresql"}

	bound, err := mapreport.BindJasperSQL(
		"SELECT city_name, city_code FROM customers WHERE country = $P{p_country}",
		map[string]any{"p_country": "USA"},
		map[string]mapreport.ParamMeta{
			"p_country": {Name: "p_country", Class: "java.lang.String", Kind: "string"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	opts, err := datasources.RunBoundParameterLookup(
		context.Background(), ds, stubConnector{conn: conn},
		bound.SQL, bound.Params, "city_name", "city_code",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 {
		t.Fatalf("opts=%v", opts)
	}
	if opts[0]["label"] != "Austin" || opts[0]["value"] != "AUS" {
		t.Fatalf("opt0=%v", opts[0])
	}
	if opts[1]["label"] != "Dallas" || opts[1]["value"] != "DAL" {
		t.Fatalf("opt1=%v", opts[1])
	}
	if !strings.Contains(conn.lastSQL, "$1") {
		t.Fatalf("expected bound placeholder in sql=%q", conn.lastSQL)
	}
	if len(conn.lastArgs) != 1 || conn.lastArgs[0] != "USA" {
		t.Fatalf("args=%v", conn.lastArgs)
	}
}

func TestRunBoundParameterLookup_CollectionIN(t *testing.T) {
	conn := &lookupStubConn{
		rows: &lookupStubRows{
			cols: []string{"state"},
			data: [][]any{{"TX"}, {"CA"}},
		},
	}
	ds := &datasources.Datasource{Name: "sales", DBType: "postgresql"}
	bound, err := mapreport.BindJasperSQL(
		"SELECT DISTINCT state FROM t WHERE $X{IN,country,p_countries}",
		map[string]any{"p_countries": []any{"USA", "CAN"}},
		map[string]mapreport.ParamMeta{
			"p_countries": {Name: "p_countries", Class: "java.util.Collection", Kind: "collection"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	opts, err := datasources.RunBoundParameterLookup(
		context.Background(), ds, stubConnector{conn: conn},
		bound.SQL, bound.Params, "state", "state",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 2 {
		t.Fatalf("opts=%v", opts)
	}
	if len(conn.lastArgs) != 2 {
		t.Fatalf("args=%v sql=%q", conn.lastArgs, conn.lastSQL)
	}
	if !strings.Contains(conn.lastSQL, "IN (") {
		t.Fatalf("sql=%q", conn.lastSQL)
	}
}

func TestRunBoundParameterLookup_PromotesEqualityForCollection(t *testing.T) {
	conn := &lookupStubConn{
		rows: &lookupStubRows{
			cols: []string{"state"},
			data: [][]any{{"NJ"}, {"NY"}, {"AL"}},
		},
	}
	ds := &datasources.Datasource{Name: "sales", DBType: "postgresql"}
	// Legacy cascade lookup form taught by Designer: col = $P{multiParent}
	bound, err := mapreport.BindJasperSQL(
		"SELECT DISTINCT state FROM t WHERE region = $P{p_region}",
		map[string]any{"p_region": []any{"Mid Atl", "E S Cen"}},
		map[string]mapreport.ParamMeta{
			"p_region": {Name: "p_region", Class: "java.util.Collection", Kind: "collection"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bound.SQL, "IN (") {
		t.Fatalf("expected IN promotion, sql=%q", bound.SQL)
	}
	if len(bound.Params) != 2 {
		t.Fatalf("params=%v", bound.Params)
	}
	opts, err := datasources.RunBoundParameterLookup(
		context.Background(), ds, stubConnector{conn: conn},
		bound.SQL, bound.Params, "state", "state",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 3 {
		t.Fatalf("opts=%v", opts)
	}
	if len(conn.lastArgs) != 2 || conn.lastArgs[0] != "Mid Atl" || conn.lastArgs[1] != "E S Cen" {
		t.Fatalf("args=%v sql=%q", conn.lastArgs, conn.lastSQL)
	}
}
