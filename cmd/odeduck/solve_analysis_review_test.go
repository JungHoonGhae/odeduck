package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveAnalysisReviewRunsTheActualEngineWithAdditionalAuthority(t *testing.T) {
	for _, flag := range []string{"--review-analyses", "--review-source-reports", "--review-full-scope"} {
		fullScope := flag == "--review-full-scope"
		cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
			if policy.ReviewAnalyses != (flag != "--review-source-reports") || policy.ReviewFullScope != fullScope || policy.ReviewRecipient != "claude" {
				t.Fatal("analysis authorization was widened or lost")
			}
			e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
				Search: func(_ context.Context, q string) (catalog.Result, error) {
					return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
				},
				Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
					return goalwork.Inspection{PK: pk}, nil
				},
				Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
					if r.PK == "definitions" {
						return goalwork.Acquired{Delivery: "STD", Rows: []goalwork.Row{{"definition": "Recorded fixture units"}}}, nil
					}
					if !r.ScanCSV || len(r.WhereIn["n"]) != 2 {
						t.Fatal("CLI calculation lost its source value-set request")
					}
					return goalwork.Acquired{Delivery: "FILE", Rows: []goalwork.Row{{"n": "9007199254740993"}, {"n": "1"}}, ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64), CSV: &dataset.CSVProvenance{DataRecords: []int{1, 2}, StartLines: []int{2, 3}}, Selection: &dataset.SelectionReport{Mode: "exact_string_sets_full_scan_v1", ScannedRows: 2, MatchedRows: 2, ReturnedRows: 2, Exhausted: true}}, nil
				},
				Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
					if in.Analysis == nil || in.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
						t.Fatal("review bypassed real calculation")
					}
					packets := map[string]goalwork.EvidencePacket{}
					var ids []string
					for _, packet := range in.EvidencePackets() {
						packets[packet.ID] = packet
						ids = append(ids, packet.ID)
					}
					if len(in.Analysis.SourceContext) != 1 || !in.Analysis.SourceContext[0].Proposed || in.Analysis.SourceContext[0].Source.PK != "definitions" || len(packets) != 2 || len(packets[in.Analysis.SourceContext[0].PacketID].Records) != 1 || packets[in.Analysis.SourceContext[0].PacketID].Records[0].Values["definition"] != "Recorded fixture units" || len(in.Artifact.Sources) != 1 {
						t.Fatal("CLI lost explicit original STD support or made it a calculation source")
					}
					f := goalwork.ReviewFinding{Verdict: "supported", Reason: "fixture arithmetic judgement", PacketIDs: ids}
					a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "total", Finding: f}}}
					for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
						a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
					}
					if fullScope {
						if in.Contract.Coverage != "population" || in.Analysis.FullScope == nil {
							t.Fatal("CLI narrowed full requested scope")
						}
						a.SourceCoverage = []goalwork.SourceCoverageReview{{Observation: "o1", Finding: f}}
					}
					return a, nil
				},
			})
			if err != nil {
				return goalwork.View{}, err
			}
			c := goalwork.GoalContract{Outcome: goal, Region: "fixture", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "values"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Type: "number", Description: "sum"}}}
			if fullScope {
				c.Coverage = "population"
			}
			p := goalwork.Composition{ID: "sum", Base: "o1", Purpose: "sum recorded values", Select: []string{"total"}, Measures: []goalwork.Measure{{As: "n", Field: "o1.n", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"fixture calculation"}}
			decisions := []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "values", Role: "r"}, {Action: "inspect", PK: "values"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "values", Delivery: "file", Asset: "values.csv", ScanCSV: true, WhereIn: map[string][]string{"n": {"1", "9007199254740993"}}}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "sum"}}
			if policy.ReviewAnalyses {
				decisions = append(decisions[:4], goalwork.Decision{Action: "search", Query: "definitions", Role: "context"}, goalwork.Decision{Action: "inspect", PK: "definitions"}, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "definitions", Delivery: "standard"}})
			}
			index := 0
			return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
				if index < len(decisions) {
					d := decisions[index]
					index++
					return d, nil
				}
				if policy.ReviewAnalyses && len(v.Compositions) == 0 {
					if len(v.Evidence) == 0 {
						return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.Observations[1].RowsSHA256, Rows: []int{1}, Fields: []string{"definition"}}}, nil
					}
					p.Support = []goalwork.SupportBinding{{PacketID: v.Evidence[0].ID, Targets: []string{"o1"}, Purpose: "Check reported units"}}
					return goalwork.Decision{Action: "compose", Composition: &p}, nil
				}
				if len(v.Executions) == 0 {
					return goalwork.Decision{Action: "execute", CompositionID: "sum"}, nil
				}
				if len(v.Evidence) == 0 || policy.ReviewAnalyses && len(v.Evidence) == 1 {
					return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1, 2}, Fields: []string{"n"}}}, nil
				}
				if len(v.Gaps) > 0 {
					return goalwork.Decision{Action: "abstain", Reason: "analysis review not authorized"}, nil
				}
				return goalwork.Decision{Action: "review_result", CompositionID: "sum"}, nil
			}, progress)
		})
		var out bytes.Buffer
		cmd.SetArgs([]string{"관측한 수치의 합계를 계산해줘", flag, "--agent=claude", "--share-evidence", "--require-semantic=false"})
		if fullScope {
			cmd.SetArgs([]string{"요청 범위의 모든 원천 수치를 합산해줘", flag, "--review-analyses", "--agent=claude", "--share-evidence", "--require-semantic=false"})
		}
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if (err == nil) != (flag != "--review-source-reports") {
			t.Fatalf("%s: %v", flag, err)
		}
		var v goalwork.View
		d := json.NewDecoder(strings.NewReader(out.String()))
		d.UseNumber()
		if d.Decode(&v) != nil || v.Artifact == nil || v.Artifact.Rows[0]["total"] != json.Number("9007199254740994") {
			t.Fatalf("not actual precision-preserving result: %s", out.String())
		}
		if flag == "--review-analyses" && (v.Status != "output_ready" || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod) {
			t.Fatal("CLI did not return reviewed calculation")
		}
		if fullScope && (v.Status != "output_ready" || v.Evaluation.Review.Method != goalwork.FullScopeReviewMethod) {
			t.Fatal("CLI lost the additional full-scope review contract")
		}
	}
}

// The command and Engine are real; only acquisition and semantic judgement are
// fixtures. A passing transition is not an actual-model accuracy result.
func TestSolveReusesOriginalCellsWithoutAnotherDisclosure(t *testing.T) {
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, _ func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.Dependencies{
			Search: func(context.Context, string) (catalog.Result, error) {
				return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
			},
			Inspect: func(context.Context, string) (goalwork.Inspection, error) {
				return goalwork.Inspection{PK: "records"}, nil
			},
			Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
				a := goalwork.Acquired{Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64), Rows: []goalwork.Row{{"A": "001", "B": "PRIVATE_CELL"}}, Table: &dataset.TableProvenance{Sheet: "table", Member: "xl/worksheets/sheet1.xml", Range: r.XLSX.Range, RowNumbers: []int{2}}}
				if r.XLSX.Range == "A1:B2" {
					a.Rows = append([]goalwork.Row{{"A": "Original code", "B": "PRIVATE_CELL"}}, a.Rows...)
					a.Table.RowNumbers = []int{1, 2}
				}
				return a, nil
			},
			Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				if in.Analysis == nil || len(in.EvidencePackets()) != 1 || in.Evidence.Selection.Observation != "o2" || in.Analysis.Sources[0].Disclosure[0].PacketRow != 2 || in.Artifact.Rows[0]["o1.A"] != "001" {
					t.Fatal("CLI lost verified original-cell attribution")
				}
				b, _ := json.Marshal(in)
				if strings.Contains(string(b), "PRIVATE_CELL") || strings.Count(string(b), `"Original code"`) != 1 {
					t.Fatal("CLI duplicated context or disclosed private fields")
				}
				f := goalwork.ReviewFinding{Verdict: "supported", Reason: "Fixture source report judgement", PacketIDs: []string{in.Evidence.ID}}
				a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "code", Finding: f}}}
				for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
					a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
				}
				return a, nil
			},
		})
		if err != nil {
			return goalwork.View{}, err
		}
		step := func(d goalwork.Decision) {
			t.Helper()
			v, err := e.Advance(ctx, e.View().Revision, d)
			if err != nil || len(v.Gaps) > 0 {
				t.Fatalf("%s: %v %+v", d.Action, err, v.Gaps)
			}
		}
		c := goalwork.GoalContract{Outcome: goal, Region: "source", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "record"}}, Outputs: []goalwork.OutputRequirement{{ID: "code", Role: "r", Type: "string", Description: "original code"}}}
		step(goalwork.Decision{Action: "define", Contract: &c})
		step(goalwork.Decision{Action: "search", Query: "records", Role: "r"})
		step(goalwork.Decision{Action: "inspect", PK: "records"})
		for _, rectangle := range []string{"A2:B2", "A1:B2"} {
			step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "file", Asset: "records.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "table", Range: rectangle}}})
		}
		p := goalwork.Composition{ID: "record", Base: "o1", Purpose: "report original code", Select: []string{"o1.A"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "code", Field: "o1.A"}}, Assumptions: []string{"fixture source report only"}}
		step(goalwork.Decision{Action: "compose", Composition: &p})
		step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
		o := e.View().Observations[1]
		step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2}, Fields: []string{"A"}}})
		step(goalwork.Decision{Action: "review_result", CompositionID: p.ID})
		return e.View(), nil
	})
	var out bytes.Buffer
	cmd.SetArgs([]string{"원천에 기록된 코드를 보고해줘", "--review-analyses", "--agent=claude", "--share-evidence", "--require-semantic=false"})
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var v goalwork.View
	if json.Unmarshal(out.Bytes(), &v) != nil || v.Status != "output_ready" || len(v.Evidence) != 1 || v.Budget.EvidencePacketsRemaining != 7 || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod {
		t.Fatal("CLI did not retain the actual reused-evidence result and budget")
	}
}
