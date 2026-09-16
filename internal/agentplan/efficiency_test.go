package agentplan

import (
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"strings"
	"testing"
)

func TestGoalInitialPromptDoesNotSendEveryOperatorContract(t *testing.T) {
	prompt, err := goalPrompt(goalwork.View{Goal: "새로운 사실을 찾아주세요"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt) > 12000 {
		t.Fatalf("initial prompt repeats %d bytes before any source is discovered", len(prompt))
	}
	if !strings.Contains(prompt, "read_guide") {
		t.Fatal("detailed action contracts must remain discoverable")
	}
}
