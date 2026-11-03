package datasources_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type stubFinder struct {
	ds  *datasources.Datasource
	err error
}

func (f stubFinder) Find(ctx context.Context, name string) (*datasources.Datasource, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ds, nil
}

type stubConnector struct {
	conn datasources.Conn
	err  error
}

func (c stubConnector) ConnectPG(ctx context.Context, ds *datasources.Datasource) (datasources.Conn, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.conn, nil
}

type stubRows struct {
	data [][]any
	i    int
	err  error
}

func (r *stubRows) Close()                                       {}
func (r *stubRows) Err() error                                   { return r.err }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Next() bool {
	if r.i >= len(r.data) {
		return false
	}
	r.i++
	return true
}
func (r *stubRows) Scan(dest ...any) error { return nil }
func (r *stubRows) Values() ([]any, error) {
	if r.i == 0 || r.i > len(r.data) {
		return nil, pgx.ErrNoRows
	}
	return r.data[r.i-1], nil
}
func (r *stubRows) RawValues() [][]byte { return nil }
func (r *stubRows) Conn() *pgx.Conn     { return nil }

type stubConn struct {
	rows *stubRows
	err  error
}

func (c stubConn) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if c.err != nil {
		return nil, c.err
	}
	c.rows.i = 0
	return c.rows, nil
}
func (c stubConn) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (c stubConn) Close(ctx context.Context) error                               { return nil }

func TestExpandSQLParameterOptions(t *testing.T) {
	def := map[string]any{
		"datasource": "population_db",
		"parameters": []any{
			map[string]any{
				"name":   "p_industry",
				"type":   "select",
				"label":  "Industry",
				"source": "sql:select DISTINCT major_industry from state_economics",
			},
			map[string]any{
				"name": "p_geometry",
				"type": "string",
			},
		},
	}
	finder := stubFinder{ds: &datasources.Datasource{Name: "population_db", DBType: "postgresql"}}
	conn := stubConn{rows: &stubRows{data: [][]any{{"Energy"}, {"Tech"}}}}
	out := datasources.ExpandSQLParameterOptions(context.Background(), def, finder, stubConnector{conn: conn})
	params := out["parameters"].([]any)
	industry := params[0].(map[string]any)
	opts, ok := industry["options"].([]map[string]any)
	if !ok || len(opts) != 2 {
		t.Fatalf("options=%v (%T)", industry["options"], industry["options"])
	}
	if opts[0]["value"] != "Energy" || opts[1]["value"] != "Tech" {
		t.Fatalf("opts=%v", opts)
	}
	if params[1].(map[string]any)["options"] != nil {
		t.Fatal("unexpected options on non-sql param")
	}
}

func TestExpandSQLParameterOptions_MySQLSkippedWithoutOpener(t *testing.T) {
	def := map[string]any{
		"datasource": "customers_mysql",
		"parameters": []any{
			map[string]any{
				"name":   "p_country",
				"type":   "select",
				"source": "sql:select DISTINCT country from customers;",
			},
		},
	}
	finder := stubFinder{ds: &datasources.Datasource{Name: "customers_mysql", DBType: "mysql"}}
	out := datasources.ExpandSQLParameterOptions(context.Background(), def, finder, stubConnector{})
	params := out["parameters"].([]any)
	if params[0].(map[string]any)["options"] != nil {
		t.Fatal("expected options left empty when connector has no MySQL opener")
	}
}

type stubMySQLConnector struct {
	stubConnector
	db  *sql.DB
	err error
}

func (c stubMySQLConnector) OpenMySQL(ctx context.Context, ds *datasources.Datasource) (*sql.DB, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.db, nil
}

func TestExpandSQLParameterOptions_MySQL(t *testing.T) {
	sql.Register("mysql-options-fake", &fakeMySQLDriver{rows: [][]driver.Value{
		{[]byte("USA")},
		{[]byte("Canada")},
	}})
	db, err := sql.Open("mysql-options-fake", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	def := map[string]any{
		"datasource": "customers_mysql",
		"parameters": []any{
			map[string]any{
				"name":   "p_country",
				"type":   "select",
				"source": "sql:select DISTINCT country from customers;",
			},
		},
	}
	finder := stubFinder{ds: &datasources.Datasource{Name: "customers_mysql", DBType: "mariadb"}}
	out := datasources.ExpandSQLParameterOptions(context.Background(), def, finder, stubMySQLConnector{db: db})
	params := out["parameters"].([]any)
	opts, ok := params[0].(map[string]any)["options"].([]map[string]any)
	if !ok || len(opts) != 2 {
		t.Fatalf("options=%v (%T)", params[0].(map[string]any)["options"], params[0].(map[string]any)["options"])
	}
	if opts[0]["value"] != "USA" || opts[1]["value"] != "Canada" {
		t.Fatalf("opts=%v", opts)
	}
}

func TestExpandSQLParameterOptions_OracleSkippedWithoutOpener(t *testing.T) {
	def := map[string]any{
		"datasource": "airports_oracle",
		"parameters": []any{
			map[string]any{
				"name":   "p_state",
				"type":   "select",
				"source": "sql:select DISTINCT STATE_NAME from FRA_SAMPLE_MAP_AIRPORTS",
			},
		},
	}
	finder := stubFinder{ds: &datasources.Datasource{Name: "airports_oracle", DBType: "oracle"}}
	out := datasources.ExpandSQLParameterOptions(context.Background(), def, finder, stubConnector{})
	params := out["parameters"].([]any)
	if params[0].(map[string]any)["options"] != nil {
		t.Fatal("expected options left empty when connector has no Oracle opener")
	}
}

type stubOracleConnector struct {
	stubConnector
	db  *sql.DB
	err error
}

func (c stubOracleConnector) OpenOracle(ctx context.Context, ds *datasources.Datasource) (*sql.DB, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.db, nil
}

func TestExpandSQLParameterOptions_Oracle(t *testing.T) {
	sql.Register("oracle-options-fake", &fakeMySQLDriver{rows: [][]driver.Value{
		{"California"},
		{"Texas"},
	}})
	db, err := sql.Open("oracle-options-fake", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	def := map[string]any{
		"datasource": "airports_oracle",
		"parameters": []any{
			map[string]any{
				"name":   "p_state",
				"type":   "select",
				"source": "sql:select DISTINCT STATE_NAME from FRA_SAMPLE_MAP_AIRPORTS",
			},
		},
	}
	finder := stubFinder{ds: &datasources.Datasource{Name: "airports_oracle", DBType: "oracle"}}
	out := datasources.ExpandSQLParameterOptions(context.Background(), def, finder, stubOracleConnector{db: db})
	params := out["parameters"].([]any)
	opts, ok := params[0].(map[string]any)["options"].([]map[string]any)
	if !ok || len(opts) != 2 {
		t.Fatalf("options=%v (%T)", params[0].(map[string]any)["options"], params[0].(map[string]any)["options"])
	}
	if opts[0]["value"] != "California" || opts[1]["value"] != "Texas" {
		t.Fatalf("opts=%v", opts)
	}
}

type fakeMySQLDriver struct {
	rows [][]driver.Value
}

func (d *fakeMySQLDriver) Open(name string) (driver.Conn, error) {
	return &fakeMySQLConn{rows: d.rows}, nil
}

type fakeMySQLConn struct {
	rows [][]driver.Value
}

func (c *fakeMySQLConn) Prepare(query string) (driver.Stmt, error) { return nil, nil }
func (c *fakeMySQLConn) Close() error                              { return nil }
func (c *fakeMySQLConn) Begin() (driver.Tx, error)                 { return nil, nil }
func (c *fakeMySQLConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return &fakeMySQLRows{cols: []string{"country"}, data: c.rows}, nil
}

type fakeMySQLRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *fakeMySQLRows) Columns() []string { return r.cols }
func (r *fakeMySQLRows) Close() error      { return nil }
func (r *fakeMySQLRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}
