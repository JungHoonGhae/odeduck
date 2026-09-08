package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveUsesSharedGoalLoopAndReturnsHonestAbstention(t *testing.T) {
	cmd := solveCommand(func(_ context.Context, goal string, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
		if goal != "더위에 취약한 동네를 찾고 싶다" || !policy.RequireSemantic || provider != "auto" {
			t.Fatalf("%s %+v %s", goal, policy, provider)
		}
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{})
		if err != nil {
			t.Fatal(err)
		}
		return goalwork.Run(context.Background(), e, func(context.Context, goalwork.View) (goalwork.Decision, error) {
			return goalwork.Decision{Action: "abstain", Reason: "공식 지역 대응표 필요"}, nil
		}, progress)
	})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"더위에 취약한 동네를 찾고 싶다"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("abstention must not exit as successful composition")
	}
	var view goalwork.View
	if err := json.Unmarshal(out.Bytes(), &view); err != nil || view.Status != "abstained" || view.Artifact != nil {
		t.Fatalf("%s %v", out.String(), err)
	}
}

func TestSolveExplicitRetryKeepsHistoryWithoutReportingGoalComplete(t *testing.T) {
	calls := 0
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) { return goalwork.Inspection{PK: "123"}, nil },
			Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
				calls++
				if calls == 1 {
					return goalwork.Acquired{}, &goalwork.AcquisitionError{Kind: goalwork.FailureTransient, Cause: errors.New("PRIVATE_TRANSPORT")}
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001"}}}, nil
			},
		})
		if err != nil {
			return goalwork.View{}, err
		}
		contract := &goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "2025", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "role"}}, Outputs: []goalwork.OutputRequirement{{ID: "x", Role: "r", Type: "string", Description: "value"}}}
		decisions := []goalwork.Decision{{Action: "define", Contract: contract}, {Action: "search", Query: "fixture", Role: "r"}, {Action: "inspect", PK: "123"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "api"}}, {Action: "retry_sample", RetryOf: 4}, {Action: "abstain", Reason: "acquisition recovered; remaining comparison evidence needed"}}
		n := 0
		return goalwork.Run(ctx, e, func(context.Context, goalwork.View) (goalwork.Decision, error) { d := decisions[n]; n++; return d, nil }, progress)
	})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"original comparison goal", "--require-semantic=false"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("successful reacquisition is not completed goal")
	}
	var view goalwork.View
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || view.Goal != "original comparison goal" || len(view.SampleAttempts) != 2 || view.SampleAttempts[0].Status != "failed" || view.SampleAttempts[1].RetryOf != 4 || view.SampleAttempts[1].Status != "acquired" || view.Budget.SamplesRemaining != 6 || view.Status != "abstained" {
		t.Fatal("CLI lost shared retry outcome or budget")
	}
	if strings.Contains(out.String()+stderr.String(), "PRIVATE_TRANSPORT") {
		t.Fatal("CLI exposed private error cause")
	}
}

func TestSolveExitRequiresMoreThanStructuralOutput(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		evaluation   *goalwork.GoalEvaluation
		complete     bool
	}{
		{"unattributed success status", "output_ready", &goalwork.GoalEvaluation{Status: "requirements_met"}, false},
		{"missing evaluation", "output_ready", nil, false},
		{"legacy unsafe success", "output_ready", &goalwork.GoalEvaluation{Status: "requirements_met", NeedsSemanticReview: true}, false},
		{"review pending", "review_required", &goalwork.GoalEvaluation{Status: "requirements_met", NeedsSemanticReview: true}, false},
		{"partial", "budget_exhausted", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := solveCommand(func(context.Context, string, goalwork.Policy, string, func(goalwork.View)) (goalwork.View, error) {
				return goalwork.View{Goal: "test", Status: tc.status, Evaluation: tc.evaluation, Artifact: &goalwork.Artifact{Status: "sample_executed", Rows: []goalwork.Row{{"exact": json.Number("9007199254740993")}}}}, nil
			})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"test"})
			err := cmd.Execute()
			if (err == nil) != tc.complete {
				t.Fatalf("status %s error %v", tc.status, err)
			}
			if !strings.Contains(out.String(), "9007199254740993") {
				t.Fatal("CLI rounded exact output")
			}
			var view goalwork.View
			if json.Unmarshal(out.Bytes(), &view) != nil || view.Artifact == nil || view.Status != tc.status {
				t.Fatalf("partial JSON lost: %s", out.String())
			}
		})
	}
}

func TestSolveRunsSingleSourceProjectionWithoutReportingSemanticApproval(t *testing.T) {
	sawReview := false
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "source"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: "source"}, nil
			},
			Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
				return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"value": json.Number("9007199254740993")}}}, nil
			},
		})
		if err != nil {
			return goalwork.View{}, err
		}
		c := goalwork.GoalContract{Outcome: "source record", Region: "fixture", Period: "source date", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "record", Description: "source record"}}, Outputs: []goalwork.OutputRequirement{{ID: "value", Role: "record", Type: "number", Description: "published value"}}}
		p := goalwork.Composition{ID: "record", Purpose: "project source value", Base: "o1", Select: []string{"o1.value"}, Roles: []goalwork.RoleBinding{{Role: "record", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "value", Field: "o1.value"}}, Assumptions: []string{"fixture interpretation only"}}
		decisions := []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "record"}, {Action: "inspect", PK: "source"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "source", Delivery: "api"}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "record"}}
		n := 0
		return goalwork.Run(ctx, e, func(_ context.Context, view goalwork.View) (goalwork.Decision, error) {
			if n >= len(decisions) {
				if !sawReview {
					if view.Status != "review_required" {
						t.Fatal("CLI did not deliver the review state to its planner")
					}
					sawReview = true
					return goalwork.Decision{Action: "search", Query: "official field meaning", Role: "scope"}, nil
				}
				return goalwork.Decision{Action: "abstain", Reason: "fixture does not establish field meaning"}, nil
			}
			b, _ := json.Marshal(view)
			if strings.Contains(string(b), "9007199254740993") {
				t.Fatal("source value leaked to external planner")
			}
			d := decisions[n]
			n++
			return d, nil
		}, progress)
	})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"원천의 기록을 보여줘", "--require-semantic=false"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unreviewed record was reported as verified goal completion")
	}
	var v goalwork.View
	decoder := json.NewDecoder(&out)
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if !sawReview || v.Status != "abstained" || v.Revision != 8 || v.Evaluation.ExecutionRevision != 6 || !v.Evaluation.NeedsSemanticReview || len(v.Searches) != 2 || v.Artifact == nil || v.Artifact.Status != "sample_executed" || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o1.value"] != json.Number("9007199254740993") {
		t.Fatalf("CLI lost the single-source result: %+v", v)
	}
}
