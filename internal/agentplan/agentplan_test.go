package agentplan

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
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
		{ProviderCodex, []string{"--sandbox", "read-only", "--ephemeral"}},
		{ProviderClaude, []string{"--permission-mode", "dontAsk", "--safe-mode"}},
		{ProviderGemini, []string{"--approval-mode", "plan"}},
		{ProviderCursor, []string{"--mode", "ask"}},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			spec := providerCommand(tt.provider, tt.provider, nil, "/tmp/opendatactl-test", "goal")
			joined := strings.Join(spec.args, " ")
			for _, want := range tt.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("args %q do not contain %q", joined, want)
				}
			}
		})
	}
}

func TestProviderCommandsKeepGoalOutOfArgv(t *testing.T) {
	const sensitive = "미공개 신사업 목표"
	for _, provider := range providerOrder {
		spec := providerCommand(provider, provider, nil, "/tmp/opendatactl-test", sensitive)
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
	t.Setenv("GO_WANT_AGENT_HELPER", "1")

	plan, err := Generate(context.Background(), "수익 기회", ProviderAuto)
	if err != nil {
		t.Fatalf("Generate(auto): %v", err)
	}
	if plan.Provider != ProviderClaude {
		t.Fatalf("provider = %q, want claude fallback", plan.Provider)
	}
}

func TestAgentCLIHelper(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_HELPER") != "1" {
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
