package goalwork_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func sourceReductionFixture(t *testing.T, rows []goalwork.Row) (*goalwork.Engine, goalwork.SampleRequest) {
	t.Helper()
	e, err := goalwork.Start("집계한 원천 값", goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "source"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "source"}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			if s.Reduce != nil {
				t.Fatal("local reduction called external adapter")
			}
			return goalwork.Acquired{Rows: rows, Delivery: "FILE"}, nil
		},
		Review: func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("derived aggregate laundered as original source report")
			return goalwork.ReviewAssessment{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "record sum", Region: "fixture", Period: "source date", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Description: "sum", Role: "r", Type: "number"}}}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "source"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "source", Delivery: "file"}}} {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("setup: %v %+v", err, v.Gaps)
		}
	}
	return e, goalwork.SampleRequest{PK: "source", Delivery: "file", Reduce: &goalwork.SourceReduction{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, GroupBy: []string{"region"}, Measures: []goalwork.Measure{{As: "n_number", Field: "n", Format: "decimal_v1", Unit: "persons"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n_number"}}}}
}

func TestReductionEvidenceReferencesAComputedGroupNotOneOriginalRecord(t *testing.T) {
	e, s := sourceReductionFixture(t, []goalwork.Row{{"region": "A", "n": "5", "private": "UNSELECTED"}, {"region": "A", "n": "5", "private": "UNSELECTED"}})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Gaps) != 0 {
		t.Fatalf("reduce: %v %+v", err, v.Gaps)
	}
	group := v.Observations[1]
	if len(group.Reduction.Groups) != 1 || len(group.Reduction.Groups[0]) != 2 || group.Reduction.Groups[0][0] != 1 || group.Reduction.Groups[0][1] != 2 {
		t.Fatal("equal-valued source records were collapsed")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: group.ID, RowsSHA256: group.RowsSHA256, Rows: []int{1}, Fields: []string{"total"}}})
	if err != nil || len(v.Evidence) != 1 {
		t.Fatalf("evidence: %v %+v", err, v.Gaps)
	}
	record := v.Evidence[0].Records[0]
	if record.Values["total"] != json.Number("10") || record.Origins["total"].Kind != "computed_group" || record.Origins["total"].Observation != group.ID || record.Origins["total"].Ordinal != 1 {
		t.Fatalf("group incorrectly attributed to a single source record: %+v", record)
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "UNSELECTED") {
		t.Fatal("unselected values leaked")
	}
	p := goalwork.Composition{ID: "group-report", Base: group.ID, Purpose: "report local calculation", Select: []string{group.ID + ".total"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: group.ID + ".total"}}, Assumptions: []string{"calculated group, not publisher statement"}}
	for _, d := range []goalwork.Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}, {Action: "review_result", CompositionID: p.ID}} {
		v, err = e.Advance(context.Background(), e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
	}
	if v.Artifact == nil || v.Evaluation.Status != "requirements_met" || !v.Evaluation.NeedsSemanticReview || v.Evaluation.Review != nil || len(v.Reviews) != 0 {
		t.Fatal("derived result lost attribution or gained source-report approval")
	}
}

func TestReductionCannotBeJoinedBackToItsOwnContributingRecords(t *testing.T) {
	e, s := sourceReductionFixture(t, []goalwork.Row{{"region": "A", "n": "5"}, {"region": "A", "n": "7"}})
	if v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("reduce setup failed")
	}
	p := goalwork.Composition{ID: "back", Base: "o2", Purpose: "unsafe mixed grain", Joins: []goalwork.Join{{Right: "o1", LeftKeys: []string{"o2.region"}, RightKeys: []string{"region"}}}, Assumptions: []string{"group and member have different grain"}}
	for _, d := range []goalwork.Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if v.Artifact != nil || len(v.Executions) != 1 || v.Executions[0].Status != "failed" || !strings.Contains(v.Executions[0].Error, "overlapping") {
		t.Fatalf("same original contribution laundered through group: %+v", v.Gaps)
	}
}

func TestReductionTimeChecksEveryContributingOriginalPeriod(t *testing.T) {
	for _, secondDate := range []string{"2025-01-01", "2024-01-01"} {
		t.Run(secondDate, func(t *testing.T) {
			e, s := sourceReductionFixture(t, []goalwork.Row{{"region": "A", "n": "5", "date": "2025-01-01"}, {"region": "A", "n": "7", "date": secondDate}})
			if v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
				t.Fatal("reduce setup failed")
			}
			p := goalwork.Composition{ID: "dated", Base: "o2", Purpose: "date-bound group", Select: []string{"o2.total"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "o2.total"}}, Time: &goalwork.TemporalAlignment{Window: &goalwork.DateWindow{From: "2025-01-01", Through: "2025-12-31"}, Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "date", Meaning: "reference_period", Format: "date_v1"}}}, Assumptions: []string{"date belongs to original records, not a generated group"}}
			for _, d := range []goalwork.Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
				if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
					t.Fatal(err)
				}
			}
			v := e.View()
			if secondDate == "2025-01-01" {
				if v.Artifact == nil || len(v.Gaps) != 0 || v.Artifact.Rows[0]["o2.total"] != json.Number("12") || v.Evaluation.Temporal.Status != "checked" {
					t.Fatalf("valid original periods lost: %+v", v.Gaps)
				}
			} else if v.Artifact != nil || len(v.Executions) != 1 || v.Executions[0].Status != "failed" {
				t.Fatal("group total silently included an out-of-window source contribution")
			}
		})
	}
}

func TestSourceReductionRejectsUnpinnedOrAmbiguousRequestsWithoutPartialObservation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*goalwork.SampleRequest)
	}{
		{"unknown source", func(s *goalwork.SampleRequest) { s.Reduce.Observation = "o999" }},
		{"stale source", func(s *goalwork.SampleRequest) { s.Reduce.RowsSHA256 = strings.Repeat("0", 64) }},
		{"different delivery", func(s *goalwork.SampleRequest) { s.Delivery = "api" }},
		{"new asset", func(s *goalwork.SampleRequest) { s.Asset = "guessed.csv" }},
		{"new selection", func(s *goalwork.SampleRequest) { s.Where = map[string]string{"region": "A"} }},
		{"new scan", func(s *goalwork.SampleRequest) { s.ScanCSV = true }},
		{"unknown field", func(s *goalwork.SampleRequest) { s.Reduce.Measures[0].Field = "invented" }},
		{"shadow field", func(s *goalwork.SampleRequest) { s.Reduce.Measures[0].As = "region" }},
		{"shadow group", func(s *goalwork.SampleRequest) { s.Reduce.Aggregates[0].As = "region" }},
		{"duplicate group", func(s *goalwork.SampleRequest) { s.Reduce.GroupBy = []string{"region", "region"} }},
		{"credential field", func(s *goalwork.SampleRequest) { s.Reduce.Measures[0].Field = "serviceKey" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, s := sourceReductionFixture(t, []goalwork.Row{{"region": "A", "n": "5", "serviceKey": "999"}})
			tc.change(&s)
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 1 || v.Artifact != nil {
				t.Fatalf("invalid reduction acquired partial evidence: %v %+v", err, v.Gaps)
			}
		})
	}
}

func TestReductionDoesNotTurnMissingTermsIntoPartialTotals(t *testing.T) {
	e, s := sourceReductionFixture(t, []goalwork.Row{{"region": "A", "n": "5", "other": "2"}, {"region": "A", "n": "7", "other": nil}})
	s.Reduce.Measures[0].Field = ""
	s.Reduce.Measures[0].Op = "sum_fields"
	s.Reduce.Measures[0].Fields = []string{"n", "other"}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 1 || v.SampleAttempts[1].Status != "failed" {
		t.Fatal("missing summand silently ignored or partial groups retained")
	}
}

func TestReductionRejectsNestedGroups(t *testing.T) {
	e, s := sourceReductionFixture(t, []goalwork.Row{{"region": "A", "n": "5"}})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Gaps) != 0 {
		t.Fatal("reduce setup")
	}
	s.Reduce.Observation, s.Reduce.RowsSHA256 = "o2", v.Observations[1].RowsSHA256
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 {
		t.Fatal("nested group accepted as original source")
	}
}

func TestSourceReductionJoinsAtCompatibleGrainWithoutMultiplyingSchoolTotals(t *testing.T) {
	ctx := context.Background()
	rows := map[string][]goalwork.Row{
		"people":  {{"region": "A", "n": "10"}, {"region": "A", "n": "20"}, {"region": "B", "n": "7"}},
		"schools": {{"region": "A", "schools": "2"}, {"region": "B", "schools": "1"}},
	}
	e, err := goalwork.Start("지역별 인구와 학교 수 비교", goalwork.Policy{}, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			if s.Reduce != nil {
				t.Fatal("local reduction must not call an external source")
			}
			return goalwork.Acquired{Rows: rows[s.PK], Delivery: "FILE"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	step := func(d goalwork.Decision) goalwork.View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("%s: %v %+v", d.Action, err, v.Gaps)
		}
		return v
	}
	c := goalwork.GoalContract{Outcome: "비교", Region: "A/B", Period: "원천별 기준일", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "people", Description: "인구"}, {ID: "schools", Description: "학교"}}, Outputs: []goalwork.OutputRequirement{{ID: "population", Description: "지역별 인구 합", Role: "people", Type: "number"}, {ID: "schools", Description: "원천의 학교 수", Role: "schools", Type: "string"}}}
	step(goalwork.Decision{Action: "define", Contract: &c})
	for _, pk := range []string{"people", "schools"} {
		step(goalwork.Decision{Action: "search", Query: pk, Role: pk})
		step(goalwork.Decision{Action: "inspect", PK: pk})
		step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "file"}})
	}
	before := e.View()
	r := goalwork.SourceReduction{Observation: "o1", RowsSHA256: before.Observations[0].RowsSHA256, GroupBy: []string{"region"}, Measures: []goalwork.Measure{{As: "persons", Field: "n", Format: "decimal_v1", Unit: "persons"}}, Aggregates: []goalwork.Aggregate{{As: "population", Op: "sum", Field: "persons"}}}
	v := step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "people", Delivery: "file", Reduce: &r}})
	if len(v.Observations) != 3 || v.Observations[2].Reduction == nil || v.Observations[0].RowsSHA256 != before.Observations[0].RowsSHA256 || v.Budget.SamplesRemaining != 5 {
		t.Fatal("reduction lost origin, mutated source or bypassed shared budget")
	}
	p := goalwork.Composition{ID: "compare", Purpose: "join district aggregates", Base: "o3", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o3.region"}, RightKeys: []string{"region"}}}, Select: []string{"o3.region", "o3.population", "o2.schools"}, Roles: []goalwork.RoleBinding{{Role: "people", Observation: "o3"}, {Role: "schools", Observation: "o2"}}, Outputs: []goalwork.OutputBinding{{Output: "population", Field: "o3.population"}, {Output: "schools", Field: "o2.schools"}}, Assumptions: []string{"Fixture arithmetic only; period, identity and population coverage are not approved."}}
	step(goalwork.Decision{Action: "compose", Composition: &p})
	v = step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 2 || len(v.Artifact.Sources) != 3 {
		t.Fatalf("missing review-bound result/origins: %+v", v)
	}
	want := []goalwork.Row{{"o3.region": "A", "o3.population": json.Number("30"), "o2.schools": "2"}, {"o3.region": "B", "o3.population": json.Number("7"), "o2.schools": "1"}}
	gotJSON, _ := json.Marshal(v.Artifact.Rows)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("got %s; want %s", gotJSON, wantJSON)
	}
}
