package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGoalResourceDeliversSharedContractWithMCPFraming(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	read, err := sess.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "odeduck://guide"})
	if err != nil || len(read.Contents) != 1 {
		t.Fatalf("goal guide resource: %+v %v", read, err)
	}
	text := read.Contents[0].Text
	if strings.Count(text, goalwork.PlanningGuide()) != 1 {
		t.Fatal("resource omitted or duplicated the engine-owned planning contract")
	}
	for _, required := range []string{"sessionId", "state.revision", "decision", "--review-with", "MCP host", "기존 사용자 Artifact", "catalog_search", "record_connection_assessment", "call_api"} {
		if !strings.Contains(text, required) {
			t.Fatalf("resource omitted transport or existing workflow: %s", required)
		}
	}
	if strings.Contains(text, "STATE_JSON:") || strings.Contains(text, "Return ONE JSON object") {
		t.Fatal("CLI response framing leaked into the MCP workflow")
	}
	listed, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "goal" {
			if !strings.Contains(tool.Description, "odeduck://guide") {
				t.Fatal("goal tool does not route the host to its detailed contract")
			}
			return
		}
	}
	t.Fatal("goal tool missing")
}
