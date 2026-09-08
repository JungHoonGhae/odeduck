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

func TestPlanGoalDeliversSharedContractWithCLIFraming(t *testing.T) {
	original := providerBinariesFor
	t.Cleanup(func() { providerBinariesFor = original })
	providerBinariesFor = func(provider string) []binaryCandidate {
		if provider != ProviderClaude {
			t.Fatal("unexpected planner recipient")
		}
		return []binaryCandidate{{name: os.Args[0], prefix: []string{"-test.run=TestGoalGuideCLIHelper", "--"}}}
	}
	t.Setenv("CLAUDE_GOAL_GUIDE_HELPER", "1")
	d, provider, err := PlanGoal(context.Background(), goalwork.View{Goal: "compare source evidence"}, ProviderClaude)
	if err != nil || provider != ProviderClaude || d.Action != "abstain" {
		t.Fatalf("shared planner contract was not delivered: %s %+v %v", provider, d, err)
	}
}

func TestGoalGuideCLIHelper(t *testing.T) {
	if os.Getenv("CLAUDE_GOAL_GUIDE_HELPER") != "1" {
		return
	}
	input, err := io.ReadAll(os.Stdin)
	text := string(input)
	if err != nil || strings.Count(text, goalwork.PlanningGuide()) != 1 {
		os.Exit(2)
	}
	for _, required := range []string{"Return ONE JSON object", "no tools, code execution", "never external planning input", "STATE_JSON:", `"goal":"compare source evidence"`} {
		if !strings.Contains(text, required) {
			os.Exit(3)
		}
	}
	fmt.Println(`{"action":"abstain","reason":"fixture contract received"}`)
	os.Exit(0)
}
