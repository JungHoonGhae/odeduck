package goalwork_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestAnalysisReviewUsesDisclosedGroupsWithoutSendingAllContributors(t *testing.T) {
	e := analysisReviewEngine(t, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		if in.Analysis == nil || in.Analysis.Method != "engine_relational_replay_v1" || len(in.Analysis.AdditionalEvidence) != 2 || len(in.Analysis.Sources) != 3 {
			t.Fatal("review lacks reproducible computation and multiple sources")
		}
		b, _ := json.Marshal(in)
		if strings.Contains(string(b), "UNSELECTED_PRIVATE") || len(in.Evidence.Records) != 1 {
			t.Fatal("computed disclosure expanded to private contributor values")
		}
		if in.Artifact.Rows[0]["o3.total"] != json.Number("12") || in.Artifact.Rows[1]["o3.total"] != json.Number("12") || in.Artifact.Rows[0]["o2.spaces"] != "2" {
			t.Fatal("group sum or independent equal-valued group lost")
		}
		return supportedAnalysisReview(in), nil
	})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "compare"})
	if err != nil || v.Status != "output_ready" || v.Evaluation.Review == nil || v.Evaluation.Review.Method != "independent_model_relational_analysis_v1" || len(v.Reviews) != 1 {
		t.Fatalf("analysis cannot complete through public engine: %v %+v", err, v.Gaps)
	}
}

func supportedAnalysisReview(in goalwork.ReviewInput) goalwork.ReviewAssessment {
	ids := []string{in.Evidence.ID}
	for _, p := range in.Analysis.AdditionalEvidence {
		ids = append(ids, p.ID)
	}
	f := goalwork.ReviewFinding{Verdict: "supported", Reason: "Fixture judgement only; actual accuracy needs independent source calibration", PacketIDs: ids}
	a := goalwork.ReviewAssessment{GoalFit: f}
	for _, o := range in.Contract.Outputs {
		a.Outputs = append(a.Outputs, goalwork.OutputReview{Output: o.ID, Finding: f})
	}
	for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
		a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
	}
	return a
}

func TestAnalysisReviewRequiresEveryDimensionAndCannotMutateItsEvidence(t *testing.T) {
	for _, failure := range []string{"relations", "periods", "measurements", "coverage", "goal", "output", "missing check", "invented packet", "legacy citation", "mutated input"} {
		t.Run(failure, func(t *testing.T) {
			e := analysisReviewEngine(t, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				a := supportedAnalysisReview(in)
				for i := range a.AnalysisChecks {
					if a.AnalysisChecks[i].Topic == failure {
						a.AnalysisChecks[i].Finding.Verdict = "insufficient"
					}
				}
				switch failure {
				case "goal":
					a.GoalFit.Verdict = "unsupported"
				case "output":
					a.Outputs[0].Finding.Verdict = "unsupported"
				case "missing check":
					a.AnalysisChecks = a.AnalysisChecks[:3]
				case "invented packet":
					a.GoalFit.PacketIDs = []string{"ep_fake"}
				case "legacy citation":
					a.GoalFit.PacketID = in.Evidence.ID
				case "mutated input":
					in.Analysis.AdditionalEvidence[0].Records[0].Values["spaces"] = "invented"
				}
				return a, nil
			})
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "compare"})
			if err != nil || v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview || len(v.Reviews) != 1 || v.Evidence[1].Records[0].Values["spaces"] != "2" {
				t.Fatalf("unsafe analysis: %v %+v", err, v.Gaps)
			}
		})
	}
}

func TestAnalysisReviewCannotRepackageEvidenceIntoSourceReviewForAnotherAttempt(t *testing.T) {
	calls := 0
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		if in.Analysis == nil {
			t.Fatal("same cells reached a second reviewer through another protocol")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.Verdict = "insufficient"
		return a, nil
	})
	executeResult(t, e, resultRecipe())
	for _, row := range []int{1, 2} {
		v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, Rows: []int{row}, Fields: []string{"record", "year"}}})
		if err != nil || len(v.Gaps) != 0 {
			t.Fatal("split evidence setup")
		}
	}
	advanceSourceReview(t, e)
	readSourceReviewEvidence(t, e)
	v := advanceSourceReview(t, e)
	if calls != 1 || len(v.Reviews) != 1 || v.Status == "output_ready" {
		t.Fatalf("repackaged review bypassed dedup: %+v", v.Gaps)
	}
}

func analysisReviewEngine(t *testing.T, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error), selections ...goalwork.EvidenceRequest) *goalwork.Engine {
	t.Helper()
	rows := map[string][]goalwork.Row{
		"counts": {{"region": "A", "n": "5", "private": "UNSELECTED_PRIVATE"}, {"region": "A", "n": "7", "private": "UNSELECTED_PRIVATE"}, {"region": "B", "n": "12", "private": "UNSELECTED_PRIVATE"}},
		"spaces": {{"region": "A", "spaces": "2"}, {"region": "B", "spaces": "3"}},
	}
	e, err := goalwork.Start("관측한 두 지역의 기록된 이용자 수 합계와 공간 수를 비교해줘", goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Rows: rows[s.PK], Delivery: "FILE"}, nil
		},
		Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	step := func(d goalwork.Decision) goalwork.View {
		t.Helper()
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("%s: %v %+v", d.Action, err, v.Gaps)
		}
		return v
	}
	c := goalwork.GoalContract{Outcome: "관측한 지역 비교", Region: "A와 B", Period: "원천 기록 기준", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "counts", Description: "이용자 기록"}, {ID: "spaces", Description: "공간 기록"}}, Outputs: []goalwork.OutputRequirement{{ID: "people", Role: "counts", Description: "지역별 이용자 합계", Type: "number"}, {ID: "spaces", Role: "spaces", Description: "지역별 공간 수", Type: "string"}}}
	step(goalwork.Decision{Action: "define", Contract: &c})
	for _, pk := range []string{"counts", "spaces"} {
		step(goalwork.Decision{Action: "search", Query: pk, Role: pk})
		step(goalwork.Decision{Action: "inspect", PK: pk})
		step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "file"}})
	}
	step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "counts", Delivery: "file", Reduce: &goalwork.SourceReduction{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, GroupBy: []string{"region"}, Measures: []goalwork.Measure{{As: "n_number", Field: "n", Format: "decimal_v1", Unit: "persons"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n_number"}}}}})
	p := goalwork.Composition{ID: "compare", Base: "o3", Purpose: "compare recorded regional totals", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o3.region"}, RightKeys: []string{"region"}}}, Select: []string{"o3.region", "o3.total", "o2.spaces"}, Roles: []goalwork.RoleBinding{{Role: "counts", Observation: "o1"}, {Role: "spaces", Observation: "o2"}}, Outputs: []goalwork.OutputBinding{{Output: "people", Field: "o3.total"}, {Output: "spaces", Field: "o2.spaces"}}, Assumptions: []string{"Arithmetic fixture, not independent semantic accuracy evidence"}}
	step(goalwork.Decision{Action: "compose", Composition: &p})
	step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if len(selections) == 0 {
		selections = []goalwork.EvidenceRequest{
			{Observation: "o1", Rows: []int{1}, Fields: []string{"region", "n"}},
			{Observation: "o2", Rows: []int{1, 2}, Fields: []string{"region", "spaces"}},
			{Observation: "o3", Rows: []int{1, 2}, Fields: []string{"region", "total"}},
		}
	}
	for _, selection := range selections {
		for _, o := range e.View().Observations {
			if o.ID == selection.Observation {
				selection.RowsSHA256 = o.RowsSHA256
			}
		}
		step(goalwork.Decision{Action: "read_evidence", Evidence: &selection})
	}
	return e
}

func TestAnalysisReviewDoesNotDiscloseAResultFromMissingEvidence(t *testing.T) {
	for _, missing := range []string{"original field", "direct row", "computed row"} {
		t.Run(missing, func(t *testing.T) {
			selections := []goalwork.EvidenceRequest{
				{Observation: "o1", Rows: []int{1}, Fields: []string{"region", "n"}},
				{Observation: "o2", Rows: []int{1, 2}, Fields: []string{"region", "spaces"}},
				{Observation: "o3", Rows: []int{1, 2}, Fields: []string{"region", "total"}},
			}
			switch missing {
			case "original field":
				selections[0].Fields = []string{"region"}
			case "direct row":
				selections[1].Rows = []int{1}
			case "computed row":
				selections[2].Rows = []int{1}
			}
			e := analysisReviewEngine(t, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				t.Fatal("undisclosed result reached model")
				return goalwork.ReviewAssessment{}, nil
			}, selections...)
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "compare"})
			if err != nil || v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview || len(v.Reviews) != 0 || len(v.Gaps) != 1 {
				t.Fatalf("missing=%s: %v %+v", missing, err, v.Gaps)
			}
		})
	}
}

func TestAnalysisReviewNeedsContributorsNotUnrelatedFilteredSourceValues(t *testing.T) {
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		if in.Analysis == nil || len(in.Analysis.Sources[0].DirectRows) != 1 || in.Analysis.Sources[0].DirectRows[0] != 1 {
			t.Fatal("no original record participation proof")
		}
		b, _ := json.Marshal(in)
		if strings.Contains(string(b), "museum B") {
			t.Fatal("unrelated filtered row was disclosed")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.Verdict = "insufficient" // This tests disclosure, not whether filtering fits the original goal.
		return a, nil
	})
	p := resultRecipe()
	p.Time = &goalwork.TemporalAlignment{Window: &goalwork.DateWindow{From: "2025-01-01", Through: "2025-12-31"}, Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	executeResult(t, e, p)
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, Rows: []int{1}, Fields: []string{"record", "year"}}})
	if err != nil || len(v.Gaps) != 0 {
		t.Fatal("selected source setup")
	}
	v = advanceSourceReview(t, e)
	if len(v.Reviews) != 1 || v.Evaluation.Review == nil || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod {
		t.Fatalf("unrelated values required: %+v", v.Gaps)
	}
}

func TestAnalysisReviewCannotHideCancellingContributorsBehindTheSameTotal(t *testing.T) {
	e, err := goalwork.Start("관측한 연결 레코드의 수치를 합산해줘", goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			rows := []goalwork.Row{{"code": "A"}}
			if s.PK == "values" {
				rows = []goalwork.Row{{"code": "A", "n": "0"}, {"code": "A", "n": "10"}, {"code": "A", "n": "-10"}}
			}
			return goalwork.Acquired{Rows: rows, Delivery: "REST"}, nil
		},
		Review: func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("hidden cancelling terms reached reviewer")
			return goalwork.ReviewAssessment{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	step := func(d goalwork.Decision) {
		t.Helper()
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("%s: %v %+v", d.Action, err, v.Gaps)
		}
	}
	c := goalwork.GoalContract{Outcome: "sum", Region: "fixture", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "values"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Type: "number", Description: "sum"}}}
	step(goalwork.Decision{Action: "define", Contract: &c})
	for _, pk := range []string{"values", "mapping"} {
		step(goalwork.Decision{Action: "search", Query: pk, Role: "r"})
		step(goalwork.Decision{Action: "inspect", PK: pk})
		step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "api"}})
	}
	p := goalwork.Composition{ID: "sum", Base: "o1", Purpose: "sum after join", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, Select: []string{"total"}, Measures: []goalwork.Measure{{As: "n", Field: "o1.n", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Field: "n", Op: "sum"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"fixture arithmetic"}}
	step(goalwork.Decision{Action: "compose", Composition: &p})
	step(goalwork.Decision{Action: "execute", CompositionID: "sum"})
	if e.View().Artifact.Rows[0]["total"] != json.Number("0") {
		t.Fatal("independent literal sum differs")
	}
	for i, fields := range [][]string{{"code", "n"}, {"code"}} {
		o := e.View().Observations[i]
		step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: fields}})
	}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "sum"})
	if err != nil || len(v.Reviews) != 0 || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "contributing") {
		t.Fatalf("same result hid source terms: %v %+v", err, v.Gaps)
	}
}
