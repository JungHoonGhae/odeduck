package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGoalMCPSingleSourceAnalysisMatchesActualEngine(t *testing.T) {
	ctx := context.Background()
	deps := goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"amount": "9007199254740993"}, {"amount": "1"}}}, nil
		},
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, false, func(goalwork.Policy) goalwork.Dependencies { return deps })
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		r, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || r.IsError || len(r.Content) != 1 {
			t.Fatalf("MCP result: %+v %v", r, err)
		}
		content, ok := r.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("missing exact JSON response")
		}
		decoder := json.NewDecoder(strings.NewReader(content.Text))
		decoder.UseNumber()
		var out goalOut
		if err := decoder.Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	v := call(map[string]any{"goal": "표본 원천 수치를 합산해줘", "requireSemantic": false})
	c := goalwork.GoalContract{Outcome: "sample sum", Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "records", Description: "sample"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "records", Type: "number", Description: "sample sum"}}}
	p := goalwork.Composition{ID: "sum", Purpose: "sum source records", Base: "o1", Measures: []goalwork.Measure{{As: "amount", Field: "o1.amount", Format: "decimal_v1", Unit: "fixture units"}}, Aggregates: []goalwork.Aggregate{{As: "total", Field: "amount", Op: "sum"}}, Roles: []goalwork.RoleBinding{{Role: "records", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"fixture only"}}
	e, err := goalwork.Start(v.State.Goal, goalwork.Policy{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	var local goalwork.View
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "records"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "sum"}} {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
		local, err = e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, result := range []goalwork.View{v.State, local} {
		if result.Artifact == nil || result.Status != "review_required" || !result.Evaluation.NeedsSemanticReview || result.Artifact.Status != "sample_executed" || len(result.Artifact.Rows) != 1 || result.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
			t.Fatalf("shared single-source analysis failed: %+v", result)
		}
	}
	if v.State.Artifact.Sources[0].RequestSHA256 != local.Artifact.Sources[0].RequestSHA256 {
		t.Fatal("MCP changed source request")
	}
}
