package csvfill

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/acugis/acugis-report-server-go/internal/jasper"
)

const publishedXinSQL = "SELECT \"c\".\"customer\", \"c\".\"region\", \"c\".\"quantity\", \"c\".\"amount\", \"c\".\"active\", \"c\".\"order_date\"\n" +
	"FROM \"data\".\"customers\" \"c\"\n" +
	"WHERE $X{IN, \"c\".\"region\", p_cregion}"

func TestPublishedCSVXinMatchesDesignerSelections(t *testing.T) {
	src := filepath.Join("..", "..", "..", "..", "acugis-report-designer", "testdata", "csv", "customers.csv")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	jrxmlDir := filepath.Join(dir, "jrxml")
	pathsDir := filepath.Join(dir, "resource-paths")
	csvPath := filepath.Join(pathsDir, "sales", "data", "customers.csv")
	if err := os.MkdirAll(jrxmlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(csvPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="csv"/>
  <property name="acugis.datasource.resource" value="data/customers.csv"/>
  <property name="acugis.datasource.delimiter" value="comma"/>
  <query language="collection"><![CDATA[` + publishedXinSQL + `]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(jrxmlDir, "customers.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jrxmlDir, "customers.resource-base"), []byte("sales\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	one := jasper.ReportFillParams("p_cregion=North")
	if one["p_cregion"] != "North" {
		t.Fatalf("one query key %#v", one["p_cregion"])
	}
	if !reflect.DeepEqual(collectionfile.NormalizeParams(publishedXinSQL, one)["p_cregion"], []string{"North"}) {
		t.Fatalf("published single %#v", collectionfile.NormalizeParams(publishedXinSQL, one)["p_cregion"])
	}
	many := jasper.ReportFillParams("p_cregion=North&p_cregion=South")
	if !reflect.DeepEqual(many["p_cregion"], []string{"North", "South"}) {
		t.Fatalf("repeated keys %#v", many["p_cregion"])
	}
	if !reflect.DeepEqual(collectionfile.NormalizeParams(publishedXinSQL, many)["p_cregion"], []string{"North", "South"}) {
		t.Fatalf("published multiple %#v", collectionfile.NormalizeParams(publishedXinSQL, many)["p_cregion"])
	}

	cases := []struct {
		raw  string
		want int
	}{
		{"p_cregion=North", 4},
		{"p_cregion=North&p_cregion=South", 5},
		{"p_cregion=", 7},
		{"", 7},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			ok, _, _, _, rdr, err := Prepare(jrxmlDir, pathsDir, "customers", tc.raw)
			if err != nil || !ok {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			raw, err := io.ReadAll(rdr)
			if err != nil {
				t.Fatal(err)
			}
			vals, err := url.ParseQuery(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			var rows []map[string]any
			if err := json.Unmarshal([]byte(vals.Get("_collectionData")), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.want {
				t.Fatalf("rows %d want %d (%s)", len(rows), tc.want, vals.Get("_collectionData"))
			}
		})
	}
}
