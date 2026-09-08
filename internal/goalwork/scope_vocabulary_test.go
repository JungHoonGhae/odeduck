package goalwork

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func vocabularyGoal(t *testing.T, left, right []Row, declarations ...SourceDeclaration) *Engine {
	t.Helper()
	e, err := Start("시설의 접근성 기록을 출처와 함께 비교", Policy{}, Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) {
			i := Inspection{PK: pk}
			if pk == "left" && len(declarations) > 0 {
				i.Declarations = map[string]SourceDeclaration{"file": declarations[0]}
			}
			return i, nil
		},
		Sample: func(_ context.Context, request SampleRequest, _ Inspection) (Acquired, error) {
			rows := left
			if request.PK == "right" {
				rows = right
			}
			return Acquired{Rows: rows, Delivery: "FILE", ContentSHA256: digest(rows)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	vocabularyStep(t, e, Decision{Action: "define", Contract: &GoalContract{Outcome: "comparison", Region: "source regions", Period: "unverified source snapshots", Coverage: "sample", Roles: []RoleRequirement{{ID: "facility", Description: "facility"}, {ID: "access", Description: "accessibility"}}, Outputs: []OutputRequirement{{ID: "name", Description: "name", Role: "facility", Type: "string"}, {ID: "access", Description: "observed accessibility", Role: "access", Type: "string"}}}})
	for _, pk := range []string{"left", "right"} {
		role := "facility"
		if pk == "right" {
			role = "access"
		}
		vocabularyStep(t, e, Decision{Action: "search", Query: pk, Role: role})
		vocabularyStep(t, e, Decision{Action: "inspect", PK: pk})
		vocabularyStep(t, e, Decision{Action: "sample", Sample: &SampleRequest{PK: pk, Delivery: "file", Asset: "fixture.csv"}})
	}
	return e
}

func vocabularyStep(t *testing.T, e *Engine, d Decision) View {
	t.Helper()
	v, err := e.Advance(context.Background(), e.View().Revision, d)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func vocabularyComposition(t *testing.T, id, vocabulary string) Composition {
	t.Helper()
	// JSON tests the same wire field even before the implementation knows it.
	var scope JoinScope
	b, _ := json.Marshal(map[string]any{"leftParts": []string{"o1.address"}, "rightParts": []string{"province", "district"}, "rule": "right_prefix_v1", "vocabulary": vocabulary})
	if err := json.Unmarshal(b, &scope); err != nil {
		t.Fatal(err)
	}
	return Composition{ID: id, Purpose: "source label comparison", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.name"}, RightKeys: []string{"name"}, Scopes: []JoinScope{scope}}}, Select: []string{"o1.name", "o1.address", "o2.province", "o2.access"}, Assumptions: []string{"label equivalence is not same-facility, current operation or temporal compatibility approval"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "access", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "name", Field: "o1.name"}, {Output: "access", Field: "o2.access"}}}
}

func TestGoalReplansWithPublishedSidoLabelsWithoutRewritingSources(t *testing.T) {
	left := []Row{{"name": "인천광역시립박물관", "address": "인천광역시 연수구 청량로160번길 26"}}
	right := []Row{{"name": "인천광역시립박물관", "province": "인천", "district": "연수구", "access": "fixture observation"}}
	e := vocabularyGoal(t, left, right)
	before := e.View().Observations
	literal := vocabularyComposition(t, "literal", "")
	vocabularyStep(t, e, Decision{Action: "compose", Composition: &literal})
	failed := vocabularyStep(t, e, Decision{Action: "execute", CompositionID: literal.ID})
	if failed.Artifact != nil || len(failed.Executions) != 1 || failed.Executions[0].Metrics[0].ScopeChecks[0].ConflictPairs != 1 {
		t.Fatal("literal comparison must retain the actual representation failure")
	}
	normalized := vocabularyComposition(t, "published-labels", "kr_sido_labels_20260907_v1")
	vocabularyStep(t, e, Decision{Action: "compose", Composition: &normalized})
	v := vocabularyStep(t, e, Decision{Action: "execute", CompositionID: normalized.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 1 {
		t.Fatalf("published spelling correspondence did not recover comparison: status=%s gaps=%+v", v.Status, v.Gaps)
	}
	row := v.Artifact.Rows[0]
	if row["o1.address"] != left[0]["address"] || row["o2.province"] != "인천" || digest(v.Observations) != digest(before) || len(v.Executions) != 2 || !v.Evaluation.NeedsSemanticReview {
		t.Fatal("normalization rewrote original evidence, failure history or acceptance")
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "청량로160번길") || !strings.Contains(string(b), "kr_sido_labels_20260907_v1") || !strings.Contains(string(b), "6bb5ea7101f87b4fa161f1ab9926e3a989f82e21ede73f9ebefdf8226486b1ff") {
		t.Fatal("planning must retain vocabulary provenance without source values")
	}
}

func TestScopeVocabularyDoesNotBecomeGlobalNameOrHistoricalIdentity(t *testing.T) {
	for _, tc := range []struct{ name, left, province, district, vocabulary string }{
		{"wrong province", "경기도 중구 fixture", "인천", "중구", "kr_sido_labels_20260907_v1"},
		{"wrong district", "인천광역시 서구 fixture", "인천", "연수구", "kr_sido_labels_20260907_v1"},
		{"word substring", "인천광역시청 연수구 fixture", "인천", "연수구", "kr_sido_labels_20260907_v1"},
		{"unlisted old name", "강원도 fixture", "강원", "fixture", "kr_sido_labels_20260907_v1"},
		{"not a city prefix", "기관 인천광역시 연수구", "인천", "연수구", "kr_sido_labels_20260907_v1"},
		{"numeric code is not label", "28 연수구 fixture", "인천", "연수구", "kr_sido_labels_20260907_v1"},
		{"unknown version", "인천광역시 연수구 fixture", "인천광역시", "연수구", "guessed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := vocabularyGoal(t, []Row{{"name": "same", "address": tc.left}}, []Row{{"name": "same", "province": tc.province, "district": tc.district, "access": "yes"}})
			p := vocabularyComposition(t, "test", tc.vocabulary)
			vocabularyStep(t, e, Decision{Action: "compose", Composition: &p})
			v := vocabularyStep(t, e, Decision{Action: "execute", CompositionID: p.ID})
			if v.Artifact != nil || len(v.Gaps) != 1 || v.Status != "exploring" {
				t.Fatalf("unsupported identity/label must remain unresolved: %+v", v)
			}
		})
	}
}

func TestPublishedVocabularyRequiresRowFieldsAndPreservesNameKeys(t *testing.T) {
	e := vocabularyGoal(t, []Row{{"name": "검단선사박물관", "address": "인천광역시 서구 fixture"}}, []Row{{"name": "인천광역시검단선사박물관", "province": "인천", "district": "서구", "access": "yes"}})
	p := vocabularyComposition(t, "names", "kr_sido_labels_20260907_v1")
	vocabularyStep(t, e, Decision{Action: "compose", Composition: &p})
	v := vocabularyStep(t, e, Decision{Action: "execute", CompositionID: p.ID})
	if v.Artifact != nil || v.Executions[0].Metrics[0].ScopeChecks[0].CandidatePairs != 0 {
		t.Fatal("scope vocabulary changed exact facility-name keys")
	}
	e = vocabularyGoal(t, []Row{{"name": "same", "address": "인천광역시 서구 fixture"}}, []Row{{"name": "same", "province": "인천", "district": "서구", "access": "yes"}}, SourceDeclaration{Status: "publisher_declared", Description: "인천광역시"})
	p = vocabularyComposition(t, "citation", "kr_sido_labels_20260907_v1")
	p.Joins[0].Scopes[0].LeftParts = nil
	p.Joins[0].Scopes[0].LeftCitation = &ScopeCitation{Observation: "o1", Field: "description", Quote: "인천광역시"}
	vocabularyStep(t, e, Decision{Action: "compose", Composition: &p})
	v = vocabularyStep(t, e, Decision{Action: "execute", CompositionID: p.ID})
	if v.Artifact != nil || !strings.Contains(v.Gaps[0].Detail, "row-bound") {
		t.Fatal("vocabulary laundered a source-scope hypothesis into row text")
	}
}

func TestEveryPublishedSidoLabelCanBeComparedWithoutChangingItsLexeme(t *testing.T) {
	var manifest struct {
		Labels []struct{ Full, Short string }
	}
	if err := json.Unmarshal(sidoVocabularyJSON, &manifest); err != nil || len(manifest.Labels) != 17 {
		t.Fatalf("invalid frozen vocabulary: %v", err)
	}
	for _, label := range manifest.Labels {
		t.Run(label.Full, func(t *testing.T) {
			e := vocabularyGoal(t, []Row{{"name": "same", "address": label.Full + " fixture"}}, []Row{{"name": "same", "province": label.Short, "district": "fixture", "access": "yes"}})
			p := vocabularyComposition(t, "labels", "kr_sido_labels_20260907_v1")
			vocabularyStep(t, e, Decision{Action: "compose", Composition: &p})
			v := vocabularyStep(t, e, Decision{Action: "execute", CompositionID: p.ID})
			if v.Artifact == nil || v.Artifact.Rows[0]["o2.province"] != label.Short || v.Status != "review_required" {
				t.Fatalf("published spelling pair failed or lost original text: %s %+v", label.Full, v.Gaps)
			}
		})
	}
}

func TestScopeVocabularyRetainsEveryExistingTokenRule(t *testing.T) {
	for _, tc := range []struct{ rule, address string }{
		{"equal_tokens_v1", "\t인천광역시  서구 "},
		{"left_prefix_v1", "인천광역시"},
		{"right_prefix_v1", "인천광역시 서구 fixture"},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			e := vocabularyGoal(t, []Row{{"name": "same", "address": tc.address}}, []Row{{"name": "same", "province": " 인천 ", "district": "서구", "access": "yes"}})
			p := vocabularyComposition(t, "token-rule", "kr_sido_labels_20260907_v1")
			p.Joins[0].Scopes[0].Rule = tc.rule
			vocabularyStep(t, e, Decision{Action: "compose", Composition: &p})
			v := vocabularyStep(t, e, Decision{Action: "execute", CompositionID: p.ID})
			if v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o1.address"] != tc.address || v.Artifact.Rows[0]["o2.province"] != " 인천 " {
				t.Fatalf("token rule or original whitespace lost: %+v", v.Gaps)
			}
			v.Artifact.Metrics[0].ScopeChecks[0].Vocabulary.Sources[0].SHA256 = "caller mutation"
			if e.View().Artifact.Metrics[0].ScopeChecks[0].Vocabulary.Sources[0].SHA256 == "caller mutation" {
				t.Fatal("vocabulary evidence view is not detached")
			}
		})
	}
}
