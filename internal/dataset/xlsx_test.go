package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func xlsxFixture(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, body := range members {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestInspectXLSXHandlesFallbackRelationshipMergedHeadersAndInlineStrings(t *testing.T) {
	body := xlsxFixture(t, map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="" sheetId="1" r:id="missing"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
			<row r="1"><c r="A1" t="inlineStr"><is><t>인구</t></is></c></row>
			<row r="2"><c r="A2" t="inlineStr"><is><r><t>기준</t></r><r><t>연월</t></r></is></c><c r="B2" t="inlineStr"><is><t>지역</t></is></c></row>
			<row r="3"><c r="A3"><v>202501</v></c><c r="B3" t="inlineStr"><is><t>제주</t></is></c></row>
		</sheetData><mergeCells><mergeCell ref="A1:B1"/></mergeCells></worksheet>`,
	})

	files, warnings, err := inspectXLSX(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || len(files) != 1 {
		t.Fatalf("files=%+v warnings=%+v", files, warnings)
	}
	if files[0].Name != "xl/worksheets/sheet1.xml" || strings.Join(files[0].Columns, ",") != "인구 / 기준연월,인구 / 지역" || files[0].SampleRows != 1 {
		t.Fatalf("observed merged worksheet = %+v", files[0])
	}
}

func TestInspectXLSXReportsMissingAndMalformedWorksheets(t *testing.T) {
	body := xlsxFixture(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="missing" sheetId="1" r:id="rId1"/><sheet name="broken" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/missing.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/broken.xml"/></Relationships>`,
		"xl/worksheets/broken.xml":   `<worksheet><sheetData>`,
	})

	files, warnings, err := inspectXLSX(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warnings, " ")
	if len(files) != 0 || !strings.Contains(joined, "파일을 찾지 못했습니다") || !strings.Contains(joined, "해석 실패") || !strings.Contains(joined, "스키마를 찾지 못했습니다") {
		t.Fatalf("files=%+v warnings=%+v", files, warnings)
	}
}

func TestInspectXLSXRejectsInvalidContainerAndMissingWorkbook(t *testing.T) {
	if _, _, err := inspectXLSX(bytes.NewReader([]byte("not a zip")), 9); err == nil || !strings.Contains(err.Error(), "ZIP 해석 실패") {
		t.Fatalf("invalid container error = %v", err)
	}

	body := xlsxFixture(t, map[string]string{"xl/worksheets/sheet1.xml": `<worksheet/>`})
	if _, _, err := inspectXLSX(bytes.NewReader(body), int64(len(body))); err == nil || !strings.Contains(err.Error(), "workbook 해석 실패") {
		t.Fatalf("missing workbook error = %v", err)
	}
}

func TestInspectorRecognizesXLSXFromDeclaredFormat(t *testing.T) {
	body := xlsxFixture(t, map[string]string{
		"xl/workbook.xml":          `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheets><sheet name="data" sheetId="1"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>지역</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>제주</t></is></c></row></sheetData></worksheet>`,
	})
	const downloadURL = "https://files.test/download"
	transport := fixtureTransport{gets: map[string]*fetch.Response{
		downloadURL: {Status: http.StatusOK, ContentType: "application/octet-stream", Body: body},
	}}

	observed, err := NewInspector(transport, "https://portal.test").Observe(context.Background(), Asset{
		Name: "download", Format: " xlsx ", Request: Request{Method: http.MethodGet, URL: downloadURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Files) != 1 || observed.Files[0].Format != "XLSX" {
		t.Fatalf("declared XLSX observation = %+v", observed)
	}
}

func TestInspectXLSXRejectsTooManyArchiveMembers(t *testing.T) {
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for index := 0; index <= maxArchiveEntries; index++ {
		entry, err := writer.Create("member-" + xlsxColumnName(index))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectXLSX(bytes.NewReader(body.Bytes()), int64(body.Len())); err == nil || !strings.Contains(err.Error(), "허용 개수") {
		t.Fatalf("archive member limit error = %v", err)
	}
}

func TestXLSXCoordinateRangeAndCellBoundaries(t *testing.T) {
	for input, want := range map[string]xlsxCoordinate{
		"A1":  {Column: 0, Row: 1},
		"z9":  {Column: 25, Row: 9},
		"AA2": {Column: 26, Row: 2},
	} {
		got, ok := parseXLSXCoordinate(input)
		if !ok || got != want {
			t.Errorf("parseXLSXCoordinate(%q) = %+v, %v; want %+v", input, got, ok, want)
		}
	}
	for _, input := range []string{"", "A", "1", "A0", "A-1", "A1x"} {
		if got, ok := parseXLSXCoordinate(input); ok {
			t.Errorf("parseXLSXCoordinate(%q) = %+v, true; want invalid", input, got)
		}
	}
	if _, _, ok := parseXLSXRange("B2:A1"); ok {
		t.Fatal("reverse XLSX range was accepted")
	}
	start, end, ok := parseXLSXRange("B2:C3")
	if !ok || start != (xlsxCoordinate{Column: 1, Row: 2}) || end != (xlsxCoordinate{Column: 2, Row: 3}) {
		t.Fatalf("range = %+v:%+v, %v", start, end, ok)
	}
	if got := xlsxCellValue(xlsxCell{Type: "s", Value: "99"}, map[int]string{0: "only"}); got != "" {
		t.Fatalf("out-of-range shared string = %q", got)
	}
	if got := xlsxColumnName(27); got != "AB" {
		t.Fatalf("column 27 = %q, want AB", got)
	}
}

func TestBoundXLSXMergeRangeToObservedWindow(t *testing.T) {
	start, end, ok := boundedXLSXMergeRange("A1:IV2147483647", maxXLSXRowsScanned)
	if !ok {
		t.Fatal("valid merge range was rejected")
	}
	if start != (xlsxCoordinate{Column: 0, Row: 1}) {
		t.Fatalf("start = %+v", start)
	}
	if end != (xlsxCoordinate{Column: maxXLSXColumns - 1, Row: maxXLSXRowsScanned}) {
		t.Fatalf("bounded end = %+v", end)
	}
	if _, _, ok := boundedXLSXMergeRange("A65:B1000000", maxXLSXRowsScanned); ok {
		t.Fatal("merge entirely beyond the observed row window was accepted")
	}
}

func TestDecodeXLSXWorksheetStoresOnlyObservedRowsAndColumns(t *testing.T) {
	var xmlBody strings.Builder
	xmlBody.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for row := 1; row <= maxXLSXRowsScanned+8; row++ {
		fmt.Fprintf(&xmlBody, `<row r="%d">`, row)
		for column := 0; column < maxXLSXColumns+32; column++ {
			fmt.Fprintf(&xmlBody, `<c r="%s%d"><v>%d</v></c>`, xlsxColumnName(column), row, column)
		}
		xmlBody.WriteString(`</row>`)
	}
	xmlBody.WriteString(`</sheetData><mergeCells><mergeCell ref="A1:IV2147483647"/></mergeCells></worksheet>`)

	body := xlsxFixture(t, map[string]string{"xl/worksheets/sheet1.xml": xmlBody.String()})
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	worksheet, err := decodeXLSXWorksheet(zr.File[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(worksheet.Rows) != maxXLSXRowsScanned {
		t.Fatalf("stored rows = %d, want %d", len(worksheet.Rows), maxXLSXRowsScanned)
	}
	for index, row := range worksheet.Rows {
		if len(row.Cells) != maxXLSXColumns {
			t.Fatalf("row %d stored cells = %d, want %d", index+1, len(row.Cells), maxXLSXColumns)
		}
	}
	if len(worksheet.Merges) != 1 || worksheet.Merges[0].Ref != "A1:IV2147483647" {
		t.Fatalf("merges = %+v", worksheet.Merges)
	}
}

func TestDecodeXLSXWorkbookAndRelationshipsRejectUnboundedSlices(t *testing.T) {
	var workbookXML strings.Builder
	workbookXML.WriteString(`<workbook><sheets>`)
	for index := 0; index <= maxXLSXWorksheets; index++ {
		fmt.Fprintf(&workbookXML, `<sheet name="sheet-%d" r:id="rId%d"/>`, index, index)
	}
	workbookXML.WriteString(`</sheets></workbook>`)
	body := xlsxFixture(t, map[string]string{"xl/workbook.xml": workbookXML.String()})
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeXLSXWorkbook(zr.File[0]); err == nil || !strings.Contains(err.Error(), "worksheet 개수") {
		t.Fatalf("workbook sheet limit error = %v", err)
	}

	var relationshipsXML strings.Builder
	relationshipsXML.WriteString(`<Relationships>`)
	for index := 0; index <= maxXLSXRelationships; index++ {
		fmt.Fprintf(&relationshipsXML, `<Relationship Id="rId%d" Type="worksheet" Target="worksheets/sheet%d.xml"/>`, index, index)
	}
	relationshipsXML.WriteString(`</Relationships>`)
	body = xlsxFixture(t, map[string]string{"xl/_rels/workbook.xml.rels": relationshipsXML.String()})
	zr, err = zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeXLSXRelationships(zr.File[0]); err == nil || !strings.Contains(err.Error(), "관계 개수") {
		t.Fatalf("workbook relationship limit error = %v", err)
	}
}

func TestDecodeXLSXSharedStringsRetainsOnlyReferencedBoundedValues(t *testing.T) {
	sharedXML := `<sst><si><t>discard me</t></si><si><r><t>제</t></r><r><t>주</t></r></si><si><t>also discard</t></si><si><t>지역</t></si></sst>`
	body := xlsxFixture(t, map[string]string{"xl/sharedStrings.xml": sharedXML})
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	values, err := decodeXLSXSharedStrings(zr.File[0], map[int]struct{}{1: {}, 3: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[1] != "제주" || values[3] != "지역" {
		t.Fatalf("selected shared strings = %+v", values)
	}

	tooLong := `<sst><si><t>` + strings.Repeat("x", maxXLSXCellTextBytes+1) + `</t></si></sst>`
	body = xlsxFixture(t, map[string]string{"xl/sharedStrings.xml": tooLong})
	zr, err = zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeXLSXSharedStrings(zr.File[0], map[int]struct{}{0: {}}); err == nil || !strings.Contains(err.Error(), "cell text") {
		t.Fatalf("shared string size error = %v", err)
	}
}

func TestInspectXLSXHandlesDuplicateWorksheetTargetsWithoutDuplicatingSharedStrings(t *testing.T) {
	body := xlsxFixture(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="relationships"><sheets><sheet name="first" r:id="rId1"/><sheet name="second" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/shared.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/shared.xml"/></Relationships>`,
		"xl/worksheets/shared.xml":   `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>1</v></c></row><row r="2"><c r="A2" t="s"><v>2</v></c></row></sheetData></worksheet>`,
		"xl/sharedStrings.xml":       `<sst><si><t>unused</t></si><si><t>지역</t></si><si><t>제주</t></si></sst>`,
	})
	files, warnings, err := inspectXLSX(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || len(files) != 2 {
		t.Fatalf("files=%+v warnings=%+v", files, warnings)
	}
	if files[0].Name != "first" || files[1].Name != "second" || files[0].Columns[0] != "지역 / 제주" || files[1].Columns[0] != "지역 / 제주" {
		t.Fatalf("duplicate target observations = %+v", files)
	}
}
