package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGoalMCPReplansScopeUsingPublishedVocabulary(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Search: func(_ context.Context, q string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
			},
			Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: pk}, nil
			},
			Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if r.PK == "111" {
					return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001", "address": "인천광역시 연수구 fixture"}}}, nil
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001", "province": "인천", "district": "연수구"}}}, nil
			},
		}
	})
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		r, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("MCP scope call failed: %v %+v", err, r)
		}
		var out goalOut
		if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	v := call(map[string]any{"goal": "compare scoped source records", "requireSemantic": false})
	step := func(d goalwork.Decision) {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	step(goalwork.Decision{Action: "define", Contract: &goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "id", Role: "r", Description: "original ID", Type: "string"}}}})
	for _, pk := range []string{"111", "222"} {
		step(goalwork.Decision{Action: "search", Query: pk, Role: "r"})
		step(goalwork.Decision{Action: "inspect", PK: pk})
		step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "api"}})
	}
	p := goalwork.Composition{ID: "literal", Purpose: "comparison", Base: "o1", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.id"}, RightKeys: []string{"id"}, Scopes: []goalwork.JoinScope{{LeftParts: []string{"o1.address"}, RightParts: []string{"province", "district"}, Rule: "right_prefix_v1"}}}}, Select: []string{"o1.id", "o2.province"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "id", Field: "o1.id"}}, Assumptions: []string{"matching text is not verified identifier semantics"}}
	step(goalwork.Decision{Action: "compose", Composition: &p})
	step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.State.Artifact != nil || v.State.Executions[0].Metrics[0].ScopeChecks[0].ConflictPairs != 1 {
		t.Fatal("literal scope failure lost")
	}
	p.ID = "vocabulary"
	p.Joins[0].Scopes[0].Vocabulary = "kr_sido_labels_20260907_v1"
	step(goalwork.Decision{Action: "compose", Composition: &p})
	step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.State.Status != "review_required" || v.State.Artifact == nil || v.State.Artifact.Rows[0]["o2.province"] != "인천" || len(v.State.Executions) != 2 {
		t.Fatalf("MCP did not preserve original labels and replan: %+v", v.State.Gaps)
	}
	c := v.State.Artifact.Metrics[0].ScopeChecks[0]
	if c.Vocabulary == nil || c.Vocabulary.ID != p.Joins[0].Scopes[0].Vocabulary || len(c.Vocabulary.Sources) != 2 || c.MeaningVerified {
		t.Fatal("MCP lost vocabulary provenance or promoted geographic meaning")
	}
}
