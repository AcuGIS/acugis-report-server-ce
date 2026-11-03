package collectionfile

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeParamsKeepsXinAsAList(t *testing.T) {
	sql := `SELECT "c"."customer" FROM "data"."customers" "c" WHERE $X{IN, "c"."region", p_cregion}`
	in := map[string]any{"p_cregion": "North", "other": "leave"}
	got := NormalizeParams(sql, in)
	if in["p_cregion"] != "North" {
		t.Fatalf("caller map was rewritten: %+v", in)
	}
	if got["other"] != "leave" {
		t.Fatalf("scalar key %+v", got)
	}
	if !reflect.DeepEqual(got["p_cregion"], []string{"North"}) {
		t.Fatalf("single selection %#v", got["p_cregion"])
	}
	many := NormalizeParams(sql, map[string]any{"p_cregion": "North, South"})
	if !reflect.DeepEqual(many["p_cregion"], []string{"North", "South"}) {
		t.Fatalf("multiple selection %#v", many["p_cregion"])
	}
	again := NormalizeParams(sql, many)
	if !reflect.DeepEqual(again["p_cregion"], []string{"North", "South"}) {
		t.Fatalf("not idempotent %#v", again["p_cregion"])
	}
	jsonOne := NormalizeParams(sql, map[string]any{"p_cregion": `["North"]`})
	if !reflect.DeepEqual(jsonOne["p_cregion"], []string{"North"}) {
		t.Fatalf("json one %#v", jsonOne["p_cregion"])
	}
	jsonMany := NormalizeParams(sql, map[string]any{"p_cregion": `["North", "South"]`})
	if !reflect.DeepEqual(jsonMany["p_cregion"], []string{"North", "South"}) {
		t.Fatalf("json many %#v", jsonMany["p_cregion"])
	}
	slice := NormalizeParams(sql, map[string]any{"p_cregion": []string{"North"}})
	if !reflect.DeepEqual(slice["p_cregion"], []string{"North"}) {
		t.Fatalf("one-element slice %#v", slice["p_cregion"])
	}
	for _, raw := range []any{nil, "", "[]", []string{}} {
		empty := NormalizeParams(sql, map[string]any{"p_cregion": raw})
		if !reflect.DeepEqual(empty["p_cregion"], []string{}) {
			t.Fatalf("empty %#v from %#v", empty["p_cregion"], raw)
		}
	}
	missing := NormalizeParams(sql, map[string]any{})
	if !reflect.DeepEqual(missing["p_cregion"], []string{}) {
		t.Fatalf("missing %#v", missing["p_cregion"])
	}

	scalar := `SELECT "c"."customer" FROM "data"."customers" "c" WHERE "c"."region" = $P{p_cregion}`
	kept := NormalizeParams(scalar, map[string]any{"p_cregion": "North, South"})
	if kept["p_cregion"] != "North, South" {
		t.Fatalf("scalar $P was split: %#v", kept["p_cregion"])
	}
	inList := `SELECT customer FROM customers WHERE region IN ($P{p_cregion})`
	listed := NormalizeParams(inList, map[string]any{"p_cregion": "North, South"})
	if !reflect.DeepEqual(listed["p_cregion"], []string{"North", "South"}) {
		t.Fatalf("IN ($P) %#v", listed["p_cregion"])
	}
}

func TestQueryXinQualifiedCommaJSONAndEmpty(t *testing.T) {
	table, err := ParseCSV(strings.NewReader("customer,region\nAda,North\nBea,South\n,North\n"), Options{Delimiter: ','})
	if err != nil {
		t.Fatal(err)
	}
	sql := `SELECT "c"."customer" FROM "data"."customers" "c" WHERE $X{IN, "c"."region", p_cregion}`
	cases := []struct {
		name  string
		sql   string
		param any
		want  int
	}{
		{"string one", sql, "North", 2},
		{"string many", sql, "North, South", 3},
		{"json one", sql, `["North"]`, 2},
		{"json many", sql, `["North", "South"]`, 3},
		{"slice one", sql, []string{"North"}, 2},
		{"slice many", sql, []string{"North", "South"}, 3},
		{"empty string", sql, "", 3},
		{"null", sql, nil, 3},
		{"not in north", `SELECT customer FROM customers WHERE $X{NOTIN, region, p_cregion}`, "North", 1},
		{"empty not in", `SELECT customer FROM customers WHERE $X{NOTIN, region, p_cregion}`, "", 0},
		{"in dollar p many", `SELECT customer FROM customers WHERE region IN ($P{p_cregion})`, "North, South", 3},
		{"not in dollar p", `SELECT customer FROM customers WHERE region NOT IN ($P{p_cregion})`, "North", 1},
		{"scalar comma stays whole", `SELECT customer FROM customers WHERE region = $P{p_cregion}`, "North, South", 0},
		{"scalar north", `SELECT customer FROM customers WHERE region = $P{p_cregion}`, "North", 2},
		{"scalar null", `SELECT customer FROM customers WHERE region = $P{p_cregion}`, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Query(table, tc.sql, map[string]any{"p_cregion": tc.param}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Rows) != tc.want {
				t.Fatalf("rows %d want %d (%#v)", len(got.Rows), tc.want, got.Rows)
			}
		})
	}
	got, err := Query(table, sql, map[string]any{}, 0)
	if err != nil || len(got.Rows) != 3 {
		t.Fatalf("missing param rows %#v err %v", got, err)
	}
}
