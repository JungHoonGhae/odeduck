package goalwork_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func unmatchedEngine(t *testing.T, inputs [][]goalwork.Row, joins []goalwork.Join, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	pks := []string{"left", "right", "third"}
	rows := map[string][]goalwork.Row{}
	for i, input := range inputs {
		rows[pks[i]] = input
	}
	policy := goalwork.Policy{EvidenceRecipient: "claude"}
	if review != nil {
		policy.ReviewRecipient, policy.ReviewAnalyses = "claude", true
	}
	e, err := goalwork.Start("관측한 자료를 비교하고 대응하지 않는 기록도 구분해서 보여줘", policy, goalwork.Dependencies{
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
	c := goalwork.GoalContract{Outcome: "source comparison", Region: "observed records", Period: "source dates", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "left", Description: "left records"}}, Outputs: []goalwork.OutputRequirement{{ID: "label", Description: "original label", Role: "left", Type: "string"}}, Explanations: []goalwork.ExplanationRequirement{{ID: "excluded", Topic: "coverage", Description: "unmatched source records"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	for _, pk := range pks[:len(inputs)] {
		advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: pk, Role: "left"})
		advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: pk})
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "file"}})
	}
	p := goalwork.Composition{ID: "compare", Base: "o1", Purpose: "compare source records", Joins: joins, Select: []string{"o1.label"}, Assumptions: []string{"fixture comparison, not population identity"}, Roles: []goalwork.RoleBinding{{Role: "left", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "label", Field: "o1.label"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	return e
}

func advanceUnmatched(t *testing.T, e *goalwork.Engine, d goalwork.Decision) goalwork.View {
	t.Helper()
	v, err := e.Advance(context.Background(), e.View().Revision, d)
	if err != nil || len(v.Gaps) != 0 {
		t.Fatalf("%s: %v %+v", d.Action, err, v.Gaps)
	}
	return v
}

func TestUnmatchedRecordsRetainEachJoinStageAndSourceTupleWithoutValues(t *testing.T) {
	e := unmatchedEngine(t, [][]goalwork.Row{
		{{"code": "a", "label": "PRIVATE_A"}, {"code": "b", "label": "PRIVATE_B"}, {"code": nil, "label": "PRIVATE_NULL"}},
		{{"code": "a", "id": "x"}, {"code": "a", "id": "y"}, {"code": "z", "id": "z"}},
		{{"id": "x"}},
	}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}, {Right: "o3", LeftKeys: []string{"o2.id"}, RightKeys: []string{"id"}}}, nil)
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: "compare"})
	a, b := v.Artifact.Metrics[0], v.Artifact.Metrics[1]
	if len(v.Artifact.Rows) != 1 || a.MatchedLeftRows != 1 || a.OutputRows != 2 || !reflect.DeepEqual(a.UnmatchedLeft, []map[string]int{{"o1": 2}, {"o1": 3}}) || !reflect.DeepEqual(a.UnmatchedRight, []int{3}) || !reflect.DeepEqual(b.UnmatchedLeft, []map[string]int{{"o1": 1, "o2": 2}}) || len(b.UnmatchedRight) != 0 {
		t.Fatalf("stage-local exclusions lost or confused with final participation: %+v", v.Artifact.Metrics)
	}
	encoded, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("unmatched diagnostics disclosed unselected values")
	}
	v.Artifact.Metrics[0].UnmatchedLeft[0]["o1"] = 99
	v.Executions[0].Metrics[0].UnmatchedRight[0] = 99
	if e.View().Artifact.Metrics[0].UnmatchedLeft[0]["o1"] != 2 || e.View().Executions[0].Metrics[0].UnmatchedRight[0] != 3 {
		t.Fatal("returned diagnostics mutate retained execution")
	}
}

func TestEmptyJoinRetainsAddressesForEvidenceAndReplanning(t *testing.T) {
	e := unmatchedEngine(t, [][]goalwork.Row{{{"code": "a", "label": "PRIVATE_A"}}, {{"code": "b"}}}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, nil)
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "execute", CompositionID: "compare"})
	if err != nil || v.Artifact != nil || len(v.Gaps) != 1 || len(v.Executions) != 1 || v.Executions[0].Status != "failed" {
		t.Fatalf("empty result was accepted: %v %+v", err, v)
	}
	m := v.Executions[0].Metrics[0]
	if !reflect.DeepEqual(m.UnmatchedLeft, []map[string]int{{"o1": 1}}) || !reflect.DeepEqual(m.UnmatchedRight, []int{1}) {
		t.Fatal("failed join discarded source addresses")
	}
	// Gaps are append-only, so examine the new action directly.
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.Observations[1].RowsSHA256, Rows: m.UnmatchedRight, Fields: []string{"code"}}})
	if err != nil || len(v.Gaps) != 1 || v.Evidence[0].Records[0].Values["code"] != "b" {
		t.Fatal("failed counterpart cannot be inspected under original disclosure policy")
	}
}

func TestUnmatchedRecordsIncludeScopeAndTemporalRejections(t *testing.T) {
	e := unmatchedEngine(t, [][]goalwork.Row{
		{{"code": "a", "label": "ok", "scope": "A", "year": "2024"}, {"code": "a", "label": "other scope", "scope": "B", "year": "2024"}, {"code": "a", "label": "other period", "scope": "A", "year": "2023"}, {"code": "a", "label": "unknown scope", "scope": nil, "year": "2024"}},
		{{"code": "a", "scope": "A", "year": "2024"}, {"code": "z", "scope": "A", "year": "2024"}},
	}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, nil)
	p := e.View().Compositions[0]
	p.ID = "scoped"
	p.Joins[0].Scopes = []goalwork.JoinScope{{LeftParts: []string{"o1.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}
	p.Time = &goalwork.TemporalAlignment{Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}, {Observation: "o2", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	m := v.Artifact.Metrics[0]
	if !reflect.DeepEqual(m.UnmatchedLeft, []map[string]int{{"o1": 2}, {"o1": 3}, {"o1": 4}}) || !reflect.DeepEqual(m.UnmatchedRight, []int{2}) || m.TemporalRejectedPairs != 1 || m.ScopeChecks[0].ConflictPairs != 1 || m.ScopeChecks[0].UnknownPairs != 1 {
		t.Fatalf("literal key candidates were counted as accepted pairs: %+v", m)
	}
}

func TestAnalysisReviewNeedsUnmatchedComparisonEvidenceWithoutExtraOutputDisclosure(t *testing.T) {
	calls := 0
	e := unmatchedEngine(t, [][]goalwork.Row{
		{{"code": "a", "label": "observed A"}, {"code": "b", "label": "UNSELECTED_PRIVATE"}}, {{"code": "a"}, {"code": "c"}},
	}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		b, _ := json.Marshal(in)
		if strings.Contains(string(b), "UNSELECTED_PRIVATE") {
			t.Fatal("excluded output field was disclosed without selection")
		}
		return supportedAnalysisReview(in), nil
	})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: "compare"})
	for i, fields := range [][]string{{"code", "label"}, {"code"}} {
		o := e.View().Observations[i]
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: fields}})
	}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "compare"})
	if err != nil || calls != 0 || len(v.Reviews) != 0 || v.Status == "output_ready" || len(v.Gaps) != 1 {
		t.Fatalf("matched-only disclosure hid excluded region from review: %v calls=%d gaps=%+v", err, calls, v.Gaps)
	}
	o := v.Observations[0]
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2}, Fields: []string{"code"}}})
	if err != nil || len(v.Gaps) != 1 {
		t.Fatal("excluded comparison evidence unavailable")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: "compare"})
	if err != nil || calls != 0 || len(v.Reviews) != 0 || len(v.Gaps) != 2 {
		t.Fatal("unmatched right source was exempt from comparison evidence")
	}
	o = v.Observations[1]
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2}, Fields: []string{"code"}}})
	if err != nil || len(v.Gaps) != 2 {
		t.Fatal("right comparison evidence unavailable")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: "compare"})
	if err != nil || calls != 1 || v.Status != "output_ready" {
		t.Fatalf("comparison evidence still cannot reach review: %v %+v", err, v.Gaps)
	}
}
