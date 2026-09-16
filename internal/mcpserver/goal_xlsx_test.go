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

func TestGoalMCPXLSXSelectionAndProvenanceUseSharedContract(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: "123", Assets: []string{"source.xlsx"}}, nil
			},
			Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if r.XLSX == nil || r.XLSX.Sheet != "districts" || r.XLSX.Range != "A27:B27" {
					t.Error("MCP lost selector")
				}
				return goalwork.Acquired{Rows: []goalwork.Row{{"A": "001", "B": "148"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), Table: &dataset.TableProvenance{Sheet: "districts", Range: "A27:B27", RowNumbers: []int{27}, FormulaCells: []string{"B27"}}}, nil
			},
			Layout: func(context.Context, goalwork.LayoutRequest, goalwork.Inspection) (dataset.FileLayout, error) {
				return dataset.FileLayout{SHA256: strings.Repeat("a", 64), Sheets: []dataset.XLSXSheetRef{{Name: "districts", Member: "xl/worksheets/sheet1.xml"}}}, nil
			},
		}
	})
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "goal", Arguments: args})
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
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "123"}, {Action: "layout", Layout: &goalwork.LayoutRequest{PK: "123", Asset: "source.xlsx"}}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "file", Asset: "source.xlsx", LayoutID: "l1", XLSX: &dataset.XLSXSelection{Sheet: "districts", Range: "A27:B27"}}}} {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Gaps) > 0 || len(v.State.Observations) != 1 || v.State.Observations[0].Table.RowNumbers[0] != 27 || v.State.Observations[0].Table.FormulaCells[0] != "B27" || v.State.SampleAttempts[0].Request.XLSX.Range != "A27:B27" || v.State.Status != "exploring" {
		t.Fatal("MCP lost evidence or asserted completion")
	}
}
