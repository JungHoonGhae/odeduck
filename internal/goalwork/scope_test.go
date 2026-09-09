package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestScopeTokenRulesPreserveBoundariesAndUnknowns(t *testing.T) {
	for _, tc := range []struct {
		name, rule                 string
		left, right                Row
		matched, conflict, unknown int
	}{
		{"prefix", "left_prefix_v1", Row{"a": "경상북도", "b": "김천시"}, Row{"value": "경상북도 김천시 남면 fixture"}, 1, 0, 0},
		{"wrong city", "left_prefix_v1", Row{"a": "경상북도", "b": "김천시"}, Row{"value": "전라남도 여수시 남면 fixture"}, 0, 1, 0},
		{"token boundary", "left_prefix_v1", Row{"a": "경상북도", "b": "김천시"}, Row{"value": "경상북도 김천시청"}, 0, 1, 0},
		{"whitespace", "equal_tokens_v1", Row{"a": " 경상북도\t", "b": "김천시"}, Row{"value": "\u3000경상북도  김천시 "}, 1, 0, 0},
		{"equal is not prefix", "equal_tokens_v1", Row{"a": "P", "b": "C"}, Row{"value": "P C extra"}, 0, 1, 0},
		{"reverse prefix", "right_prefix_v1", Row{"a": "P C", "b": "street"}, Row{"value": "P C"}, 1, 0, 0},
		{"no alias guessing", "equal_tokens_v1", Row{"a": "경북", "b": "김천시"}, Row{"value": "경상북도 김천시"}, 0, 1, 0},
		{"null", "left_prefix_v1", Row{"a": "P", "b": "C"}, Row{"value": nil}, 0, 0, 1},
		{"blank", "left_prefix_v1", Row{"a": "P", "b": " "}, Row{"value": "P C"}, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.left["key"], tc.right["key"] = "same", "same"
			p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.key"}, RightKeys: []string{"key"}, Scopes: []JoinScope{{LeftParts: []string{"l.a", "l.b"}, RightParts: []string{"value"}, Rule: tc.rule}}}}}
			rows, metrics, err := execute(p, map[string][]Row{"l": {tc.left}, "r": {tc.right}}, 1000)
			if (err == nil) != (tc.matched == 1) || len(rows) != tc.matched || len(metrics) != 1 || len(metrics[0].ScopeChecks) != 1 {
				t.Fatalf("%+v %+v %v", rows, metrics, err)
			}
			c := metrics[0].ScopeChecks[0]
			if c.CandidatePairs != 1 || c.MatchedPairs != tc.matched || c.ConflictPairs != tc.conflict || c.UnknownPairs != tc.unknown || c.MeaningVerified {
				t.Fatalf("incorrect pair accounting: %+v", c)
			}
		})
	}
}

func TestAllScopeChecksApplyAndTheirCountsAreNotDistinctRecords(t *testing.T) {
	p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.key"}, RightKeys: []string{"key"}, Scopes: []JoinScope{
		{LeftParts: []string{"l.city"}, RightParts: []string{"city"}, Rule: "equal_tokens_v1"},
		{LeftParts: []string{"l.province"}, RightParts: []string{"province"}, Rule: "equal_tokens_v1"},
	}}}}
	rows, metrics, err := execute(p, map[string][]Row{"l": {{"key": "x", "city": "C", "province": "P"}}, "r": {{"key": "x", "city": "other", "province": nil}}}, 1000)
	if err == nil || rows != nil || len(metrics) != 1 || len(metrics[0].ScopeChecks) != 2 {
		t.Fatalf("%+v %+v %v", rows, metrics, err)
	}
	a, b := metrics[0].ScopeChecks[0], metrics[0].ScopeChecks[1]
	if a.ConflictPairs != 1 || b.UnknownPairs != 1 || a.CandidatePairs != 1 || b.CandidatePairs != 1 {
		t.Fatalf("short-circuited or conflated scope evidence: %+v", metrics)
	}
}

func TestScopeChecksPrecedeExpansionAndCannotDisappearInProjection(t *testing.T) {
	inputs := map[string][]Row{}
	for i := 0; i < 200; i++ {
		inputs["l"] = append(inputs["l"], Row{"key": "shared-name", "scope": fmt.Sprint(i)})
		inputs["r"] = append(inputs["r"], Row{"key": "shared-name", "scope": fmt.Sprint(i)})
	}
	p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.key"}, RightKeys: []string{"key"}, Scopes: []JoinScope{{LeftParts: []string{"l.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}}}, Aggregates: []Aggregate{{As: "count", Op: "count"}}, Select: []string{"count"}}
	rows, metrics, err := execute(p, inputs, 1000)
	if err != nil || len(rows) != 1 || rows[0]["count"] != float64(200) || metrics[0].OutputRows != 200 || metrics[0].ScopeChecks[0].CandidatePairs != 40000 || metrics[0].ScopeChecks[0].ConflictPairs != 39800 {
		t.Fatalf("scope applied after expansion/projection or double counted: %v %+v %+v", err, rows, metrics)
	}
}

func TestScopeContractsFailClosedBeforeComparingValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scopes []JoinScope
		right  any
	}{
		{"unsupported", []JoinScope{{LeftParts: []string{"l.scope"}, RightParts: []string{"scope"}, Rule: "fuzzy"}}, "P C"},
		{"missing field", []JoinScope{{LeftParts: []string{"l.missing"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}, "P C"},
		{"duplicate field", []JoinScope{{LeftParts: []string{"l.scope", "l.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}, "P C"},
		{"empty field set", []JoinScope{{RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}, "P C"},
		{"numeric coercion", []JoinScope{{LeftParts: []string{"l.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}, json.Number("1")},
		{"large value", []JoinScope{{LeftParts: []string{"l.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}, strings.Repeat("x", 2049)},
		{"token ceiling", []JoinScope{{LeftParts: []string{"l.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}, strings.Repeat("x ", 65)},
		{"scope ceiling", make([]JoinScope, 5), "P C"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.key"}, RightKeys: []string{"key"}, Scopes: tc.scopes}}}
			rows, _, err := execute(p, map[string][]Row{"l": {{"key": "x", "scope": "P C"}}, "r": {{"key": "x", "scope": tc.right}}}, 1000)
			if err == nil || rows != nil {
				t.Fatal("invalid scope produced rows")
			}
		})
	}
}

func TestScopeAndTimeConditionsBothGateCandidatePairs(t *testing.T) {
	p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.key"}, RightKeys: []string{"key"}, Scopes: []JoinScope{{LeftParts: []string{"l.scope"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}}}, Time: &TemporalAlignment{Bindings: []TimeBinding{{Observation: "l", FromField: "year", Format: "year_v1", Meaning: "reference_period"}, {Observation: "r", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}}
	rows, metrics, err := execute(p, map[string][]Row{"l": {{"key": "x", "scope": "P C", "year": "2025"}}, "r": {{"key": "x", "scope": "P C", "year": "2025"}, {"key": "x", "scope": "P C", "year": "2019"}, {"key": "x", "scope": "P other", "year": "2025"}, {"key": "x", "year": "2025"}}}, 1000)
	if err != nil || len(rows) != 1 || metrics[0].TemporalRejectedPairs != 1 {
		t.Fatalf("%+v %+v %v", rows, metrics, err)
	}
	c := metrics[0].ScopeChecks[0]
	if c.CandidatePairs != 4 || c.MatchedPairs != 2 || c.ConflictPairs != 1 || c.UnknownPairs != 1 {
		t.Fatalf("scope/time populations conflated: %+v", c)
	}
}

func TestRowBoundScopeRejectsCrossCityNamesAndKeepsReplanning(t *testing.T) {
	ctx := context.Background()
	inputs := map[string][]Row{
		"people":      {{"area": "남면", "province": "경상북도", "city": "김천시", "population": json.Number("1390")}},
		"shelters":    {{"area": "남면", "address": "전라남도 여수시 남면 두모리 1397-2", "name": "두포경로당"}},
		"alternative": {{"area": "남면", "address": "경상북도 김천시 남면 fixture", "name": "fixture alternative"}},
	}
	e, err := Start("같은 지역의 고령인구와 쉼터 표본 비교", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) { return Inspection{PK: pk}, nil },
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			return Acquired{Rows: inputs[s.PK], Delivery: "FILE"}, nil
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
	c := GoalContract{Outcome: "지역 표본 비교", Region: "전국 표본", Period: "원천별 시점", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "population", Description: "인구", Role: "people", Type: "number"}, {ID: "name", Description: "쉼터", Role: "shelters", Type: "string"}}}
	step(Decision{Action: "define", Contract: &c})
	for _, pk := range []string{"people", "shelters"} {
		step(Decision{Action: "search", Query: pk, Role: pk})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "file", Asset: "fixture.csv"}})
	}
	var p Composition
	if err := json.Unmarshal([]byte(`{"id":"scoped","purpose":"same-region candidate","base":"o1","joins":[{"right":"o2","leftKeys":["o1.area"],"rightKeys":["area"],"scopes":[{"leftParts":["o1.province","o1.city"],"rightParts":["address"],"rule":"left_prefix_v1"}]}],"select":["o1.population","o2.name"],"roles":[{"role":"people","observation":"o1"},{"role":"shelters","observation":"o2"}],"outputs":[{"output":"population","field":"o1.population"},{"output":"name","field":"o2.name"}],"assumptions":["Textual geographic scope only; meaning and time remain unverified."]}`), &p); err != nil {
		t.Fatal(err)
	}
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "exploring" || v.Artifact != nil || len(v.Executions) != 1 || len(v.Gaps) != 1 {
		t.Fatalf("cross-city name collision was not rejected for replanning: %+v", v)
	}
	b, _ := json.Marshal(e.PlanningView())
	if !strings.Contains(string(b), `"conflictPairs":1`) || strings.Contains(string(b), "두모리") || strings.Contains(string(b), "두포경로당") {
		t.Fatalf("scope feedback missing or raw values leaked: %s", b)
	}
	step(Decision{Action: "search", Query: "alternative", Role: "shelters"})
	step(Decision{Action: "inspect", PK: "alternative"})
	step(Decision{Action: "sample", Sample: &SampleRequest{PK: "alternative", Delivery: "file", Asset: "fixture.csv"}})
	p.ID = "alternative"
	p.Joins[0].Right = "o3"
	p.Select[1] = "o3.name"
	p.Roles[1].Observation = "o3"
	p.Outputs[1].Field = "o3.name"
	step(Decision{Action: "compose", Composition: &p})
	v = step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o3.name"] != "fixture alternative" || len(v.Executions) != 2 || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("scope-aware alternative or unverified identity boundary lost: %+v", v)
	}
}
