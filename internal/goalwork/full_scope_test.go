package goalwork_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestFullScopeReviewRetainsPopulationGoalAndRequiresSourceFinding(t *testing.T) {
	e := fullScopeEngine(t, "csv", true, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		if in.Contract.Coverage != "population" || in.Analysis == nil || in.Analysis.FullScope == nil || !in.Analysis.FullScope.Eligible || len(in.Artifact.Rows) != 2 {
			t.Fatal("full-scope review narrowed the original request")
		}
		return supportedFullScopeReview(in), nil
	})
	reviewFullScope(t, e)
	v := e.View()
	if v.Status != "output_ready" || v.Evaluation.Review.Method != goalwork.FullScopeReviewMethod || v.Contract.Coverage != "population" {
		t.Fatal("qualified full-scope review missing")
	}
}

func supportedFullScopeReview(in goalwork.ReviewInput) goalwork.ReviewAssessment {
	a := supportedAnalysisReview(in)
	a.SourceCoverage = []goalwork.SourceCoverageReview{{Observation: "o1", Finding: goalwork.ReviewFinding{Verdict: "supported", Reason: "Fixture source coverage judgement, not model calibration", PacketIDs: []string{in.Evidence.ID}}}}
	return a
}

func TestFullScopeCannotApproveWithoutEverySourceFinding(t *testing.T) {
	for _, failure := range []string{"missing", "duplicate", "unknown", "unrelated citation", "insufficient", "unsupported", "mutated extent"} {
		t.Run(failure, func(t *testing.T) {
			e := fullScopeEngine(t, "csv", true, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				a := supportedFullScopeReview(in)
				switch failure {
				case "missing":
					a.SourceCoverage = nil
				case "duplicate":
					a.SourceCoverage = append(a.SourceCoverage, a.SourceCoverage[0])
				case "unknown":
					a.SourceCoverage[0].Observation = "another-goal"
				case "unrelated citation":
					a.SourceCoverage[0].Finding.PacketIDs = []string{"ep_invented"}
				case "mutated extent":
					in.Analysis.FullScope.Sources[0].RowsSHA256 = "forged"
				default:
					a.SourceCoverage[0].Finding.Verdict = failure
				}
				return a, nil
			})
			reviewFullScope(t, e)
			v := e.View()
			if v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview || len(v.Reviews) != 1 || v.Evaluation.FullScope.Sources[0].RowsSHA256 != v.Observations[0].RowsSHA256 {
				t.Fatalf("unsafe source finding approved or mutated the retained extent: %+v", v.Gaps)
			}
		})
	}
}

func TestFullScopeRejectsUnprovedAcquisitionAndUnchangedAuthority(t *testing.T) {
	for _, mode := range []string{"disabled", "truncated", "prefix", "unknown", "api", "missing hash", "bad position", "xlsx missing row", "xlsx wrong range"} {
		t.Run(mode, func(t *testing.T) {
			e := fullScopeEngine(t, mode, mode != "disabled", func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				t.Fatal("unproved source reached review")
				return goalwork.ReviewAssessment{}, nil
			})
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "execute", CompositionID: "all"})
			if err != nil || v.Evaluation.Status != "partial" || v.Contract.Coverage != "population" || v.Status == "output_ready" {
				t.Fatalf("unproved acquisition enabled full scope: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestFullScopeInspectsOriginalOfSourceGroupAndXLSXRectangle(t *testing.T) {
	for _, mode := range []string{"group", "xlsx"} {
		t.Run(mode, func(t *testing.T) {
			e := fullScopeEngine(t, mode, true, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				if mode == "xlsx" && (in.Artifact.Requests[0].XLSX == nil || in.Evidence.Records[0].Values["A"] != "wanted") {
					t.Fatal("XLSX coverage must use actual rectangle acquisition and original column addresses")
				}
				if len(in.Analysis.FullScope.Sources) != 1 || in.Analysis.FullScope.Sources[0].Observation != "o1" {
					t.Fatal("coverage attributed to a computed group")
				}
				if mode == "group" && in.Artifact.Rows[0]["o2.total"] != json.Number("12") {
					t.Fatal("full-scope review changed the independently expected sum")
				}
				a := supportedFullScopeReview(in)
				a.SourceCoverage[0].Finding.Verdict = "insufficient" // Complete selection alone is not goal-wide coverage.
				return a, nil
			})
			reviewFullScope(t, e)
			v := e.View()
			if v.Status != "review_required" || len(v.Reviews) != 1 {
				t.Fatalf("complete extent bypassed scope review: %+v", v.Gaps)
			}
		})
	}
}

func TestFullScopeRequiresTheCitedEvidenceToBelongToEachOriginal(t *testing.T) {
	e := fullScopeEngine(t, "csv", true, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		a := supportedFullScopeReview(in)
		// A valid packet for the first source is not evidence for the second.
		a.SourceCoverage = append(a.SourceCoverage, goalwork.SourceCoverageReview{Observation: "o2", Finding: a.SourceCoverage[0].Finding})
		return a, nil
	})
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "counts", Delivery: "file", Asset: "another.csv", ScanCSV: true, Where: map[string]string{"scope": "wanted"}}})
	p := e.View().Compositions[0]
	p.ID, p.Joins = "two", []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.area"}, RightKeys: []string{"area"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	for _, o := range v.Observations {
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2}, Fields: []string{"scope", "area", "n"}}})
	}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || v.Status == "output_ready" || len(v.Reviews) != 1 || v.Reviews[0].Status != "failed" || len(v.Gaps) != 1 {
		t.Fatalf("unrelated source citation approved: %+v %v", v.Gaps, err)
	}
}

func TestFullScopeAuthorityRequiresAnalysisAndDisclosure(t *testing.T) {
	for _, policy := range []goalwork.Policy{{ReviewFullScope: true}, {ReviewFullScope: true, ReviewRecipient: "claude", EvidenceRecipient: "claude"}, {ReviewFullScope: true, ReviewAnalyses: true}} {
		if _, err := goalwork.Start("report all source records", policy, goalwork.Dependencies{Review: func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("startup called reviewer")
			return goalwork.ReviewAssessment{}, nil
		}}); err == nil {
			t.Fatal("full-scope authority enabled without analysis/disclosure")
		}
	}
}

func reviewFullScope(t *testing.T, e *goalwork.Engine) {
	t.Helper()
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: "all"})
	if v.Status != "review_required" || !v.Evaluation.NeedsSemanticReview || v.Contract.Coverage != "population" {
		t.Fatal("full extent approved before review")
	}
	o := v.Observations[0]
	fields := []string{"scope", "area", "n"}
	if o.Table != nil {
		fields = []string{"A", "B", "C"}
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2}, Fields: fields}})
	if len(v.Observations) == 2 {
		o = v.Observations[1]
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"total"}}})
	}
	if _, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "all"}); err != nil {
		t.Fatal(err)
	}
}

type extentTransport func(*http.Request) (*http.Response, error)

func (f extentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fullScopeEngine(t *testing.T, mode string, enabled bool, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	var workbook bytes.Buffer
	if strings.HasPrefix(mode, "xlsx") {
		z := zip.NewWriter(&workbook)
		for name, content := range map[string]string{
			"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="table" r:id="rId1"/></sheets></workbook>`,
			"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
			"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row r="2"><c r="A2" t="inlineStr"><is><t>wanted</t></is></c><c r="B2" t="inlineStr"><is><t>A</t></is></c><c r="C2"><v>5</v></c></row><row r="3"><c r="A3" t="inlineStr"><is><t>wanted</t></is></c><c r="B3" t="inlineStr"><is><t>B</t></is></c><c r="C3"><v>7</v></c></row></sheetData></worksheet>`,
		} {
			w, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, content); err != nil {
				t.Fatal(err)
			}
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: extentTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == "https://data.test/counts.xlsx" && workbook.Len() > 0 {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(workbook.Bytes())), Request: r}, nil
		}
		if r.URL.String() != "https://data.test/counts.csv" {
			t.Fatal("unexpected fixture fetch")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("scope,area,n\nwanted,A,5\nwanted,B,7\nother,C,99\n")), Request: r}, nil
	})}))
	reader := dataset.NewInspector(client, "")
	policy := goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true, ReviewFullScope: enabled}
	e, err := goalwork.Start("Report all recorded counts in the published two-region table", policy, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "counts"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "counts"}, nil
		},
		Sample: func(ctx context.Context, request goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			limit := 2
			if mode == "truncated" || mode == "prefix" {
				limit = 1
			}
			asset := dataset.Asset{Name: "counts.csv", Request: dataset.Request{Method: "GET", URL: "https://data.test/counts.csv"}}
			where := map[string]string{"scope": "wanted"}
			var sample dataset.TableSample
			var err error
			if request.XLSX != nil {
				asset.Name, asset.Format, asset.Request.URL = request.Asset, "XLSX", "https://data.test/counts.xlsx"
				sample, err = reader.SampleXLSX(ctx, asset, *request.XLSX)
			} else if mode == "prefix" {
				sample, err = reader.SampleCSVSelected(ctx, asset, limit, where)
			} else {
				sample, err = reader.SampleCSVScanned(ctx, asset, limit, dataset.CSVSelection{Equals: where})
			}
			if err != nil {
				return goalwork.Acquired{}, err
			}
			a := goalwork.Acquired{Delivery: "FILE", ContentSHA256: sample.SHA256, ContractSHA256: strings.Repeat("b", 64), CSV: sample.CSV, Selection: sample.Selection, Table: sample.Table}
			for _, row := range sample.Rows {
				a.Rows = append(a.Rows, goalwork.Row(row))
			}
			switch mode {
			case "unknown":
				a.Selection = nil
			case "api":
				a.Delivery = "REST"
			case "missing hash":
				a.ContentSHA256 = ""
			case "bad position":
				a.CSV.DataRecords[1] = 4
			case "xlsx", "xlsx missing row", "xlsx wrong range":
				if mode == "xlsx missing row" {
					a.Table.RowNumbers = []int{2}
				}
				if mode == "xlsx wrong range" {
					a.Table.Range = "A2:C4"
				}
			}
			return a, nil
		},
		Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "complete published comparison", Region: "both source regions", Period: "source reference date", Coverage: "population", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "counts"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Description: "source count", Type: "string"}}}
	if mode == "group" {
		c.Outputs[0].Type = "number"
	}
	p := goalwork.Composition{ID: "all", Base: "o1", Purpose: "report every source region", Select: []string{"o1.area", "o1.n"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "n", Field: "o1.n"}}, Assumptions: []string{"source bounds and requested coverage require separate review"}}
	request := goalwork.SampleRequest{PK: "counts", Delivery: "file", Asset: "counts.csv", ScanCSV: true, Where: map[string]string{"scope": "wanted"}}
	if strings.HasPrefix(mode, "xlsx") {
		request.Asset, request.ScanCSV, request.Where = "counts.xlsx", false, nil
		request.XLSX = &dataset.XLSXSelection{Sheet: "table", Range: "A2:C3"}
		p.Select, p.Outputs = []string{"o1.B", "o1.C"}, []goalwork.OutputBinding{{Output: "n", Field: "o1.C"}}
	}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "counts", Role: "r"}, {Action: "inspect", PK: "counts"}, {Action: "sample", Sample: &request}} {
		advanceUnmatched(t, e, d)
	}
	if mode == "group" {
		o := e.View().Observations[0]
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: o.PK, Delivery: "file", Reduce: &goalwork.SourceReduction{Observation: o.ID, RowsSHA256: o.RowsSHA256, Measures: []goalwork.Measure{{As: "count", Field: "n", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "count"}}}}})
		p.Base, p.Select, p.Outputs = "o2", []string{"o2.total"}, []goalwork.OutputBinding{{Output: "n", Field: "o2.total"}}
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	return e
}
