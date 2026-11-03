package jasper_test

import (
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/jasper"
	"github.com/acugis/acugis-report-server-go/internal/reports"
)

func TestAttachCollectionDataFormURLEncoded(t *testing.T) {
	rows := []map[string]any{
		{"sales": "12.50", "state": "CA"},
	}
	method, rawQuery, ct, body, err := jasper.AttachCollectionData("_repName=demo&_dataSource=Astra1&_repFormat=pdf&p_state=CA", rows)
	if err != nil {
		t.Fatal(err)
	}
	if method != "POST" {
		t.Fatalf("method=%s", method)
	}
	if rawQuery != "" {
		t.Fatalf("rawQuery should be empty, got %q", rawQuery)
	}
	if !strings.Contains(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("ct=%s", ct)
	}
	b, _ := io.ReadAll(body)
	vals, err := url.ParseQuery(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("_repName") != "demo" || vals.Get("_dataSource") != "Astra1" {
		t.Fatalf("missing report params: %v", vals)
	}
	if !strings.Contains(vals.Get("_collectionData"), `"sales"`) {
		t.Fatalf("collection missing: %s", vals.Get("_collectionData"))
	}
	if vals.Get("p_state") != "CA" {
		t.Fatalf("param lost: %v", vals)
	}
}

func TestExtractCollectionQueryPrefersCassandraLanguage(t *testing.T) {
	jrxml := []byte(`<?xml version="1.0"?>
<jasperReport>
  <property name="acugis.datasource.provider" value="cassandra"/>
  <queryString language="cassandra"><![CDATA[SELECT customer_id, sales FROM customers WHERE state = $P{p_state}]]></queryString>
</jasperReport>`)
	got := reports.ExtractCollectionQueryFromJRXML(jrxml)
	if !strings.Contains(got, "SELECT customer_id") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "$P{p_state}") {
		t.Fatalf("params not preserved: %q", got)
	}
}
