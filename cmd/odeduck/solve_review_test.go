package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveSourceReviewUsesActualEngineAndExplicitAuthority(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
			if policy.ReviewRecipient != provider || provider != "claude" {
				t.Fatal("review authorization changed")
			}
			e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
				Search: func(context.Context, string) (catalog.Result, error) {
					return catalog.Result{Hits: []catalog.Hit{{PK: "record"}}}, nil
				},
				Inspect: func(context.Context, string) (goalwork.Inspection, error) {
					return goalwork.Inspection{PK: "record"}, nil
				},
				Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
					return goalwork.Acquired{Rows: []goalwork.Row{{"label": "published label"}}}, nil
				},
				Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
					if in.Goal != goal || in.Artifact.Rows[0]["o1.label"] != "published label" {
						t.Fatal("review did not see actual executed result")
					}
					f := goalwork.ReviewFinding{Verdict: "supported", Reason: "recorded label only", PacketID: in.Evidence.ID}
					return goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "label", Finding: f}}}, nil
				},
			})
			if err != nil {
				return goalwork.View{}, err
			}
			c := goalwork.GoalContract{Outcome: goal, Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "label", Role: "r", Type: "string", Description: "recorded label"}}}
			p := goalwork.Composition{ID: "report", Base: "o1", Purpose: "source report", Select: []string{"o1.label"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "label", Field: "o1.label"}}, Assumptions: []string{"source only"}}
			ds := []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "record", Role: "r"}, {Action: "inspect", PK: "record"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "record", Delivery: "api"}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}}
			i := 0
			return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
				if i < len(ds) {
					d := ds[i]
					i++
					return d, nil
				}
				if len(v.Evidence) == 0 {
					return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1}, Fields: []string{"label"}}}, nil
				}
				return goalwork.Decision{Action: "review_result", CompositionID: p.ID}, nil
			}, progress)
		})
		args := []string{"report the published label", "--review-source-reports", "--agent=claude", "--require-semantic=false"}
		if enabled {
			args = append(args, "--share-evidence")
		}
		var out bytes.Buffer
		cmd.SetArgs(args)
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if (err == nil) != enabled {
			t.Fatalf("enabled=%t error=%v", enabled, err)
		}
		if enabled {
			var v goalwork.View
			if json.Unmarshal(out.Bytes(), &v) != nil || v.Status != "output_ready" || v.Evaluation.Review == nil || v.Evaluation.NeedsSemanticReview {
				t.Fatalf("not actual reviewed output: %s", out.String())
			}
		}
	}
}
