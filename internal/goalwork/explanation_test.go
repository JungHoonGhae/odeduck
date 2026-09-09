package goalwork

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestExplanationContractCannotReplaceDataRequirementsOrForgeTopics(t *testing.T) {
	c := GoalContract{Outcome: "표본 비교와 근거", Region: "fixture", Period: "미확정", Coverage: "sample", Roles: []RoleRequirement{{ID: "source", Description: "원천"}}, Outputs: []OutputRequirement{{ID: "value", Role: "source", Type: "number", Description: "수치"}}, Explanations: []ExplanationRequirement{{ID: "limits", Topic: "temporal", Description: "시간 한계"}}}
	if err := validateContract(c); err != nil {
		t.Fatal(err)
	}
	for _, req := range []ExplanationRequirement{{ID: "value", Topic: "temporal", Description: "데이터 값 대신 설명"}, {ID: "limits", Topic: "goal_solved", Description: "성공 선언"}, {ID: "limits", Topic: "temporal"}} {
		bad := c
		bad.Explanations = []ExplanationRequirement{req}
		if err := validateContract(bad); err == nil {
			t.Fatal("invalid explanation contract accepted")
		}
	}
	c.Explanations = append(c.Explanations, c.Explanations[0])
	if err := validateContract(c); err == nil {
		t.Fatal("duplicate explanation IDs accepted")
	}
}

func TestEvidenceExplanationsDistinguishUnexecutedChecksAndScope(t *testing.T) {
	var reqs []ExplanationRequirement
	for _, topic := range []string{"temporal", "coverage", "identity", "provenance", "measurement"} {
		reqs = append(reqs, ExplanationRequirement{ID: topic, Topic: topic, Description: topic})
	}
	p := Composition{Measures: []Measure{{As: "capacity", Unit: "persons"}}}
	sources := []Observation{{ID: "o1", PK: "300", RowCount: 1000}, {ID: "o2", PK: "301", RowCount: 10}}
	reports := explainEvidence(GoalContract{Explanations: reqs}, p, sources, 5, evaluateTemporal(p, nil), nil)
	if len(reports) != 5 || !strings.Contains(reports[0].Text, "계산하지 않았습니다") || !strings.Contains(reports[1].Text, "1010행") || !strings.Contains(reports[1].Text, "동일 모집단의 크기가 아닙니다") || !strings.Contains(reports[2].Text, "동일성은 검증되지 않았습니다") || !strings.Contains(reports[4].Text, "capacity=persons") {
		t.Fatalf("misleading evidence explanation: %+v", reports)
	}
}

func TestScopeExplanationUsesComputedCountsNotDeclaredBindings(t *testing.T) {
	c := GoalContract{Explanations: []ExplanationRequirement{{ID: "scope", Topic: "identity", Description: "scope limits"}}}
	p := Composition{Joins: []Join{{Scopes: []JoinScope{{Rule: "left_prefix_v1"}}}}}
	missing := explainEvidence(c, p, nil, 0, TemporalEvaluation{}, nil)
	if !strings.Contains(missing[0].Text, "검사하지 않았습니다") {
		t.Fatal("declared binding became computed evidence")
	}
	checked := explainEvidence(c, p, nil, 1, TemporalEvaluation{}, []JoinMetric{{ScopeChecks: []ScopeCheck{{ConflictPairs: 9, UnknownPairs: 2}}}})
	for _, want := range []string{"불일치 9건", "판정 불가 2건", "미등록 표기", "중복될 수", "실제 지역 동일성의 검증이 아닙니다"} {
		if !strings.Contains(checked[0].Text, want) {
			t.Fatalf("scope explanation lost %s: %+v", want, checked)
		}
	}
}

func TestSpatialArtifactExplanationNamesExecutedCandidateBoundaryWithoutRowValues(t *testing.T) {
	e, s := nearestFixtureRows(t, "", []Row{{"id": "private-anchor", "lat": "0", "lon": "0"}}, ExplanationRequirement{ID: "mobility", Topic: "measurement", Description: "actual route limits"}, ExplanationRequirement{ID: "identity", Topic: "identity", Description: "executed identity checks"})
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatalf("nearest setup: %+v %v", v.Gaps, err)
	}
	p := Composition{ID: "distances", Purpose: "conditional distance table", Base: "o3", Select: []string{"o3.anchor.id", "o3.distance_m"}, Assumptions: []string{"fixture only"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "facility", Field: "o3.anchor.id"}, {Output: "distance", Field: "o3.distance_m"}}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if v, err := e.Advance(context.Background(), e.View().Revision, d); err != nil || len(v.Gaps) != 0 {
			t.Fatalf("execution: %+v %v", v.Gaps, err)
		}
	}
	v := e.View()
	if v.Artifact == nil || len(v.Evaluation.Explanations) != 2 || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("missing conditional artifact: %+v", v)
	}
	identity := v.Evaluation.Explanations[1].Text
	if strings.Contains(identity, "복합키 일치를 계산") || !strings.Contains(identity, "추가 키 결합은 실행하지 않았습니다") {
		t.Fatalf("zero-join projection claimed key matching: %s", identity)
	}
	report := v.Evaluation.Explanations[0].Text
	for _, want := range []string{"o3", "o1", "o2", "PK=222", "spherical_nearest_records_v1", "6371008.8", "1002행", "1002회", "2쌍", "전체 교통망", "휠체어", "가까움", "좌표계"} {
		if !strings.Contains(report, want) {
			t.Fatalf("missing executed spatial boundary %q: %s", want, report)
		}
	}
	for _, absent := range []string{"수치 sum은", "private-anchor", "BIS1001", "111195."} {
		if strings.Contains(report, absent) {
			t.Fatalf("unexecuted operation or raw row exposed: %s", report)
		}
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "private-anchor") || strings.Contains(string(b), "BIS1001") {
		t.Fatal("raw spatial values reached external planning view")
	}
}

func TestMeasurementExplanationFollowsExecutedOperations(t *testing.T) {
	c := GoalContract{Explanations: []ExplanationRequirement{{ID: "units", Topic: "measurement"}}}
	for _, tc := range []struct {
		name string
		p    Composition
		want string
	}{
		{"projection", Composition{}, "수치 변환·집계를 실행하지 않았습니다"},
		{"count", Composition{Aggregates: []Aggregate{{Op: "count"}}}, "count는 조합 결과의 행"},
		{"sum", Composition{Aggregates: []Aggregate{{Op: "sum"}}}, "수치 sum은 정확한 십진 계산"},
		{"convert", Composition{Measures: []Measure{{As: "n", Unit: "persons"}}}, "수치 변환 1개"},
		{"sum_fields", Composition{Measures: []Measure{{As: "n", Op: "sum_fields", Unit: "persons"}}}, "행 내부 필드 합산 1개"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := explainEvidence(c, tc.p, nil, 1, TemporalEvaluation{}, nil)[0].Text
			if !strings.Contains(report, tc.want) || (tc.name != "sum" && strings.Contains(report, "수치 sum은")) {
				t.Fatalf("misleading operation explanation: %s", report)
			}
		})
	}
}

func TestSourceCitationExplanationDoesNotCallQuoteLocationIdentityEvidence(t *testing.T) {
	c := GoalContract{Explanations: []ExplanationRequirement{{ID: "scope", Topic: "identity", Description: "scope"}}}
	claim := &CitedScopeClaim{ClaimSHA256: "one", Status: "proposed_scope"}
	report := explainEvidence(c, Composition{}, nil, 1, TemporalEvaluation{}, []JoinMetric{{ScopeChecks: []ScopeCheck{{LeftClaim: claim, RightClaim: claim}}}})
	for _, want := range []string{"인용 가설 1개", "부정 표현", "혼합 범위", "개별 행의 범위는 검증하지 않았습니다"} {
		if !strings.Contains(report[0].Text, want) {
			t.Fatalf("missing claim limitation: %s", want)
		}
	}
}
