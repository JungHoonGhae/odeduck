package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestSourceScopeCitationIsGroundedWithoutInventingRecordColumns(t *testing.T) {
	ctx := context.Background()
	d := SourceDeclaration{Status: "publisher_declared", SourceURL: "https://www.data.go.kr/data/111/fileData.do", Description: "이 자료는 인천광역시 연수구 내 각 동별 노인 인구를 담은 통계입니다.", Notices: []string{"Mixed or historical coverage still requires review."}}
	e, err := Start("지역별 인구와 시설 비교", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) {
			return Inspection{PK: pk, Declarations: map[string]SourceDeclaration{"file": d}}, nil
		},
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			if s.PK == "111" {
				return Acquired{Delivery: "FILE", Rows: []Row{{"area": "A", "value": json.Number("3943")}}}, nil
			}
			return Acquired{Delivery: "FILE", Rows: []Row{{"area": "A", "address": "인천광역시 연수구 fixture", "name": "matched"}, {"area": "A", "address": "전라남도 여수시 fixture", "name": "RAW_FOREIGN_ROW_NOT_FOR_PLANNER"}}}, nil
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
	c := GoalContract{Outcome: "표본 비교", Region: "전국 표본", Period: "미확정", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "인구"}, {ID: "facilities", Description: "시설"}}, Outputs: []OutputRequirement{{ID: "value", Description: "인구", Role: "people", Type: "number"}, {ID: "name", Description: "시설", Role: "facilities", Type: "string"}}}
	step(Decision{Action: "define", Contract: &c})
	for i, pk := range []string{"111", "222"} {
		step(Decision{Action: "search", Query: pk, Role: []string{"people", "facilities"}[i]})
		step(Decision{Action: "inspect", PK: pk})
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "file", Asset: "fixture.csv"}})
	}
	var p Composition
	if err := json.Unmarshal([]byte(`{"id":"source-scope","purpose":"source-scope hypothesis","base":"o1","joins":[{"right":"o2","leftKeys":["o1.area"],"rightKeys":["area"],"scopes":[{"leftCitation":{"observation":"o1","field":"description","quote":"인천광역시 연수구"},"rightParts":["address"],"rule":"left_prefix_v1"}]}],"select":["o1.value","o2.name"],"roles":[{"role":"people","observation":"o1"},{"role":"facilities","observation":"o2"}],"outputs":[{"output":"value","field":"o1.value"},{"output":"name","field":"o2.name"}],"assumptions":["Source-scope interpretation is proposed, not verified record identity or temporal scope."]}`), &p); err != nil {
		t.Fatal(err)
	}
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o2.name"] != "matched" {
		t.Fatalf("source citation did not constrain actual candidate rows: status=%s gaps=%+v", v.Status, v.Gaps)
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "RAW_FOREIGN_ROW_NOT_FOR_PLANNER") || !strings.Contains(string(b), `"status":"proposed_scope"`) || !strings.Contains(string(b), `"declarationSha256":"`+digest(d)+`"`) || !strings.Contains(string(b), `"conflictPairs":1`) {
		t.Fatalf("grounded but unapproved claim or private-row isolation lost: %s", b)
	}
	if !v.Evaluation.NeedsSemanticReview || len(v.Observations[0].Columns) != 2 {
		t.Fatal("citation fabricated record fields or cleared semantic review")
	}
	if strings.Contains(fmt.Sprint(v.Artifact.Rows), "인천광역시") {
		t.Fatal("source scope became fabricated output data")
	}
	claim := v.Artifact.Metrics[0].ScopeChecks[0].LeftClaim
	if claim == nil || claim.PK != "111" || claim.SourceURL != d.SourceURL || claim.ObservedAt != v.Observations[0].ObservedAt || claim.ObservationSHA256 != digest(v.Observations[0]) || claim.FirstMatchByte != strings.Index(d.Description, "인천광역시 연수구") || claim.Occurrences != 1 || len(claim.ClaimSHA256) != 64 || claim.EvidenceSHA256 != digest(d.Description) {
		t.Fatalf("citation provenance incomplete: %+v", claim)
	}
	claim.Status = "verified"
	v.Observations[0].Declaration.Description = "tampered"
	fresh := e.View()
	if fresh.Artifact.Metrics[0].ScopeChecks[0].LeftClaim.Status != "proposed_scope" || fresh.Observations[0].Declaration.Description != d.Description {
		t.Fatal("returned view mutated source claim")
	}
}

func TestScopeCitationCannotBorrowOtherSourcesOrInventEvidence(t *testing.T) {
	base := ScopeCitation{Observation: "l", Field: "description", Quote: "P C"}
	decl := &SourceDeclaration{Status: "publisher_declared", Description: "P C", Provider: "P C", Name: "P C", SpatialCoverage: "P C"}
	for _, tc := range []struct {
		name   string
		change func(*JoinScope, *[]Observation)
	}{
		{"wrong side", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Observation = "r" }},
		{"unused source", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Observation = "unused" }},
		{"invented quote", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Quote = "different city" }},
		{"provider is not scope", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Field = "provider" }},
		{"title is not scope", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Field = "name" }},
		{"no field", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Field = "unknown" }},
		{"both inputs", func(s *JoinScope, _ *[]Observation) { s.LeftParts = []string{"l.scope"} }},
		{"empty quote", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Quote = " " }},
		{"large quote", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Quote = strings.Repeat("x", 257) }},
		{"invalid UTF-8", func(s *JoinScope, _ *[]Observation) { s.LeftCitation.Quote = string([]byte{0xff}) }},
		{"no declaration", func(_ *JoinScope, obs *[]Observation) { (*obs)[0].Declaration = nil }},
		{"no metadata", func(_ *JoinScope, obs *[]Observation) { *obs = nil }},
		{"duplicate metadata", func(_ *JoinScope, obs *[]Observation) { *obs = append(*obs, (*obs)[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			citation := base
			scope := JoinScope{LeftCitation: &citation, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}
			observations := []Observation{{ID: "l", Declaration: decl}, {ID: "r", Declaration: decl}, {ID: "unused", Declaration: decl}}
			tc.change(&scope, &observations)
			p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.id"}, RightKeys: []string{"id"}, Scopes: []JoinScope{scope}}}}
			rows, _, err := execute(p, map[string][]Row{"l": {{"id": "x", "scope": "P C"}}, "r": {{"id": "x", "scope": "P C"}}}, 1000, observations...)
			if err == nil || len(rows) != 0 {
				t.Fatalf("invalid citation produced candidate rows: %+v %v", rows, err)
			}
		})
	}
}

func TestQuotedNegationAndMixedCoverageNeverBecomeVerifiedScope(t *testing.T) {
	for _, description := range []string{"P C is excluded from this dataset.", "P C and P D are covered; records need their own scope."} {
		p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.id"}, RightKeys: []string{"id"}, Scopes: []JoinScope{{LeftParts: []string{"l.scope"}, RightCitation: &ScopeCitation{Observation: "r", Field: "description", Quote: "P C"}, Rule: "equal_tokens_v1"}}}}}
		declaration := &SourceDeclaration{Status: "publisher_declared", Description: description, Truncated: true}
		rows, metrics, err := execute(p, map[string][]Row{"l": {{"id": "x", "scope": "P C"}}, "r": {{"id": "x"}}}, 1000, Observation{ID: "r", Declaration: declaration})
		if err != nil || len(rows) != 1 {
			t.Fatalf("quotation not evaluated: %+v %v", metrics, err)
		}
		claim := metrics[0].ScopeChecks[0].RightClaim
		if metrics[0].ScopeChecks[0].MeaningVerified || claim.Status != "proposed_scope" || !claim.DeclarationTruncated || claim.EvidenceSHA256 != digest(description) {
			t.Fatal("quotation occurrence became semantic validation or erased source limitations")
		}
	}
}

func TestSourceScopeCorrectionChangesClaimHashWithoutRewritingEarlierClaim(t *testing.T) {
	p := Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: []string{"l.id"}, RightKeys: []string{"id"}, Scopes: []JoinScope{{LeftCitation: &ScopeCitation{Observation: "l", Field: "spatialCoverage", Quote: "P C"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}}}}
	input := map[string][]Row{"l": {{"id": "x"}}, "r": {{"id": "x", "scope": "P C"}}}
	observation := Observation{ID: "l", Declaration: &SourceDeclaration{Status: "publisher_declared", SpatialCoverage: "P C"}}
	_, first, err := execute(p, input, 1000, observation)
	if err != nil {
		t.Fatal(err)
	}
	before := first[0].ScopeChecks[0].LeftClaim
	beforeHash := before.ClaimSHA256
	observation.Declaration.Notices = []string{"Correction: scope may be incomplete."}
	_, second, err := execute(p, input, 1000, observation)
	if err != nil {
		t.Fatal(err)
	}
	after := second[0].ScopeChecks[0].LeftClaim
	if beforeHash == after.ClaimSHA256 || before.DeclarationSHA256 == after.DeclarationSHA256 || before.ObservationSHA256 == after.ObservationSHA256 || before.ClaimSHA256 != beforeHash || before.EvidenceSHA256 != after.EvidenceSHA256 {
		t.Fatal("source correction was lost, or rewrote earlier quotation evidence")
	}
}
