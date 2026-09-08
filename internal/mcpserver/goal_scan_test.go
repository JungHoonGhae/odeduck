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

func TestGoalMCPFullCSVScanPreservesRequestAndPartialRetention(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: "123", Assets: []string{"source.csv"}}, nil
			},
			Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if !r.ScanCSV || r.Where["city"] != "target" {
					t.Error("MCP lost full scan selector")
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), Selection: &dataset.SelectionReport{Mode: "exact_strings_full_scan_v1", ScannedRows: 2000, MatchedRows: 1200, ReturnedRows: 1, Exhausted: true}, CSV: &dataset.CSVProvenance{Encoding: "euc-kr", DataRecords: []int{801}, StartLines: []int{802}}}, nil
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
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "123"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "file", Asset: "source.csv", ScanCSV: true, Where: map[string]string{"city": "target"}}}} {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Gaps) > 0 || len(v.State.Observations) != 1 || !v.State.SampleAttempts[0].Request.ScanCSV || v.State.Status != "exploring" {
		t.Fatal("MCP lost request or asserted completion")
	}
	o := v.State.Observations[0]
	if o.Selection == nil || !o.Selection.Exhausted || o.Selection.MatchedRows != 1200 || o.Selection.ReturnedRows != 1 || o.CSV.Encoding != "euc-kr" || o.CSV.DataRecords[0] != 801 {
		t.Fatal("MCP confused full scan with full retention")
	}
}
