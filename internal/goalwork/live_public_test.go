package goalwork

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

// Explicitly opt-in, read-only hard-negative acquisition benchmark. A real
// attempted comparison failed because the geographic labels differ in grain.
// This preserves that failure, NOT a successful autonomous goal benchmark.
func TestLivePublicChungnamDirectJoinRejectsDifferentGeographicGrain(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_COMPOSITION") != "1" {
		t.Skip("opt-in public data acquisition and local semantic index required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	policy := Policy{RequireSemantic: true}
	e, err := Start("충남 고령인구와 쉼터의 지역별 표본 비교, 기간과 식별 가정 명시", policy, LiveDependencies(fetch.New(), "", nil, catalog.Searcher{}, policy))
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("action=%s gaps=%+v err=%v", d.Action, v.Gaps, err)
		}
		return v
	}
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "지역별 고령인구와 쉼터 이름 표본 비교", Region: "충청남도 표본", Period: "원천별 기준일 상이; 현재 운영 판단 아님", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "고령인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "region", Role: "people", Description: "집계 지역", Type: "string"}, {ID: "elderly", Role: "people", Description: "65세 이상 인구", Type: "number"}, {ID: "shelter", Role: "shelters", Description: "쉼터 이름", Type: "string"}}, Explanations: []ExplanationRequirement{{ID: "time_limits", Topic: "temporal", Description: "시점 차이"}, {ID: "identity_limits", Topic: "identity", Description: "지역명 비교 가정"}, {ID: "coverage_limits", Topic: "coverage", Description: "표본 범위"}}}})
	for _, source := range []struct{ pk, query, role string }{{"3062428", "충청남도 노인인구 현황", "people"}, {"15118638", "충청남도 재난안전포털 무더위쉼터", "shelters"}} {
		step(Decision{Action: "search", Query: source.query, Role: source.role})
		v := step(Decision{Action: "inspect", PK: source.pk})
		asset := ""
		for _, n := range v.Nodes {
			if n.Hit.PK == source.pk && n.Inspection != nil {
				for _, name := range n.Inspection.Assets {
					if strings.HasSuffix(strings.ToLower(name), ".csv") {
						asset = name
						break
					}
				}
			}
		}
		if asset == "" {
			t.Fatal("no inspected CSV asset for", source.pk)
		}
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: source.pk, Delivery: "file", Asset: asset}})
	}
	fields := []string{"o1.65-69세", "o1.70-74세", "o1.75-79세", "o1.80-84세", "o1.85-89세", "o1.90-94세", "o1.95-99세", "o1.100세이상"}
	p := Composition{ID: "chungnam-sample", Purpose: "자료별 시점이 다른 충남 표본의 지역명 기반 비교; 현재 운영 여부나 식별 검증 아님", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.구 분"}, RightKeys: []string{"시군구명"}, Normalization: "trim"}}, Measures: []Measure{{As: "elderly", Op: "sum_fields", Fields: fields, Format: "grouped_decimal_v1", Trim: true, Unit: "persons"}}, Select: []string{"o1.구 분", "elderly", "o2.쉼터명칭", "o2.연도"}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "region", Field: "o1.구 분"}, {Output: "elderly", Field: "elderly"}, {Output: "shelter", Field: "o2.쉼터명칭"}}, Assumptions: []string{"Both provider contracts scope these labels to Chungnam; names alone are not canonical identifiers.", "Eight named age bands are interpreted as disjoint 65+ counts in persons; field semantics need review.", "Population asset date and shelter row year may differ; no current availability or aligned-time claim."}}
	step(Decision{Action: "compose", Composition: &p})
	v, err := e.Advance(ctx, e.View().Revision, Decision{Action: "execute", CompositionID: p.ID})
	if err != nil || v.Status != "exploring" || v.Artifact != nil || len(v.Executions) != 1 || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "empty full join") {
		t.Fatalf("geographic-grain hard negative changed; inspect source drift, do not assume identity: status=%s gaps=%+v err=%v", v.Status, v.Gaps, err)
	}
	if v.Observations[0].ColumnProfiles["구 분"].MaxTokens != 1 || v.Observations[1].ColumnProfiles["시군구명"].MinTokens < 2 {
		t.Fatal("source shapes changed; inspect before revising benchmark")
	}
	report := struct {
		Status     string
		Sources    []Observation
		Executions []ExecutionRecord
	}{v.Status, v.Observations, v.Executions}
	b, _ := json.Marshal(report)
	t.Log(string(b))
}

// Fixed sources and a predeclared Gongju sample: this isolates executable
// source-map acquisition, not autonomous discovery or nationwide coverage.
func TestLivePublicGongjuComparisonThroughOfficialMapping(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_COMPOSITION") != "1" {
		t.Skip("opt-in public data acquisition and local semantic index required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	policy := Policy{RequireSemantic: true}
	e, err := Start("공주시 고령인구와 쉼터의 표본 비교; 시점 차이와 식별 해석의 한계 포함", policy, LiveDependencies(fetch.New(), "", nil, catalog.Searcher{}, policy))
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("action=%s gaps=%+v err=%v", d.Action, v.Gaps, err)
		}
		return v
	}
	contract := GoalContract{Outcome: "공주시 고령인구와 쉼터 이름 표본 비교", Region: "공주시 표본 (전국 대표 아님)", Period: "원천별 기준일 상이; 현재 운영 판단 아님", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "고령인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "region", Role: "people", Description: "집계 지역", Type: "string"}, {ID: "elderly", Role: "people", Description: "65세 이상 인구", Type: "number"}, {ID: "shelter", Role: "shelters", Description: "쉼터 이름", Type: "string"}}, Explanations: []ExplanationRequirement{{ID: "time_limits", Topic: "temporal", Description: "시점 차이"}, {ID: "identity_limits", Topic: "identity", Description: "코드·지역명 비교 가정"}, {ID: "coverage_limits", Topic: "coverage", Description: "표본 범위"}}}
	step(Decision{Action: "define", Contract: &contract})
	for _, source := range []struct {
		pk, query, role string
		where           map[string]string
	}{
		{"3062428", "충청남도 노인인구 현황", "people", map[string]string{"구 분": "공주시"}},
		{"15118638", "충청남도 재난안전포털 무더위쉼터", "shelters", nil},
		{"15063424", "법정동 행정구역 코드 시도 시군구 읍면동", "mapping", map[string]string{"시도명": "충청남도", "시군구명": "공주시"}},
	} {
		step(Decision{Action: "search", Query: source.query, Role: source.role})
		v := step(Decision{Action: "inspect", PK: source.pk})
		asset := ""
		for _, n := range v.Nodes {
			if n.Hit.PK == source.pk && n.Inspection != nil {
				for _, name := range n.Inspection.Assets {
					if strings.HasSuffix(strings.ToLower(name), ".csv") {
						asset = name
						break
					}
				}
			}
		}
		if asset == "" {
			t.Fatal("no inspected CSV for", source.pk)
		}
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: source.pk, Delivery: "file", Asset: asset, Where: source.where}})
	}
	p := Composition{ID: "gongju-via-map", Purpose: "공주시 표본의 인구 맥락과 쉼터 이름 비교; 현재 운영·시간 정합성·전국 대표성 주장 아님", Base: "o1",
		Joins:    []Join{{Right: "o3", LeftKeys: []string{"o1.구 분"}, RightKeys: []string{"시군구명"}}, {Right: "o2", LeftKeys: []string{"o3.법정동코드"}, RightKeys: []string{"법정동코드"}}},
		Measures: []Measure{{As: "elderly", Op: "sum_fields", Fields: []string{"o1.65-69세", "o1.70-74세", "o1.75-79세", "o1.80-84세", "o1.85-89세", "o1.90-94세", "o1.95-99세", "o1.100세이상"}, Format: "grouped_decimal_v1", Trim: true, Unit: "persons"}},
		Select:   []string{"o1.구 분", "elderly", "o2.쉼터명칭", "o2.연도", "o3.법정동코드"},
		Roles:    []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "region", Field: "o1.구 분"}, {Output: "elderly", Field: "elderly"}, {Output: "shelter", Field: "o2.쉼터명칭"}},
		Assumptions: []string{"Population provider scope is Chungnam; official mapping is selected on 시도명=충청남도 AND 시군구명=공주시. Name equality is not canonical identity.", "The shelter and mapping legal-code fields are compared literally; namespace revisions/temporal correspondence remain unverified.", "Eight explicitly named age bands are interpreted as disjoint persons aged 65+; this is semantic interpretation, not independently certified.", "Source asset dates differ. No current facility operation or nationwide coverage claim. Repeated municipality population is context per shelter, not an additive total across output rows."},
	}
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) == 0 || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("no sample comparison: %+v", v.Evaluation)
	}
	if _, ok := v.Artifact.Rows[0]["elderly"].(json.Number); !ok {
		t.Fatal("numeric elderly output missing")
	}
	if v.Observations[2].Selection == nil || !v.Observations[2].Selection.Exhausted || v.Observations[2].Selection.ScannedRows <= 1000 {
		t.Fatal("official mapping was not acquired beyond the initial prefix")
	}
	b, _ := json.Marshal(struct {
		Status     string
		Rows       int
		Sources    []Observation
		Metrics    []JoinMetric
		Examples   []Row
		Evaluation GoalEvaluation
	}{v.Status, len(v.Artifact.Rows), v.Artifact.Sources, v.Artifact.Metrics, v.Artifact.Rows[:min(3, len(v.Artifact.Rows))], *v.Evaluation})
	t.Log(string(b))
}

// Reacquires the three public sources behind the actual Kimcheon/Yeosu false
// candidate. This is a fixed-source hard negative, not autonomous discovery.
func TestLivePublicGimcheonScopeRejectsYeosuNameCollision(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_COMPOSITION") != "1" {
		t.Skip("opt-in public data acquisition and local semantic index required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	policy := Policy{RequireSemantic: true}
	e, err := Start("김천시 고령인구와 같은 지역 쉼터의 표본 비교", policy, LiveDependencies(fetch.New(), "", nil, catalog.Searcher{}, policy))
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("action=%s gaps=%+v err=%v", d.Action, v.Gaps, err)
		}
		return v
	}
	c := GoalContract{Outcome: "같은 지역 표본 비교", Region: "김천시", Period: "원천별 시점 상이", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "고령인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "region", Description: "인구 지역", Role: "people", Type: "string"}, {ID: "name", Description: "쉼터 이름", Role: "shelters", Type: "string"}}}
	step(Decision{Action: "define", Contract: &c})
	for _, source := range []struct {
		pk, query, role, delivery string
		where                     map[string]string
	}{
		{"15159663", "경상북도 김천시 65세이상 노인인구 현황", "people", "file", nil},
		{"15013199", "전국무더위쉼터표준데이터", "shelters", "standard", nil},
		{"15137835", "경기도 의정부시 빅데이터공유활용 행정동 코드", "mapping", "file", map[string]string{"시도명": "경상북도", "시군구명": "김천시"}},
	} {
		step(Decision{Action: "search", Query: source.query, Role: source.role})
		v := step(Decision{Action: "inspect", PK: source.pk})
		asset := ""
		if source.delivery == "file" {
			for _, n := range v.Nodes {
				if n.Hit.PK == source.pk && n.Inspection != nil {
					for _, name := range n.Inspection.Assets {
						if strings.HasSuffix(strings.ToLower(name), ".csv") {
							asset = name
							break
						}
					}
				}
			}
			if asset == "" {
				t.Fatal("no inspected CSV for", source.pk)
			}
		}
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: source.pk, Delivery: source.delivery, Asset: asset, Where: source.where}})
	}
	p := Composition{ID: "scoped-gimcheon", Purpose: "동명 충돌의 실제 주소 범위 대조", Base: "o1", Joins: []Join{
		{Right: "o3", LeftKeys: []string{"o1.시도", "o1.시군구", "o1.행정동명"}, RightKeys: []string{"시도명", "시군구명", "행정동명"}},
		{Right: "o2", LeftKeys: []string{"o3.법정동명"}, RightKeys: []string{"LEGALDONG_NM"}, Scopes: []JoinScope{{LeftParts: []string{"o1.시도", "o1.시군구"}, RightParts: []string{"LNMADR"}, Rule: "left_prefix_v1"}}},
	}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "region", Field: "o1.행정동명"}, {Output: "name", Field: "o2.SHLTR_NM"}}, Assumptions: []string{"Compare provider-declared province/city tokens to recorded address prefix; no guessed aliases, geographic parsing or identity certification.", "Record time, namespace revisions and full geographic coverage remain unverified."}}
	step(Decision{Action: "compose", Composition: &p})
	v, err := e.Advance(ctx, e.View().Revision, Decision{Action: "execute", CompositionID: p.ID})
	if err != nil || v.Status != "exploring" || v.Artifact != nil || len(v.Executions) != 1 || len(v.Executions[0].Metrics) != 2 || len(v.Executions[0].Metrics[1].ScopeChecks) != 1 {
		t.Fatalf("hard negative changed; inspect source drift before changing gate: %+v %v", v.Executions, err)
	}
	check := v.Executions[0].Metrics[1].ScopeChecks[0]
	if check.CandidatePairs != 9 || check.ConflictPairs != 9 || check.MatchedPairs != 0 || check.UnknownPairs != 0 {
		t.Fatalf("expected nine conflicting public name matches; source drift requires review: %+v", check)
	}
	b, _ := json.Marshal(struct {
		Status     string
		Sources    []Observation
		Executions []ExecutionRecord
	}{v.Status, v.Observations, v.Executions})
	t.Log(string(b))
}

// Fixed-source check for the missing parent columns in the autonomous Yeonsu
// candidate. The citation remains a proposed source-scope interpretation.
func TestLivePublicYeonsuComparisonRetainsCitedScopeProvenance(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_COMPOSITION") != "1" {
		t.Skip("opt-in public acquisition and semantic index required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	policy := Policy{RequireSemantic: true}
	e, err := Start("연수구 고령인구와 쉼터의 표본 비교 및 원천 범위 근거", policy, LiveDependencies(fetch.New(), "", nil, catalog.Searcher{}, policy))
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("%s: %+v %v", d.Action, v.Gaps, err)
		}
		return v
	}
	c := GoalContract{Outcome: "지역 표본 비교", Region: "인천광역시 연수구", Period: "원천 시점 상이; 현재 운영 아님", Coverage: "sample", Roles: []RoleRequirement{{ID: "people", Description: "고령인구"}, {ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "population", Description: "고령인구", Role: "people", Type: "number"}, {ID: "name", Description: "쉼터", Role: "shelters", Type: "string"}, {ID: "address", Description: "소재지", Role: "shelters", Type: "string"}}, Explanations: []ExplanationRequirement{{ID: "identity", Topic: "identity", Description: "원천 범위와 행 식별 해석"}, {ID: "time", Topic: "temporal", Description: "기록 시점 한계"}}}
	step(Decision{Action: "define", Contract: &c})
	for _, source := range []struct{ pk, query, role string }{
		{"15064935", "인천광역시 연수구 동별 독거노인 현황", "people"},
		{"15157635", "인천광역시 연수구 무더위쉼터 현황", "shelters"},
	} {
		step(Decision{Action: "search", Query: source.query, Role: source.role})
		v := step(Decision{Action: "inspect", PK: source.pk})
		asset := ""
		for _, n := range v.Nodes {
			if n.Hit.PK == source.pk && n.Inspection != nil {
				for _, name := range n.Inspection.Assets {
					if strings.HasSuffix(strings.ToLower(name), ".csv") {
						asset = name
						break
					}
				}
			}
		}
		if asset == "" {
			t.Fatal("no inspected CSV for", source.pk)
		}
		step(Decision{Action: "sample", Sample: &SampleRequest{PK: source.pk, Delivery: "file", Asset: asset}})
	}
	p := Composition{ID: "yeonsu-cited-scope", Purpose: "원천 설명에 근거한 지역 가설과 쉼터 주소의 대조; 의미·시간 미검증", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.행정동명"}, RightKeys: []string{"행정동"}, Normalization: "trim", Scopes: []JoinScope{{LeftCitation: &ScopeCitation{Observation: "o1", Field: "description", Quote: "인천광역시 연수구"}, RightParts: []string{"주소"}, Rule: "left_prefix_v1"}}}}, Measures: []Measure{{As: "elderly", Field: "o1.만65세이상인구수", Format: "decimal_v1", Unit: "persons"}}, Select: []string{"o1.행정동명", "elderly", "o2.쉼터명", "o2.주소"}, Roles: []RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "population", Field: "elderly"}, {Output: "name", Field: "o2.쉼터명"}, {Output: "address", Field: "o2.주소"}}, Assumptions: []string{"Population scope is a proposed interpretation of the retained description, not a fabricated record column or verified identity.", "Name equality and address prefix are not proof of historical administrative identity or current shelter availability.", "Different source dates; repeated population is context per facility, not an additive output total."}}
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 68 || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("source drift or incorrect acceptance: %+v", v.Evaluation)
	}
	check := v.Artifact.Metrics[0].ScopeChecks[0]
	if check.MatchedPairs != 68 || check.ConflictPairs != 0 || check.UnknownPairs != 0 || check.LeftClaim == nil || check.LeftClaim.Status != "proposed_scope" || check.LeftClaim.DeclarationSHA256 != digest(v.Observations[0].Declaration) || check.MeaningVerified {
		t.Fatalf("citation or comparison evidence lost: %+v", check)
	}
	b, _ := json.Marshal(struct {
		Status   string
		Rows     int
		Sources  []Observation
		Metrics  []JoinMetric
		Examples []Row
	}{v.Status, len(v.Artifact.Rows), v.Artifact.Sources, v.Artifact.Metrics, v.Artifact.Rows[:3]})
	t.Log(string(b))
}
