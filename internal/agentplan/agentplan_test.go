package agentplan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/oddsock/internal/catalog"
)

func TestDecodePlanProviderShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"direct", `{"concepts":["공매 할인율","입찰 결과","낙찰 가격"],"summary":"거래 기회를 찾는다"}`},
		{"claude structured", `{"type":"result","structured_output":{"concepts":["상권 매출","유동 인구","폐업 현황"]}}`},
		{"gemini response", "{\"response\":\"```json\\n{\\\"concepts\\\":[\\\"도매 경락가\\\",\\\"농산물 출하량\\\",\\\"소매 가격\\\"]}\\n```\"}"},
		{"codex jsonl", "{\"type\":\"thread.started\"}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"{\\\"concepts\\\":[\\\"입찰 공고\\\",\\\"계약 실적\\\",\\\"발주 계획\\\"]}\"}}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := decodePlan([]byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Concepts) != 3 {
				t.Fatalf("concepts = %#v", plan.Concepts)
			}
		})
	}
}

func TestDecodePlanDeduplicatesAndBounds(t *testing.T) {
	plan, err := decodePlan([]byte(`{"concepts":[" 공매   물건 ","공매 물건","입찰 결과","낙찰가","상권","지원사업","위험","인구","초과"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Concepts) != 8 {
		t.Fatalf("got %d concepts: %#v", len(plan.Concepts), plan.Concepts)
	}
	if plan.Concepts[0] != "공매 물건" {
		t.Fatalf("normalized concept = %q", plan.Concepts[0])
	}
}

func TestDecodeStructuredPlanPreservesRoleEdgeAndContribution(t *testing.T) {
	body := `{"axes":[
		{"role":"anchor","query":"공매 부동산"},
		{"role":"상권 수요","query":"상권 점포 개폐업","contribution":"쇠퇴 상권인지 구분",
		 "edge":{"kinds":["spatial","temporal"],"expectedKeys":["법정동코드","기준연월"]}}
	],"summary":"가격과 수요를 연결한다"}`
	plan, err := decodePlanWith([]byte(body), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Axes) != 2 || plan.Axes[1].Role != "상권 수요" {
		t.Fatalf("axes = %+v", plan.Axes)
	}
	if strings.Join(plan.Axes[1].Edge.Kinds, ",") != "spatial,temporal" {
		t.Fatalf("edge = %+v", plan.Axes[1].Edge)
	}
	if got := strings.Join(plan.Concepts, "|"); got != "공매 부동산|상권 점포 개폐업" {
		t.Fatalf("derived concepts = %q", got)
	}
}

func TestDecodeStructuredInitialPlanRequiresAnchor(t *testing.T) {
	_, err := decodePlanWith([]byte(`{"axes":[
		{"role":"가격","query":"실거래가"},
		{"role":"수요","query":"상권 매출"}
	]}`), 2, true)
	if err == nil || !strings.Contains(err.Error(), "anchor") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeStructuredInitialPlanRequiresThreeAxes(t *testing.T) {
	_, err := decodePlanWith([]byte(`{"axes":[
		{"role":"anchor","query":"공매 부동산"},
		{"role":"수요","query":"상권 매출"}
	]}`), 3, true)
	if err == nil || !strings.Contains(err.Error(), "최소 3개") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeStructuredPlanKeepsOneAxisPerRole(t *testing.T) {
	plan, err := decodePlanWith([]byte(`{"axes":[
		{"role":"anchor","query":"공매 부동산"},
		{"role":"수요","query":"상권 매출"},
		{"role":"수요","query":"유동 인구"}
	]}`), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Axes) != 2 || plan.Axes[1].Query != "상권 매출" {
		t.Fatalf("axes = %+v", plan.Axes)
	}
}

func TestBridgePromptUsesObservedHitsAndRejectsNoveltyAlone(t *testing.T) {
	prompt := bridgePrompt("공매 투자", Plan{Axes: []catalog.DiscoveryAxis{{Role: "anchor", Query: "공매 물건"}}}, []catalog.Hit{{
		PK: "1", Title: "온비드 공매 물건", Org: "한국자산관리공사", MatchedQuery: "공매 물건",
	}})
	for _, want := range []string{"post-retrieval", "온비드 공매 물건", "공통 key와 Incremental Value", "유효한 추가 축이 없으면"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("bridge prompt missing %q", want)
		}
	}
}

func TestDecodeSelectionPlanRejectsUnknownAndIncompleteCandidates(t *testing.T) {
	edge := catalog.EdgeHypothesis{Kinds: []string{"spatial", "temporal"}, ExpectedKeys: []string{"법정동코드", "기준연월"}}
	candidates := []catalog.Hit{
		{PK: "good", Role: "상권 수요", Contribution: "쇠퇴와 가격 기회를 구분", EdgeHypothesis: &edge},
		{PK: "incomplete", Role: "위험"},
	}
	output := []byte(`{"selections":[
		{"pk":"unknown","role":"수요","incrementalValue":"판단","whyCandidate":"근거","edge":{"kinds":["spatial"],"expectedKeys":["법정동코드"]}},
		{"pk":"incomplete","role":"위험","incrementalValue":"판단","whyCandidate":"","edge":{"kinds":["entity"],"expectedKeys":["id"]}},
		{"pk":"good","role":"위조 역할","incrementalValue":"위조 가치","whyCandidate":"점포 개폐업 preview, 부산 한정",
		 "edge":{"kinds":["entity"],"expectedKeys":["inventedId"]}}
	]}`)
	plan, err := decodeSelectionPlan(output, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selections) != 1 || plan.Selections[0].PK != "good" {
		t.Fatalf("selections = %+v", plan.Selections)
	}
	selection := plan.Selections[0]
	if selection.Role != "상권 수요" || selection.IncrementalValue != "쇠퇴와 가격 기회를 구분" ||
		strings.Join(selection.Edge.ExpectedKeys, ",") != "법정동코드,기준연월" {
		t.Fatalf("selection must inherit the retrieved hit contract: %+v", selection)
	}
}

func TestDecodeSelectionPlanProviderShapes(t *testing.T) {
	edge := catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}
	candidates := []catalog.Hit{{PK: "bridge", Role: "수요", Contribution: "수요 변화", EdgeHypothesis: &edge}}
	tests := []struct {
		name string
		body string
	}{
		{"direct", `{"selections":[{"pk":"bridge","whyCandidate":"제목 근거"}]}`},
		{"claude structured", `{"type":"result","structured_output":{"selections":[{"pk":"bridge","whyCandidate":"제목 근거"}]}}`},
		{"gemini response", "{\"response\":\"```json\\n{\\\"selections\\\":[{\\\"pk\\\":\\\"bridge\\\",\\\"whyCandidate\\\":\\\"제목 근거\\\"}]}\\n```\"}"},
		{"codex jsonl", "{\"type\":\"thread.started\"}\n{\"type\":\"item.completed\",\"item\":{\"text\":\"{\\\"selections\\\":[{\\\"pk\\\":\\\"bridge\\\",\\\"whyCandidate\\\":\\\"제목 근거\\\"}]}\"}}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := decodeSelectionPlan([]byte(tt.body), candidates)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Selections) != 1 || plan.Selections[0].PK != "bridge" {
				t.Fatalf("selections = %+v", plan.Selections)
			}
		})
	}
}

func TestDecodeSelectionPlanTreatsNoValidChoiceAsAbstention(t *testing.T) {
	plan, err := decodeSelectionPlan([]byte(`{"selections":[]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selections) != 0 || plan.AbstentionReason == "" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDecodePlanRejectsOneAxis(t *testing.T) {
	_, err := decodePlan([]byte(`{"concepts":["공매","공매"]}`))
	if err == nil || !strings.Contains(err.Error(), "최소 2개") {
		t.Fatalf("err = %v", err)
	}
}

func TestProviderCommandsAreReadOnly(t *testing.T) {
	tests := []struct {
		provider string
		want     []string
	}{
		{ProviderCodex, []string{"--sandbox", "read-only", "--ephemeral", "--disable", "shell_tool"}},
		{ProviderClaude, []string{"--permission-mode", "dontAsk", "--safe-mode", "--restricted", "--tools"}},
		{ProviderGemini, []string{"--approval-mode", "plan", "--policy"}},
		{ProviderCursor, []string{"--mode", "ask"}},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			spec := providerCommand(tt.provider, tt.provider, nil, "/tmp/oddsock-test", "goal")
			joined := strings.Join(spec.args, " ")
			for _, want := range tt.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("args %q do not contain %q", joined, want)
				}
			}
		})
	}
}

func TestProviderCommandsDisableToolsForUntrustedCatalogPrompts(t *testing.T) {
	codex := providerCommand(ProviderCodex, "codex", nil, "/tmp/oddsock-test", "goal")
	if joined := strings.Join(codex.args, " "); !strings.Contains(joined, "--disable shell_tool") {
		t.Fatalf("Codex tools not disabled: %s", joined)
	}
	claude := providerCommand(ProviderClaude, "claude", nil, "/tmp/oddsock-test", "goal")
	if joined := strings.Join(claude.args, " "); !strings.Contains(joined, "--restricted") || !strings.Contains(joined, "--tools  --strict-mcp-config") {
		t.Fatalf("Claude tools not disabled: %s", joined)
	}
	gemini := providerCommand(ProviderGemini, "gemini", nil, "/tmp/oddsock-test", "goal")
	wantPolicy := filepath.Join("/tmp/oddsock-test", "deny-tools.toml")
	if !containsArgPair(gemini.args, "--policy", wantPolicy) {
		t.Fatalf("Gemini deny policy missing: %q", gemini.args)
	}
}

func containsArgPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestProviderEnvironmentDoesNotForwardUnrelatedSecrets(t *testing.T) {
	t.Setenv("ODDSOCK_TEST_SECRET", "must-not-leak")
	t.Setenv("CURSOR_API_KEY", "must-not-leak")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8443")
	env := strings.Join(providerEnvironment(ProviderCursor), "\n")
	if strings.Contains(env, "must-not-leak") || strings.Contains(env, "ODDSOCK_TEST_SECRET") || strings.Contains(env, "CURSOR_API_KEY") {
		t.Fatalf("minimal provider environment leaked a secret: %s", env)
	}
	if !strings.Contains(env, "PATH=/usr/bin") || !strings.Contains(env, "NO_COLOR=1") ||
		!strings.Contains(env, "HTTPS_PROXY=http://proxy.example:8443") {
		t.Fatalf("minimal provider environment omitted required settings: %s", env)
	}
}

func TestCursorNeverReceivesUntrustedCatalogMetadata(t *testing.T) {
	prior := Plan{Provider: ProviderCursor, Axes: []catalog.DiscoveryAxis{{Role: "anchor", Query: "공매"}}}
	if _, err := Expand(context.Background(), "목표", ProviderCursor, prior, []catalog.Hit{{Title: "외부 metadata"}}); !errors.Is(err, ErrUntrustedMetadataIsolation) {
		t.Fatalf("Expand cursor error = %v", err)
	}
	if _, err := Compose(context.Background(), "목표", ProviderCursor, nil, []catalog.Hit{{Title: "외부 metadata"}}); !errors.Is(err, ErrUntrustedMetadataIsolation) {
		t.Fatalf("Compose cursor error = %v", err)
	}
}

func TestProviderCommandsKeepGoalOutOfArgv(t *testing.T) {
	const sensitive = "미공개 신사업 목표"
	for _, provider := range providerOrder {
		spec := providerCommand(provider, provider, nil, "/tmp/oddsock-test", sensitive)
		if strings.Contains(strings.Join(spec.args, " "), sensitive) {
			t.Errorf("%s exposes the goal in argv: %q", provider, spec.args)
		}
		if spec.stdin != sensitive {
			t.Errorf("%s stdin = %q, want goal", provider, spec.stdin)
		}
	}
}

func TestCommandErrorDetailUnderstandsProviderJSON(t *testing.T) {
	stdout := []byte(`{"is_error":true,"result":"subscription disabled"}`)
	if got := commandErrorDetail(stdout, nil); got != "subscription disabled" {
		t.Fatalf("detail = %q", got)
	}
}

func TestPlanningPromptDoesNotAskAgentToClaimExistence(t *testing.T) {
	prompt := planningPrompt("돈이 될 만한 서비스")
	for _, want := range []string{"3~8개", "단순 동의어", "실제 존재한다고 단정하지 않는다", "돈이 될 만한 서비스"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestGenerateAutoFallsBackAfterInstalledAgentFails(t *testing.T) {
	original := providerBinariesFor
	t.Cleanup(func() { providerBinariesFor = original })
	providerBinariesFor = func(provider string) []binaryCandidate {
		switch provider {
		case ProviderCodex, ProviderClaude:
			return []binaryCandidate{{
				name:   os.Args[0],
				prefix: []string{"-test.run=TestAgentCLIHelper", "--", provider},
			}}
		default:
			return nil
		}
	}
	t.Setenv("CLAUDE_AGENTPLAN_TEST_HELPER", "1")

	plan, err := Generate(context.Background(), "수익 기회", ProviderAuto)
	if err != nil {
		t.Fatalf("Generate(auto): %v", err)
	}
	if plan.Provider != ProviderClaude {
		t.Fatalf("provider = %q, want claude fallback", plan.Provider)
	}
}

func TestAgentCLIHelper(t *testing.T) {
	if os.Getenv("CLAUDE_AGENTPLAN_TEST_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg != "--" || i+1 >= len(os.Args) {
			continue
		}
		switch os.Args[i+1] {
		case ProviderCodex:
			fmt.Fprintln(os.Stderr, `{"error":"not authenticated"}`)
			os.Exit(1)
		case ProviderClaude:
			fmt.Println(`{"concepts":["공매 물건","낙찰 가격","입찰 결과"]}`)
			os.Exit(0)
		}
	}
	os.Exit(2)
}
