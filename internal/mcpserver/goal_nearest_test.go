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

func TestGoalMCPNearestUsesRetainedCoordinatesAndPreservesScanEvidence(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{
			Search: func(_ context.Context, q string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
			},
			Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: pk, Assets: []string{"source.csv"}}, nil
			},
			Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
				return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001", "lat": "0", "lon": "0"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64), CSV: &dataset.CSVProvenance{Encoding: "utf-8", DataRecords: []int{1}, StartLines: []int{2}}}, nil
			},
			ScanCSV: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
				if s.Nearest == nil || s.Nearest.Anchor != "o1" || s.Nearest.Candidate != "o2" || !s.ScanCSV {
					t.Fatal("MCP lost nearest request")
				}
				if err := visit(dataset.CSVScanRecord{DataRecord: 1002, StartLine: 1003, Values: map[string]string{"id": "ICB001", "lat": "0", "lon": "1"}}); err != nil {
					return dataset.CSVScanReport{}, "", err
				}
				return dataset.CSVScanReport{Encoding: "utf-8", SHA256: strings.Repeat("a", 64), Columns: []string{"id", "lat", "lon"}, ScannedRows: 1002, MatchedRows: 1, Exhausted: true}, strings.Repeat("b", 64), nil
			},
		}
	})
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "goal", Arguments: goalTestFull(args)})
		if err != nil || res.IsError {
			t.Fatalf("MCP failure: %+v %v", res, err)
		}
		var out goalOut
		if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	v := call(map[string]any{"goal": "compare locations", "requireSemantic": false})
	c := goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []goalwork.OutputRequirement{{ID: "distance", Role: "r", Description: "distance", Type: "number"}}}
	steps := []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "111", Role: "r"}, {Action: "inspect", PK: "111"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "111", Delivery: "file", Asset: "source.csv"}}, {Action: "search", Query: "222", Role: "r"}, {Action: "inspect", PK: "222"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv"}}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv", ScanCSV: true, Nearest: &goalwork.NearestSelection{Method: "spherical_nearest_records_v1", Anchor: "o1", Candidate: "o2", AnchorLatitude: "lat", AnchorLongitude: "lon", Latitude: "lat", Longitude: "lon", K: 1}}}}
	for _, d := range steps {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Gaps) > 0 || len(v.State.Observations) != 3 || v.State.Status != "exploring" {
		t.Fatal("MCP nearest failed or claimed goal completion")
	}
	o := v.State.Observations[2]
	if o.Spatial == nil || o.Spatial.MeaningVerified || !o.Spatial.Scan.Exhausted || o.Spatial.Pairs[0].CandidateDataRecord != 1002 || v.State.SampleAttempts[2].Request.Nearest.K != 1 {
		t.Fatal("MCP lost spatial evidence or request")
	}
	bad := goalwork.Composition{ID: "wrong-prefix", Purpose: "reject unrelated original record", Base: "o3", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o3.candidate.lat"}, RightKeys: []string{"lat"}}}, Assumptions: []string{"same latitude is not same source record"}}
	for _, d := range []goalwork.Decision{{Action: "compose", Composition: &bad}, {Action: "execute", CompositionID: bad.ID}} {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Executions) != 1 || v.State.Executions[0].Metrics[0].LineageRejectedPairs != 1 || v.State.Status != "exploring" {
		t.Fatal("MCP lost lineage conflict or failed replanning state")
	}
	good := goalwork.Composition{ID: "source-sum", Purpose: "fixture arithmetic on one actual candidate record", Base: "o3", Measures: []goalwork.Measure{{As: "n", Field: "o3.candidate.lon", Format: "decimal_v1", Unit: "fixture units"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o3"}}, Outputs: []goalwork.OutputBinding{{Output: "distance", Field: "total"}}, Assumptions: []string{"arithmetic/lineage fixture only, not an approved real-world measure"}}
	for _, d := range []goalwork.Decision{{Action: "compose", Composition: &good}, {Action: "execute", CompositionID: good.ID}} {
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if v.State.Status != "review_required" || v.State.Artifact == nil || len(v.State.Artifact.Sources) != 3 || v.State.Artifact.Rows[0]["total"] != float64(1) {
		t.Fatalf("MCP blocked distinct-source sum or lost indirect provenance: %+v", v.State.Gaps)
	}
}
