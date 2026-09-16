package goalwork

import (
	"context"
	"strings"
	"testing"
)

func TestReadGuidePreservesGoalAndReviewBoundary(t *testing.T) {
	e, _ := Start("original goal", Policy{}, Dependencies{})
	v, err := e.Advance(context.Background(), 0, Decision{Action: "read_guide", Topic: "compose"})
	if err != nil || v.Revision != 1 || v.Contract != nil || v.Goal != "original goal" || v.GuideTopic != "compose" || v.Status != "exploring" {
		t.Fatalf("guide changed goal: %+v %v", v, err)
	}
	instructions := PlanningInstructions(v)
	if !strings.Contains(instructions, "sum_fields") || strings.Contains(instructions, "FILE XLSX exception") {
		t.Fatal("wrong chapter or entire reference sent")
	}
	if len(instructions) >= len(PlanningGuide())/2 {
		t.Fatal("ordinary chapter became full reference")
	}
	before := v.Policy
	v, err = e.Advance(context.Background(), v.Revision, Decision{Action: "read_guide", Topic: "review"})
	if err != nil || v.Policy != before || v.Policy.ReviewRecipient != "" {
		t.Fatal("guide granted review permission")
	}
	// Switching topics must not make a prior local reference unavailable.
	v, err = e.Advance(context.Background(), v.Revision, Decision{Action: "read_guide", Topic: "compose"})
	if err != nil || v.GuideTopic != "compose" {
		t.Fatal("local help was rejected as a replayed acquisition")
	}
	// Merely reading help must not erase an executed result's unresolved review.
	e.state.Status = "review_required"
	v, err = e.Advance(context.Background(), v.Revision, Decision{Action: "read_guide", Topic: "scope"})
	if err != nil || v.Status != "review_required" {
		t.Fatal("reading help cleared review state")
	}
}
