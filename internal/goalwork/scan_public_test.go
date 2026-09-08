package goalwork

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

// The streaming reader must visit ALL matching records, not merely the first
// 1000. Independent G2 rows across the file verify ordinal/value preservation.
func TestLiveFullCSVScanMatchesIndependentTransitReference(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_CSV_SCAN") != "1" {
		t.Skip("opt-in public full CSV scan")
	}
	b, err := os.ReadFile("testdata/goalbench-v1/mobility-comparative-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Sources []struct {
			URL       string            `json:"url"`
			SHA       string            `json:"contentSha256"`
			Total     int               `json:"totalCSVRecords"`
			Matched   int               `json:"incheonSourceRecords"`
			Selection map[string]string `json:"selection"`
			Records   []struct {
				Ordinal int               `json:"csvRecord"`
				Values  map[string]string `json:"values"`
			} `json:"records"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(b, &ref); err != nil {
		t.Fatal(err)
	}
	source := ref.Sources[1]
	wanted := map[int]map[string]string{}
	for _, r := range source.Records {
		wanted[r.Ordinal-1] = r.Values
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	i := dataset.NewInspector(fetch.New(), "")
	found := 0
	report, err := i.ScanCSV(ctx, dataset.Asset{Name: "stops.csv", Format: "CSV", Request: dataset.Request{Method: http.MethodGet, URL: source.URL}}, source.Selection, func(r dataset.CSVScanRecord) error {
		if expected, ok := wanted[r.DataRecord]; ok {
			found++
			if !reflect.DeepEqual(r.Values, expected) {
				t.Errorf("data record %d (reference CSV record %d) differs: got %+v expected %+v", r.DataRecord, r.DataRecord+1, r.Values, expected)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Exhausted || report.SHA256 != source.SHA || report.ScannedRows != source.Total || report.MatchedRows != source.Matched || found != len(wanted) || report.Bytes <= 8<<20 {
		t.Fatalf("full-source scan differs from independent oracle: %+v found=%d expected=%d", report, found, len(wanted))
	}
	t.Logf("fully scanned %d bytes, %d data records; %d exact Incheon matches; %d independent stop records matched; SHA256 %s", report.Bytes, report.ScannedRows, report.MatchedRows, found, report.SHA256)
}
