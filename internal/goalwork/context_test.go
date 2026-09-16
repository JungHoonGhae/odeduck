package goalwork_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestGoalContextSurvivesPlanningAndIndependentReviewWithoutApprovingIt(t *testing.T) {
	contextText := "Earlier examples concerned a different topic. This is user context, not source evidence."
	reviewed := false
	e, err := goalwork.StartRequest(goalwork.Request{Goal: "Report the recorded labels", Context: contextText}, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"record": "museum A", "year": "2025"}, {"record": "museum B", "year": "2024"}}}, nil
		},
		Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			reviewed = true
			if in.Context != contextText {
				t.Fatal("user context lost at separate review")
			}
			a := supportedSourceReview(in)
			a.GoalFit.Verdict = "insufficient"
			return a, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.PlanningView().Context != contextText {
		t.Fatal("planner lost relevant conversation")
	}
	c := resultContract()
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "records"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	executeResult(t, e, resultRecipe())
	readSourceReviewEvidence(t, e)
	v := advanceSourceReview(t, e)
	if !reviewed || v.Status == "output_ready" || v.Context != contextText {
		t.Fatal("context replaced required independent review")
	}
}

func TestGoalContextRejectsOversizedCredentialsAndInvalidText(t *testing.T) {
	for _, text := range []string{strings.Repeat("a", 8001), "serviceKey=fixture-secret-never-return", string([]byte{0xff})} {
		if _, err := goalwork.StartRequest(goalwork.Request{Goal: "original goal", Context: text}, goalwork.Policy{}, goalwork.Dependencies{}); err == nil {
			t.Fatal("invalid context accepted")
		}
	}
}
