package collectionfile

import (
	"strings"
	"testing"
)

func TestJSONDiscoveryAndRecords(t *testing.T) {
	root := `[
	  {"customer":"Ada","region":"North","quantity":4,"amount":19.95,"active":true},
	  {"customer":"Bob","region":"South","quantity":7,"amount":42.5,"active":false}
	]`
	doc, err := ParseJSON(strings.NewReader(root))
	if err != nil {
		t.Fatal(err)
	}
	cols := doc.Collections()
	if len(cols) != 1 || cols[0].Path != RootCollection || cols[0].Rows != 2 {
		t.Fatalf("collections %+v", cols)
	}
	table, err := doc.Table(RootCollection)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 2 || table.Rows[0]["customer"] != "Ada" || table.Rows[0]["quantity"] != int64(4) || table.Rows[0]["amount"] != 19.95 || table.Rows[0]["active"] != true {
		t.Fatalf("root row %+v", table.Rows[0])
	}
	if _, ok := table.Rows[0]["quantity"].(int64); !ok {
		t.Fatalf("quantity type %T", table.Rows[0]["quantity"])
	}
	if _, ok := table.Rows[0]["amount"].(float64); !ok {
		t.Fatalf("amount type %T", table.Rows[0]["amount"])
	}

	wrapped := `{"customers":[{"id":1,"name":"Ada"},{"id":2,"name":"Bob"}],"data":[{"id":3}],"results":[{"id":4}],"response":{"items":[{"id":5,"name":"Cara"}]}}`
	doc, err = ParseJSON(strings.NewReader(wrapped))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range doc.Collections() {
		names[c.Path] = true
		if c.Path == RootCollection {
			t.Fatal("wrapper object should not be a record collection")
		}
	}
	for _, want := range []string{"customers", "data", "results", "response.items"} {
		if !names[want] {
			t.Fatalf("missing %s in %+v", want, doc.Collections())
		}
	}
	selected, err := doc.Select("customers", "")
	if err != nil || selected != "customers" {
		t.Fatalf("select %s %v", selected, err)
	}
	table, err = doc.Table(selected)
	if err != nil || len(table.Rows) != 2 || table.Rows[1]["name"] != "Bob" || table.Rows[0]["id"] != int64(1) {
		t.Fatalf("customers %+v %v", table.Rows, err)
	}
	fromSQL, err := doc.Select("data", `SELECT "name" FROM "data"."response.items"`)
	if err != nil || fromSQL != "response.items" {
		t.Fatalf("from select %s %v", fromSQL, err)
	}
}

func TestJSONNestedFlattenAndArrays(t *testing.T) {
	doc, err := ParseJSON(strings.NewReader(`[{
	  "id": 123,
	  "name": "Ada",
	  "address": {"city": "Boston", "state": "MA"},
	  "tags": ["east", "priority"]
	}]`))
	if err != nil {
		t.Fatal(err)
	}
	table, err := doc.Table(RootCollection)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 1 {
		t.Fatalf("rows %d", len(table.Rows))
	}
	row := table.Rows[0]
	if row["id"] != int64(123) || row["name"] != "Ada" || row["address.city"] != "Boston" || row["address.state"] != "MA" {
		t.Fatalf("flat %+v", row)
	}
	if row["tags"] != `["east","priority"]` {
		t.Fatalf("tags %#v", row["tags"])
	}
	dup, err := ParseJSON(strings.NewReader(`{"a.b":1,"a":{"b":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dup.Table(""); err == nil || !strings.Contains(err.Error(), "duplicate field") {
		t.Fatalf("duplicate %v", err)
	}
}

func TestJSONTypesMissingAndNull(t *testing.T) {
	doc, err := ParseJSON(strings.NewReader(`[
	  {"id":1,"name":"Ada","region":"North","when":"2024-03-01","at":"2024-03-01T15:04:05Z","big":9007199254740993,"quantity":4},
	  {"id":2,"name":"Bob","when":"2024-03-02T00:00:00Z","at":"2024-03-02T16:00:00Z","big":3,"quantity":7.5},
	  {"id":3,"name":null,"region":"South","quantity":1}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	table, err := doc.Table("")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]Kind{}
	for _, c := range table.Columns {
		kinds[c.Name] = c.Kind
	}
	if kinds["id"] != KindInteger || kinds["quantity"] != KindDecimal || kinds["when"] != KindDateTime || kinds["at"] != KindDateTime || kinds["big"] != KindInteger || kinds["name"] != KindString {
		t.Fatalf("kinds %+v", kinds)
	}
	if _, ok := table.Rows[0]["big"].(int64); !ok || table.Rows[0]["big"] != int64(9007199254740993) {
		t.Fatalf("big %T %#v", table.Rows[0]["big"], table.Rows[0]["big"])
	}
	dated, err := ParseJSON(strings.NewReader(`[{"when":"2024-03-01"}]`))
	if err != nil {
		t.Fatal(err)
	}
	dateTable, err := dated.Table("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dateTable.Rows[0]["when"].(Date); !ok || dateTable.Columns[0].Kind != KindDate {
		t.Fatalf("date %T %+v", dateTable.Rows[0]["when"], dateTable.Columns)
	}
	if _, ok := table.Rows[1]["when"].(DateTime); !ok {
		t.Fatalf("widened when %T", table.Rows[1]["when"])
	}
	if _, ok := table.Rows[0]["region"]; !ok || table.Rows[1]["region"] != nil || table.Rows[2]["name"] != nil || table.Rows[2]["region"] != "South" {
		t.Fatalf("null/missing %+v", table.Rows)
	}
	if _, ok := table.Rows[1]["quantity"].(float64); !ok || table.Rows[0]["quantity"] != float64(4) {
		t.Fatalf("widened quantity %#v %#v", table.Rows[0]["quantity"], table.Rows[1]["quantity"])
	}
}

func TestJSONRejectsUnusableRootsAndLimits(t *testing.T) {
	for _, in := range []string{`1`, `"text"`, `true`, `null`, `[1,2]`} {
		if _, err := ParseJSON(strings.NewReader(in)); err == nil {
			t.Fatalf("accepted %s", in)
		}
	}
	scalarArray, err := ParseJSON(strings.NewReader(`{"items":[1,2]}`))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := scalarArray.Table("")
	if err != nil || kept.Rows[0]["items"] != "[1,2]" {
		t.Fatalf("scalar array %+v %v", kept, err)
	}
	if _, err := ParseJSON(strings.NewReader(`{"ok":true} extra`)); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("trailing %v", err)
	}
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < MaxJSONFields+1; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"f`)
		b.WriteString(strings.Repeat("x", 0))
		b.WriteString(itoa(i))
		b.WriteString(`":1`)
	}
	b.WriteString("}")
	wide, err := ParseJSON(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wide.Table(""); err == nil || !strings.Contains(err.Error(), "too many fields") {
		t.Fatalf("fields %v", err)
	}
	deep := `{"a":1}`
	for i := 0; i < maxDocumentDepth+2; i++ {
		deep = `{"k":` + deep + `}`
	}
	if _, err := ParseJSON(strings.NewReader(deep)); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("depth %v", err)
	}
}

func TestJSONQuerySubset(t *testing.T) {
	doc, err := ParseJSON(strings.NewReader(`[
	  {"customer":"Ada","region":"North","quantity":4},
	  {"customer":"Bob","region":"South","quantity":7},
	  {"customer":"Cara","region":"North","quantity":1}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	table, err := doc.Table(RootCollection)
	if err != nil {
		t.Fatal(err)
	}
	queried, err := Query(table, `SELECT "customer", "quantity" FROM "data"."root" WHERE "region" = $P{region} ORDER BY "quantity" DESC`, map[string]any{"region": "North"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(queried.Columns) != 2 || queried.Columns[0].Name != "customer" || len(queried.Rows) != 2 {
		t.Fatalf("projection %+v rows %+v", queried.Columns, queried.Rows)
	}
	if queried.Rows[0]["customer"] != "Ada" || queried.Rows[1]["customer"] != "Cara" {
		t.Fatalf("order %+v", queried.Rows)
	}
	one, err := Query(table, `SELECT "customer" FROM "data"."root" WHERE $X{IN, "region", region}`, map[string]any{"region": []string{"North"}}, 0)
	if err != nil || len(one.Rows) != 2 {
		t.Fatalf("one in %+v %v", one, err)
	}
	many, err := Query(table, `SELECT "customer" FROM "data"."root" WHERE $X{IN, "region", region}`, map[string]any{"region": []string{"North", "South"}}, 0)
	if err != nil || len(many.Rows) != 3 {
		t.Fatalf("many in %+v %v", many, err)
	}
	notIn, err := Query(table, `SELECT "customer" FROM "data"."root" WHERE $X{NOTIN, "region", region}`, map[string]any{"region": []string{"North"}}, 0)
	if err != nil || len(notIn.Rows) != 1 || notIn.Rows[0]["customer"] != "Bob" {
		t.Fatalf("notin %+v %v", notIn, err)
	}
	if _, err := Query(table, `SELECT "customer" FROM "data"."root" JOIN "data"."other" ON 1=1`, nil, 0); err == nil || !strings.Contains(err.Error(), "JOIN") {
		t.Fatalf("join %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
