package mapreport_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/internal/mapreport"
)

func TestJRXMLParameterLookups_NestedProperties(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cascade.jrxml")
	body := `<?xml version="1.0"?>
<jasperReport>
  <parameter name="p_country" class="java.lang.String"/>
  <parameter name="p_city" class="java.util.Collection">
    <property name="acugis.parameter.p_city.lookup.enabled" value="true"/>
    <property name="acugis.parameter.p_city.lookup.datasource" value="Current Report Dataset"/>
    <property name="acugis.parameter.p_city.lookup.depends" value="p_country"/>
    <property name="acugis.parameter.p_city.lookup.displayField" value="city"/>
    <property name="acugis.parameter.p_city.lookup.valueField" value="city"/>
    <property name="acugis.parameter.p_city.lookup.sql"><![CDATA[
SELECT DISTINCT city FROM customers WHERE country = $P{p_country} ORDER BY city
]]></property>
  </parameter>
  <queryString><![CDATA[SELECT 1]]></queryString>
</jasperReport>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	lookups, err := mapreport.JRXMLParameterLookups(path)
	if err != nil {
		t.Fatal(err)
	}
	city, ok := lookups["p_city"]
	if !ok {
		t.Fatalf("lookups=%v", lookups)
	}
	if !city.Enabled {
		t.Fatal("expected enabled")
	}
	if city.Datasource != mapreport.LookupDatasourceCurrentReport {
		t.Fatalf("datasource=%q", city.Datasource)
	}
	if len(city.Depends) != 1 || city.Depends[0] != "p_country" {
		t.Fatalf("depends=%v", city.Depends)
	}
	if city.DisplayField != "city" || city.ValueField != "city" {
		t.Fatalf("fields display=%q value=%q", city.DisplayField, city.ValueField)
	}
	if !containsAll(city.SQL, "SELECT DISTINCT city", "$P{p_country}") {
		t.Fatalf("sql=%q", city.SQL)
	}
	if _, ok := lookups["p_country"]; ok {
		t.Fatal("p_country should not have lookup")
	}
}

func TestJRXMLParameterLookups_MultiDependsAndSQLRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.jrxml")
	body := `<?xml version="1.0"?>
<jasperReport>
  <parameter name="p_city" class="java.lang.String">
    <property name="acugis.parameter.p_city.lookup.enabled" value="true"/>
    <property name="acugis.parameter.p_city.lookup.depends" value="p_country,p_state"/>
    <property name="acugis.parameter.p_city.lookup.sql"><![CDATA[
SELECT city FROM t WHERE country=$P{p_country} AND region=$P{p_region}
]]></property>
  </parameter>
</jasperReport>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	lookups, err := mapreport.JRXMLParameterLookups(path)
	if err != nil {
		t.Fatal(err)
	}
	city := lookups["p_city"]
	want := []string{"p_country", "p_state", "p_region"}
	if len(city.Depends) != len(want) {
		t.Fatalf("depends=%v want %v", city.Depends, want)
	}
	for i, d := range want {
		if city.Depends[i] != d {
			t.Fatalf("depends[%d]=%q want %q (full=%v)", i, city.Depends[i], d, city.Depends)
		}
	}
}

func TestJRXMLParameterLookups_DisabledOrMissingSQLSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skip.jrxml")
	body := `<?xml version="1.0"?>
<jasperReport>
  <parameter name="p_a" class="java.lang.String">
    <property name="acugis.parameter.p_a.lookup.enabled" value="false"/>
    <property name="acugis.parameter.p_a.lookup.sql" value="SELECT 1"/>
  </parameter>
  <parameter name="p_b" class="java.lang.String">
    <property name="acugis.parameter.p_b.lookup.enabled" value="true"/>
  </parameter>
</jasperReport>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	lookups, err := mapreport.JRXMLParameterLookups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lookups) != 0 {
		t.Fatalf("lookups=%v", lookups)
	}
}

func TestJRXMLParameterLookups_MissingFile(t *testing.T) {
	lookups, err := mapreport.JRXMLParameterLookups(filepath.Join(t.TempDir(), "missing.jrxml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lookups) != 0 {
		t.Fatalf("lookups=%v", lookups)
	}
}

func TestResolveLookupDatasource(t *testing.T) {
	if got := mapreport.ResolveLookupDatasource("Current Report Dataset", "sales_db"); got != "sales_db" {
		t.Fatalf("got=%q", got)
	}
	if got := mapreport.ResolveLookupDatasource("", "sales_db"); got != "sales_db" {
		t.Fatalf("got=%q", got)
	}
	if got := mapreport.ResolveLookupDatasource("other_ds", "sales_db"); got != "other_ds" {
		t.Fatalf("got=%q", got)
	}
}

func TestJRXMLParameters_DefaultValueExpression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.jrxml")
	body := `<?xml version="1.0"?>
<jasperReport>
  <parameter name="p_city" class="java.util.Collection">
    <defaultValueExpression><![CDATA[java.util.Arrays.asList("Austin")]]></defaultValueExpression>
  </parameter>
  <parameter name="p_country" class="java.lang.String">
    <defaultValueExpression><![CDATA["USA"]]></defaultValueExpression>
  </parameter>
  <parameter name="p_limit" class="java.lang.Integer">
    <defaultValueExpression><![CDATA[Integer.valueOf(10)]]></defaultValueExpression>
  </parameter>
  <queryString><![CDATA[SELECT 1]]></queryString>
</jasperReport>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	params, err := mapreport.JRXMLParameters(path)
	if err != nil {
		t.Fatal(err)
	}
	city := params["p_city"]
	got, ok := city.Default.([]string)
	if !ok || len(got) != 1 || got[0] != "Austin" {
		t.Fatalf("p_city default=%#v", city.Default)
	}
	if params["p_country"].Default != "USA" {
		t.Fatalf("p_country default=%#v", params["p_country"].Default)
	}
	if params["p_limit"].Default != 10 {
		t.Fatalf("p_limit default=%#v", params["p_limit"].Default)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
