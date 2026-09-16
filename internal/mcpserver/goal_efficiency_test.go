package mcpserver

import (
	"context"
	"encoding/json"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

func TestGoalDefaultReturnsChangesAndExplicitSnapshot(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies { return goalwork.Dependencies{} })
	c := connectTestClient(t, s)
	call := func(args map[string]any) map[string]json.RawMessage {
		t.Helper()
		r, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: "goal", Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("call: %+v %v", r, err)
		}
		var out map[string]json.RawMessage
		if err = json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := call(map[string]any{"goal": "目標", "requireSemantic": false})
	var id string
	json.Unmarshal(first["sessionId"], &id)
	contract := goalwork.GoalContract{Outcome: "result", Region: "source", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "v", Role: "r", Description: "value", Type: "string"}}}
	next := call(map[string]any{"sessionId": id, "revision": 0, "decision": goalwork.Decision{Action: "define", Contract: &contract}})
	if string(next["update"]) != `"delta"` || len(next["changes"]) == 0 {
		t.Fatalf("expected delta, got keys %v", next)
	}
	var state map[string]json.RawMessage
	json.Unmarshal(next["state"], &state)
	if _, ok := state["runtime"]; ok {
		t.Fatal("unchanged runtime was retransmitted")
	}
	full := call(map[string]any{"sessionId": id, "fullState": true})
	json.Unmarshal(full["state"], &state)
	if len(state["contract"]) == 0 || len(state["runtime"]) == 0 || string(full["update"]) != `"snapshot"` {
		t.Fatal("full recovery snapshot missing")
	}
}

// Full-state fixtures test computation contracts independently of transport size.
func goalTestFull(args map[string]any) map[string]any {
	out := make(map[string]any, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	out["fullState"] = true
	return out
}
