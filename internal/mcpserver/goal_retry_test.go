package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGoalMCPRetryIsSessionBoundAndRetainsFailedEvidence(t *testing.T) {
	calls := 0
	ready := false
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, false, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) { return goalwork.Inspection{PK: "123"}, nil },
			Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
				calls++
				if !ready {
					return goalwork.Acquired{}, &goalwork.AcquisitionError{Kind: goalwork.FailureAccessRequired, Cause: errors.New("PRIVATE_KEY")}
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001"}}}, nil
			},
		}
	})
	a, b := connectTestClient(t, s), connectTestClient(t, s)
	call := func(client *mcp.ClientSession, args map[string]any) (goalOut, bool) {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		body := res.Content[0].(*mcp.TextContent).Text
		if strings.Contains(body, "PRIVATE_KEY") {
			t.Fatal("private cause leaked")
		}
		var out goalOut
		if !res.IsError {
			if err := json.Unmarshal([]byte(body), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out, res.IsError
	}
	view, bad := call(a, map[string]any{"goal": "original goal", "requireSemantic": false})
	if bad {
		t.Fatal("start failed")
	}
	contract := goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "2025", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "role"}}, Outputs: []goalwork.OutputRequirement{{ID: "x", Role: "r", Type: "string", Description: "value"}}}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "fixture", Role: "r"}, {Action: "inspect", PK: "123"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "api"}}} {
		view, bad = call(a, map[string]any{"sessionId": view.SessionID, "revision": view.State.Revision, "decision": d})
		if bad {
			t.Fatal("setup failed")
		}
	}
	args := map[string]any{"sessionId": view.SessionID, "revision": view.State.Revision, "decision": goalwork.Decision{Action: "retry_sample", RetryOf: view.State.SampleAttempts[0].Revision}}
	if _, bad = call(b, args); !bad || calls != 1 {
		t.Fatal("another MCP session retried private goal")
	}
	ready = true
	view, bad = call(a, args)
	if bad || calls != 2 || view.State.Goal != "original goal" || len(view.State.Observations) != 1 || len(view.State.SampleAttempts) != 2 || view.State.SampleAttempts[0].Status != "failed" || view.State.SampleAttempts[1].Status != "acquired" || view.State.Budget.SamplesRemaining != 6 || view.State.Status != "exploring" {
		t.Fatal("MCP retry did not preserve failure, original goal and shared budgets")
	}
	if _, bad = call(a, args); !bad || calls != 2 {
		t.Fatal("stale retry reissued acquisition")
	}
}
