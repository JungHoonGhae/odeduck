package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveAnalysisReviewRunsTheActualEngineWithAdditionalAuthority(t *testing.T) {
	for _, flag := range []string{"--review-analyses", "--review-source-reports"} {
		cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
			if policy.ReviewAnalyses != (flag == "--review-analyses") || policy.ReviewRecipient != "claude" {
				t.Fatal("analysis authorization was widened or lost")
			}
			e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
				Search: func(_ context.Context, q string) (catalog.Result, error) {
					return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
				},
				Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
					return goalwork.Inspection{PK: pk}, nil
				},
				Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
					if r.PK == "definitions" {
						return goalwork.Acquired{Delivery: "STD", Rows: []goalwork.Row{{"definition": "Recorded fixture units"}}}, nil
					}
					if !r.ScanCSV || len(r.WhereIn["n"]) != 2 {
						t.Fatal("CLI calculation lost its source value-set request")
					}
					return goalwork.Acquired{Delivery: "FILE", Rows: []goalwork.Row{{"n": "9007199254740993"}, {"n": "1"}}}, nil
				},
				Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
					if in.Analysis == nil || in.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
						t.Fatal("review bypassed real calculation")
					}
					if len(in.Analysis.SourceContext) != 1 || !in.Analysis.SourceContext[0].Proposed || in.Analysis.SourceContext[0].Source.PK != "definitions" || in.Analysis.SourceContext[0].Evidence.Records[0].Values["definition"] != "Recorded fixture units" || len(in.Artifact.Sources) != 1 {
						t.Fatal("CLI lost explicit original STD support or made it a calculation source")
					}
					f := goalwork.ReviewFinding{Verdict: "supported", Reason: "fixture arithmetic judgement", PacketIDs: []string{in.Evidence.ID}}
					a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "total", Finding: f}}}
					for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
						a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
					}
					return a, nil
				},
			})
			if err != nil {
				return goalwork.View{}, err
			}
			c := goalwork.GoalContract{Outcome: goal, Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "values"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Type: "number", Description: "sum"}}}
			p := goalwork.Composition{ID: "sum", Base: "o1", Purpose: "sum recorded values", Select: []string{"total"}, Measures: []goalwork.Measure{{As: "n", Field: "o1.n", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"fixture calculation"}}
			decisions := []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "values", Role: "r"}, {Action: "inspect", PK: "values"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "values", Delivery: "file", ScanCSV: true, WhereIn: map[string][]string{"n": {"1", "9007199254740993"}}}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "sum"}}
			if policy.ReviewAnalyses {
				decisions = append(decisions[:4], goalwork.Decision{Action: "search", Query: "definitions", Role: "context"}, goalwork.Decision{Action: "inspect", PK: "definitions"}, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "definitions", Delivery: "standard"}})
			}
			index := 0
			return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
				if index < len(decisions) {
					d := decisions[index]
					index++
					return d, nil
				}
				if policy.ReviewAnalyses && len(v.Compositions) == 0 {
					if len(v.Evidence) == 0 {
						return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.Observations[1].RowsSHA256, Rows: []int{1}, Fields: []string{"definition"}}}, nil
					}
					p.Support = []goalwork.SupportBinding{{PacketID: v.Evidence[0].ID, Targets: []string{"o1"}, Purpose: "Check reported units"}}
					return goalwork.Decision{Action: "compose", Composition: &p}, nil
				}
				if len(v.Executions) == 0 {
					return goalwork.Decision{Action: "execute", CompositionID: "sum"}, nil
				}
				if len(v.Evidence) == 0 || policy.ReviewAnalyses && len(v.Evidence) == 1 {
					return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1, 2}, Fields: []string{"n"}}}, nil
				}
				if len(v.Gaps) > 0 {
					return goalwork.Decision{Action: "abstain", Reason: "analysis review not authorized"}, nil
				}
				return goalwork.Decision{Action: "review_result", CompositionID: "sum"}, nil
			}, progress)
		})
		var out bytes.Buffer
		cmd.SetArgs([]string{"관측한 수치의 합계를 계산해줘", flag, "--agent=claude", "--share-evidence", "--require-semantic=false"})
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if (err == nil) != (flag == "--review-analyses") {
			t.Fatalf("%s: %v", flag, err)
		}
		var v goalwork.View
		d := json.NewDecoder(strings.NewReader(out.String()))
		d.UseNumber()
		if d.Decode(&v) != nil || v.Artifact == nil || v.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
			t.Fatalf("not actual precision-preserving result: %s", out.String())
		}
		if flag == "--review-analyses" && (v.Status != "output_ready" || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod) {
			t.Fatal("CLI did not return reviewed calculation")
		}
	}
}
