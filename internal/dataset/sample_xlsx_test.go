package dataset

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func sampleWorkbook(t *testing.T, sheet string, extra map[string]string) (*Inspector, Asset) {
	t.Helper()
	members := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="구·군별" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet,
		"xl/sharedStrings.xml":       `<sst><si><t xml:space="preserve"> 부평구 </t></si></sst>`,
	}
	for name, body := range extra {
		members[name] = body
	}
	body := xlsxFixture(t, members)
	u := "https://data.test/schools.xlsx"
	return NewInspector(fixtureTransport{gets: map[string]*fetch.Response{u: {Status: 200, Body: body}}}, ""), Asset{Name: "schools.xlsx", Format: "XLSX", Request: Request{Method: http.MethodGet, URL: u}}
}

func TestSampleXLSXRetainsExactRectangleStringsAndFormulaCache(t *testing.T) {
	i, a := sampleWorkbook(t, `<worksheet><sheetData>
<row r="14"><c r="A14" t="inlineStr"><is><t>서구</t></is></c><c r="AG14"><v>191</v></c></row>
<row r="27"><c r="A27" t="s"><v>0</v></c><c r="B27" t="inlineStr"><is><t xml:space="preserve"> 001 </t></is></c><c r="AG27"><f>SUM(E27:F27)</f><v>148</v></c><c r="AK27"><v>44836</v></c></row>
<row r="28"><c r="A28" t="inlineStr"><is><t>미상</t></is></c><c r="B28" t="b"><v>0</v></c><c r="AG28"><f>SUM(E28:F28)</f></c></row>
<row r="38"><c r="AG38"><v>960</v></c></row>
</sheetData></worksheet>`, nil)
	s, err := i.SampleXLSX(context.Background(), a, XLSXSelection{Sheet: "구·군별", Range: "A27:AK28"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 2 || s.Rows[0]["A"] != " 부평구 " || s.Rows[0]["B"] != " 001 " || s.Rows[0]["AG"] != "148" || s.Rows[0]["AK"] != "44836" || s.Rows[1]["B"] != false || s.Rows[1]["AG"] != nil {
		t.Fatalf("source values changed: %+v", s.Rows)
	}
	if s.Table == nil || s.Table.Sheet != "구·군별" || s.Table.Range != "A27:AK28" || !reflect.DeepEqual(s.Table.RowNumbers, []int{27, 28}) || !reflect.DeepEqual(s.Table.FormulaCells, []string{"AG27", "AG28"}) || s.Table.FormulasWithoutCache != 1 || s.SHA256 == "" || s.Prefix {
		t.Fatalf("provenance missing: %+v", s)
	}
}

func TestSampleXLSXRejectsAmbiguousOrMalformedSelectedSource(t *testing.T) {
	for name, sheet := range map[string]string{
		"duplicate row":         `<worksheet><sheetData><row r="27"/><row r="27"/></sheetData></worksheet>`,
		"duplicate cell":        `<worksheet><sheetData><row r="27"><c r="A27"><v>1</v></c><c r="A27"><v>2</v></c></row></sheetData></worksheet>`,
		"wrong row reference":   `<worksheet><sheetData><row r="27"><c r="A28"><v>1</v></c></row></sheetData></worksheet>`,
		"missing shared string": `<worksheet><sheetData><row r="27"><c r="A27" t="s"><v>10</v></c></row></sheetData></worksheet>`,
		"cell error":            `<worksheet><sheetData><row r="27"><c r="A27" t="e"><v>#REF!</v></c></row></sheetData></worksheet>`,
		"malformed tail":        `<worksheet><sheetData><row r="27"><c r="A27"><v>1</v></c></row></sheetData><broken>`,
	} {
		t.Run(name, func(t *testing.T) {
			i, a := sampleWorkbook(t, sheet, nil)
			if _, err := i.SampleXLSX(context.Background(), a, XLSXSelection{Sheet: "구·군별", Range: "A27:AK28"}); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestSampleXLSXRequiresExactSheetInternalRelationshipAndBoundedRange(t *testing.T) {
	sheet := `<worksheet><sheetData><row r="27"><c r="A27"><v>1</v></c></row></sheetData></worksheet>`
	for _, selection := range []XLSXSelection{{Sheet: "unknown", Range: "A27:B28"}, {Sheet: "구·군별", Range: "A1:XFD1048576"}, {Sheet: "구·군별", Range: "B28:A27"}, {Sheet: "구·군별", Range: "a27:b28"}, {Sheet: "", Range: "A27:B28"}} {
		i, a := sampleWorkbook(t, sheet, nil)
		if _, err := i.SampleXLSX(context.Background(), a, selection); err == nil {
			t.Fatalf("invalid selection accepted: %+v", selection)
		}
	}
	for _, target := range []string{"https://evil.test/sheet.xml", "../../sheet.xml"} {
		i, a := sampleWorkbook(t, sheet, map[string]string{"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="` + target + `" TargetMode="External"/></Relationships>`})
		if _, err := i.SampleXLSX(context.Background(), a, XLSXSelection{Sheet: "구·군별", Range: "A27:B28"}); err == nil {
			t.Fatal("external sheet target accepted")
		}
	}
	i, a := sampleWorkbook(t, sheet, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := i.SampleXLSX(ctx, a, XLSXSelection{Sheet: "구·군별", Range: "A27:B28"}); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatal("cancelled read proceeded")
	}
}

func TestSampleXLSXRichTextExcludesPronunciationHintsAndRetainsBlankPositions(t *testing.T) {
	i, a := sampleWorkbook(t, `<worksheet><sheetData><row r="28"><c r="A28" t="s"><v>0</v></c><c r="B28" t="inlineStr"><is><r><t> 東</t></r><r><t>京 </t></r><rPh sb="0" eb="2"><t>とうきょう</t></rPh></is></c></row></sheetData></worksheet>`, map[string]string{"xl/sharedStrings.xml": `<sst><si><r><t> 東</t></r><r><t>京 </t></r><rPh sb="0" eb="2"><t>とうきょう</t></rPh></si></sst>`})
	s, err := i.SampleXLSX(context.Background(), a, XLSXSelection{Sheet: "구·군별", Range: "A27:B29"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 3 || s.Rows[0]["A"] != nil || s.Rows[2]["B"] != nil || s.Rows[1]["A"] != " 東京 " || s.Rows[1]["B"] != " 東京 " {
		t.Fatalf("source text/position changed: %+v", s.Rows)
	}
}

func TestSampleXLSXRejectsAmbiguousMembersAndSheetContracts(t *testing.T) {
	sheet := `<worksheet><sheetData><row r="27"><c r="A27"><v>1</v></c></row></sheetData></worksheet>`
	for name, extra := range map[string]map[string]string{
		"normalized member collision":   {"xl/worksheets/../worksheets/sheet1.xml": sheet},
		"traversal member":              {"../outside.xml": "<x/>"},
		"duplicate sheet":               {"xl/workbook.xml": `<workbook xmlns:r="r"><sheets><sheet name="구·군별" r:id="rId1"/><sheet name="구·군별" r:id="rId2"/></sheets></workbook>`},
		"duplicate relationship":        {"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/></Relationships>`},
		"external mode relative target": {"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml" TargetMode="External"/></Relationships>`},
	} {
		t.Run(name, func(t *testing.T) {
			i, a := sampleWorkbook(t, sheet, extra)
			if _, err := i.SampleXLSX(context.Background(), a, XLSXSelection{Sheet: "구·군별", Range: "A27:B28"}); err == nil {
				t.Fatal("ambiguous contract accepted")
			}
		})
	}
}
