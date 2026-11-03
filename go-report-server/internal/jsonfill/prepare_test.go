package jsonfill

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareURLFetchesDocumentWithoutPackaging(t *testing.T) {
	dir := t.TempDir()
	jrxml := `<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="json"/>
  <property name="acugis.datasource.json.source" value="url"/>
  <property name="acugis.datasource.json.url" value="https://example.com/customers.json"/>
  <property name="acugis.datasource.json.collection" value="root"/>
  <query language="collection"><![CDATA[SELECT "customer" FROM "data"."root" WHERE "region" = $P{region}]]></query>
</jasperReport>`
	if err := os.WriteFile(filepath.Join(dir, "customers.jrxml"), []byte(jrxml), 0o644); err != nil {
		t.Fatal(err)
	}
	var fetched string
	previous := fetchJSON
	fetchJSON = func(_ context.Context, raw string) ([]byte, error) {
		fetched = raw
		return []byte(`[{"customer":"Ada","region":"North"},{"customer":"Bob","region":"South"}]`), nil
	}
	t.Cleanup(func() { fetchJSON = previous })
	ok, _, _, _, rdr, err := Prepare(context.Background(), dir, t.TempDir(), "customers", "region=North&_dataSource=secret")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if fetched != "https://example.com/customers.json" {
		t.Fatalf("fetched %s", fetched)
	}
	raw, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := url.ParseQuery(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("_dataSource") != "" {
		t.Fatal("datasource present")
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(vals.Get("_collectionData")), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["customer"] != "Ada" {
		t.Fatalf("rows %+v", rows)
	}
	if strings.Contains(string(raw), "example.com") {
		t.Fatal("url leaked into fill body")
	}
}
