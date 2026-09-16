package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestGoalReviewWithFixesRecipientAndReviewScope(t *testing.T) {
	called := false
	cmd := goalCommand(func(_ context.Context, request goalwork.Request, p goalwork.Policy, provider string, _ func(goalwork.View)) (goalwork.View, error) {
		called = true
		if request.Context != "Earlier examples are excluded." || provider != "codex" || p.EvidenceRecipient != "codex" || p.ReviewRecipient != "codex" || !p.ReviewAnalyses || !p.ReviewFullScope || !p.RequireSemantic {
			t.Fatal("unified review lost fixed policy", p, provider)
		}
		return goalwork.View{Goal: request.Goal, Status: "abstained"}, nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"흥미로운 사실을 발견해줘", "--review-with", "codex", "--context", "Earlier examples are excluded."})
	if err := cmd.Execute(); err == nil || !called {
		t.Fatal("review mode not invoked or false completion")
	}
}

func TestGoalRequiresReviewSetupBeforeModelSpend(t *testing.T) {
	called := false
	cmd := goalCommand(func(context.Context, goalwork.Request, goalwork.Policy, string, func(goalwork.View)) (goalwork.View, error) {
		called = true
		return goalwork.View{}, nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"목표에 맞는 결과를 만들어줘"})
	err := cmd.Execute()
	if called || err == nil || !strings.Contains(err.Error(), "--review-with") {
		t.Fatal("missing completion prerequisite was not surfaced before execution", err)
	}
}
