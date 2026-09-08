package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLayoutXLSXDiscoversSheetsAndStructureWithoutCellValues(t *testing.T) {
	i, a := sampleWorkbook(t, `<worksheet><dimension ref="A1:ZZ9999"/><sheetData><row r="22"><c r="A22" t="inlineStr"><is><t>PRIVATE_HEADER_OR_DATA</t></is></c></row><row r="24"><c r="A24" t="s"><v>0</v></c><c r="C24" t="inlineStr"><is><t>PRIVATE_VALUE</t></is></c></row><row r="27"><c r="A27" t="inlineStr"><is><t>SECRET</t></is></c><c r="AK27"><f>PRIVATE_FORMULA</f><v>44836</v></c></row></sheetData><mergeCells count="1"><mergeCell ref="A24:B26"/></mergeCells></worksheet>`, nil)
	list, err := i.LayoutXLSX(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Sheets) != 1 || list.Sheets[0].Name != "구·군별" || list.Sheet != nil || list.SHA256 == "" {
		t.Fatalf("sheet discovery missing: %+v", list)
	}
	l, err := i.LayoutXLSX(context.Background(), a, list.Sheets[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if l.Sheet == nil || l.Sheet.Extent != "A22:AK27" || l.Sheet.StoredRows != 3 || l.Sheet.StoredCells != 5 || len(l.Sheet.Rows) != 3 || l.Sheet.Rows[2].Number != 27 || l.Sheet.Rows[2].Cells != 2 || len(l.Sheet.Merges) != 1 || l.Sheet.Merges[0] != "A24:B26" || l.SHA256 != list.SHA256 {
		t.Fatalf("structural evidence wrong: %+v", l)
	}
	b, _ := json.Marshal(l)
	for _, v := range []string{"PRIVATE", "SECRET", "44836", "부평구", "ZZ9999"} {
		if strings.Contains(string(b), v) {
			t.Fatalf("layout leaked values or trusted stale dimension: %s", b)
		}
	}
}

func TestLayoutXLSXRejectsMalformedStructureAndUnknownSheet(t *testing.T) {
	for _, sheet := range []string{
		`<worksheet><sheetData><row r="2"/><row r="2"/></sheetData></worksheet>`,
		`<worksheet><sheetData><row r="2"><c r="A3"/></row></sheetData></worksheet>`,
		`<worksheet><sheetData><row r="2"><c r="A2"/><c r="A2"/></row></sheetData></worksheet>`,
		`<worksheet><sheetData/><mergeCells><mergeCell ref="a1:b2"/></mergeCells></worksheet>`,
		`<worksheet><sheetData/></worksheet><worksheet/>`,
		`<worksheet><sheetData/><broken>`,
	} {
		i, a := sampleWorkbook(t, sheet, nil)
		if _, err := i.LayoutXLSX(context.Background(), a, "구·군별"); err == nil {
			t.Fatal("malformed structure accepted")
		}
	}
	i, a := sampleWorkbook(t, `<worksheet><sheetData/></worksheet>`, nil)
	if _, err := i.LayoutXLSX(context.Background(), a, "unknown"); err == nil {
		t.Fatal("unknown sheet accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := i.LayoutXLSX(ctx, a, ""); err == nil {
		t.Fatal("cancelled work proceeded")
	}
}

func TestLayoutXLSXTruncatesOnlyRetainedShapesNotScanCounts(t *testing.T) {
	var xml strings.Builder
	xml.WriteString(`<worksheet><sheetData>`)
	for n := 1; n <= 129; n++ {
		fmt.Fprintf(&xml, `<row r="%d"><c r="A%d" t="inlineStr"><is><t>PRIVATE</t></is></c></row>`, n, n)
	}
	xml.WriteString(`</sheetData><mergeCells>`)
	for n := 1; n <= 129; n++ {
		fmt.Fprintf(&xml, `<mergeCell ref="B%d:C%d"/>`, n, n)
	}
	xml.WriteString(`</mergeCells></worksheet>`)
	i, a := sampleWorkbook(t, xml.String(), nil)
	l, err := i.LayoutXLSX(context.Background(), a, "구·군별")
	if err != nil {
		t.Fatal(err)
	}
	if l.Sheet.Extent != "A1:A129" || l.Sheet.StoredRows != 129 || l.Sheet.StoredCells != 129 || len(l.Sheet.Rows) != 128 || !l.Sheet.RowsTruncated || len(l.Sheet.Merges) != 128 || !l.Sheet.MergesTruncated {
		t.Fatalf("partial layout misrepresented complete scan: %+v", l.Sheet)
	}
}
