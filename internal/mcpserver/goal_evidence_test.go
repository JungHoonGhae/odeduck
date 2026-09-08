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

func TestGoalMCPEvidenceUsesTrustedServerPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			reviewer := ""
			if enabled {
				reviewer = "claude"
			}
			registerGoalTool(s, enabled, reviewer, func(goalwork.Policy) goalwork.Dependencies {
				return goalwork.Dependencies{
					Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
						b, _ := json.Marshal(in)
						if !enabled || in.Recipient != "claude" || strings.Contains(string(b), "NOT_SELECTED") {
							t.Fatal("review disclosed unauthorized input")
						}
						f := goalwork.ReviewFinding{Verdict: "supported", Reason: "fixture recorded value only", PacketID: in.Evidence.ID}
						return goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "v", Finding: f}}}, nil
					},
					Search: func(context.Context, string) (catalog.Result, error) {
						return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
					},
					Inspect: func(context.Context, string) (goalwork.Inspection, error) {
						return goalwork.Inspection{PK: "records"}, nil
					},
					Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
						return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"value": json.Number("9007199254740993"), "unused": "NOT_SELECTED"}}}, nil
					},
				}
			})
			client := connectTestClient(t, s)
			call := func(args map[string]any) goalOut {
				t.Helper()
				r, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
				if err != nil || r.IsError {
					t.Fatalf("MCP call: %+v %v", r, err)
				}
				content := r.Content[0].(*mcp.TextContent).Text
				if strings.Contains(content, "NOT_SELECTED") {
					t.Fatal("MCP exposed unselected row field")
				}
				decoder := json.NewDecoder(strings.NewReader(content))
				decoder.UseNumber()
				var out goalOut
				if err := decoder.Decode(&out); err != nil {
					t.Fatal(err)
				}
				return out
			}
			// A tool argument cannot set the trusted server's disclosure policy.
			bad, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: map[string]any{"goal": "fixture", "evidenceRecipient": "mcp_host"}})
			if err == nil && !bad.IsError {
				t.Fatal("model-controlled disclosure authority was accepted")
			}
			v := call(map[string]any{"goal": "fixture", "requireSemantic": false})
			if (v.State.Policy.EvidenceRecipient == "mcp_host") != enabled {
				t.Fatal("server disclosure policy lost")
			}
			c := goalwork.GoalContract{Outcome: "fixture", Region: "fixture", Period: "source date", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "v", Role: "r", Type: "number", Description: "value"}}}
			for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "r"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}} {
				v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
			}
			p := goalwork.Composition{ID: "reported value", Purpose: "selected source value", Base: "o1", Select: []string{"o1.value"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "v", Field: "o1.value"}}, Assumptions: []string{"fixture interpretation only"}}
			for _, d := range []goalwork.Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
				v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
			}
			if v.State.Status != "review_required" {
				t.Fatal("MCP did not reach review through real execution")
			}
			v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.State.Observations[0].RowsSHA256, Rows: []int{1}, Fields: []string{"value"}}}})
			if v.State.Status != "exploring" || v.State.Evaluation.ExecutionRevision != 6 || !v.State.Evaluation.NeedsSemanticReview {
				t.Fatal("MCP review continuation relabeled or approved the result")
			}
			if enabled {
				if len(v.State.Evidence) != 1 || v.State.Evidence[0].Records[0].Values["value"] != json.Number("9007199254740993") {
					t.Fatal("MCP changed exact evidence")
				}
				v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": goalwork.Decision{Action: "review_result", CompositionID: p.ID}})
				if v.State.Status != "output_ready" || v.State.Evaluation.NeedsSemanticReview || v.State.Evaluation.Review.Provider != "claude" || v.State.Evaluation.Review.ExecutionRevision != 6 || v.State.Artifact.Rows[0]["o1.value"] != json.Number("9007199254740993") {
					t.Fatalf("MCP lost actual review transition: %+v", v.State)
				}
			} else if len(v.State.Evidence) != 0 || len(v.State.Gaps) != 1 || !strings.Contains(v.State.Gaps[0].Detail, "disabled") {
				t.Fatal("MCP default disclosure was not blocked")
			}
			bad, err = client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: map[string]any{"goal": "fixture", "reviewRecipient": "claude"}})
			if err == nil && !bad.IsError {
				t.Fatal("model-controlled review authority was accepted")
			}
			other := connectTestClient(t, s)
			foreign, err := other.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: map[string]any{"sessionId": v.SessionID}})
			if err == nil && !foreign.IsError {
				t.Fatal("another MCP session read retained evidence")
			}
		})
	}
}
