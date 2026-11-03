package reports

import (
	"strings"
	"testing"
)

func TestExtractMainDatasetSimpleSQL(t *testing.T) {
	jrxml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<jasperReport xmlns="http://jasperreports.sourceforge.net/jasperreports">
  <queryString language="SQL"><![CDATA[SELECT id, name FROM public.customers]]></queryString>
  <field name="id" class="java.lang.Integer"/>
  <field name="name" class="java.lang.String"/>
</jasperReport>`)
	md, err := ExtractMainDataset(jrxml)
	if err != nil {
		t.Fatal(err)
	}
	if md.Query != "SELECT id, name FROM public.customers" {
		t.Fatalf("query=%q", md.Query)
	}
	if md.Language != "SQL" {
		t.Fatalf("language=%q", md.Language)
	}
	if len(md.Fields) != 2 || md.Fields[0].Name != "id" || md.Fields[1].Name != "name" {
		t.Fatalf("fields=%+v", md.Fields)
	}
}

func TestExtractMainDatasetIgnoresChartSubdataset(t *testing.T) {
	jrxml := []byte(`<jasperReport>
  <queryString language="SQL"><![CDATA[SELECT SUM(amount) AS revenue FROM orders]]></queryString>
  <field name="revenue" class="java.math.BigDecimal"/>
  <subDataset name="chart">
    <queryString language="SQL"><![CDATA[SELECT chart_only FROM chart_table]]></queryString>
    <field name="chart_only" class="java.lang.String"/>
    <parameter name="chart_param" class="java.lang.String"/>
  </subDataset>
</jasperReport>`)
	md, err := ExtractMainDataset(jrxml)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md.Query, "chart_only") || !strings.Contains(md.Query, "revenue") {
		t.Fatalf("query=%q", md.Query)
	}
	if len(md.Fields) != 1 || md.Fields[0].Name != "revenue" {
		t.Fatalf("fields=%+v", md.Fields)
	}
	if len(md.Parameters) != 0 {
		t.Fatalf("parameters=%+v", md.Parameters)
	}
}

func TestExtractMainDatasetSubdatasetBeforeMainQuery(t *testing.T) {
	jrxml := []byte(`<jasperReport>
  <detail>
    <band>
      <datasetRun subDataset="x">
        <queryString><![CDATA[SELECT should_not_profile FROM dataset_run]]></queryString>
      </datasetRun>
    </band>
  </detail>
  <subDataset name="chart">
    <parameter name="sub_only" class="java.lang.String">
      <defaultValueExpression><![CDATA["sub"]]></defaultValueExpression>
    </parameter>
    <queryString language="SQL"><![CDATA[SELECT from_chart FROM chart]]></queryString>
    <field name="from_chart" class="java.lang.String"/>
  </subDataset>
  <subreport>
    <queryString><![CDATA[SELECT from_subreport]]></queryString>
  </subreport>
  <parameter name="p_region" class="java.lang.String">
    <defaultValueExpression><![CDATA["West"]]></defaultValueExpression>
  </parameter>
  <queryString language="SQL"><![CDATA[SELECT main_col FROM main_table]]></queryString>
  <field name="main_col" class="java.lang.String"/>
</jasperReport>`)
	md, err := ExtractMainDataset(jrxml)
	if err != nil {
		t.Fatal(err)
	}
	if md.Query != "SELECT main_col FROM main_table" {
		t.Fatalf("query=%q", md.Query)
	}
	if len(md.Fields) != 1 || md.Fields[0].Name != "main_col" {
		t.Fatalf("fields=%+v", md.Fields)
	}
	if len(md.Parameters) != 1 || md.Parameters[0].Name != "p_region" || md.Parameters[0].DefaultExpr != `"West"` {
		t.Fatalf("parameters=%+v", md.Parameters)
	}
	// The recursive helper returns the first query it walks, which is the datasetRun query.
	mixed := ExtractCollectionQueryInfo(jrxml)
	if !strings.Contains(mixed.Query, "should_not_profile") {
		t.Fatalf("expected recursive helper to see the earlier query, got %q", mixed.Query)
	}
}

func TestExtractMainDatasetFieldsAndParametersOnlyMain(t *testing.T) {
	jrxml := []byte(`<jasperReport>
  <parameter name="p_start" class="java.sql.Timestamp" isForPrompting="true">
    <defaultValueExpression><![CDATA["2025-01-01"]]></defaultValueExpression>
  </parameter>
  <parameter name="p_flag" class="java.lang.Boolean" isForPrompting="false"/>
  <queryString language="SQL"><![CDATA[SELECT a, b FROM t WHERE d = $P{p_start}]]></queryString>
  <field name="a" class="java.lang.String"/>
  <field name="b" class="java.lang.Integer"/>
  <subDataset name="other">
    <parameter name="p_sub" class="java.lang.String"/>
    <queryString><![CDATA[SELECT sub_a, sub_b FROM sub]]></queryString>
    <field name="sub_a" class="java.lang.String"/>
    <field name="sub_b" class="java.lang.Integer"/>
  </subDataset>
</jasperReport>`)
	md, err := ExtractMainDataset(jrxml)
	if err != nil {
		t.Fatal(err)
	}
	if len(md.Fields) != 2 {
		t.Fatalf("fields=%+v", md.Fields)
	}
	if len(md.Parameters) != 2 {
		t.Fatalf("parameters=%+v", md.Parameters)
	}
	if md.Parameters[0].Name != "p_start" || !md.Parameters[0].ForPrompting || md.Parameters[0].DefaultExpr != `"2025-01-01"` {
		t.Fatalf("p_start=%+v", md.Parameters[0])
	}
	if md.Parameters[1].Name != "p_flag" || md.Parameters[1].ForPrompting {
		t.Fatalf("p_flag=%+v", md.Parameters[1])
	}
	for _, p := range md.Parameters {
		if p.Name == "p_sub" {
			t.Fatal("subdataset parameter included")
		}
	}
}

func TestExtractMainDatasetProviderProperty(t *testing.T) {
	jrxml := []byte(`<jasperReport>
  <property name="acugis.datasource.provider" value="bigquery"/>
  <subDataset name="x">
    <property name="acugis.datasource.provider" value="cassandra"/>
  </subDataset>
  <query language="SQL"><![CDATA[SELECT 1 AS n]]></query>
  <field name="n" class="java.lang.Long"/>
</jasperReport>`)
	md, err := ExtractMainDataset(jrxml)
	if err != nil {
		t.Fatal(err)
	}
	if md.Provider != "bigquery" {
		t.Fatalf("provider=%q", md.Provider)
	}
	if md.Query != "SELECT 1 AS n" || md.Language != "SQL" {
		t.Fatalf("query=%q lang=%q", md.Query, md.Language)
	}
}

func TestExtractMainDatasetNoQuery(t *testing.T) {
	md, err := ExtractMainDataset([]byte(`<jasperReport><field name="a" class="java.lang.String"/></jasperReport>`))
	if err != nil {
		t.Fatal(err)
	}
	if md.HasQuery() {
		t.Fatalf("expected no query, got %+v", md)
	}
}
