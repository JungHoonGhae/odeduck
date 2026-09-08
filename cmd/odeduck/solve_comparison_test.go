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
	reviewed := false
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				if in.Analysis == nil || in.Analysis.Method != "engine_relational_replay_v3" || len(in.Analysis.SourceContext) != 1 {
					return goalwork.ReviewAssessment{}, fmt.Errorf("CLI did not deliver comparison review v3")
				}
				c := in.Analysis.SourceContext[0]
				if c.Comparison == nil || c.Source.Comparison != nil || c.Request.Compare == nil || c.ComparisonSources[0].ArtifactSource != "o1" || c.ComparisonSources[1].Source == nil || c.ComparisonSources[1].Request == nil || c.ComparisonSources[1].Request.PK != "right" || len(in.Artifact.Sources) != 1 {
					return goalwork.ReviewAssessment{}, fmt.Errorf("CLI lost single-copy recipe, original requests or computational distinction")
				}
				reviewed = true
				f := goalwork.ReviewFinding{Verdict: "insufficient", Reason: "fixture: numerical discrepancy does not establish applicability"}
				for _, p := range in.EvidencePackets() {
					f.PacketIDs = append(f.PacketIDs, p.ID)
				}
				a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "id", Finding: f}}}
				for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
					a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
				}
				return a, nil
			},
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
			if len(v.Evidence) == 1 {
				return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1}, Fields: []string{"id"}}}, nil
			}
			if len(v.Compositions) == 0 {
				p := goalwork.Composition{ID: "report", Base: "o1", Purpose: "report original identifiers with proposed comparison support", Select: []string{"o1.id"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "id", Field: "o1.id"}}, Assumptions: []string{"comparison does not approve source meaning"}, Support: []goalwork.SupportBinding{{PacketID: v.Evidence[0].ID, Targets: []string{"o1"}, Purpose: "Check source applicability"}}}
				return goalwork.Decision{Action: "compose", Composition: &p}, nil
			}
			if len(v.Executions) == 0 {
				return goalwork.Decision{Action: "execute", CompositionID: "report"}, nil
			}
			if len(v.Reviews) == 0 {
				return goalwork.Decision{Action: "review_result", CompositionID: "report"}, nil
			}
			return goalwork.Decision{Action: "abstain", Reason: "computed discrepancy is not a source-applicability verdict"}, nil
		}, progress)
	})
	var out bytes.Buffer
	cmd.SetArgs([]string{"compare original source measurements", "--require-semantic=false", "--agent=claude", "--share-evidence", "--review-analyses"})
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
	if !reviewed || v.Status != "abstained" || v.Artifact == nil || len(v.Reviews) != 1 || len(v.Observations) != 3 || len(v.Evidence) != 2 || v.SampleAttempts[2].Request.Compare == nil || v.Policy.EvidenceRecipient != "claude" {
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
