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
	registerGoalTool(s, goalwork.Policy{}, func(goalwork.Policy) goalwork.Dependencies { return deps })
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

func TestGoalMCPSourceReductionPreservesPrecisionAndOrigin(t *testing.T) {
	ctx := context.Background()
	deps := goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			if s.Reduce != nil {
				t.Fatal("local grouping reached provider")
			}
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"amount": "9007199254740993"}, {"amount": "1"}}}, nil
		},
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerGoalTool(s, goalwork.Policy{EvidenceRecipient: "mcp_host"}, func(goalwork.Policy) goalwork.Dependencies { return deps })
	client := connectTestClient(t, s)
	call := func(args map[string]any) goalOut {
		t.Helper()
		r, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("MCP reduction: %v %+v", err, r)
		}
		d := json.NewDecoder(strings.NewReader(r.Content[0].(*mcp.TextContent).Text))
		d.UseNumber()
		var out goalOut
		if err := d.Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	v := call(map[string]any{"goal": "원천 합계를 계산해줘", "requireSemantic": false})
	step := func(d goalwork.Decision) {
		t.Helper()
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
		if len(v.State.Gaps) != 0 {
			t.Fatalf("%s: %+v", d.Action, v.State.Gaps)
		}
	}
	c := goalwork.GoalContract{Outcome: "sum", Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Type: "number", Description: "sum"}}}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "r"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}} {
		step(d)
	}
	r := goalwork.SourceReduction{Observation: "o1", RowsSHA256: v.State.Observations[0].RowsSHA256, Measures: []goalwork.Measure{{As: "n", Field: "amount", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}}
	step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api", Reduce: &r}})
	step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.State.Observations[1].RowsSHA256, Rows: []int{1}, Fields: []string{"total"}}})
	p := goalwork.Composition{ID: "result", Purpose: "report computed group", Base: "o2", Select: []string{"o2.total"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "o2.total"}}, Assumptions: []string{"arithmetic only; no meaning approval"}}
	step(goalwork.Decision{Action: "compose", Composition: &p})
	step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.State.Status != "review_required" || v.State.Artifact.Rows[0]["o2.total"] != json.Number("9007199254740994") || len(v.State.Artifact.Sources) != 2 || v.State.Evidence[0].Records[0].Origins["total"].Kind != "computed_group" {
		t.Fatal("MCP lost group origin/precision or approved the goal")
	}
}

func TestGoalMCPAnalysisReviewUsesTrustedPolicyAndActualCalculation(t *testing.T) {
	for _, fullScope := range []bool{false, true} {
		ctx := context.Background()
		deps := goalwork.Dependencies{
			Search: func(_ context.Context, q string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
			},
			Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: pk}, nil
			},
			Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				if s.PK == "definitions" {
					return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"definition": "Recorded quantity in fixture units", "private": "UNSELECTED_DEFINITION"}}}, nil
				}
				a := goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"n": "9007199254740993"}, {"n": "1"}}}
				if fullScope {
					a.Delivery, a.ContentSHA256, a.ContractSHA256 = "FILE", strings.Repeat("a", 64), strings.Repeat("b", 64)
					a.CSV = &dataset.CSVProvenance{DataRecords: []int{1, 2}, StartLines: []int{2, 3}}
					a.Selection = &dataset.SelectionReport{Mode: "exact_strings_full_scan_v1", ScannedRows: 2, MatchedRows: 2, ReturnedRows: 2, Exhausted: true}
				}
				return a, nil
			},
			Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				if in.Recipient != "claude" || in.Analysis == nil || in.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
					t.Fatal("MCP lost authorized actual calculation")
				}
				if len(in.Analysis.SourceContext) != 1 || !in.Analysis.SourceContext[0].Proposed || in.Analysis.SourceContext[0].Source.PK != "definitions" || in.Analysis.SourceContext[0].Evidence.Records[0].Values["definition"] != "Recorded quantity in fixture units" || len(in.Artifact.Sources) != 1 {
					t.Fatal("MCP lost explicit original API support or made it a calculation source")
				}
				b, _ := json.Marshal(in)
				if strings.Contains(string(b), "UNSELECTED_DEFINITION") {
					t.Fatal("MCP support widened disclosure")
				}
				f := goalwork.ReviewFinding{Verdict: "supported", Reason: "fixture calculation judgement", PacketIDs: []string{in.Evidence.ID}}
				a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "total", Finding: f}}}
				for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
					a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
				}
				if fullScope {
					if in.Analysis.FullScope == nil || in.Contract.Coverage != "population" {
						t.Fatal("MCP narrowed original population scope")
					}
					a.SourceCoverage = []goalwork.SourceCoverageReview{{Observation: "o1", Finding: f}}
				}
				return a, nil
			},
		}
		s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		registerGoalTool(s, goalwork.Policy{EvidenceRecipient: "mcp_host", ReviewRecipient: "claude", ReviewAnalyses: true, ReviewFullScope: fullScope}, func(goalwork.Policy) goalwork.Dependencies { return deps })
		client := connectTestClient(t, s)
		call := func(args map[string]any) goalOut {
			t.Helper()
			r, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
			if err != nil || r.IsError {
				t.Fatalf("MCP analysis: %v %+v", err, r)
			}
			d := json.NewDecoder(strings.NewReader(r.Content[0].(*mcp.TextContent).Text))
			d.UseNumber()
			var out goalOut
			if err := d.Decode(&out); err != nil {
				t.Fatal(err)
			}
			return out
		}
		goal := "표본 원천 수치의 합계를 계산해줘"
		if fullScope {
			goal = "요청 범위의 모든 원천 수치의 합계를 계산해줘"
		}
		v := call(map[string]any{"goal": goal, "requireSemantic": false})
		step := func(d goalwork.Decision) {
			t.Helper()
			v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
			if len(v.State.Gaps) != 0 {
				t.Fatalf("%s: %+v", d.Action, v.State.Gaps)
			}
		}
		c := goalwork.GoalContract{Outcome: "sample sum", Region: "fixture", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "recorded values"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Description: "sum", Type: "number"}}}
		sample := goalwork.SampleRequest{PK: "values", Delivery: "api"}
		method := goalwork.AnalysisReviewMethod
		if fullScope {
			c.Outcome, c.Coverage = goal, "population"
			sample.Delivery, sample.Asset, sample.ScanCSV = "file", "values.csv", true
			method = goalwork.FullScopeReviewMethod
		}
		p := goalwork.Composition{ID: "sum", Base: "o1", Purpose: "sum values", Select: []string{"total"}, Measures: []goalwork.Measure{{As: "n", Field: "o1.n", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"fixture only"}}
		for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "values", Role: "r"}, {Action: "inspect", PK: "values"}, {Action: "sample", Sample: &sample}, {Action: "search", Query: "definitions", Role: "context"}, {Action: "inspect", PK: "definitions"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "definitions", Delivery: "api"}}} {
			step(d)
		}
		step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.State.Observations[1].RowsSHA256, Rows: []int{1}, Fields: []string{"definition"}}})
		p.Support = []goalwork.SupportBinding{{PacketID: v.State.Evidence[0].ID, Targets: []string{"o1"}, Purpose: "Check reported units"}}
		step(goalwork.Decision{Action: "compose", Composition: &p})
		step(goalwork.Decision{Action: "execute", CompositionID: "sum"})
		step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.State.Observations[0].RowsSHA256, Rows: []int{1, 2}, Fields: []string{"n"}}})
		step(goalwork.Decision{Action: "review_result", CompositionID: "sum"})
		if v.State.Status != "output_ready" || v.State.Evaluation.Review.Method != method || v.State.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
			t.Fatal("MCP did not return reviewed exact result")
		}
	}
}
