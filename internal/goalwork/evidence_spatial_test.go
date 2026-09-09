package goalwork_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSelectedEvidenceSpatialValuesKeepOriginalParentsAndComputedPair(t *testing.T) {
	hash := strings.Repeat("a", 64)
	e, err := goalwork.Start("fixture comparison", goalwork.Policy{EvidenceRecipient: "claude"}, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			row := goalwork.Row{"id": "ANCHOR_VALUE", "lat": "0", "lon": "0"}
			if s.PK == "candidate" {
				row = goalwork.Row{"id": "UNSELECTED_PREFIX", "lat": "0", "lon": "90"}
			}
			return goalwork.Acquired{Delivery: "FILE", Rows: []goalwork.Row{row}, ContentSHA256: hash, ContractSHA256: hash, CSV: &dataset.CSVProvenance{DataRecords: []int{1}, StartLines: []int{2}}}, nil
		},
		ScanCSV: func(_ context.Context, _ goalwork.SampleRequest, _ goalwork.Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
			for n := 1; n <= 1002; n++ {
				row := map[string]string{"id": "UNSELECTED_PREFIX", "lat": "0", "lon": "90"}
				if n == 1002 {
					row = map[string]string{"id": "BEYOND_PREFIX", "lat": "0", "lon": "1"}
				}
				if err := visit(dataset.CSVScanRecord{DataRecord: n, StartLine: n + 1, Values: row}); err != nil {
					return dataset.CSVScanReport{}, "", err
				}
			}
			return dataset.CSVScanReport{SHA256: hash, Columns: []string{"id", "lat", "lon"}, Bytes: 40000, ScannedRows: 1002, MatchedRows: 1002, Exhausted: true}, hash, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := resultContract()
	actions := []goalwork.Decision{{Action: "define", Contract: &c}}
	for _, pk := range []string{"anchor", "candidate"} {
		actions = append(actions, goalwork.Decision{Action: "search", Query: pk, Role: "records"}, goalwork.Decision{Action: "inspect", PK: pk}, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "file", Asset: "source.csv", ScanCSV: pk == "candidate"}})
	}
	actions = append(actions, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "candidate", Delivery: "file", Asset: "source.csv", ScanCSV: true, Nearest: &goalwork.NearestSelection{Method: "spherical_nearest_records_v1", Anchor: "o1", Candidate: "o2", AnchorLatitude: "lat", AnchorLongitude: "lon", Latitude: "lat", Longitude: "lon", K: 1}}})
	for _, d := range actions {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("setup: %+v %v", v.Gaps, err)
		}
	}
	v := readEvidence(t, e, map[string]any{"observation": "o3", "rowsSha256": e.View().Observations[2].RowsSHA256, "rows": []int{1}, "fields": []string{"anchor.id", "candidate.id", "distance_m", "rank"}})
	if len(v.Gaps) > 0 {
		t.Fatal(v.Gaps)
	}
	r := v.Evidence[0].Records[0]
	if r.Values["anchor.id"] != "ANCHOR_VALUE" || r.Values["candidate.id"] != "BEYOND_PREFIX" {
		t.Fatal("spatial values replaced by prefix")
	}
	for _, tc := range []struct {
		field, source, kind string
		ordinal             int
	}{{"anchor.id", "o1", "csv_data_record", 1}, {"candidate.id", "o2", "csv_data_record", 1002}, {"distance_m", "o3", "computed_pair", 1}, {"rank", "o3", "computed_pair", 1}} {
		a := r.Origins[tc.field]
		if a.Observation != tc.source || a.Kind != tc.kind || a.Ordinal != tc.ordinal {
			t.Fatalf("wrong origin: %+v", a)
		}
	}
}
