package goalwork

import (
	"context"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

func TestSpatialTemporalAlignmentUsesBothOriginalSourcesAndActualSelectedRows(t *testing.T) {
	for _, candidateYear := range []string{"2025", "2024", ""} {
		e, s := nearestFixtureRows(t, "", []Row{{"id": "A", "lat": "0", "lon": "0", "year": "2025"}})
		old := e.deps.ScanCSV
		e.deps.ScanCSV = func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
			return old(ctx, s, i, func(r dataset.CSVScanRecord) error { r.Values["year"] = candidateYear; return visit(r) })
		}
		if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
			t.Fatal("nearest setup failed")
		}
		p := Composition{ID: "time", Purpose: "field-bound original periods", Base: "o3", Time: &TemporalAlignment{Window: &DateWindow{From: "2025-01-01", Through: "2025-12-31"}, Bindings: []TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}, {Observation: "o2", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}, Select: []string{"o3.anchor.id", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "facility", Field: "o3.anchor.id"}, {Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"calendar compatibility only; original field meanings remain unverified"}}
		for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
			if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
				t.Fatal(err)
			}
		}
		v := e.View()
		if candidateYear == "2025" {
			if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 2 || v.Evaluation.Temporal.Status != "checked" || v.Evaluation.Temporal.MeaningVerified {
				t.Fatalf("used wrong prefix date or lost actual pair periods: %+v", v.Gaps)
			}
		} else {
			if v.Artifact != nil || len(v.Executions) != 1 || len(v.Executions[0].Metrics) != 1 || v.Executions[0].Metrics[0].TemporalRejectedPairs != 2 {
				t.Fatalf("unknown/disjoint dates passed zero-join spatial output: %+v", v.Gaps)
			}
			if candidateYear == "" && v.Executions[0].Metrics[0].TemporalUnknownPairs != 2 {
				t.Fatal("unknown date not retained as unknown")
			}
		}
	}
}

func TestSpatialContainerCannotStandInForBothOriginalTimeBindings(t *testing.T) {
	e, s := nearestFixtureRows(t, "", []Row{{"id": "A", "lat": "0", "lon": "0", "year": "2025"}})
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "mixed-time", Purpose: "reject dates from different originals as one validity interval", Base: "o3", Time: &TemporalAlignment{Bindings: []TimeBinding{{Observation: "o3", FromField: "candidate.year", ThroughField: "anchor.year", Format: "year_v1", Meaning: "validity"}}}, Assumptions: []string{"two source dates are not two ends of one validity interval"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	if v := e.View(); v.Artifact != nil || len(v.Executions) != 1 || v.Executions[0].Status != "failed" {
		t.Fatal("spatial container hid a missing original time binding")
	}
}
