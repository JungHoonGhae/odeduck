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

func TestReviewUsesSelectedSameFileContextWithoutJoiningHeaderRows(t *testing.T) {
	calls := 0
	e := contextReviewEngine(t, "same file", func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		if calls == 1 {
			if in.Analysis != nil {
				t.Fatal("unread header was included")
			}
			a := supportedSourceReview(in)
			a.Outputs[0].Output = "n"
			a.GoalFit.Verdict = "insufficient"
			return a, nil
		}
		if in.Analysis == nil || len(in.Analysis.SourceContext) != 1 || len(in.Artifact.Sources) != 1 || len(in.Artifact.Rows) != 1 || len(in.Analysis.Sources) != 1 {
			t.Fatal("header missing or treated as contributing data")
		}
		c := in.Analysis.SourceContext[0]
		if len(c.Targets) != 1 || c.Targets[0] != "o1" || c.Source.ID != "o2" || c.Request.XLSX.Range != "A1:C1" || c.Evidence.Records[0].Values["A"] != "Snapshot 2024-04-01; geography effective 2024-07-01" {
			t.Fatal("context lost source request, selected values or target revision")
		}
		b, _ := json.Marshal(in)
		if len(c.Source.Columns) != 1 || len(c.Source.ColumnProfiles) != 1 || len(c.Source.ColumnTypes) != 1 {
			t.Fatal("context metadata was not projected to selected fields")
		}
		if strings.Contains(string(b), "UNSELECTED_PRIVATE") || strings.Contains(string(b), "OTHER_REVISION") {
			t.Fatal("context widened disclosure")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.PacketIDs = []string{in.Evidence.ID, c.Evidence.ID}
		return a, nil
	})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if v.Status != "review_required" || len(v.Reviews) != 1 {
		t.Fatal("fixture pre-context review did not require missing evidence")
	}
	readContext(t, e)
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if calls != 2 || v.Status != "output_ready" || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod || v.Reviews[0].EvidenceSHA256 == v.Reviews[1].EvidenceSHA256 {
		t.Fatalf("new context did not enter bounded review: %+v", v.Gaps)
	}
}

func contextReviewEngine(t *testing.T, variant string, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	policy := goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: variant != "source only"}
	e, err := goalwork.Start("Report the source counts and distinguish census date from geography date", policy, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			a := goalwork.Acquired{Rows: []goalwork.Row{{"A": "12"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64)}
			if s.Member != "" || s.XLSX.Range == "A1:C1" {
				a.Rows = []goalwork.Row{{"A": "Snapshot 2024-04-01; geography effective 2024-07-01", "B": "UNSELECTED_PRIVATE", "C": "OTHER_REVISION"}}
				if variant == "content" {
					a.ContentSHA256 = strings.Repeat("c", 64)
				}
				if variant == "contract" {
					a.ContractSHA256 = strings.Repeat("d", 64)
				}
				if variant == "missing content" {
					a.ContentSHA256 = ""
				}
				if variant == "missing contract" {
					a.ContractSHA256 = ""
				}
				if variant == "delivery" {
					a.Delivery = "REST"
				}
			}
			if variant == "invalid hash" {
				a.ContentSHA256 = strings.Repeat("x", 64)
			}
			return a, nil
		}, Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "source counts and dates", Region: "observed source", Period: "declared source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "published count"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	for i, selection := range []string{"A2:A2", "A1:C1"} {
		pk, asset := "source", "table.xlsx"
		if i == 1 && variant == "pk" {
			pk = "other"
		}
		if i == 1 && variant == "asset" {
			asset = "other.xlsx"
		}
		if i == 0 || pk == "other" {
			advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: pk, Role: "r"})
			advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: pk})
		}
		s := goalwork.SampleRequest{PK: pk, Delivery: "file", Asset: asset, XLSX: &dataset.XLSXSelection{Sheet: "table", Range: selection}}
		if i == 1 && variant == "member" {
			s.Member = "other.csv"
			s.XLSX = nil
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	}
	p := goalwork.Composition{ID: "report", Base: "o1", Purpose: "report actual counts", Select: []string{"o1.A"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "n", Field: "o1.A"}}, Assumptions: []string{"source report, not present-day capacity"}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	o := e.View().Observations[0]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"A"}}})
	return e
}

func readContext(t *testing.T, e *goalwork.Engine) {
	t.Helper()
	o := e.View().Observations[1]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"A"}}})
}

func TestReviewContextRequiresMatchingFileRevisionAndAnalysisAuthority(t *testing.T) {
	for _, variant := range []string{"content", "contract", "missing content", "missing contract", "invalid hash", "pk", "asset", "member", "delivery", "source only"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			e := contextReviewEngine(t, variant, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				calls++
				b, _ := json.Marshal(in)
				if in.Analysis != nil || strings.Contains(string(b), "geography effective") {
					t.Fatal("unassociated or unauthorized context reached reviewer")
				}
				a := supportedSourceReview(in)
				a.Outputs[0].Output = "n"
				a.GoalFit.Verdict = "insufficient"
				return a, nil
			})
			readContext(t, e)
			v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if calls != 1 || v.Status != "review_required" {
				t.Fatal("unresolved source context was approved")
			}
		})
	}
}

func TestReviewContextCannotBeMutatedOrRepackedForAnotherReview(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "repacked", true: "mutated"}[mutate], func(t *testing.T) {
			calls := 0
			e := contextReviewEngine(t, "same file", func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				calls++
				a := supportedAnalysisReview(in)
				a.GoalFit.Verdict = "insufficient"
				if mutate {
					in.Analysis.SourceContext[0].Evidence.Records[0].Values["A"] = "forged context"
				}
				return a, nil
			})
			readContext(t, e)
			if !mutate {
				o := e.View().Observations[1]
				advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"C"}}})
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if err != nil || calls != 1 || v.Status == "output_ready" || v.Evidence[1].Records[0].Values["A"] == "forged context" {
				t.Fatal("context mutation affected source or approval")
			}
			if mutate {
				if len(v.Gaps) != 1 || v.Reviews[0].Status != "failed" {
					t.Fatal("review input mutation was not rejected")
				}
				return
			}
			// A genuinely new packet containing the same context cells cannot
			// fund a second review of this execution.
			o := v.Observations[1]
			advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"C", "A"}}})
			v, err = e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if err != nil || calls != 1 || len(v.Reviews) != 1 || len(v.Gaps) != 1 || v.Status == "output_ready" {
				t.Fatal("repacking bypassed canonical evidence dedup")
			}
		})
	}
}
