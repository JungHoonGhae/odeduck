package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGoalMCPZIPMemberSelectionAndRecordProvenanceUseSharedContract(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, false, "", func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: "123", Assets: []string{"source.zip"}}, nil
			},
			Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if r.Member != "folder/data.csv" || r.LayoutID != "l1" {
					t.Error("MCP lost selector")
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"A": "001", "B": "148"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), Archive: &dataset.ArchiveProvenance{Member: "folder/data.csv", MemberSHA256: strings.Repeat("b", 64)}, CSV: &dataset.CSVProvenance{DataRecords: []int{2}, StartLines: []int{3}, Encoding: "utf-8"}}, nil
			},
			Layout: func(context.Context, goalwork.LayoutRequest, goalwork.Inspection) (dataset.FileLayout, error) {
				return dataset.FileLayout{SHA256: strings.Repeat("a", 64), Format: "ZIP", Members: []dataset.ZIPMember{{Name: "folder/data.csv", Format: "CSV"}}}, nil
			},
		}
	})
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("MCP failure: %+v %v", res, err)
		}
		var out goalOut
		if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	v := call(map[string]any{"goal": "source comparison", "requireSemantic": false})
	contract := goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "id", Role: "r", Description: "code", Type: "string"}}}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "123"}, {Action: "layout", Layout: &goalwork.LayoutRequest{PK: "123", Asset: "source.zip"}}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "file", Asset: "source.zip", LayoutID: "l1", Member: "folder/data.csv"}}} {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Gaps) > 0 || len(v.State.Observations) != 1 || v.State.Observations[0].CSV.DataRecords[0] != 2 || v.State.Observations[0].Archive.Member != "folder/data.csv" || v.State.SampleAttempts[0].Request.Member != "folder/data.csv" || v.State.Status != "exploring" {
		t.Fatal("MCP lost evidence or asserted completion")
	}
}
