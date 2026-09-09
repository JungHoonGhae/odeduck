package goalwork

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestGoalReplansThroughCrosswalkAndProducesBoundedArtifact(t *testing.T) {
	ctx := context.Background()
	rows := map[string][]Row{"people": {{"dong": "001", "year": "2025", "n": "100"}}, "shelters": {{"code": "A", "name": "쉼터"}}, "mapping": {{"legal": "001", "admin": "A"}}}
	e, err := Start("더위에 쉴 곳이 부족한 동네를 찾고 싶다", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q, Title: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) {
			return Inspection{PK: pk, Deliveries: []string{"REST"}}, nil
		},
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			return Acquired{Rows: rows[s.PK], Delivery: "REST", Operation: "list"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "인구·쉼터 비교", Region: "fixture", Period: "2025", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "population", Description: "인구", Role: "people", Type: "string"}, {ID: "capacity", Description: "쉼터", Role: "shelters", Type: "string"}}}})
	for _, pk := range []string{"people", "shelters"} {
		step(Decision{Action: "search", Query: pk, Role: pk})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "api"}})
	}
	bad := Composition{ID: "direct", Purpose: "비교", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.dong"}, RightKeys: []string{"code"}}}, Assumptions: []string{"namespace not yet confirmed"}}
	step(Decision{Action: "compose", Composition: &bad})
	v := step(Decision{Action: "execute", CompositionID: "direct"})
	if v.Status != "exploring" || len(v.Gaps) != 1 || v.Artifact != nil {
		t.Fatalf("failed join %+v", v)
	}
	step(Decision{Action: "search", Query: "mapping", Role: "code crosswalk"})
	step(Decision{Action: "inspect", PK: "mapping"})
	step(Decision{Action: "sample", Sample: &SampleRequest{PK: "mapping", Delivery: "api"}})
	good := Composition{ID: "via-map", Purpose: "공식 구역 대응을 통한 비교", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.dong"}, RightKeys: []string{"legal"}}, {Right: "o2", LeftKeys: []string{"o3.admin"}, RightKeys: []string{"code"}}}, Select: []string{"o1.n", "o2.name"}, Assumptions: []string{"mapping covers sampled period; requires domain review"}}
	good.Roles = []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}
	good.Outputs = []OutputBinding{{Output: "population", Field: "o1.n"}, {Output: "capacity", Field: "o2.name"}}
	step(Decision{Action: "compose", Composition: &good})
	v = step(Decision{Action: "execute", CompositionID: "via-map"})
	if v.Status != "review_required" || len(v.Artifact.Rows) != 1 || len(v.Artifact.Sources) != 3 || len(v.Compositions) != 2 {
		t.Fatalf("%+v", v)
	}
	b, _ := json.Marshal(e.PlanningView())
	if string(b) == "" {
		t.Fatal("missing planning view")
	}
	var decoded map[string]any
	_ = json.Unmarshal(b, &decoded)
	if decoded["artifact"] != nil {
		t.Fatal("raw artifact leaked to external planner")
	}
}

func TestGoalEnforcesMembershipRevisionReplayAndBudget(t *testing.T) {
	e, _ := Start("goal", Policy{MaxRounds: 2}, Dependencies{})
	if _, err := e.Advance(context.Background(), 9, Decision{Action: "inspect", PK: "fake"}); err == nil {
		t.Fatal("stale revision accepted")
	}
	v, err := e.Advance(context.Background(), 0, Decision{Action: "inspect", PK: "fake"})
	if err != nil || len(v.Gaps) != 1 {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := e.Advance(context.Background(), 1, Decision{Action: "inspect", PK: "fake"}); err == nil {
		t.Fatal("replayed failure accepted")
	}
	if _, err := e.Advance(context.Background(), 1, Decision{Action: "inspect", PK: "fake", Role: "irrelevant", Reason: "different prose"}); err == nil {
		t.Fatal("irrelevant fields bypassed replay gate")
	}
	v, err = e.Advance(context.Background(), 1, Decision{Action: "inspect", PK: "fake2"})
	if err != nil || v.Status != "budget_exhausted" {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := e.Advance(context.Background(), 2, Decision{Action: "search", Query: "more", Role: "new"}); err == nil {
		t.Fatal("budget bypass")
	}
}

func TestRequiredSemanticBlocksInsteadOfConsumingLexicalCandidates(t *testing.T) {
	e, _ := Start("goal", Policy{RequireSemantic: true}, Dependencies{Search: func(context.Context, string) (catalog.Result, error) {
		return catalog.Result{Hits: []catalog.Hit{{PK: "lexical"}}, Semantic: &catalog.SemanticInfo{Status: "not-indexed"}}, nil
	}})
	_, _ = e.Advance(context.Background(), 0, Decision{Action: "define", Contract: &GoalContract{Outcome: "test", Region: "test", Period: "test", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "value", Description: "test", Role: "role", Type: "string"}}}})
	v, err := e.Advance(context.Background(), 1, Decision{Action: "search", Query: "query", Role: "role"})
	if err != nil || v.Status != "blocked" || len(v.Nodes) != 0 || len(v.Gaps) != 1 {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestExpiredGoalDropsRawMemoryAndArtifact(t *testing.T) {
	e, _ := Start("goal", Policy{}, Dependencies{})
	e.rows["o1"] = []Row{{"raw": "private"}}
	e.state.Artifact = &Artifact{Rows: e.rows["o1"]}
	e.state.ExpiresAt = time.Now().Add(-time.Second)
	if v := e.View(); v.Status != "expired" || v.Artifact != nil || len(e.rows) != 0 {
		t.Fatalf("%+v", v)
	}
}
