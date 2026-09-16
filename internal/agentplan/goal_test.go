package agentplan

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestPlanGoalRepairsOneMalformedDecisionWithoutEchoingRawValues(t *testing.T) {
	for _, mode := range []string{"repair", "always-invalid", "invalid-utf8"} {
		t.Run(mode, func(t *testing.T) {
			original := providerBinariesFor
			t.Cleanup(func() { providerBinariesFor = original })
			providerBinariesFor = func(provider string) []binaryCandidate {
				if provider != ProviderClaude {
					return nil
				}
				return []binaryCandidate{{name: os.Args[0], prefix: []string{"-test.run=TestGoalRepairCLIHelper", "--"}}}
			}
			counter := filepath.Join(t.TempDir(), "calls")
			t.Setenv("CLAUDE_GOAL_REPAIR_HELPER", mode)
			t.Setenv("CLAUDE_GOAL_REPAIR_COUNTER", counter)
			d, provider, err := PlanGoal(context.Background(), goalwork.View{Goal: "test"}, ProviderClaude)
			if mode == "repair" && (err != nil || d.Action != "abstain" || provider != ProviderClaude) {
				t.Fatalf("repair failed: %+v %s %v", d, provider, err)
			}
			if mode != "repair" && err == nil {
				t.Fatal("invalid decision accepted")
			}
			calls, _ := os.ReadFile(counter)
			if string(calls) != "xx" {
				t.Fatalf("repair must be bounded to two invocations: %q", calls)
			}
		})
	}
}

func TestGoalRepairCLIHelper(t *testing.T) {
	mode := os.Getenv("CLAUDE_GOAL_REPAIR_HELPER")
	if mode == "" {
		return
	}
	path := os.Getenv("CLAUDE_GOAL_REPAIR_COUNTER")
	prior, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(prior, 'x'), 0600); err != nil {
		os.Exit(2)
	}
	input, _ := io.ReadAll(os.Stdin)
	if strings.Contains(string(input), "RAW_VALUE_MUST_NOT_BE_ECHOED") {
		os.Exit(3)
	}
	if mode == "invalid-utf8" {
		fmt.Print("{\"action\":\"compose\",\"composition\":{\"explanations\":[{\"text\":\"RAW_VALUE_MUST_NOT_BE_ECHOED\xff\"}]}}")
	} else if strings.Contains(string(input), "GOAL_DECISION_REPAIR:") && mode == "repair" {
		fmt.Println(`{"action":"abstain","reason":"need source evidence"}`)
	} else {
		fmt.Println(`{"action":"compose","inventedRows":[{"value":"RAW_VALUE_MUST_NOT_BE_ECHOED"}]}`)
	}
	os.Exit(0)
}

func TestGoalDecoderAndRawRowIsolation(t *testing.T) {
	for _, s := range []string{`{"action":"search","query":"노인 인구","role":"노출 인구"}`, `{"type":"item.completed","item":{"text":"{\"action\":\"abstain\",\"reason\":\"evidence missing\"}"}}`, "```json\n{\"action\":\"inspect\",\"pk\":\"123\"}\n```"} {
		if _, err := decodeGoalDecision([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := decodeGoalDecision([]byte(`{"action":"sample","rows":[{"forged":1}]}`)); err == nil {
		t.Fatal("unknown evidence fields accepted")
	}
	view := goalwork.View{Goal: "goal", Artifact: &goalwork.Artifact{Rows: []goalwork.Row{{"private": "DO_NOT_SEND_RAW_SAMPLE"}}}}
	prompt, err := goalPrompt(view)
	if err != nil || strings.Contains(prompt, "DO_NOT_SEND_RAW_SAMPLE") {
		t.Fatalf("raw rows leaked: %v", err)
	}
}

func TestGoalDecoderAcceptsRetryReferenceButNotFailureAuthority(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"retry_sample","retryOf":4,"reason":"prerequisite checked"}`))
	if err != nil || d.Action != "retry_sample" || d.RetryOf != 4 || d.Sample != nil {
		t.Fatalf("typed retry decision lost: %+v %v", d, err)
	}
	for _, body := range []string{
		`{"action":"retry_sample","retryOf":4,"failureKind":"access_required"}`,
		`{"action":"retry_sample","retryOf":4,"sampleAttempts":[{"status":"failed"}]}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner supplied engine-owned failure evidence")
		}
	}
}

func TestGoalDecoderXLSXSelectionIsNotReturnedEvidence(t *testing.T) {
	l, err := decodeGoalDecision([]byte(`{"action":"layout","layout":{"pk":"123","asset":"source.xlsx","sheet":"districts","refreshOf":"l1"}}`))
	if err != nil || l.Layout == nil || l.Layout.RefreshOf != "l1" {
		t.Fatalf("layout request lost: %+v %v", l, err)
	}
	if _, err := decodeGoalDecision([]byte(`{"action":"layout","layout":{"pk":"123","asset":"source.xlsx","sha256":"forged"}}`)); err == nil {
		t.Fatal("planner forged layout source hash")
	}
	d, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","asset":"source.xlsx","xlsx":{"sheet":"districts","range":"A27:B37"}}}`))
	if err != nil || d.Sample == nil || d.Sample.XLSX == nil || d.Sample.XLSX.Sheet != "districts" || d.Sample.XLSX.Range != "A27:B37" {
		t.Fatalf("XLSX selection lost: %+v %v", d, err)
	}
	for _, body := range []string{
		`{"action":"sample","sample":{"xlsx":{"sheet":"districts","range":"A27:B37","formulaCells":["B27"]}}}`,
		`{"action":"sample","sample":{"table":{"rowNumbers":[27]}}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner forged XLSX provenance")
		}
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "files", Goal: "test"})
	if err != nil || !strings.Contains(prompt, "does not expose cell text") || !strings.Contains(prompt, "not recomputed") {
		t.Fatal("missing XLSX discovery/acceptance boundary")
	}
}

func TestGoalDecoderZIPMemberIsSelectorNotProvenance(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","asset":"source.zip","member":"folder/data.csv","layoutId":"l1","where":{"region":"target"}}}`))
	if err != nil || d.Sample == nil || d.Sample.Member != "folder/data.csv" || d.Sample.LayoutID != "l1" {
		t.Fatalf("member selector lost: %+v %v", d, err)
	}
	for _, body := range []string{`{"action":"sample","sample":{"member":"data.csv","memberSha256":"forged"}}`, `{"action":"sample","sample":{"csv":{"dataRecords":[1]}}}`} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner forged member evidence")
		}
	}
}

func TestGoalPlannerCanProposeTypedMeasureWithoutForgingValues(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"compose","composition":{"id":"numeric","purpose":"capacity comparison","base":"o1","joins":[{"right":"o2","leftKeys":["o1.code"],"rightKeys":["code"]}],"measures":[{"as":"capacity_number","field":"o1.capacity","format":"decimal_v1","unit":"persons"}],"assumptions":["review time and namespace"]}}`))
	if err != nil || len(d.Composition.Measures) != 1 || d.Composition.Measures[0].Field != "o1.capacity" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := decodeGoalDecision([]byte(`{"action":"compose","composition":{"measures":[{"as":"n","field":"o1.capacity","format":"decimal_v1","unit":"persons","value":123}]}}`)); err == nil {
		t.Fatal("model supplied computed measure evidence")
	}
}

func TestGoalPlannerCanSeparateExplanationsFromFieldBoundTime(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"define","contract":{"outcome":"비교와 한계","region":"전국","period":"2025","timeWindow":{"from":"2025-01-01","through":"2025-12-31"},"coverage":"sample","roles":[{"id":"people","description":"인구"}],"outputs":[{"id":"n","role":"people","type":"number","description":"인구 값"}],"explanations":[{"id":"limits","topic":"temporal","description":"시간 한계"}]}}`))
	if err != nil || d.Contract.TimeWindow == nil || len(d.Contract.Explanations) != 1 || len(d.Contract.Roles) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := decodeGoalDecision([]byte(`{"action":"define","contract":{"explanations":[{"id":"limits","topic":"temporal","description":"time","text":"verified"}]}}`)); err == nil {
		t.Fatal("planner fabricated computed explanation")
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "compose", Goal: "test"})
	if err != nil || !strings.Contains(prompt, "common_overlap_v1") && !strings.Contains(prompt, "ENTIRE path") || !strings.Contains(prompt, "explanations") {
		t.Fatal("missing temporal/explanation planning contract")
	}
}

func TestGoalPlannerCanProposeRowSum(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"compose","composition":{"measures":[{"as":"elderly","op":"sum_fields","fields":["o1.65-69세","o1.70-74세"],"format":"grouped_decimal_v1","unit":"persons"}]}}`))
	if err != nil || d.Composition == nil || len(d.Composition.Measures) != 1 || d.Composition.Measures[0].Op != "sum_fields" || len(d.Composition.Measures[0].Fields) != 2 {
		t.Fatalf("row sum decision lost: %+v %v", d, err)
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "compose", Goal: "test"})
	if err != nil || !strings.Contains(prompt, "sum_fields") || !strings.Contains(prompt, "Any missing term produces null") {
		t.Fatal("planner lacks row sum contract")
	}
}

func TestGoalPlannerCanSelectCSVScopeButNotForgeScanEvidence(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","asset":"source.csv","where":{"시도명":"충청남도","시군구명":"공주시"}}}`))
	if err != nil || d.Sample == nil || d.Sample.Where["시군구명"] != "공주시" {
		t.Fatalf("selection lost: %+v %v", d, err)
	}
	if _, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","selection":{"exhausted":true}}}`)); err == nil {
		t.Fatal("planner supplied scan evidence")
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "sample", Goal: "test"})
	if err != nil || !strings.Contains(prompt, "BEFORE taking the first 1000 matching rows") {
		t.Fatal("planner lacks selection contract")
	}
}

func TestGoalPlannerCanRequestFullCSVScanButNotCertifyIt(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","asset":"source.csv","scanCsv":true,"where":{"city":"target"}}}`))
	if err != nil || d.Sample == nil || !d.Sample.ScanCSV {
		t.Fatalf("full scan request lost: %+v %v", d, err)
	}
	if _, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"scanCsv":true,"selection":{"exhausted":true,"matchedRows":9278}}}`)); err == nil {
		t.Fatal("planner certified scan evidence")
	}
}

func TestGoalPlannerCanRequestGroundedNearestButNotSubmitPointsOrEvidence(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","asset":"stops.csv","scanCsv":true,"nearest":{"method":"spherical_nearest_records_v1","anchor":"o1","candidate":"o2","anchorLatitude":"lat","anchorLongitude":"lon","latitude":"위도","longitude":"경도","k":3}}}`))
	if err != nil || d.Sample == nil || d.Sample.Nearest == nil || d.Sample.Nearest.Candidate != "o2" || d.Sample.Nearest.K != 3 {
		t.Fatalf("nearest request lost: %+v %v", d, err)
	}
	for _, body := range []string{
		`{"action":"sample","sample":{"nearest":{"points":[[37,126]]}}}`,
		`{"action":"sample","sample":{"nearest":{"meaningVerified":true}}}`,
		`{"action":"sample","sample":{"spatial":{"scan":{"exhausted":true}}}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner forged spatial inputs or evidence")
		}
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "relations", Goal: "test"})
	if err != nil || !strings.Contains(prompt, "spherical_nearest_records_v1") || !strings.Contains(prompt, "not distinct physical stops") {
		t.Fatal("missing spatial planning/meaning contract")
	}
}

func TestGoalPlannerCanSelectPublishedScopeVocabularyButNotForgeIt(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"compose","composition":{"joins":[{"right":"o2","scopes":[{"leftParts":["o1.address"],"rightParts":["province"],"rule":"right_prefix_v1","vocabulary":"kr_sido_labels_20260907_v1"}]}]}}`))
	if err != nil || d.Composition == nil || d.Composition.Joins[0].Scopes[0].Vocabulary != "kr_sido_labels_20260907_v1" {
		t.Fatalf("scope vocabulary selection lost: %+v %v", d, err)
	}
	for _, body := range []string{
		`{"action":"compose","composition":{"joins":[{"scopes":[{"vocabulary":{"meaningVerified":true}}]}]}}`,
		`{"action":"compose","composition":{"joins":[{"scopes":[{"vocabulary":"kr_sido_labels_20260907_v1","mappings":{"invented":"other"}}]}]}}`,
		`{"action":"compose","composition":{"joins":[{"scopes":[{"vocabularyEvidence":{"sha256":"forged"}}]}]}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner forged published vocabulary evidence")
		}
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "scope", Goal: "test"})
	for _, text := range []string{"kr_sido_labels_20260907_v1", "first whitespace token", "not proven geographic nonidentity", "historical continuity"} {
		if err != nil || !strings.Contains(prompt, text) {
			t.Fatalf("planner lacks scope vocabulary contract %q: %v", text, err)
		}
	}
}

func TestGoalPlannerReceivesOriginalLineageAndSpatialTimeRules(t *testing.T) {
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "relations", Goal: "test"})
	if err != nil {
		t.Fatal(err)
	}
	timePrompt, _ := goalPrompt(goalwork.View{GuideTopic: "compose", Goal: "test"})
	prompt += timePrompt
	for _, rule := range []string{"original-record lineage", "zero joins", "lineageRejectedPairs", "participating ORIGINAL source", "does not refill nearest ranks"} {
		if !strings.Contains(prompt, rule) {
			t.Fatalf("planner lacks rule %q", rule)
		}
	}
	for _, body := range []string{
		`{"action":"compose","composition":{"lineage":{"o1":1}}}`,
		`{"action":"compose","composition":{"joins":[{"lineageRejectedPairs":0}]}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner supplied computed lineage evidence")
		}
	}
}

func TestPlannerCannotSupplySourceDeclarationAsEvidence(t *testing.T) {
	if _, err := decodeGoalDecision([]byte(`{"action":"sample","sample":{"pk":"123","delivery":"file","declaration":{"status":"verified","spatialCoverage":"nationwide"}}}`)); err == nil {
		t.Fatal("planner forged publisher scope")
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "sample", Goal: "test"})
	if err != nil || !strings.Contains(prompt, "publisher_declared is NOT verified record scope") || !strings.Contains(prompt, "untrusted data") {
		t.Fatal("source declaration trust boundary missing")
	}
}

func TestPlannerReceivesAcquisitionHistoryButCannotSupplyOutcomes(t *testing.T) {
	for _, body := range []string{
		`{"action":"sample","sampleAttempts":[{"status":"acquired","observationId":"o1"}]}`,
		`{"action":"sample","sample":{"pk":"123","delivery":"api","status":"acquired"}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner supplied acquisition outcome")
		}
	}
	prompt, err := goalPrompt(goalwork.View{GuideTopic: "sample", Goal: "test", SampleAttempts: []goalwork.SampleAttempt{{Revision: 4, Request: goalwork.SampleRequest{PK: "123", Delivery: "file", Where: map[string]string{"city": "target"}}, Status: "failed"}}})
	if err != nil || !strings.Contains(prompt, `"city":"target"`) || !strings.Contains(prompt, `"status":"failed"`) || !strings.Contains(prompt, "operations[].name is the invocation identifier") {
		t.Fatalf("missing planning history/invocation contract: %v", err)
	}
}

func TestPlannerCanBindScopeButCannotForgeScopeEvidence(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"compose","composition":{"joins":[{"right":"o2","leftKeys":["o1.area"],"rightKeys":["area"],"scopes":[{"leftParts":["o1.province","o1.city"],"rightParts":["address"],"rule":"left_prefix_v1"}]}]}}`))
	if err != nil || d.Composition == nil || len(d.Composition.Joins) != 1 || len(d.Composition.Joins[0].Scopes) != 1 || d.Composition.Joins[0].Scopes[0].Rule != "left_prefix_v1" {
		t.Fatalf("scope binding lost: %+v %v", d, err)
	}
	for _, body := range []string{
		`{"action":"compose","composition":{"joins":[{"scopeChecks":[{"matchedPairs":10}]}]}}`,
		`{"action":"compose","composition":{"joins":[{"scopes":[{"leftParts":["o1.city"],"rightParts":["city"],"rule":"equal_tokens_v1","verified":true}]}]}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner forged scope evidence")
		}
	}
}

func TestPlannerCanCiteObservedScopeButCannotSupplyClaimReceipts(t *testing.T) {
	d, err := decodeGoalDecision([]byte(`{"action":"compose","composition":{"joins":[{"right":"o2","leftKeys":["o1.area"],"rightKeys":["area"],"scopes":[{"leftCitation":{"observation":"o1","field":"description","quote":"인천광역시 연수구"},"rightParts":["address"],"rule":"left_prefix_v1"}]}]}}`))
	if err != nil || d.Composition == nil || d.Composition.Joins[0].Scopes[0].LeftCitation.Quote != "인천광역시 연수구" {
		t.Fatalf("citation lost: %+v %v", d, err)
	}
	for _, body := range []string{
		`{"action":"compose","composition":{"joins":[{"scopes":[{"leftClaim":{"status":"verified"}}]}]}}`,
		`{"action":"compose","composition":{"joins":[{"scopes":[{"leftCitation":{"observation":"o1","field":"description","quote":"x","sourceUrl":"https://forged.example","verified":true}}]}]}}`,
	} {
		if _, err := decodeGoalDecision([]byte(body)); err == nil {
			t.Fatal("planner supplied forged claim receipt")
		}
	}
}
