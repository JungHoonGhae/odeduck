package goalwork_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestNearestPreservesOriginalScalarStatesInEvidenceAndResult(t *testing.T) {
	const contractHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	sourceHash := strings.Repeat("a", 64)
	e, err := goalwork.Start("compare observed locations", goalwork.Policy{EvidenceRecipient: "claude"}, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk, Assets: []string{"source.csv"}}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			if s.PK == "111" {
				return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"id": "001", "lat": "0", "lon": "0", "empty": "", "null": nil, "zero": json.Number("0"), "flag": false}}}, nil
			}
			return goalwork.Acquired{Delivery: "FILE", Rows: []goalwork.Row{{"id": "far", "lat": "0", "lon": "90", "empty": ""}}, ContentSHA256: sourceHash, ContractSHA256: contractHash, CSV: &dataset.CSVProvenance{DataRecords: []int{1}, StartLines: []int{2}}}, nil
		},
		ScanCSV: func(_ context.Context, _ goalwork.SampleRequest, _ goalwork.Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
			// The selected row is beyond the retained prefix, but its blank cell
			// is still a source string, never a fabricated null.
			for n := 1; n <= 1002; n++ {
				values := map[string]string{"id": "far", "lat": "0", "lon": "90", "empty": ""}
				if n == 1002 {
					values["id"], values["lon"] = "nearest", "1"
				}
				if err := visit(dataset.CSVScanRecord{DataRecord: n, StartLine: n + 1, Values: values}); err != nil {
					return dataset.CSVScanReport{}, "", err
				}
			}
			return dataset.CSVScanReport{SHA256: sourceHash, Columns: []string{"id", "lat", "lon", "empty"}, ScannedRows: 1002, MatchedRows: 1002, Exhausted: true}, contractHash, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "conditional location comparison", Region: "fixture", Period: "historic", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "anchor", Description: "anchor"}, {ID: "candidate", Description: "candidate"}}, Outputs: []goalwork.OutputRequirement{{ID: "distance", Role: "candidate", Description: "conditional distance", Type: "number"}}}
	advance := func(d goalwork.Decision) goalwork.View {
		t.Helper()
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("advance %s: %v %v", d.Action, v.Gaps, err)
		}
		return v
	}
	for _, d := range []goalwork.Decision{
		{Action: "define", Contract: &c},
		{Action: "search", Query: "111", Role: "anchor"}, {Action: "inspect", PK: "111"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "111", Delivery: "api"}},
		{Action: "search", Query: "222", Role: "candidate"}, {Action: "inspect", PK: "222"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv"}},
		{Action: "sample", Sample: &goalwork.SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv", ScanCSV: true, Nearest: &goalwork.NearestSelection{Method: "spherical_nearest_records_v1", Anchor: "o1", Candidate: "o2", AnchorLatitude: "lat", AnchorLongitude: "lon", Latitude: "lat", Longitude: "lon", K: 1}}},
	} {
		advance(d)
	}
	fields := []string{"candidate.empty", "anchor.empty", "anchor.null", "anchor.zero", "anchor.flag"}
	v := advance(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o3", RowsSHA256: e.View().Observations[2].RowsSHA256, Rows: []int{1}, Fields: fields}})
	r := v.Evidence[0].Records[0]
	assertScalars := func(values map[string]any, prefix string) {
		t.Helper()
		if values[prefix+"candidate.empty"] != "" || values[prefix+"anchor.empty"] != "" || values[prefix+"anchor.zero"] != json.Number("0") || values[prefix+"anchor.flag"] != false {
			t.Fatalf("source scalar states changed: %+v", values)
		}
		if value, present := values[prefix+"anchor.null"]; !present || value != nil {
			t.Fatal("source null became missing")
		}
	}
	assertScalars(r.Values, "")
	if origin := r.Origins["candidate.empty"]; origin.Kind != "csv_data_record" || origin.Observation != "o2" || origin.Ordinal != 1002 || origin.Field != "empty" {
		t.Fatalf("source address changed: %+v", origin)
	}
	p := goalwork.Composition{ID: "nearest", Purpose: "conditional comparison", Base: "o3", Roles: []goalwork.RoleBinding{{Role: "anchor", Observation: "o1"}, {Role: "candidate", Observation: "o3"}}, Outputs: []goalwork.OutputBinding{{Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"coordinate datum and identity unverified"}}
	for _, f := range append(fields, "distance_m") {
		p.Select = append(p.Select, "o3."+f)
	}
	advance(goalwork.Decision{Action: "compose", Composition: &p})
	v = advance(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 1 {
		t.Fatalf("missing conditional result: %s", v.Status)
	}
	assertScalars(v.Artifact.Rows[0], "o3.")
}
