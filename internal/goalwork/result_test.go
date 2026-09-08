package goalwork_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSingleSourceResultUsesObservedFieldsWithoutInventingAJoin(t *testing.T) {
	contract := resultContract()
	e := observedResultEngine(t, contract)
	p := resultRecipe()
	view := executeResult(t, e, p)
	if view.Artifact == nil {
		t.Fatalf("single-source result was blocked: %+v", view.Gaps)
	}
	if view.Artifact.Status != "sample_executed" || view.Status != "review_required" || !view.Evaluation.NeedsSemanticReview {
		t.Fatalf("execution was confused with approval: %+v", view)
	}
	if len(view.Artifact.Rows) != 2 || view.Artifact.Rows[0]["o1.record"] != "private-A" || len(view.Artifact.Rows[0]) != 1 {
		t.Fatalf("projection changed observed records: %+v", view.Artifact.Rows)
	}
	if len(view.Artifact.Sources) != 1 || view.Artifact.Sources[0].ID != "o1" || len(view.Artifact.Requests) != 1 || view.Artifact.Requests[0].PK != "records" {
		t.Fatal("result lost its source or acquisition request")
	}
	if len(view.Artifact.Metrics) != 0 {
		t.Fatalf("unexecuted join metrics: %+v", view.Artifact.Metrics)
	}
	b, err := json.Marshal(e.PlanningView())
	if err != nil || strings.Contains(string(b), "private-A") || strings.Contains(string(b), "private-B") {
		t.Fatal("projection exposed source values to the external planner")
	}
}

func resultContract() goalwork.GoalContract {
	return goalwork.GoalContract{Outcome: "observed records", Region: "fixture", Period: "2025 source snapshot", Coverage: "sample",
		Roles:        []goalwork.RoleRequirement{{ID: "records", Description: "source records"}},
		Outputs:      []goalwork.OutputRequirement{{ID: "record", Description: "source record label", Role: "records", Type: "string"}},
		Explanations: []goalwork.ExplanationRequirement{{ID: "coverage", Topic: "coverage", Description: "observed range"}, {ID: "identity", Topic: "identity", Description: "identity limits"}},
	}
}

func TestSingleSourceResultExplainsOnlyExecutedOperations(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	v := executeResult(t, e, resultRecipe())
	if v.Artifact == nil {
		t.Fatalf("missing result: %+v", v.Gaps)
	}
	for _, text := range append(v.Artifact.Limitations, v.Evaluation.Explanations[0].Text, v.Evaluation.Explanations[1].Text) {
		if strings.Contains(text, "inner join") || strings.Contains(text, "복합키 일치를 계산") {
			t.Fatalf("unexecuted operation in evidence: %s", text)
		}
	}
}

func TestSingleSourceAnalysisPreservesExactAggregationAndRoleLineage(t *testing.T) {
	c := resultContract()
	c.Outputs = []goalwork.OutputRequirement{{ID: "total", Role: "records", Type: "number", Description: "sample amount sum"}}
	e := observedResultEngine(t, c)
	p := resultRecipe()
	p.Select = nil
	p.Measures = []goalwork.Measure{{As: "amount", Field: "o1.amount", Format: "decimal_v1", Unit: "fixture units"}}
	p.Aggregates = []goalwork.Aggregate{{As: "total", Op: "sum", Field: "amount"}}
	p.Outputs = []goalwork.OutputBinding{{Output: "total", Field: "total"}}
	v := executeResult(t, e, p)
	if v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["total"] != json.Number("0.3") {
		t.Fatalf("independent decimal sum differs: %+v", v)
	}
	if v.Status != "review_required" || v.Evaluation.Status != "requirements_met" || !v.Evaluation.NeedsSemanticReview {
		t.Fatal("arithmetic was mistaken for semantic verification")
	}
}

func TestSingleSourceResultCannotSatisfyMissingRoleOrPopulation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*goalwork.GoalContract)
	}{
		{"missing role", func(c *goalwork.GoalContract) {
			c.Roles = append(c.Roles, goalwork.RoleRequirement{ID: "other", Description: "another required observation"})
		}},
		{"population", func(c *goalwork.GoalContract) { c.Coverage = "population" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := resultContract()
			tc.modify(&c)
			e := observedResultEngine(t, c)
			v := executeResult(t, e, resultRecipe())
			if v.Artifact == nil || v.Status != "exploring" || v.Evaluation.Status != "partial" {
				t.Fatalf("single source bypassed required scope: %+v", v)
			}
			if v.Goal != "original user goal" {
				t.Fatal("result changed the original goal")
			}
		})
	}
}

func TestSingleSourceResultRetainsRequestedTimeWindow(t *testing.T) {
	c := resultContract()
	c.TimeWindow = &goalwork.DateWindow{From: "2025-01-01", Through: "2025-12-31"}
	e := observedResultEngine(t, c)
	p := resultRecipe()
	p.Time = &goalwork.TemporalAlignment{Window: c.TimeWindow, Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	v := executeResult(t, e, p)
	if v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o1.record"] != "private-A" {
		t.Fatalf("original date window was ignored: %+v", v)
	}
	if v.Evaluation.Temporal.Status != "checked" || v.Evaluation.Temporal.RejectedPairs != 1 || v.Evaluation.Temporal.MeaningVerified {
		t.Fatalf("single-source temporal evidence differs: %+v", v.Evaluation)
	}
	if strings.Contains(v.Evaluation.Temporal.Limitation, "nearest") {
		t.Fatal("ordinary time filtering claimed nearest-selection work")
	}
}

func TestSingleSourceEmptyTimeResultDoesNotInventSpatialWork(t *testing.T) {
	c := resultContract()
	c.TimeWindow = &goalwork.DateWindow{From: "2026-01-01", Through: "2026-12-31"}
	e := observedResultEngine(t, c)
	p := resultRecipe()
	p.Time = &goalwork.TemporalAlignment{Window: c.TimeWindow, Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	v := executeResult(t, e, p)
	if v.Artifact != nil || v.Status != "exploring" || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "empty result after original-source temporal alignment") {
		t.Fatalf("unmatched time was mislabeled or accepted: %+v", v)
	}
}

func resultRecipe() goalwork.Composition {
	return goalwork.Composition{ID: "records", Purpose: "selected source fields", Base: "o1", Select: []string{"o1.record"},
		Roles: []goalwork.RoleBinding{{Role: "records", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "record", Field: "o1.record"}},
		Assumptions: []string{"fixture only; real-world interpretation requires review"},
	}
}

func observedResultEngine(t *testing.T, contract goalwork.GoalContract) *goalwork.Engine {
	t.Helper()
	e, err := goalwork.Start("original user goal", goalwork.Policy{}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records", Title: "source records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{
				{"record": "private-A", "amount": "0.10", "year": "2025"},
				{"record": "private-B", "amount": "0.20", "year": "2024"},
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []goalwork.Decision{
		{Action: "define", Contract: &contract},
		{Action: "search", Query: "records", Role: "records"},
		{Action: "inspect", PK: "records"},
		{Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}},
	} {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("acquisition setup: %+v %v", v.Gaps, err)
		}
	}
	return e
}

func executeResult(t *testing.T, e *goalwork.Engine, p goalwork.Composition) goalwork.View {
	t.Helper()
	var v goalwork.View
	for _, d := range []goalwork.Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		var err error
		v, err = e.Advance(context.Background(), e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
	}
	return v
}
