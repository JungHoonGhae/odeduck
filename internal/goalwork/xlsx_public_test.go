package goalwork

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

// Scripted source-reader replay, NOT unseeded discovery or goal completion.
// The independent reference retains original OOXML cell values and row numbers.
func TestLiveXLSXSchoolReaderMatchesIndependentCitywideReference(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_XLSX") != "1" {
		t.Skip("opt-in public XLSX source replay")
	}
	b, err := os.ReadFile("testdata/goalbench-v1/education-citywide-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Sources []struct {
			URL     string `json:"url"`
			SHA     string `json:"contentSha256"`
			Records []struct {
				Sheet string            `json:"sheet"`
				Row   int               `json:"row"`
				Cells map[string]string `json:"cells"`
			} `json:"records"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(b, &reference); err != nil {
		t.Fatal(err)
	}
	source := reference.Sources[1]
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	i := dataset.NewInspector(fetch.New(), "")
	asset := dataset.Asset{Name: "schools.xlsx", Format: "XLSX", Request: dataset.Request{Method: http.MethodGet, URL: source.URL}}
	list, err := i.LayoutXLSX(ctx, asset, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sheet := range list.Sheets {
		found = found || sheet.Name == source.Records[0].Sheet
	}
	if !found {
		t.Fatal("reference worksheet was not discoverable")
	}
	layout, err := i.LayoutXLSX(ctx, asset, source.Records[0].Sheet)
	if err != nil {
		t.Fatal(err)
	}
	if layout.SHA256 != source.SHA || list.SHA256 != source.SHA || layout.Sheet == nil || layout.Sheet.StoredRows < 11 {
		t.Fatal("layout source revision or structure missing")
	}
	for _, record := range source.Records {
		found := false
		for _, shape := range layout.Sheet.Rows {
			found = found || shape.Number == record.Row
		}
		if !found {
			t.Fatalf("reference row %d is not discoverable", record.Row)
		}
	}
	t.Logf("discovered %d sheets; %s extent %s, %d stored rows, %d stored cells; merges truncated=%v", len(list.Sheets), layout.Sheet.Name, layout.Sheet.Extent, layout.Sheet.StoredRows, layout.Sheet.StoredCells, layout.Sheet.MergesTruncated)
	s, err := i.SampleXLSX(ctx, asset, dataset.XLSXSelection{Sheet: "구·군별", Range: "A27:AM37"})
	if err != nil {
		t.Fatal(err)
	}
	if s.SHA256 != source.SHA {
		t.Fatalf("source changed: got %s, expected %s; do not rewrite oracle", s.SHA256, source.SHA)
	}
	if len(s.Rows) != len(source.Records) || s.Table == nil || len(s.Table.FormulaCells) == 0 {
		t.Fatalf("missing rows/formula provenance: %+v", s.Table)
	}
	for n, record := range source.Records {
		if s.Table.RowNumbers[n] != record.Row || s.Table.Sheet != record.Sheet {
			t.Fatal("source position changed")
		}
		for col, want := range record.Cells {
			if got := s.Rows[n][col]; got != want {
				t.Fatalf("%s!%s%d: got %#v, want %#v", record.Sheet, col, record.Row, got, want)
			}
		}
		for col, got := range s.Rows[n] {
			if _, exists := record.Cells[col]; !exists && got != nil && got != "" {
				t.Fatalf("unexpected source cell %s%d: %#v", col, record.Row, got)
			}
		}
	}
	t.Logf("matched %d original worksheet rows; %d cached formula cells; SHA256 %s", len(s.Rows), len(s.Table.FormulaCells), s.SHA256)
}
