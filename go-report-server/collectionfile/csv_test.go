package collectionfile

import (
	"strings"
	"testing"
	"time"
)

func TestParseCommaQuotedEscapesAndUTF8(t *testing.T) {
	in := "customer,note\r\n" +
		"Ada,\"hello \"\"world\"\"\"\r\n" +
		"José,東京\r\n"
	table, err := ParseCSV(strings.NewReader(in), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 2 || table.Rows[0]["note"] != `hello "world"` || table.Rows[1]["customer"] != "José" || table.Rows[1]["note"] != "東京" {
		t.Fatalf("%#v", table.Rows)
	}
}

func TestParseDelimitersAndAuto(t *testing.T) {
	cases := []struct {
		name string
		in   string
		opt  Options
		want string
	}{
		{"comma", "region,qty\nNorth,2\n", Options{Delimiter: ','}, "North"},
		{"tab", "region\tqty\nNorth\t2\n", Options{Delimiter: '\t'}, "North"},
		{"semicolon", "region;qty\nNorth;2\n", Options{Delimiter: ';'}, "North"},
		{"pipe", "region|qty\nNorth|2\n", Options{Delimiter: '|'}, "North"},
		{"auto semicolon", "region;qty\nNorth;2\nSouth;3\n", Options{Auto: true}, "North"},
		{"auto tab", "region\tqty\nNorth\t2\n", Options{Auto: true}, "North"},
		{"auto pipe", "region|qty\nNorth|2\n", Options{Auto: true}, "North"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table, err := ParseCSV(strings.NewReader(tc.in), tc.opt)
			if err != nil {
				t.Fatal(err)
			}
			if table.Rows[0]["region"] != tc.want || table.Rows[0]["qty"] != int64(2) {
				t.Fatalf("%#v delim=%q", table.Rows[0], table.Delimiter)
			}
		})
	}
}

func TestBlankStringVersusNull(t *testing.T) {
	in := "name,qty\n" +
		"\"\",1\n" +
		",2\n" +
		"NULL,3\n" +
		"\\N,4\n" +
		"\"NULL\",5\n"
	table, err := ParseCSV(strings.NewReader(in), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	if table.Columns[0].Kind != KindString || table.Columns[1].Kind != KindInteger {
		t.Fatalf("kinds %#v", table.Columns)
	}
	if table.Rows[0]["name"] != "" {
		t.Fatalf("quoted empty = %#v", table.Rows[0]["name"])
	}
	if table.Rows[1]["name"] != nil || table.Rows[2]["name"] != nil || table.Rows[3]["name"] != nil {
		t.Fatalf("nulls %#v %#v %#v", table.Rows[1]["name"], table.Rows[2]["name"], table.Rows[3]["name"])
	}
	if table.Rows[4]["name"] != "NULL" {
		t.Fatalf("quoted NULL = %#v", table.Rows[4]["name"])
	}
}

func TestTypeInference(t *testing.T) {
	in := "customer,quantity,amount,active,order_date,order_at,code\n" +
		"Ada,2,19.50,true,2024-03-01,2024-03-01T15:04:05Z,007\n" +
		"Bea,3,4,false,2024-03-02,2024-03-02 09:00:00,008\n" +
		",,8.25,,,,\n"
	table, err := ParseCSV(strings.NewReader(in), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	want := []Kind{KindString, KindInteger, KindDecimal, KindBoolean, KindDate, KindDateTime, KindString}
	for i, kind := range want {
		if table.Columns[i].Kind != kind {
			t.Fatalf("col %s kind %s want %s", table.Columns[i].Name, table.Columns[i].Kind, kind)
		}
	}
	if table.Rows[0]["quantity"] != int64(2) {
		t.Fatalf("qty %#v", table.Rows[0]["quantity"])
	}
	if table.Rows[0]["amount"] != 19.50 {
		t.Fatalf("amount %#v", table.Rows[0]["amount"])
	}
	if table.Rows[1]["amount"] != float64(4) {
		t.Fatalf("widened amount %#v", table.Rows[1]["amount"])
	}
	if table.Rows[0]["active"] != true || table.Rows[1]["active"] != false {
		t.Fatalf("bool %#v %#v", table.Rows[0]["active"], table.Rows[1]["active"])
	}
	if d, ok := table.Rows[0]["order_date"].(Date); !ok || d.Time.Format("2006-01-02") != "2024-03-01" {
		t.Fatalf("date %#v", table.Rows[0]["order_date"])
	}
	if d, ok := table.Rows[0]["order_at"].(DateTime); !ok || !d.Time.Equal(time.Date(2024, 3, 1, 15, 4, 5, 0, time.UTC)) {
		t.Fatalf("datetime %#v", table.Rows[0]["order_at"])
	}
	if table.Rows[2]["quantity"] != nil || table.Rows[2]["active"] != nil || table.Rows[2]["order_date"] != nil {
		t.Fatalf("empty numeric/date became %#v", table.Rows[2])
	}
	if table.Rows[0]["code"] != "007" {
		t.Fatalf("leading zero %#v", table.Rows[0]["code"])
	}
}

func TestMixedTypesWidenToString(t *testing.T) {
	in := "value\n1\n2\nnorth\n"
	table, err := ParseCSV(strings.NewReader(in), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	if table.Columns[0].Kind != KindString || table.Rows[0]["value"] != "1" || table.Rows[2]["value"] != "north" {
		t.Fatalf("%#v %#v", table.Columns[0], table.Rows)
	}
}

func TestAmbiguousDateStaysString(t *testing.T) {
	in := "when\n01/02/2024\n2024/03/01\n"
	table, err := ParseCSV(strings.NewReader(in), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	if table.Columns[0].Kind != KindString {
		t.Fatalf("kind %s", table.Columns[0].Kind)
	}
}

func TestHeaderOnlyAndMalformed(t *testing.T) {
	table, err := ParseCSV(strings.NewReader("customer,region\n"), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 0 || len(table.Columns) != 2 || table.Columns[0].Kind != KindString {
		t.Fatalf("%#v", table)
	}
	if _, err := ParseCSV(strings.NewReader(""), Options{}); err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatalf("empty: %v", err)
	}
	if _, err := ParseCSV(strings.NewReader("a,b\n1\n"), Options{}); err == nil || !strings.Contains(err.Error(), "row") {
		t.Fatalf("malformed: %v", err)
	}
	if _, err := ParseCSV(strings.NewReader("a,a\n1,2\n"), Options{}); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestQueryProjectionAndFilters(t *testing.T) {
	in := "customer,region,quantity,amount,active,order_date\n" +
		"Ada,North,2,19.50,true,2024-03-01\n" +
		"Bea,South,1,4.00,false,2024-04-01\n" +
		"\"\",North,3,8.00,true,2024-05-01\n" +
		",West,4,1.00,false,2024-06-01\n"
	table, err := ParseCSV(strings.NewReader(in), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Query(table, `SELECT "customer", "region" FROM "data"."customers"`, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Columns) != 2 || len(got.Rows) != 4 || got.Rows[0]["quantity"] != nil {
		t.Fatalf("projection %#v", got.Rows[0])
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE region = 'North' AND quantity >= 2`, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("filter rows %d", len(got.Rows))
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE region = $P{region}`, map[string]any{"region": "South"}, 0)
	if err != nil || len(got.Rows) != 1 || got.Rows[0]["customer"] != "Bea" {
		t.Fatalf("param %#v %v", got, err)
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE region <> $P{region}`, map[string]any{"region": "South"}, 0)
	if err != nil || len(got.Rows) != 3 {
		t.Fatalf("ne %#v %v", got, err)
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE customer = $P{name}`, map[string]any{"name": ""}, 0)
	if err != nil || len(got.Rows) != 1 || got.Rows[0]["customer"] != "" {
		t.Fatalf("blank %#v %v", got, err)
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE customer = $P{name}`, map[string]any{"name": nil}, 0)
	if err != nil || len(got.Rows) != 0 {
		t.Fatalf("null param matched %#v %v", got, err)
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE region IN ('North', 'South') AND active = true`, nil, 0)
	if err != nil || len(got.Rows) != 2 {
		t.Fatalf("in %#v %v", got, err)
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE $X{IN, region, regions}`, map[string]any{"regions": []string{"West"}}, 0)
	if err != nil || len(got.Rows) != 1 {
		t.Fatalf("xin %#v %v", got, err)
	}
	got, err = Query(table, `SELECT customer FROM customers WHERE quantity > 2 AND order_date < '2024-06-01'`, nil, 0)
	if err != nil || len(got.Rows) != 1 || got.Rows[0]["customer"] != "" {
		t.Fatalf("compare %#v %v", got, err)
	}
	if _, err := Query(table, `SELECT customer FROM customers JOIN other ON 1=1`, nil, 0); err == nil || !strings.Contains(err.Error(), "JOIN") {
		t.Fatalf("join %v", err)
	}
	if _, err := Query(table, `SELECT customer FROM customers WHERE region LIKE 'N%'`, nil, 0); err == nil || !strings.Contains(err.Error(), "LIKE") {
		t.Fatalf("like %v", err)
	}
	if _, err := Query(table, `SELECT SUM(quantity) FROM customers`, nil, 0); err == nil || !strings.Contains(err.Error(), "function") {
		t.Fatalf("agg %v", err)
	}
	if _, err := Query(table, `SELECT customer FROM customers GROUP BY customer`, nil, 0); err == nil || !strings.Contains(err.Error(), "GROUP BY") {
		t.Fatalf("group %v", err)
	}
	if _, err := Query(table, `SELECT customer FROM customers WHERE region = 'North' OR quantity = 'nope'`, nil, 0); err == nil {
		t.Fatal("expected type error")
	}
}
