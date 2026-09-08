package goalwork

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestGoalMeasureOutputRetainsLineageAndPrecisionThroughPublicView(t *testing.T) {
	ctx := context.Background()
	e, _ := Start("쉼터 수용인원 표본 비교", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q, Title: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) { return Inspection{PK: pk}, nil },
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			return Acquired{Rows: []Row{{"code": "001", "capacity": "9007199254740993", "extra": "1"}}, Delivery: "STD"}, nil
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
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "쉼터 수용인원", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []RoleRequirement{{ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "capacity", Description: "수용인원", Role: "shelters", Type: "number"}}}})
	for _, pk := range []string{"shelters", "mapping"} {
		step(Decision{Action: "search", Query: pk, Role: pk})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "standard"}})
	}
	p := Composition{ID: "wrong-origin", Purpose: "수용인원", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, Measures: []Measure{{As: "capacity_number", Op: "sum_fields", Fields: []string{"o2.capacity", "o2.extra"}, Format: "decimal_v1", Unit: "persons"}}, Select: []string{"capacity_number"}, Roles: []RoleBinding{{Role: "shelters", Observation: "o1"}}, Outputs: []OutputBinding{{Output: "capacity", Field: "capacity_number"}}, Assumptions: []string{"fixture namespace/time only; unit interpretation needs review"}}
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "exploring" || v.Evaluation.Status != "partial" {
		t.Fatal("conversion bypassed output lineage")
	}
	p.ID = "correct-origin"
	p.Measures[0].Fields = []string{"o1.capacity", "o1.extra"}
	step(Decision{Action: "compose", Composition: &p})
	v = step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact.Rows[0]["capacity_number"] != json.Number("9007199254740994") || v.Artifact.Recipe.Measures[0].Unit != "persons" {
		t.Fatalf("%+v", v)
	}
	if e.PlanningView().Artifact != nil {
		t.Fatal("measure values leaked to planner")
	}
}

func TestGoalRequirementsKeepPartialCompositionExploring(t *testing.T) {
	ctx := context.Background()
	e, _ := Start("인구와 쉼터를 함께 비교할 표본", Policy{}, Dependencies{
		Search: func(_ context.Context, query string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: query, Title: query}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) { return Inspection{PK: pk}, nil },
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			return Acquired{Rows: []Row{{"code": "001", "value": s.PK}}, Delivery: "REST"}, nil
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
	contract := GoalContract{Outcome: "인구와 쉼터 표본 비교표", Region: "대한민국", Period: "관측시점 기준; 원천 기준일 별도 검토", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "population", Description: "인구 자료 값", Role: "people", Type: "string"}, {ID: "capacity", Description: "쉼터 자료 값", Role: "shelters", Type: "string"}}}
	step(Decision{Action: "define", Contract: &contract})
	for _, pk := range []string{"people", "mapping", "shelters"} {
		step(Decision{Action: "search", Query: pk, Role: pk})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "api"}})
	}
	partial := Composition{ID: "partial", Purpose: "인구와 대응만", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, Select: []string{"o1.value"}, Assumptions: []string{"같은 코드의 표본; 의미는 별도 검토"}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}}, Outputs: []OutputBinding{{Output: "population", Field: "o1.value"}}}
	step(Decision{Action: "compose", Composition: &partial})
	v := step(Decision{Action: "execute", CompositionID: "partial"})
	if v.Status != "exploring" || v.Artifact == nil || v.Evaluation == nil || v.Evaluation.Status != "partial" {
		t.Fatalf("partial stopped exploration: %+v", v)
	}
	if e.PlanningView().Evaluation == nil || e.PlanningView().Artifact != nil {
		t.Fatal("planner must receive computed gaps, not raw partial rows")
	}
	contract.Roles = contract.Roles[:1]
	contract.Outputs = contract.Outputs[:1]
	v = step(Decision{Action: "define", Contract: &contract})
	if len(v.Contract.Roles) != 2 {
		t.Fatal("planner shrank original requirements")
	}
	full := partial
	full.ID = "full"
	full.Joins = append(full.Joins, Join{Right: "o3", LeftKeys: []string{"o2.code"}, RightKeys: []string{"code"}})
	full.Select = append(full.Select, "o3.value")
	full.Roles = append(full.Roles, RoleBinding{Role: "shelters", Observation: "o3"})
	full.Outputs = append(full.Outputs, OutputBinding{Output: "capacity", Field: "o3.value"})
	step(Decision{Action: "compose", Composition: &full})
	v = step(Decision{Action: "execute", CompositionID: "full"})
	if v.Status != "review_required" || v.Evaluation.Status != "requirements_met" || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("%+v", v)
	}
}

func TestRequirementsRejectUnusedObservationsAndWrongOutputLineage(t *testing.T) {
	c := GoalContract{Roles: []RoleRequirement{{ID: "people"}, {ID: "shelters"}}, Outputs: []OutputRequirement{{ID: "capacity", Role: "shelters", Type: "number"}}, Coverage: "sample"}
	p := Composition{Base: "o1", Joins: []Join{{Right: "o2"}}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "capacity", Field: "o1.value"}}}
	observations := []Observation{{ID: "o1", PK: "people"}, {ID: "o2", PK: "map"}, {ID: "o3", PK: "shelters"}}
	nodes := []Node{{Hit: catalog.Hit{PK: "people"}, Roles: []string{"people"}}, {Hit: catalog.Hit{PK: "shelters"}, Roles: []string{"shelters"}}}
	result := evaluateRequirements(c, p, []Row{{"o1.value": float64(100)}}, observations, nodes)
	if result.Status != "partial" {
		t.Fatalf("unused source or mislabelled output passed: %+v", result)
	}
}

func TestRequirementsNeverTurnSampleIntoPopulationProof(t *testing.T) {
	c := GoalContract{Coverage: "population", Roles: []RoleRequirement{{ID: "one"}}}
	r := evaluateRequirements(c, Composition{}, []Row{{"x": "1"}}, nil, nil)
	if r.Status != "partial" {
		t.Fatal("sample promoted to population")
	}
}

func TestRequiredOutputsRejectBlankStringsWithoutRejectingZeroOrFalse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		kind  string
		met   bool
	}{{"empty", "", "string", false}, {"whitespace", " \t\n\u3000", "string", false}, {"name", "경로당", "string", true}, {"zero", json.Number("0"), "number", true}, {"false", false, "boolean", true}} {
		t.Run(tc.name, func(t *testing.T) {
			c := GoalContract{Coverage: "sample", Roles: []RoleRequirement{{ID: "shelters"}}, Outputs: []OutputRequirement{{ID: "value", Role: "shelters", Type: tc.kind}}}
			p := Composition{Base: "o1", Roles: []RoleBinding{{Role: "shelters", Observation: "o1"}}, Outputs: []OutputBinding{{Output: "value", Field: "o1.value"}}}
			result := evaluateRequirements(c, p, []Row{{"o1.value": tc.value}}, []Observation{{ID: "o1", PK: "s"}}, []Node{{Hit: catalog.Hit{PK: "s"}, Roles: []string{"shelters"}}})
			if got := result.Status == "requirements_met"; got != tc.met {
				t.Fatalf("requirements met=%v want=%v: %+v", got, tc.met, result)
			}
		})
	}
}
