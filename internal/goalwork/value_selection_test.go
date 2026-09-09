package goalwork_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestValueSetSelectionCannotBeReorderedIntoAnotherAcquisition(t *testing.T) {
	request := goalwork.SampleRequest{PK: "records", Delivery: "file", ScanCSV: true, WhereIn: map[string][]string{"year": {"2025", "2024"}}}
	e := sourceReviewEngine(t, goalwork.Policy{}, nil, request)
	if request.WhereIn["year"][0] != "2025" || e.View().SampleAttempts[0].Request.WhereIn["year"][0] != "2024" {
		t.Fatal("set canonicalization mutated the caller or was not retained")
	}
	request.WhereIn["year"] = []string{"2024", "2025"}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &request})
	if !errors.Is(err, goalwork.ErrActionAlreadyAttempted) || len(v.SampleAttempts) != 1 {
		t.Fatal("reordered value set bought another acquisition")
	}
}

func TestValueSetSelectionFieldsMustReachBothReviewKinds(t *testing.T) {
	for _, analyses := range []bool{false, true} {
		request := goalwork.SampleRequest{PK: "records", Delivery: "file", ScanCSV: true, WhereIn: map[string][]string{"year": {"2024", "2025"}}}
		e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: analyses}, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("selection semantics were approved without any selected value evidence")
			return goalwork.ReviewAssessment{}, nil
		}, request)
		executeResult(t, e, resultRecipe())
		v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, Rows: []int{1, 2}, Fields: []string{"record"}}})
		if err != nil || len(v.Gaps) != 0 {
			t.Fatal("evidence setup failed")
		}
		v = advanceSourceReview(t, e)
		if len(v.Reviews) != 0 || len(v.Gaps) != 1 || !v.Evaluation.NeedsSemanticReview {
			t.Fatal("missing value-set evidence did not stop review")
		}
	}
}

func TestValueSetSelectionRejectsUnsupportedReadersAndCredentialFields(t *testing.T) {
	for _, mode := range []string{"bounded", "api", "standard", "zip", "xlsx", "reduction", "credential"} {
		t.Run(mode, func(t *testing.T) {
			e := sourceReviewEngine(t, goalwork.Policy{}, nil)
			r := goalwork.SampleRequest{PK: "records", Delivery: "file", ScanCSV: true, WhereIn: map[string][]string{"year": {"2024", "2025"}}}
			switch mode {
			case "bounded":
				r.ScanCSV = false
			case "api", "standard":
				r.Delivery = mode
			case "zip":
				r.Member = "source.csv"
			case "xlsx":
				r.XLSX = &dataset.XLSXSelection{Sheet: "Sheet1", Range: "A1:B2"}
			case "reduction":
				r.ScanCSV = false
				r.Reduce = &goalwork.SourceReduction{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256}
			case "credential":
				r.WhereIn = map[string][]string{"serviceKey": {"dummy"}}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &r})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 1 || len(v.SampleAttempts) != 1 {
				t.Fatalf("invalid selector reached acquisition: %+v %v", v.Gaps, err)
			}
		})
	}
}
