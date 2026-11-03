package excelfile

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/acugis/acugis-report-server-go/collectionfile"
	"github.com/xuri/excelize/v2"
)

func workbookBytes(t *testing.T, build func(f *excelize.File)) []byte {
	t.Helper()
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	build(f)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func openBytes(t *testing.T, body []byte) *Workbook {
	t.Helper()
	wb, err := Open(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wb.Close() })
	return wb
}

func TestSheetsTypesBlanksDatesAndFormats(t *testing.T) {
	pct := "0%"
	currency := `"$"#,##0.00`
	body := workbookBytes(t, func(f *excelize.File) {
		_, _ = f.NewSheet("Orders")
		_, _ = f.NewSheet("Sales 2026")
		_, _ = f.NewSheet("Hidden")
		_ = f.SetCellValue("Hidden", "A1", "Secret")
		_ = f.SetCellValue("Hidden", "A2", "nope")
		_ = f.SetSheetVisible("Hidden", false)
		_, _ = f.NewSheet("Empty")

		_ = f.MergeCell("Orders", "A1", "I1")
		_ = f.SetCellValue("Orders", "A1", "Orders")
		headers := []string{"Customer", "Region", "Quantity", "Amount", "Active", "Order Date", "Ordered At", "Discount", "Code"}
		for i, name := range headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 3)
			_ = f.SetCellValue("Orders", cell, name)
		}
		_ = f.SetCellValue("Orders", "A4", "Ada")
		_ = f.SetCellValue("Orders", "B4", "North")
		_ = f.SetCellValue("Orders", "C4", 4)
		_ = f.SetCellValue("Orders", "D4", 19.95)
		_ = f.SetCellValue("Orders", "E4", true)
		_ = f.SetCellValue("Orders", "F4", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		when := time.Date(2026, 10, 3, 15, 4, 5, 0, time.UTC)
		_ = f.SetCellValue("Orders", "G4", when)
		dateTimeStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 22})
		_ = f.SetCellStyle("Orders", "G4", "G5", dateTimeStyle)
		_ = f.SetCellValue("Orders", "H4", 0.25)
		pctStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &pct})
		_ = f.SetCellStyle("Orders", "H4", "H4", pctStyle)
		_ = f.SetCellValue("Orders", "I4", 45292)

		_ = f.SetCellValue("Orders", "A5", "Bob")
		_ = f.SetCellValue("Orders", "C5", 7)
		_ = f.SetCellValue("Orders", "D5", 1234.5)
		curStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &currency})
		_ = f.SetCellStyle("Orders", "D4", "D5", curStyle)
		_ = f.SetCellValue("Orders", "E5", false)
		_ = f.SetCellValue("Orders", "F5", time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
		_ = f.SetCellValue("Orders", "G5", time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC))
		_ = f.SetCellValue("Orders", "H5", 0.1)
		_ = f.SetCellStyle("Orders", "H5", "H5", pctStyle)
		_ = f.SetCellValue("Orders", "I5", 10)

		_ = f.SetCellValue("Sales 2026", "A1", "Name")
		_ = f.SetCellValue("Sales 2026", "B1", "Balance")
		_ = f.SetCellValue("Sales 2026", "A2", "Ada")
		_ = f.SetCellValue("Sales 2026", "B2", 10)
		_ = f.SetCellValue("Sheet1", "A1", "Customer")
		_ = f.SetCellValue("Sheet1", "A2", "Cara")
	})
	wb := openBytes(t, body)
	sheets, err := wb.Sheets()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(sheets, ",") != "Sheet1,Orders,Sales 2026" {
		t.Fatalf("sheets %v", sheets)
	}
	table, err := wb.Table("Orders")
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 2 {
		t.Fatalf("rows %+v", table.Rows)
	}
	ada := table.Rows[0]
	if ada["Customer"] != "Ada" || ada["Region"] != "North" || ada["Quantity"] != int64(4) {
		t.Fatalf("ada identity %+v", ada)
	}
	if ada["Amount"] != 19.95 {
		t.Fatalf("amount %#v", ada["Amount"])
	}
	if ada["Active"] != true {
		t.Fatalf("active %#v", ada["Active"])
	}
	if ada["Discount"] != 0.25 {
		t.Fatalf("discount %#v", ada["Discount"])
	}
	if ada["Code"] != int64(45292) {
		t.Fatalf("code %#v", ada["Code"])
	}
	day, ok := ada["Order Date"].(collectionfile.Date)
	if !ok || day.Time.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("date %#v", ada["Order Date"])
	}
	stamp, ok := ada["Ordered At"].(collectionfile.DateTime)
	if !ok || stamp.Time.Format(time.RFC3339) != "2026-10-03T15:04:05Z" {
		t.Fatalf("datetime %#v", ada["Ordered At"])
	}
	bob := table.Rows[1]
	if bob["Region"] != nil || bob["Amount"] != 1234.5 || bob["Active"] != false || bob["Customer"] != "Bob" {
		t.Fatalf("blank shift %+v", bob)
	}
	spaced, err := wb.Table("Sales 2026")
	if err != nil || spaced.Rows[0]["Name"] != "Ada" || spaced.Rows[0]["Balance"] != int64(10) {
		t.Fatalf("spaced %+v %v", spaced, err)
	}
}

func TestMixedTypesDuplicateHeadersEmptyAndFormulas(t *testing.T) {
	body := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetCellValue("Sheet1", "A1", "Quantity")
		_ = f.SetCellValue("Sheet1", "B1", "Unit Price")
		_ = f.SetCellValue("Sheet1", "C1", "Total")
		_ = f.SetCellValue("Sheet1", "D1", "Note")
		_ = f.SetCellValue("Sheet1", "A2", 5)
		_ = f.SetCellValue("Sheet1", "B2", 10.0)
		_ = f.SetCellValue("Sheet1", "C2", 50.25)
		_ = f.SetCellFormula("Sheet1", "C2", "A2*B2")
		_ = f.SetCellValue("Sheet1", "D2", 4)
		_ = f.SetCellValue("Sheet1", "A3", 2)
		_ = f.SetCellValue("Sheet1", "B3", 1.5)
		_ = f.SetCellValue("Sheet1", "C3", 3.5)
		_ = f.SetCellFormula("Sheet1", "C3", "A3*B3")
		_ = f.SetCellValue("Sheet1", "D3", "four")
	})
	body = numericFormulaCache(t, body)
	wb := openBytes(t, body)
	table, err := wb.Table("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if table.Columns[2].Kind != collectionfile.KindDecimal || table.Rows[0]["Total"] != 50.25 || table.Rows[1]["Total"] != 3.5 {
		t.Fatalf("formula %+v rows %+v", table.Columns, table.Rows)
	}
	if table.Columns[3].Kind != collectionfile.KindString || table.Rows[0]["Note"] != "4" || table.Rows[1]["Note"] != "four" {
		t.Fatalf("widen %+v", table.Rows)
	}

	dup := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetCellValue("Sheet1", "A1", "Customer")
		_ = f.SetCellValue("Sheet1", "B1", "Customer")
		_ = f.SetCellValue("Sheet1", "A2", "Ada")
		_ = f.SetCellValue("Sheet1", "B2", "North")
	})
	wb = openBytes(t, dup)
	if _, err := wb.Table("Sheet1"); err == nil || !strings.Contains(err.Error(), "duplicate header") {
		t.Fatalf("duplicate %v", err)
	}

	blank := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetCellValue("Sheet1", "A1", "Customer")
		_ = f.SetCellValue("Sheet1", "C1", "Amount")
		_ = f.SetCellValue("Sheet1", "A2", "Ada")
		_ = f.SetCellValue("Sheet1", "C2", 100)
	})
	wb = openBytes(t, blank)
	if _, err := wb.Table("Sheet1"); err == nil || !strings.Contains(err.Error(), "blank header") {
		t.Fatalf("blank header %v", err)
	}

	empty := workbookBytes(t, func(f *excelize.File) {})
	wb = openBytes(t, empty)
	if _, err := wb.Sheets(); err == nil || !strings.Contains(err.Error(), "no tabular") {
		t.Fatalf("empty %v", err)
	}

	uncached := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetCellValue("Sheet1", "A1", "Total")
		_ = f.SetCellFormula("Sheet1", "A2", "1+1")
	})
	wb = openBytes(t, uncached)
	if _, err := wb.Table("Sheet1"); err == nil || !strings.Contains(err.Error(), "no cached result") {
		t.Fatalf("uncached %v", err)
	}
}

func TestMergedHeaderAmbiguityAnd1904Date(t *testing.T) {
	ambiguous := workbookBytes(t, func(f *excelize.File) {
		_ = f.MergeCell("Sheet1", "A1", "B1")
		_ = f.SetCellValue("Sheet1", "A1", "Customer")
		_ = f.SetCellValue("Sheet1", "C1", "Amount")
		_ = f.SetCellValue("Sheet1", "A2", "Ada")
		_ = f.SetCellValue("Sheet1", "C2", 1)
	})
	wb := openBytes(t, ambiguous)
	if _, err := wb.Table("Sheet1"); err == nil || !strings.Contains(err.Error(), "merged cells") {
		t.Fatalf("merged %v", err)
	}

	use1904 := true
	body := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &use1904})
		_ = f.SetCellValue("Sheet1", "A1", "When")
		style, err := f.NewStyle(&excelize.Style{NumFmt: 14})
		if err != nil {
			t.Fatal(err)
		}
		_ = f.SetCellFloat("Sheet1", "A2", 0, -1, 64)
		_ = f.SetCellStyle("Sheet1", "A2", "A2", style)
	})
	wb = openBytes(t, body)
	if !wb.date1904 {
		t.Fatal("expected 1904 date system")
	}
	table, err := wb.Table("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	day, ok := table.Rows[0]["When"].(collectionfile.Date)
	if !ok || day.Time.Format("2006-01-02") != "1904-01-01" {
		t.Fatalf("1904 date %#v err=%v", table.Rows, err)
	}
}

// numericFormulaCache removes the string result type excelize writes onto
// every formula cell. Excel stores a numeric cached result without t="str".
func numericFormulaCache(t *testing.T, body []byte) []byte {
	t.Helper()
	in, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	out := zip.NewWriter(&buf)
	for _, file := range in.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(file.Name, "xl/worksheets/") {
			data = bytes.ReplaceAll(data, []byte(` t="str"`), nil)
		}
		w, err := out.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSelectSheetAndLimits(t *testing.T) {
	sheets := []string{"Customers", "Orders", "Sales 2026"}
	got, err := Select(sheets, "Customers", `SELECT "Customer" FROM "data"."Sales 2026"`)
	if err != nil || got != "Sales 2026" {
		t.Fatalf("sql select %q %v", got, err)
	}
	got, err = Select(sheets, "Orders", `SELECT * FROM "Orders"`)
	if err != nil || got != "Orders" {
		t.Fatalf("plain from %q %v", got, err)
	}
	if _, err := Select(sheets, "Missing", ""); err == nil || !strings.Contains(err.Error(), "Missing") {
		t.Fatalf("missing %v", err)
	}
	if _, err := Open(bytes.NewReader([]byte("not a workbook")), 13); err == nil || !strings.Contains(err.Error(), "not a valid") {
		t.Fatalf("invalid %v", err)
	}
	if _, err := Open(bytes.NewReader([]byte("PK")), MaxFileBytes+1); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("size %v", err)
	}
}

func TestDateAndNumberWidening(t *testing.T) {
	body := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetCellValue("Sheet1", "A1", "When")
		_ = f.SetCellValue("Sheet1", "B1", "Qty")
		_ = f.SetCellValue("Sheet1", "A2", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		_ = f.SetCellValue("Sheet1", "B2", 4)
		_ = f.SetCellValue("Sheet1", "A3", time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC))
		_ = f.SetCellValue("Sheet1", "B3", 1.5)
	})
	wb := openBytes(t, body)
	table, err := wb.Table("Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if table.Columns[0].Kind != collectionfile.KindDateTime {
		t.Fatalf("date widen %s", table.Columns[0].Kind)
	}
	if _, ok := table.Rows[0]["When"].(collectionfile.DateTime); !ok {
		t.Fatalf("widened date %#v", table.Rows[0]["When"])
	}
	if table.Columns[1].Kind != collectionfile.KindDecimal || table.Rows[0]["Qty"] != float64(4) || table.Rows[1]["Qty"] != 1.5 {
		t.Fatalf("number widen %+v", table.Rows)
	}
}
