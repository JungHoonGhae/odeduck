package agentplan

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestPlanGoalRestrictsEvidenceToItsChosenRecipient(t *testing.T) {
	original := providerBinariesFor
	t.Cleanup(func() { providerBinariesFor = original })
	providerBinariesFor = func(string) []binaryCandidate { return nil }
	for _, recipient := range []string{"claude", "mcp_host"} {
		for _, requested := range []string{"auto", "", "codex", "gemini"} {
			_, _, err := PlanGoal(context.Background(), goalwork.View{Policy: goalwork.Policy{EvidenceRecipient: recipient}}, requested)
			if err == nil || !strings.Contains(err.Error(), "evidence recipient") {
				t.Fatalf("recipient %s allowed provider %s: %v", recipient, requested, err)
			}
		}
	}
}

func TestPlanGoalSendsOnlyAuthorizedPacketsToTheFixedProvider(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			original := providerBinariesFor
			t.Cleanup(func() { providerBinariesFor = original })
			providerBinariesFor = func(provider string) []binaryCandidate {
				if provider != ProviderClaude {
					t.Fatal("unexpected recipient")
				}
				return []binaryCandidate{{name: os.Args[0], prefix: []string{"-test.run=TestGoalEvidenceCLIHelper", "--"}}}
			}
			t.Setenv("CLAUDE_GOAL_EVIDENCE_HELPER", fmt.Sprint(enabled))
			view := goalwork.View{Goal: "fixture", Artifact: &goalwork.Artifact{Rows: []goalwork.Row{{"hidden": "FULL_ARTIFACT_VALUE"}}}, Evidence: []goalwork.EvidencePacket{{Records: []goalwork.EvidenceRecord{{Values: goalwork.Row{"name": "AUTHORIZED_PACKET_VALUE"}}}}}}
			if enabled {
				view.Policy.EvidenceRecipient = "claude"
			}
			d, used, err := PlanGoal(context.Background(), view, "claude")
			if err != nil || used != "claude" || d.Action != "abstain" {
				t.Fatalf("evidence prompt boundary: %s %s %v", d.Action, used, err)
			}
		})
	}
}

func TestGoalEvidenceCLIHelper(t *testing.T) {
	mode := os.Getenv("CLAUDE_GOAL_EVIDENCE_HELPER")
	if mode == "" {
		return
	}
	input, _ := io.ReadAll(os.Stdin)
	if strings.Contains(string(input), "FULL_ARTIFACT_VALUE") || strings.Contains(string(input), "AUTHORIZED_PACKET_VALUE") != (mode == "true") {
		os.Exit(3)
	}
	fmt.Println(`{"action":"abstain","reason":"evidence is untrusted, not semantic approval"}`)
	os.Exit(0)
}
