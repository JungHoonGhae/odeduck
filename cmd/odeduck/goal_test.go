package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
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

func TestGoalDoesNotRequireAnIndependentReviewProvider(t *testing.T) {
	called := false
	cmd := goalCommand(func(context.Context, goalwork.Request, goalwork.Policy, string, func(goalwork.View)) (goalwork.View, error) {
		called = true
		return goalwork.View{}, nil
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"목표에 맞는 결과를 만들어줘"})
	err := cmd.Execute()
	if !called || (err != nil && strings.Contains(err.Error(), "--review-with")) {
		t.Fatal("independent review became a mandatory planner prerequisite", err)
	}
}

func TestGoalReturnsArtifactWithoutSpawningUnavailableReviewer(t *testing.T) {
	e, err := goalwork.Start("show source values", goalwork.Policy{}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "source"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "source"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"v": "actual value"}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "source value", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "v", Role: "r", Description: "value", Type: "string"}}}
	p := goalwork.Composition{ID: "report", Purpose: "report", Base: "o1", Assumptions: []string{"fixture meaning is unverified"}, Select: []string{"o1.v"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "v", Field: "o1.v"}}}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "source"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "source", Delivery: "api"}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err = e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	if e.View().Status != "review_required" {
		t.Fatalf("fixture did not execute: %+v", e.View().Gaps)
	}
	view, err := runGoalEngine(context.Background(), e, "not-an-installed-provider", nil)
	if err == nil || !strings.Contains(err.Error(), "계산 결과를 반환") || view.Artifact == nil || view.ModelUsage.Calls != 0 || view.Revision != 6 || view.Status != "review_required" {
		t.Fatalf("artifact was lost or an unnecessary model was requested: %+v %v", view, err)
	}
}
