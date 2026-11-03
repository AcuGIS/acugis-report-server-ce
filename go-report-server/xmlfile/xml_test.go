package xmlfile

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func parseString(t *testing.T, xml string) *Document {
	t.Helper()
	doc, err := Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCustomersCollectionAndTypes(t *testing.T) {
	doc := parseString(t, `<?xml version="1.0" encoding="UTF-8"?>
<customers>
  <customer id="1001" active="true">
    <id>1</id>
    <name>Ada</name>
    <region>North</region>
    <amount>19.95</amount>
    <orderDate>2024-03-15</orderDate>
    <address><city>Boston</city><state>MA</state></address>
  </customer>
  <customer id="1002" active="false">
    <id>2</id>
    <name>Bob</name>
    <region>South</region>
    <amount>42.50</amount>
    <orderDate>2024-04-02</orderDate>
    <address><city>Austin</city></address>
  </customer>
</customers>`)
	cols := doc.Collections()
	if len(cols) != 1 || cols[0].Path != "/customers/customer" || cols[0].Rows != 2 {
		t.Fatalf("collections %+v", cols)
	}
	table, err := doc.Table("/customers/customer")
	if err != nil {
		t.Fatal(err)
	}
	ada := table.Rows[0]
	if ada["@id"] != int64(1001) || ada["@active"] != true || ada["name"] != "Ada" || ada["region"] != "North" {
		t.Fatalf("ada %#v", ada)
	}
	if ada["amount"] != 19.95 || ada["address.city"] != "Boston" || ada["address.state"] != "MA" {
		t.Fatalf("nested %#v", ada)
	}
	if _, ok := ada["orderDate"].(collectionfile.Date); !ok {
		t.Fatalf("date %#v", ada["orderDate"])
	}
	if table.Rows[1]["address.state"] != nil {
		t.Fatalf("missing state %#v", table.Rows[1]["address.state"])
	}
	names := map[string]collectionfile.Kind{}
	for _, c := range table.Columns {
		names[c.Name] = c.Kind
	}
	if names["@id"] != collectionfile.KindInteger || names["amount"] != collectionfile.KindDecimal || names["@active"] != collectionfile.KindBoolean {
		t.Fatalf("kinds %#v", names)
	}
}

func TestMultipleCollectionsAndNestedFlatten(t *testing.T) {
	doc := parseString(t, `<report>
  <customers><customer><id>1</id><name>Ada</name></customer><customer><id>2</id><name>Bob</name></customer></customers>
  <orders><order><id>9</id></order><order><id>8</id></order></orders>
</report>`)
	var paths []string
	for _, c := range doc.Collections() {
		paths = append(paths, c.Path)
	}
	if strings.Join(paths, ",") != "/report/customers/customer,/report/orders/order" {
		t.Fatalf("paths %v", paths)
	}
	selected, err := doc.Select("/report/orders/order", "")
	if err != nil || selected != "/report/orders/order" {
		t.Fatalf("select %s %v", selected, err)
	}
	table, err := doc.Table(selected)
	if err != nil || len(table.Rows) != 2 || table.Rows[0]["id"] != int64(9) {
		t.Fatalf("orders %#v %v", table, err)
	}
}

func TestAttributesDoNotCollideWithElements(t *testing.T) {
	doc := parseString(t, `<customers>
  <customer id="123" active="true"><id>99</id><name>Ada</name><region>North</region></customer>
  <customer id="124" active="false"><id>100</id><name>Bob</name><region>South</region></customer>
</customers>`)
	table, err := doc.Table("")
	if err != nil {
		t.Fatal(err)
	}
	if table.Rows[0]["@id"] != int64(123) || table.Rows[0]["id"] != int64(99) || table.Rows[0]["name"] != "Ada" {
		t.Fatalf("row %#v", table.Rows[0])
	}
}

func TestNamespacesMatchURINotPrefix(t *testing.T) {
	one := parseString(t, `<c:customers xmlns:c="https://example.com/customer">
  <c:customer><c:id>1</c:id><c:name>Ada</c:name></c:customer>
  <c:customer><c:id>2</c:id><c:name>Bob</c:name></c:customer>
</c:customers>`)
	two := parseString(t, `<d:customers xmlns:d="https://example.com/customer">
  <d:customer><d:id>1</d:id><d:name>Ada</d:name></d:customer>
  <d:customer><d:id>2</d:id><d:name>Bob</d:name></d:customer>
</d:customers>`)
	if one.Collections()[0].Path != "/customers/customer" || two.Collections()[0].Path != one.Collections()[0].Path {
		t.Fatalf("paths %s %s", one.Collections()[0].Path, two.Collections()[0].Path)
	}
	table, err := one.Table("")
	if err != nil || table.Rows[0]["name"] != "Ada" || table.Rows[0]["id"] != int64(1) {
		t.Fatalf("row %#v %v", table, err)
	}
	for _, c := range table.Columns {
		if strings.Contains(c.Name, "xmlns") || strings.Contains(c.Name, "example.com") {
			t.Fatalf("namespace leaked into field %s", c.Name)
		}
	}
}

func TestNamespaceCollisionIsDisambiguated(t *testing.T) {
	doc := parseString(t, `<root xmlns:a="urn:a" xmlns:b="urn:b">
  <a:row><a:name>A</a:name><b:name>B</b:name></a:row>
  <a:row><a:name>C</a:name><b:name>D</b:name></a:row>
  <b:row><b:name>E</b:name></b:row>
  <b:row><b:name>F</b:name></b:row>
</root>`)
	var paths []string
	for _, c := range doc.Collections() {
		paths = append(paths, c.Path)
	}
	if strings.Join(paths, ",") != "/root/{urn:a}row,/root/{urn:b}row" {
		t.Fatalf("paths %v", paths)
	}
	table, err := doc.Table("/root/{urn:a}row")
	if err != nil {
		t.Fatal(err)
	}
	if table.Rows[0]["{urn:a}name"] != "A" || table.Rows[0]["{urn:b}name"] != "B" {
		t.Fatalf("fields %#v", table.Rows[0])
	}
	if _, ok := table.Rows[0]["name"]; ok {
		t.Fatal("colliding local names were combined")
	}
}

func TestCDATARepeatedChildrenAndNulls(t *testing.T) {
	doc := parseString(t, `<customers>
  <customer>
    <name>Ada</name>
    <description><![CDATA[North & West]]></description>
    <phone>111</phone>
    <phone>222</phone>
    <note></note>
    <city>   </city>
  </customer>
  <customer>
    <name>Bob</name>
    <description>plain</description>
    <phone>333</phone>
  </customer>
</customers>`)
	table, err := doc.Table("/customers/customer")
	if err != nil {
		t.Fatal(err)
	}
	ada := table.Rows[0]
	if ada["description"] != "North & West" || ada["phone"] != "111 | 222" || ada["note"] != nil || ada["city"] != "   " {
		t.Fatalf("ada %#v", ada)
	}
	if table.Rows[1]["note"] != nil || table.Rows[1]["city"] != nil || table.Rows[1]["phone"] != "333" {
		t.Fatalf("bob %#v", table.Rows[1])
	}
	kind := map[string]collectionfile.Kind{}
	for _, c := range table.Columns {
		kind[c.Name] = c.Kind
	}
	if kind["phone"] != collectionfile.KindString || kind["city"] != collectionfile.KindString {
		t.Fatalf("kinds %#v", kind)
	}
}

func TestMixedContentIsCharacterData(t *testing.T) {
	doc := parseString(t, "<customers><customer><description>\n  This is <b>important</b> text.\n</description></customer><customer><description>plain</description></customer></customers>")
	table, err := doc.Table("")
	if err != nil {
		t.Fatal(err)
	}
	got := table.Rows[0]["description"]
	want := "\n  This is important text.\n"
	if got != want {
		t.Fatalf("mixed %q", got)
	}
	if _, ok := table.Rows[0]["description.b"]; ok {
		t.Fatal("mixed markup became a field")
	}
}

func TestTypeInferenceWideningAndLeadingZeros(t *testing.T) {
	doc := parseString(t, `<rows>
  <row><qty>4</qty><amount>8</amount><active>true</active><day>2024-03-15</day><postalCode>00701</postalCode><code>0</code></row>
  <row><qty>5</qty><amount>1.5</amount><active>false</active><day>2024-04-02T15:04:05Z</day><postalCode>10001</postalCode><code>10</code></row>
</rows>`)
	table, err := doc.Table("/rows/row")
	if err != nil {
		t.Fatal(err)
	}
	kind := map[string]collectionfile.Kind{}
	for _, c := range table.Columns {
		kind[c.Name] = c.Kind
	}
	if kind["qty"] != collectionfile.KindInteger || kind["amount"] != collectionfile.KindDecimal || kind["active"] != collectionfile.KindBoolean {
		t.Fatalf("kinds %#v", kind)
	}
	if kind["day"] != collectionfile.KindDateTime || kind["postalCode"] != collectionfile.KindString || kind["code"] != collectionfile.KindInteger {
		t.Fatalf("kinds %#v", kind)
	}
	if table.Rows[0]["postalCode"] != "00701" || table.Rows[0]["amount"] != float64(8) || table.Rows[0]["code"] != int64(0) {
		t.Fatalf("row %#v", table.Rows[0])
	}
	if _, ok := table.Rows[0]["day"].(collectionfile.DateTime); !ok {
		t.Fatalf("widened day %#v", table.Rows[0]["day"])
	}
}

func TestIncompatibleTypesBecomeString(t *testing.T) {
	doc := parseString(t, `<rows><row><v>true</v></row><row><v>12</v></row></rows>`)
	table, err := doc.Table("")
	if err != nil {
		t.Fatal(err)
	}
	if table.Columns[0].Kind != collectionfile.KindString || table.Rows[0]["v"] != "true" || table.Rows[1]["v"] != "12" {
		t.Fatalf("table %#v", table)
	}
}

func TestMalformedAndLimits(t *testing.T) {
	if _, err := Parse(strings.NewReader(`<customers><customer></customers>`)); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("malformed %v", err)
	}
	deep := "<a>" + strings.Repeat("<b>", 40) + "x" + strings.Repeat("</b>", 40) + "</a>"
	if _, err := Parse(strings.NewReader(deep)); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("depth %v", err)
	}
	var fields strings.Builder
	fields.WriteString("<rows><row>")
	for i := 0; i < MaxFields+1; i++ {
		fields.WriteString("<f")
		fields.WriteString(strings.Repeat("x", 0))
		fields.WriteString(itoa(i))
		fields.WriteString(">1</f")
		fields.WriteString(itoa(i))
		fields.WriteString(">")
	}
	fields.WriteString("</row><row>")
	for i := 0; i < MaxFields+1; i++ {
		fields.WriteString("<f")
		fields.WriteString(itoa(i))
		fields.WriteString(">2</f")
		fields.WriteString(itoa(i))
		fields.WriteString(">")
	}
	fields.WriteString("</row></rows>")
	doc, err := Parse(strings.NewReader(fields.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Table(""); err == nil || !strings.Contains(err.Error(), "too many fields") {
		t.Fatalf("fields %v", err)
	}
	var cols strings.Builder
	cols.WriteString("<root>")
	for i := 0; i < MaxCollections+1; i++ {
		cols.WriteString("<c")
		cols.WriteString(itoa(i))
		cols.WriteString("><r><v>1</v></r><r><v>2</v></r></c")
		cols.WriteString(itoa(i))
		cols.WriteString(">")
	}
	cols.WriteString("</root>")
	if _, err := Parse(strings.NewReader(cols.String())); err == nil || !strings.Contains(err.Error(), "too many record collections") {
		t.Fatalf("collections %v", err)
	}
	big := bytes.Repeat([]byte(" "), MaxBytes+1)
	if _, err := Parse(bytes.NewReader(big)); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("size %v", err)
	}
	long := "<rows><row><name>" + strings.Repeat("a", MaxValueBytes+1) + "</name></row><row><name>b</name></row></rows>"
	if _, err := Parse(strings.NewReader(long)); err == nil || !strings.Contains(err.Error(), "value exceeds") {
		t.Fatalf("value %v", err)
	}
}

func TestRejectsDTDEntitiesAndExternalFetch(t *testing.T) {
	secret := "SENTINEL_SECRET_VALUE"
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	fileXML := `<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///` + filepath.ToSlash(path) + `">]><foo>&xxe;</foo>`
	_, err := Parse(strings.NewReader(fileXML))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("file entity %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != secret {
		t.Fatal("fixture changed")
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("PWNED"))
	}))
	defer srv.Close()
	netXML := `<!DOCTYPE foo [<!ENTITY xxe SYSTEM "` + srv.URL + `">]><foo>&xxe;</foo>`
	_, err = Parse(strings.NewReader(netXML))
	if err == nil || strings.Contains(err.Error(), "PWNED") {
		t.Fatalf("network entity %v", err)
	}
	if hits != 0 {
		t.Fatalf("external fetch hits %d", hits)
	}
	bombs := `<!DOCTYPE lol [<!ENTITY a "lol"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">]><lol>&b;</lol>`
	if _, err := Parse(strings.NewReader(bombs)); err == nil || !strings.Contains(err.Error(), "DTD") {
		t.Fatalf("expansion %v", err)
	}
	if _, err := Parse(strings.NewReader(`<name>North &amp; West</name><name>Ada</name>`)); err == nil {
		t.Fatal("two roots should fail")
	}
	doc := parseString(t, `<customers><customer><name>North &amp; West</name></customer><customer><name>&#65;da</name></customer></customers>`)
	table, err := doc.Table("")
	if err != nil || table.Rows[0]["name"] != "North & West" || table.Rows[1]["name"] != "Ada" {
		t.Fatalf("entities %#v %v", table, err)
	}
}

func TestEncodings(t *testing.T) {
	latin := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><customers><customer><name>Jos\xe9</name></customer><customer><name>Ana</name></customer></customers>")
	doc, err := Parse(bytes.NewReader(latin))
	if err != nil {
		t.Fatal(err)
	}
	table, err := doc.Table("")
	if err != nil || table.Rows[0]["name"] != "José" {
		t.Fatalf("latin %#v %v", table, err)
	}
	win := []byte("<?xml version=\"1.0\" encoding=\"Windows-1252\"?><customers><customer><name>\x80</name></customer><customer><name>A</name></customer></customers>")
	doc, err = Parse(bytes.NewReader(win))
	if err != nil {
		t.Fatal(err)
	}
	table, err = doc.Table("")
	if err != nil || table.Rows[0]["name"] != "€" {
		t.Fatalf("cp1252 %#v %v", table, err)
	}
	if _, err := Parse(strings.NewReader(`<?xml version="1.0" encoding="UTF-32"?><a/>`)); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("utf32 %v", err)
	}
	if _, err := Parse(bytes.NewReader([]byte("<?xml version=\"1.0\" encoding=\"US-ASCII\"?><a>\xff</a>"))); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("ascii %v", err)
	}
	encoded, err := utf16LE(`<?xml version="1.0" encoding="UTF-16"?><customers><customer><name>Ada</name></customer><customer><name>Bob</name></customer></customers>`)
	if err != nil {
		t.Fatal(err)
	}
	doc, err = Parse(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	table, err = doc.Table("")
	if err != nil || table.Rows[0]["name"] != "Ada" || table.Rows[1]["name"] != "Bob" {
		t.Fatalf("utf16 %#v %v", table, err)
	}
	if got := strings.Join(SupportedEncodings(), ","); !strings.Contains(got, "UTF-8") || !strings.Contains(got, "ISO-8859-1") || !strings.Contains(got, "Windows-1252") {
		t.Fatalf("encodings %s", got)
	}
}

func TestFlattenDepthAndRepeatedStructuredChildren(t *testing.T) {
	doc := parseString(t, `<customers>
  <customer><id>1</id><phone><number>111</number></phone><phone><number>222</number></phone></customer>
  <customer><id>2</id><phone><number>333</number></phone></customer>
</customers>`)
	paths := doc.Collections()
	if len(paths) < 1 || paths[0].Path != "/customers/customer" {
		t.Fatalf("paths %+v", paths)
	}
	table, err := doc.Table("/customers/customer")
	if err != nil {
		t.Fatal(err)
	}
	if table.Rows[0]["phone.number"] != "111 | 222" || len(table.Rows) != 2 {
		t.Fatalf("rows %#v", table.Rows)
	}
	nested := "<row>" + strings.Repeat("<n>", MaxFlattenDepth+2) + "<leaf>Z</leaf>" + strings.Repeat("</n>", MaxFlattenDepth+2) + "</row>"
	doc = parseString(t, "<rows>"+nested+"<row><leaf>Y</leaf></row></rows>")
	table, err = doc.Table("/rows/row")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range table.Columns {
		if strings.Contains(c.Name, "leaf") && strings.Count(c.Name, ".") > MaxFlattenDepth {
			t.Fatalf("flattened past the depth limit: %s", c.Name)
		}
		if strings.Contains(fmtSprint(table.Rows[0][c.Name]), "Z") {
			found = true
		}
	}
	if !found {
		t.Fatalf("deep value missing %#v", table.Rows[0])
	}
}

func TestSelectUsesFromTable(t *testing.T) {
	doc := parseString(t, `<report>
  <customers><customer><name>Ada</name></customer><customer><name>Bob</name></customer></customers>
  <orders><order><name>O</name></order><order><name>P</name></order></orders>
</report>`)
	selected, err := doc.Select("/report/customers/customer", `SELECT "name" FROM "data"."/report/orders/order"`)
	if err != nil || selected != "/report/orders/order" {
		t.Fatalf("selected %s %v", selected, err)
	}
}

func utf16LE(s string) ([]byte, error) {
	enc := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewEncoder()
	return io.ReadAll(transform.NewReader(bytes.NewReader([]byte(s)), enc))
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

func fmtSprint(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestTooManyRecords(t *testing.T) {
	var b strings.Builder
	b.WriteString("<rows>")
	for i := 0; i < MaxRecords+1; i++ {
		b.WriteString("<row><id>1</id></row>")
	}
	b.WriteString("</rows>")
	if _, err := Parse(strings.NewReader(b.String())); err == nil || !strings.Contains(err.Error(), "too many records") {
		t.Fatalf("records %v", err)
	}
}
