package goalwork

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

// Reduced hard negative from the 2026-09-07 natural-language run: Kimcheon
// population (15159663) joined Yeosu shelters (15013199) through the name 남면.
// A literal join and valid output types must not mean that the goal is solved.
func TestSameNameInDifferentMunicipalitiesCannotFinishGoal(t *testing.T) {
	rows := map[string][]Row{
		"people":   {{"area": "남면", "city": "김천시", "population": json.Number("1390")}},
		"shelters": {{"area": "남면", "city": "여수시", "name": "두포경로당"}},
	}
	e, err := Start("같은 지역의 고령인구와 쉼터 표본 비교", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) { return Inspection{PK: pk}, nil },
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			return Acquired{Rows: rows[s.PK], Delivery: "FILE"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := GoalContract{Outcome: "같은 지역 비교", Region: "전국 표본", Period: "원천별 시점", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "population", Description: "인구", Role: "people", Type: "number"}, {ID: "name", Description: "쉼터", Role: "shelters", Type: "string"}}}
	decisions := []Decision{{Action: "define", Contract: &c}}
	for _, pk := range []string{"people", "shelters"} {
		decisions = append(decisions, Decision{Action: "search", Query: pk, Role: pk}, Decision{Action: "inspect", PK: pk}, Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "file", Asset: "fixture.csv"}})
	}
	p := Composition{ID: "same-name", Purpose: "compare", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.area"}, RightKeys: []string{"area"}}}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "population", Field: "o1.population"}, {Output: "name", Field: "o2.name"}}, Assumptions: []string{"Names match; municipality identity and time remain unverified."}}
	decisions = append(decisions, Decision{Action: "compose", Composition: &p}, Decision{Action: "execute", CompositionID: p.ID}, Decision{Action: "abstain", Reason: "same-name records do not establish municipal identity"})
	next := 0
	v, err := Run(context.Background(), e, func(context.Context, View) (Decision, error) { d := decisions[next]; next++; return d, nil }, nil)
	if err != nil || v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Evaluation.Status != "requirements_met" {
		t.Fatalf("hard negative did not exercise structurally valid join: %+v %v", v, err)
	}
	if v.Status != "abstained" || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("same-name cross-city candidate incorrectly finished goal: status=%s evaluation=%+v", v.Status, v.Evaluation)
	}
}
