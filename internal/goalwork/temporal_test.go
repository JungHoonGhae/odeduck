package goalwork

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestGoalTimeWindowCannotBeDroppedAndReviewEvidenceReachesPlanner(t *testing.T) {
	ctx := context.Background()
	e, _ := Start("2025년 인구와 시설 비교", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q, Title: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) { return Inspection{PK: pk}, nil },
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			return Acquired{Rows: []Row{{"code": "001", "year": "2024", "value": "prior"}, {"code": "001", "year": "2025", "value": "current"}}, Delivery: "FILE"}, nil
		},
	})
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	window := &DateWindow{From: "2025-01-01", Through: "2025-12-31"}
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "비교", Region: "fixture", Period: "2025", TimeWindow: window, Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "인구"}, {ID: "facilities", Description: "시설"}}, Outputs: []OutputRequirement{{ID: "value", Role: "people", Description: "값", Type: "string"}}, Explanations: []ExplanationRequirement{{ID: "time_limits", Topic: "temporal", Description: "시간 비교 한계"}, {ID: "coverage_limits", Topic: "coverage", Description: "표본 한계"}}}})
	for _, pk := range []string{"people", "facilities"} {
		step(Decision{Action: "search", Query: pk, Role: pk})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "file"}})
	}
	p := Composition{ID: "dropped-time", Purpose: "비교", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, Assumptions: []string{"같은 코드라고 가정"}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "facilities", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "value", Field: "o1.value"}}}
	v := step(Decision{Action: "compose", Composition: &p})
	if len(v.Compositions) != 0 || len(v.Gaps) != 1 {
		t.Fatal("goal window dropped")
	}
	p.ID = "aligned"
	p.Time = &TemporalAlignment{Window: window, Bindings: []TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}, {Observation: "o2", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	step(Decision{Action: "compose", Composition: &p})
	v = step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o1.value"] != "current" || v.Evaluation.Temporal.Status != "checked" || v.Evaluation.Temporal.RejectedPairs != 3 || v.Evaluation.Temporal.MeaningVerified {
		t.Fatalf("%+v", v)
	}
	planning := e.PlanningView()
	if planning.Artifact != nil || planning.Evaluation.Temporal.RejectedPairs != 3 || !planning.Evaluation.NeedsSemanticReview {
		t.Fatal("lost temporal evidence or promoted interpretation to verification")
	}
	if len(planning.Evaluation.Explanations) != 2 || !strings.Contains(planning.Evaluation.Explanations[0].Text, "2025-01-01") || !strings.Contains(planning.Evaluation.Explanations[1].Text, "결과 1행") || len(planning.Evaluation.Explanations[0].Observations) != 2 {
		t.Fatalf("missing computed explanation: %+v", planning.Evaluation.Explanations)
	}
}

func TestGoalRetainsFailedTemporalMetricsForReplanning(t *testing.T) {
	ctx := context.Background()
	e, _ := Start("기간 비교", Policy{}, Dependencies{Search: func(_ context.Context, q string) (catalog.Result, error) {
		return catalog.Result{Hits: []catalog.Hit{{PK: q, Title: q}}}, nil
	}, Inspect: func(_ context.Context, pk string) (Inspection, error) { return Inspection{PK: pk}, nil }, Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
		return Acquired{Rows: []Row{{"code": "1", "year": s.PK}}, Delivery: "FILE"}, nil
	}})
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "비교", Region: "fixture", Period: "미확정", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "원천"}}, Outputs: []OutputRequirement{{ID: "value", Role: "role", Description: "값", Type: "string"}}}})
	for _, pk := range []string{"2024", "2025"} {
		step(Decision{Action: "search", Query: pk, Role: "role"})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "file"}})
	}
	p := Composition{ID: "different-years", Purpose: "비교", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, Assumptions: []string{"declared reference periods"}, Time: &TemporalAlignment{Bindings: []TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}, {Observation: "o2", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}}
	step(Decision{Action: "compose", Composition: &p})
	step(Decision{Action: "execute", CompositionID: p.ID})
	v := e.PlanningView()
	if v.Artifact != nil || len(v.Executions) != 1 || v.Executions[0].Status != "failed" || len(v.Executions[0].Metrics) != 1 || v.Executions[0].Metrics[0].TemporalRejectedPairs != 1 || v.Executions[0].Error == "" {
		t.Fatalf("lost failed-execution evidence: %+v", v)
	}
}

func TestTemporalCompositionUsesCommonPeriodAcrossWholePath(t *testing.T) {
	inputs := map[string][]Row{"a": {{"code": "001", "from": "2025-01-01", "to": "2025-03-31"}}, "map": {{"code": "001", "from": "2025-03-01", "to": "2025-05-31"}}, "c": {{"code": "001", "from": "2025-05-01", "to": "2025-06-30"}}}
	p := Composition{Base: "a", Joins: []Join{{Right: "map", LeftKeys: []string{"a.code"}, RightKeys: []string{"code"}}, {Right: "c", LeftKeys: []string{"map.code"}, RightKeys: []string{"code"}}}, Time: &TemporalAlignment{Bindings: []TimeBinding{{Observation: "a", FromField: "from", ThroughField: "to", Format: "date_v1", Meaning: "validity"}, {Observation: "map", FromField: "from", ThroughField: "to", Format: "date_v1", Meaning: "validity"}, {Observation: "c", FromField: "from", ThroughField: "to", Format: "date_v1", Meaning: "validity"}}}}
	_, metrics, err := execute(p, inputs, 10)
	if err == nil || !strings.Contains(err.Error(), "empty full join") || len(metrics) != 2 || metrics[1].TemporalRejectedPairs != 1 {
		t.Fatalf("pairwise temporal overlap became global proof: %v %+v", err, metrics)
	}
	inputs["c"][0]["from"] = "2025-03-15"
	rows, metrics, err := execute(p, inputs, 10)
	if err != nil || len(rows) != 1 || metrics[1].TemporalRule != "common_overlap_v1" {
		t.Fatalf("%v %+v %v", rows, metrics, err)
	}
}

func TestTemporalWindowRejectsOldRowsDespiteMatchingKeys(t *testing.T) {
	inputs := map[string][]Row{"shelters": {{"code": "001", "date": "2018-05-15"}, {"code": "001", "date": "2025-05-15"}, {"code": "001", "date": nil}}, "people": {{"code": "001", "year": "2025"}}}
	p := Composition{Base: "shelters", Joins: []Join{{Right: "people", LeftKeys: []string{"shelters.code"}, RightKeys: []string{"code"}}}, Time: &TemporalAlignment{Window: &DateWindow{From: "2025-01-01", Through: "2025-12-31"}, Bindings: []TimeBinding{{Observation: "shelters", FromField: "date", Format: "date_v1", Meaning: "reference_period"}, {Observation: "people", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}}
	rows, metrics, err := execute(p, inputs, 10)
	if err != nil || len(rows) != 1 || rows[0]["shelters.date"] != "2025-05-15" || metrics[0].TemporalRejectedPairs != 2 || metrics[0].TemporalUnknownPairs != 1 {
		t.Fatalf("%v %+v %v", rows, metrics, err)
	}
	if len(inputs["shelters"]) != 3 || inputs["shelters"][0]["date"] != "2018-05-15" {
		t.Fatal("original records changed")
	}
}

func TestTemporalContractsRequireEverySourceAndObservedDateFields(t *testing.T) {
	valid := []TimeBinding{{Observation: "a", FromField: "date", Format: "date_v1", Meaning: "reference_period"}, {Observation: "b", FromField: "date", Format: "date_v1", Meaning: "reference_period"}}
	for _, tc := range []struct {
		name     string
		bindings []TimeBinding
		window   *DateWindow
	}{
		{"missing source", valid[:1], nil},
		{"duplicate source", append(append([]TimeBinding{}, valid...), valid[0]), nil},
		{"missing meaning", []TimeBinding{{Observation: "a", FromField: "date", Format: "date_v1"}, valid[1]}, nil},
		{"metadata timestamp", []TimeBinding{{Observation: "a", FromField: "observedAt", Format: "date_v1", Meaning: "reference_period"}, valid[1]}, nil},
		{"snapshot range", []TimeBinding{{Observation: "a", FromField: "date", ThroughField: "date", Format: "date_v1", Meaning: "reference_period"}, valid[1]}, nil},
		{"open validity", []TimeBinding{{Observation: "a", FromField: "date", Format: "date_v1", Meaning: "validity"}, valid[1]}, nil},
		{"reversed window", valid, &DateWindow{From: "2025-12-31", Through: "2025-01-01"}},
		{"invalid window", valid, &DateWindow{From: "2025-02-30", Through: "2025-12-31"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.code"}, RightKeys: []string{"code"}}}, Time: &TemporalAlignment{Bindings: tc.bindings, Window: tc.window}}
			if _, _, err := execute(p, map[string][]Row{"a": {{"code": "001", "date": "2025-01-01"}}, "b": {{"code": "001", "date": "2025-01-01"}}}, 10); err == nil {
				t.Fatal("accepted unproven time contract")
			}
		})
	}
}

func TestTemporalParsingIsExplicitAndCalendarCorrect(t *testing.T) {
	for _, tc := range []struct {
		value  any
		format string
		bad    bool
	}{
		{"2024-02-29", "date_v1", false}, {"2025-02-29", "date_v1", true}, {"2025-2-01", "date_v1", true},
		{"202502", "compact_month_v1", false}, {"202502", "month_v1", true}, {"2025-02", "month_v1", false},
		{json.Number("2025"), "year_v1", false}, {json.Number("2.025e3"), "year_v1", true},
		{"0000", "year_v1", true}, {"2025-01-01T00:00:00Z", "date_v1", true}, {"2025/01/01", "date_v1", true},
	} {
		t.Run(stringifyMeasureCase(tc.value)+tc.format, func(t *testing.T) {
			p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.code"}, RightKeys: []string{"code"}}}, Time: &TemporalAlignment{Bindings: []TimeBinding{{Observation: "a", FromField: "date", Format: tc.format, Meaning: "event"}, {Observation: "b", FromField: "date", Format: tc.format, Meaning: "event"}}}}
			rows, _, err := execute(p, map[string][]Row{"a": {{"code": "1", "date": tc.value}}, "b": {{"code": "1", "date": tc.value}}}, 10)
			if tc.bad {
				if err == nil {
					t.Fatal("guessed or invalid date accepted")
				}
				return
			}
			if err != nil || len(rows) != 1 {
				t.Fatalf("%v %v", rows, err)
			}
		})
	}
}
