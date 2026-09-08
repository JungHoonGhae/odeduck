package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPComparisonUsesOriginalRevisionsAndExplicitComputedDisclosure(t *testing.T) {
	reviewed := false
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{EvidenceRecipient: "mcp_host", ReviewRecipient: "claude", ReviewAnalyses: true}, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				if in.Analysis == nil || in.Analysis.Method != "engine_relational_replay_v3" || len(in.Analysis.SourceContext) != 1 {
					return goalwork.ReviewAssessment{}, fmt.Errorf("MCP did not deliver comparison review v3")
				}
				c := in.Analysis.SourceContext[0]
				if c.Comparison == nil || c.Source.Comparison != nil || c.Request.Compare == nil || c.ComparisonSources[0].ArtifactSource != "o1" || c.ComparisonSources[1].Source == nil || c.ComparisonSources[1].Request == nil || c.ComparisonSources[1].Request.Delivery != "standard" || len(in.Artifact.Sources) != 1 {
					return goalwork.ReviewAssessment{}, fmt.Errorf("MCP lost single-copy comparison or both original contracts")
				}
				if len(in.Artifact.Explanations) != 1 || in.Artifact.Explanations[0].Citations[0].PacketID != c.PacketID || in.Contract.Explanations[0].Basis != "source" {
					return goalwork.ReviewAssessment{}, fmt.Errorf("MCP lost source explanatory output or exact citations")
				}
				reviewed = true
				f := goalwork.ReviewFinding{Verdict: "insufficient", Reason: "fixture: comparison is not a meaning verdict"}
				for _, packet := range in.EvidencePackets() {
					f.PacketIDs = append(f.PacketIDs, packet.ID)
				}
				a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "code", Finding: f}}}
				a.Explanations = []goalwork.ExplanationReview{{Explanation: "unmatched", Finding: f}}
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
			Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if r.Compare != nil {
					t.Fatal("MCP comparison reached an external acquisition")
				}
				rows := []goalwork.Row{{"code": "001", "value": "1", "private": "UNSELECTED_ORIGINAL"}, {"code": "002", "value": "2"}}
				if r.PK == "right" {
					rows = []goalwork.Row{{"label": "Other name (001)", "value": "1"}, {"label": "Other record (003)", "value": "0"}}
				}
				return goalwork.Acquired{Rows: rows, Delivery: "STD"}, nil
			},
		}
	})
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || res.IsError {
			if res != nil && len(res.Content) > 0 {
				t.Logf("MCP response: %+v", res.Content[0])
			}
			t.Fatalf("MCP failure: %v", err)
		}
		var out goalOut
		decoder := json.NewDecoder(strings.NewReader(res.Content[0].(*mcp.TextContent).Text))
		decoder.UseNumber()
		if err := decoder.Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	v := call(map[string]any{"goal": "compare source measurements", "requireSemantic": false})
	advance := func(d any) {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
		if len(v.State.Gaps) != 0 {
			t.Fatalf("MCP comparison gaps: %+v", v.State.Gaps)
		}
	}
	contract := goalwork.GoalContract{Outcome: "source comparison", Region: "fixture", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "code", Role: "r", Type: "string", Description: "original code"}}}
	contract.Explanations = []goalwork.ExplanationRequirement{{ID: "unmatched", Topic: "coverage", Basis: "source", Description: "Explain the unmatched source records"}}
	advance(goalwork.Decision{Action: "define", Contract: &contract})
	advance(goalwork.Decision{Action: "search", Query: "records", Role: "r"})
	for _, pk := range []string{"left", "right"} {
		advance(goalwork.Decision{Action: "inspect", PK: pk})
		advance(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "standard"}})
	}
	wire := fmt.Sprintf(`{"action":"sample","sample":{"pk":"left","delivery":"standard","compare":{"left":{"observation":"o1","rowsSha256":%q,"keys":[{"field":"code","rule":"exact"}]},"right":{"observation":"o2","rowsSha256":%q,"keys":[{"field":"label","rule":"trailing_parenthesized_digits_v1","digits":3}]},"checks":[{"id":"n","left":{"field":"value","format":"decimal_v1","unit":"units"},"right":{"field":"value","format":"decimal_v1","unit":"units"}}]}}}`, v.State.Observations[0].RowsSHA256, v.State.Observations[1].RowsSHA256)
	var d map[string]any
	if err := json.Unmarshal([]byte(wire), &d); err != nil {
		t.Fatal(err)
	}
	advance(d)
	body, _ := json.Marshal(v)
	if strings.Contains(string(body), "UNSELECTED_ORIGINAL") || strings.Contains(string(body), "Other name") || len(v.State.Evidence) != 0 {
		t.Fatal("MCP comparison disclosed original values without selection")
	}
	o := v.State.Observations[2]
	advance(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, Fields: []string{"metric", "value"}}})
	if len(v.State.Evidence) != 1 || len(v.State.Evidence[0].Records) != 13 || o.Comparison == nil || o.Comparison.Pairs[0] != [2]int{1, 1} || v.State.Status != "exploring" || v.State.Artifact != nil || v.State.SampleAttempts[2].Request.Compare == nil {
		t.Fatal("MCP lost local comparison, recipe or incomplete status")
	}
	for _, index := range []int{2, 3, 4, 9} {
		record := v.State.Evidence[0].Records[index]
		if record.Values["value"] != json.Number("1") || record.Origins["value"].Kind != "computed_comparison" || record.Origins["value"].Observation != o.ID {
			t.Fatalf("MCP lost matching and both unmatched sides: %+v", record)
		}
	}
	advance(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.State.Observations[0].RowsSHA256, Rows: []int{1, 2}, Fields: []string{"code"}}})
	report := goalwork.Composition{ID: "report", Base: "o1", Purpose: "report identifiers with proposed comparison support", Select: []string{"o1.code"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "code", Field: "o1.code"}}, Assumptions: []string{"comparison is not applicability approval"}, Support: []goalwork.SupportBinding{{PacketID: v.State.Evidence[0].ID, Targets: []string{"o1"}, Purpose: "Check source interpretation"}}}
	report.Explanations = []goalwork.ExplanationDraft{{ID: "unmatched", Text: "One retained record on each side did not pair; unmatched records do not establish real-world absence.", Citations: []goalwork.EvidenceCitation{{PacketID: v.State.Evidence[0].ID, PacketRow: 4, Field: "value"}, {PacketID: v.State.Evidence[0].ID, PacketRow: 5, Field: "value"}}}}
	advance(goalwork.Decision{Action: "compose", Composition: &report})
	advance(goalwork.Decision{Action: "execute", CompositionID: report.ID})
	advance(goalwork.Decision{Action: "review_result", CompositionID: report.ID})
	if !reviewed || len(v.State.Reviews) != 1 || v.State.Status == "output_ready" || len(v.State.Artifact.Sources) != 1 || v.State.Observations[2].Comparison == nil {
		t.Fatal("MCP omitted review, promoted support or changed stored comparison provenance")
	}
	if len(v.State.Artifact.Explanations) != 1 || v.State.Artifact.Explanations[0].Citations[1].PacketRow != 5 || len(v.State.Evaluation.Review.Assessment.Explanations) != 1 {
		t.Fatal("MCP JSON lost the original explanatory citations or separate finding")
	}
	// Optional wire aliases are for comparison operands only. An ordinary output
	// measure still requires its own alias under the Engine's execution contract.
	p := goalwork.Composition{ID: "missing-alias", Base: "o1", Purpose: "ordinary numeric output", Select: []string{"o1.value"}, Assumptions: []string{"explicit output measures require an alias"}, Measures: []goalwork.Measure{{Field: "o1.value", Format: "decimal_v1", Unit: "units"}}}
	advance(goalwork.Decision{Action: "compose", Composition: &p})
	v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": goalwork.Decision{Action: "execute", CompositionID: p.ID}})
	if len(v.State.Gaps) != 1 || v.State.Artifact != nil || len(v.State.Executions) != 2 || v.State.Executions[1].Status != "failed" {
		t.Fatal("comparison operand schema relaxed the output-measure contract")
	}
}
