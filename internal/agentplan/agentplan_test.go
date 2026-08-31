package agentplan

import (
	"context"
	"os"
	"path/filepath"
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
			spec := providerCommand(tt.provider, tt.provider, nil, "/tmp/gongctl-test", "goal")
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
		spec := providerCommand(provider, provider, nil, "/tmp/gongctl-test", sensitive)
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
	dir := t.TempDir()
	writeAgent := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeAgent("codex", `echo '{"error":"not authenticated"}' >&2; exit 1`)
	writeAgent("claude", `echo '{"concepts":["공매 물건","낙찰 가격","입찰 결과"]}'`)
	t.Setenv("PATH", dir)

	plan, err := Generate(context.Background(), "수익 기회", ProviderAuto)
	if err != nil {
		t.Fatalf("Generate(auto): %v", err)
	}
	if plan.Provider != ProviderClaude {
		t.Fatalf("provider = %q, want claude fallback", plan.Provider)
	}
}
