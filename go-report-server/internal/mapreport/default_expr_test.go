package mapreport_test

import (
	"reflect"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

func TestParseDefaultValueExpression(t *testing.T) {
	cases := []struct {
		expr string
		want any
	}{
		{`java.util.Arrays.asList("Austin")`, []string{"Austin"}},
		{`Arrays.asList("Austin", "Dallas")`, []string{"Austin", "Dallas"}},
		{`java.util.Collections.singletonList("Glendale")`, []string{"Glendale"}},
		{`java.util.Collections.emptyList()`, []string{}},
		{`"Austin"`, "Austin"},
		{`'Dallas'`, "Dallas"},
		{`true`, true},
		{`Boolean.TRUE`, true},
		{`Integer.valueOf(42)`, 42},
		{`null`, nil},
		{``, nil},
		{`someHelper.compute()`, nil},
	}
	for _, tc := range cases {
		got := mapreport.ParseDefaultValueExpression(tc.expr)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("expr=%q got=%#v want=%#v", tc.expr, got, tc.want)
		}
	}
}
