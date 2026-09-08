package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGoalToolKeepsStateBoundToMCPSession(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, false, func(goalwork.Policy) goalwork.Dependencies {
		return goalwork.Dependencies{Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "123", Title: "candidate"}}}, nil
		}}
	})
	a := connectTestClient(t, s)
	b := connectTestClient(t, s)
	call := func(client *mcp.ClientSession, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := call(a, map[string]any{"goal": "동네의 서로 다른 관측을 연결", "requireSemantic": false})
	if res.IsError {
		t.Fatalf("%+v", res)
	}
	data, _ := json.Marshal(res.StructuredContent)
	var start goalOut
	if err := json.Unmarshal(data, &start); err != nil {
		t.Fatal(err)
	}
	if start.SessionID == "" || start.State.Revision != 0 {
		t.Fatalf("%s", data)
	}
	contract := goalwork.GoalContract{Outcome: "표본 비교", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "hazard", Description: "폭염"}}, Outputs: []goalwork.OutputRequirement{{ID: "value", Description: "폭염값", Role: "hazard", Type: "number"}}}
	args := map[string]any{"sessionId": start.SessionID, "revision": 0, "decision": goalwork.Decision{Action: "define", Contract: &contract}}
	if res = call(b, args); !res.IsError {
		t.Fatal("another MCP session accessed goal")
	}
	if res = call(a, args); res.IsError {
		t.Fatalf("%+v", res)
	}
	if res = call(a, args); !res.IsError {
		t.Fatal("stale revision accepted")
	}
	res = call(a, map[string]any{"sessionId": start.SessionID, "revision": 1, "decision": goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "api", Where: map[string]string{"region": "target"}}}})
	data, _ = json.Marshal(res.StructuredContent)
	if res.IsError || !strings.Contains(string(data), "where selection is supported only for FILE CSV (direct or ZIP member)") {
		t.Fatalf("MCP lost selection or failed to expose typed acquisition gap: %s", data)
	}
}

func TestGoalToolProducesSameArtifactAsStandaloneEngine(t *testing.T) {
	deps := goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q, Title: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk, Deliveries: []string{"REST"}, Declarations: map[string]goalwork.SourceDeclaration{"api": {Status: "publisher_declared", Description: "fixture municipality"}}}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001", "label": s.PK, "capacity": "9007199254740993", "adjustment": "1", "year": "2025", "scope": "fixture municipality"}}, Delivery: "REST", Operation: "list"}, nil
		},
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(server, false, func(goalwork.Policy) goalwork.Dependencies { return deps })
	client := connectTestClient(t, server)
	invoke := func(args map[string]any) goalOut {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("%+v %v", res, err)
		}
		// The SDK client decodes StructuredContent numbers as float64. Decode the
		// exact textual JSON payload to audit what the server actually emitted.
		if len(res.Content) != 1 {
			t.Fatalf("missing exact JSON content: %+v", res.Content)
		}
		content, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("unexpected content: %T", res.Content[0])
		}
		b := []byte(content.Text)
		var out goalOut
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.UseNumber()
		if err = decoder.Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	remote := invoke(map[string]any{"goal": "compare observations", "requireSemantic": false})
	contract := goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "people", Description: "people"}, {ID: "shelters", Description: "shelters"}}, Outputs: []goalwork.OutputRequirement{{ID: "people", Description: "people", Role: "people", Type: "string"}, {ID: "shelters", Description: "shelters", Role: "shelters", Type: "string"}}}
	contract.Outputs = append(contract.Outputs, goalwork.OutputRequirement{ID: "capacity", Description: "capacity", Role: "shelters", Type: "number"})
	contract.TimeWindow = &goalwork.DateWindow{From: "2025-01-01", Through: "2025-12-31"}
	contract.Explanations = []goalwork.ExplanationRequirement{{ID: "time_limits", Topic: "temporal", Description: "time limitations"}, {ID: "coverage_limits", Topic: "coverage", Description: "coverage limitations"}}
	decisions := []goalwork.Decision{{Action: "define", Contract: &contract}}
	for _, pk := range []string{"people", "shelters"} {
		decisions = append(decisions, goalwork.Decision{Action: "search", Query: pk, Role: pk}, goalwork.Decision{Action: "inspect", PK: pk}, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "api", Operation: "list", Params: map[string]string{"year": "2025"}}})
	}
	p := goalwork.Composition{ID: "joined", Purpose: "bounded comparison", Base: "o1", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.id"}, RightKeys: []string{"id"}}}, Assumptions: []string{"fixture namespace and time only"}}
	p.Joins[0].Scopes = []goalwork.JoinScope{{LeftCitation: &goalwork.ScopeCitation{Observation: "o1", Field: "description", Quote: "fixture municipality"}, RightParts: []string{"scope"}, Rule: "equal_tokens_v1"}}
	p.Roles = []goalwork.RoleBinding{{Role: "people", Observation: "o1"}, {Role: "shelters", Observation: "o2"}}
	p.Outputs = []goalwork.OutputBinding{{Output: "people", Field: "o1.label"}, {Output: "shelters", Field: "o2.label"}}
	p.Measures = []goalwork.Measure{{As: "capacity_number", Op: "sum_fields", Fields: []string{"o2.capacity", "o2.adjustment"}, Format: "decimal_v1", Unit: "persons"}}
	p.Time = &goalwork.TemporalAlignment{Window: contract.TimeWindow, Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}, {Observation: "o2", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	p.Outputs = append(p.Outputs, goalwork.OutputBinding{Output: "capacity", Field: "capacity_number"})
	partial := p
	partial.ID = "missing_output"
	partial.Outputs = partial.Outputs[:1]
	decisions = append(decisions, goalwork.Decision{Action: "compose", Composition: &partial}, goalwork.Decision{Action: "execute", CompositionID: partial.ID}, goalwork.Decision{Action: "compose", Composition: &p}, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	for _, d := range decisions {
		remote = invoke(map[string]any{"sessionId": remote.SessionID, "revision": remote.State.Revision, "decision": d})
		if d.Action == "execute" && d.CompositionID == partial.ID && (remote.State.Status != "exploring" || remote.State.Evaluation == nil || remote.State.Evaluation.Status != "partial" || remote.State.Artifact == nil) {
			t.Fatalf("MCP ended on partial artifact: %+v", remote.State)
		}
	}
	e, _ := goalwork.Start("compare observations", goalwork.Policy{}, deps)
	i := 0
	local, err := goalwork.Run(context.Background(), e, func(context.Context, goalwork.View) (goalwork.Decision, error) { d := decisions[i]; i++; return d, nil }, nil)
	if err != nil || local.Status != "review_required" || remote.State.Status != local.Status {
		t.Fatalf("%+v %+v %v", local, remote, err)
	}
	a, _ := json.Marshal(local.Artifact.Rows)
	b, _ := json.Marshal(remote.State.Artifact.Rows)
	if string(a) != string(b) {
		t.Fatalf("different artifacts: %s %s", a, b)
	}
	if remote.State.Artifact.Rows[0]["capacity_number"] != json.Number("9007199254740994") {
		t.Fatalf("MCP numeric precision lost: %s", b)
	}
	if remote.State.Evaluation.Temporal.Status != "checked" || remote.State.Evaluation.Temporal.MeaningVerified || len(remote.State.Evaluation.Explanations) != 2 {
		t.Fatalf("lost evidence: %+v", remote.State.Evaluation)
	}
	if len(remote.State.Artifact.Metrics[0].ScopeChecks) != 1 || remote.State.Artifact.Metrics[0].ScopeChecks[0].MatchedPairs != 1 || !remote.State.Evaluation.NeedsSemanticReview {
		t.Fatal("MCP lost scope computation or promoted it to identity proof")
	}
	claim := remote.State.Artifact.Metrics[0].ScopeChecks[0].LeftClaim
	if claim == nil || claim.Status != "proposed_scope" || claim.Citation.Observation != "o1" || len(claim.ClaimSHA256) != 64 {
		t.Fatal("MCP lost grounded but unapproved source claim")
	}
	if len(remote.State.SampleAttempts) != 2 {
		t.Fatalf("MCP dropped acquisition history: %+v", remote.State.SampleAttempts)
	}
	for i, attempt := range remote.State.SampleAttempts {
		if attempt.Request.Operation != "list" || attempt.Request.Params["year"] != "2025" || attempt.Status != "acquired" || attempt.ObservationID != remote.State.Observations[i].ID || attempt.RequestSHA256 != local.SampleAttempts[i].RequestSHA256 {
			t.Fatalf("MCP changed trusted acquisition history: %+v", attempt)
		}
	}
}
