package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveComparisonReturnsSelectedCountsWithoutApprovingSourceMeaning(t *testing.T) {
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "left"}, {PK: "right"}}}, nil
			},
			Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: pk}, nil
			},
			Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if s.Compare != nil {
					t.Fatal("local comparison reached external acquisition")
				}
				value := "9007199254740993.3"
				if s.PK == "right" {
					value = "9007199254740993.4"
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001", "value": value, "private": "UNSELECTED_ORIGINAL"}}, Delivery: "REST"}, nil
			},
		})
		if err != nil {
			return goalwork.View{}, err
		}
		contract := goalwork.GoalContract{Outcome: goal, Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "id", Role: "r", Type: "string", Description: "original identifier"}}}
		decisions := []goalwork.Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "records", Role: "r"}}
		for _, pk := range []string{"left", "right"} {
			decisions = append(decisions, goalwork.Decision{Action: "inspect", PK: pk}, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "api"}})
		}
		step := 0
		return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
			if step < len(decisions) {
				d := decisions[step]
				step++
				return d, nil
			}
			if len(v.Observations) == 2 {
				wire := fmt.Sprintf(`{"action":"sample","sample":{"pk":"left","delivery":"api","compare":{"left":{"observation":"o1","rowsSha256":%q,"keys":[{"field":"id","rule":"exact"}]},"right":{"observation":"o2","rowsSha256":%q,"keys":[{"field":"id","rule":"exact"}]},"checks":[{"id":"n","left":{"field":"value","format":"decimal_v1","unit":"units"},"right":{"field":"value","format":"decimal_v1","unit":"units"}}]}}}`, v.Observations[0].RowsSHA256, v.Observations[1].RowsSHA256)
				var d goalwork.Decision
				err := json.Unmarshal([]byte(wire), &d)
				return d, err
			}
			if len(v.Evidence) == 0 {
				o := v.Observations[2]
				return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, Fields: []string{"metric", "value"}}}, nil
			}
			return goalwork.Decision{Action: "abstain", Reason: "computed discrepancy is not a source-applicability verdict"}, nil
		}, progress)
	})
	var out bytes.Buffer
	cmd.SetArgs([]string{"compare original source measurements", "--require-semantic=false", "--agent=claude", "--share-evidence"})
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unreviewed comparison got CLI success")
	}
	var v goalwork.View
	decoder := json.NewDecoder(&out)
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if v.Status != "abstained" || v.Artifact != nil || len(v.Observations) != 3 || len(v.Evidence) != 1 || v.SampleAttempts[2].Request.Compare == nil || v.Policy.EvidenceRecipient != "claude" {
		t.Fatalf("CLI lost comparison, disclosure policy or incomplete state: %+v", v)
	}
	packet := v.Evidence[0]
	if len(packet.Records) != 13 || packet.Records[9].Values["value"] != json.Number("0") || packet.Records[10].Values["value"] != json.Number("1") || packet.Records[10].Origins["value"].Kind != "computed_comparison" {
		t.Fatal("CLI lost exact-decimal discrepancy or attributed it to a publisher")
	}
	body, _ := json.Marshal(v)
	if strings.Contains(string(body), "UNSELECTED_ORIGINAL") || strings.Contains(string(body), "9007199254740993.") {
		t.Fatal("CLI disclosed unselected original values")
	}
}
