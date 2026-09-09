package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveSourceReductionUsesActualEngineWithoutApprovingMeaning(t *testing.T) {
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "source"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: "source"}, nil
			},
			Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if s.Reduce != nil {
					t.Fatal("local reduction reached external provider")
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"n": "9007199254740993"}, {"n": "1"}}, Delivery: "REST"}, nil
			},
		})
		if err != nil {
			return goalwork.View{}, err
		}
		c := goalwork.GoalContract{Outcome: goal, Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Type: "number", Description: "sum"}}}
		p := goalwork.Composition{ID: "result", Base: "o2", Purpose: "report local aggregate", Select: []string{"o2.total"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "o2.total"}}, Assumptions: []string{"arithmetic only, not semantic approval"}}
		ds := []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "source"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "source", Delivery: "api"}}}
		i := 0
		return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
			if i < len(ds) {
				d := ds[i]
				i++
				return d, nil
			}
			if len(v.Observations) == 1 {
				return goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "source", Delivery: "api", Reduce: &goalwork.SourceReduction{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Measures: []goalwork.Measure{{As: "number", Field: "n", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "number"}}}}}, nil
			}
			if len(v.Compositions) == 0 {
				return goalwork.Decision{Action: "compose", Composition: &p}, nil
			}
			if len(v.Executions) == 0 {
				return goalwork.Decision{Action: "execute", CompositionID: p.ID}, nil
			}
			return goalwork.Decision{Action: "abstain", Reason: "calculation retained; original goal meaning remains unverified"}, nil
		}, progress)
	})
	var out bytes.Buffer
	cmd.SetArgs([]string{"sum original records", "--require-semantic=false"})
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unreviewed calculation got CLI success")
	}
	var v goalwork.View
	d := json.NewDecoder(&out)
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if v.Status != "abstained" || v.Artifact == nil || v.Artifact.Rows[0]["o2.total"] != json.Number("9007199254740994") || len(v.Artifact.Sources) != 2 || v.Observations[1].Reduction == nil {
		t.Fatalf("CLI lost computed result, exact value or original lineage: %+v", v)
	}
}
