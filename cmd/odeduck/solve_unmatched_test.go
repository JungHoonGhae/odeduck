package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolvePreservesUnmatchedRecordAddressesFromActualEngine(t *testing.T) {
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Search: func(_ context.Context, q string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
			},
			Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: pk}, nil
			},
			Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				return goalwork.Acquired{Rows: []goalwork.Row{{"code": "001", "label": "matched"}, {"code": s.PK, "label": "excluded"}}, Delivery: "REST"}, nil
			},
		})
		if err != nil {
			return goalwork.View{}, err
		}
		c := goalwork.GoalContract{Outcome: goal, Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "label", Role: "r", Type: "string", Description: "source label"}}}
		p := goalwork.Composition{ID: "result", Base: "o1", Purpose: "compare records", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, Select: []string{"o1.label"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "label", Field: "o1.label"}}, Assumptions: []string{"record comparison only"}}
		ds := []goalwork.Decision{{Action: "define", Contract: &c}}
		p.ReportUnmatched = []string{"o1.code", "o1.label", "o2.code", "o2.label"}
		for _, pk := range []string{"left", "right"} {
			ds = append(ds, goalwork.Decision{Action: "search", Query: pk, Role: "r"}, goalwork.Decision{Action: "inspect", PK: pk}, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "api"}})
		}
		ds = append(ds, goalwork.Decision{Action: "compose", Composition: &p}, goalwork.Decision{Action: "execute", CompositionID: p.ID}, goalwork.Decision{Action: "abstain", Reason: "unmatched records need interpretation"})
		i := 0
		return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
			if v.Artifact != nil {
				t.Fatal("CLI planning received raw output")
			}
			d := ds[i]
			i++
			return d, nil
		}, progress)
	})
	var out bytes.Buffer
	cmd.SetArgs([]string{"compare source records and identify unmatched parts", "--require-semantic=false"})
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unreviewed comparison got CLI success")
	}
	var v goalwork.View
	if json.Unmarshal(out.Bytes(), &v) != nil || v.Artifact == nil || v.Status != "abstained" || len(v.Artifact.Rows) != 1 {
		t.Fatalf("comparison result unavailable: %s", out.String())
	}
	m := v.Artifact.Metrics[0]
	if len(v.Artifact.Unmatched) != 2 || v.Artifact.Unmatched[0].Values["o1.code"] != "left" || v.Artifact.Unmatched[1].Values["o2.code"] != "right" {
		t.Fatal("CLI discarded explicitly reported unmatched source values")
	}
	if len(m.UnmatchedLeft) != 1 || m.UnmatchedLeft[0]["o1"] != 2 || len(m.UnmatchedRight) != 1 || m.UnmatchedRight[0] != 2 {
		t.Fatalf("CLI discarded excluded source positions: %+v", m)
	}
}
