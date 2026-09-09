package goalwork_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func reviewPacket(t *testing.T, in goalwork.ReviewInput, id string) goalwork.EvidencePacket {
	t.Helper()
	for _, packet := range in.EvidencePackets() {
		if packet.ID == id {
			return packet
		}
	}
	t.Fatalf("review context has no actual packet %s", id)
	return goalwork.EvidencePacket{}
}

func overlappingReviewEngine(t *testing.T, change func(*goalwork.Acquired), review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	e, err := goalwork.Start("Report the two original records, including their explicit null and zero", goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			rows := []goalwork.Row{{"A": "001", "B": nil, "C": "UNSELECTED_PRIVATE"}, {"A": "002", "B": json.Number("0"), "C": "UNSELECTED_PRIVATE"}}
			a := goalwork.Acquired{Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64), Rows: rows, Table: &dataset.TableProvenance{Sheet: "records", Member: "xl/worksheets/sheet1.xml", Range: s.XLSX.Range, RowNumbers: []int{2, 3}}}
			if s.XLSX.Range == "A1:C4" {
				a.Rows = append([]goalwork.Row{{"A": "Heading", "B": "", "C": "UNSELECTED_PRIVATE"}}, rows...)
				a.Rows = append(a.Rows, goalwork.Row{"A": "Note", "B": false, "C": "UNSELECTED_PRIVATE"})
				a.Table.RowNumbers = []int{1, 2, 3, 4}
			}
			if change != nil {
				change(&a)
			}
			return a, nil
		}, Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "original records", Region: "source", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "code", Role: "r", Type: "string", Description: "original code"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "records", Role: "r"})
	advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "records"})
	for _, rectangle := range []string{"A2:C3", "A1:C4"} {
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "file", Asset: "records.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "records", Range: rectangle}}})
	}
	p := goalwork.Composition{ID: "report", Purpose: "report original cells", Base: "o1", Select: []string{"o1.A", "o1.B"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "code", Field: "o1.A"}}, Assumptions: []string{"Source records, not present-day or causal claims"}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	return e
}

func TestAnalysisReusesDisclosedOriginalCellsAtDifferentRetainedPositions(t *testing.T) {
	called := false
	e := overlappingReviewEngine(t, nil, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		called = true
		if in.Analysis == nil || len(in.Artifact.Sources) != 1 || in.Artifact.Sources[0].ID != "o1" || len(in.Artifact.Rows) != 2 || in.Artifact.Rows[0]["o1.B"] != nil || in.Artifact.Rows[1]["o1.B"] != json.Number("0") {
			t.Fatal("context changed computational participation or original cell values")
		}
		b, _ := json.Marshal(in)
		if strings.Contains(string(b), "UNSELECTED_PRIVATE") || strings.Count(string(b), `"Heading"`) != 1 || len(in.EvidencePackets()) != 1 || len(in.Analysis.SourceContext) != 1 || in.Analysis.SourceContext[0].PacketID != in.Evidence.ID {
			t.Fatal("original-cell reuse disclosed an unselected field")
		}
		uses := in.Analysis.Sources[0].Disclosure
		if len(uses) != 2 || uses[0].RetainedRow != 1 || uses[0].PacketRow != 2 || uses[1].RetainedRow != 2 || uses[1].PacketRow != 3 || uses[0].PacketID != in.Evidence.ID || len(uses[0].Fields) != 2 {
			t.Fatal("review cannot trace reused values to both retained and original positions")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.Verdict = "insufficient" // A delivery fixture, not a semantic approval.
		return a, nil
	})
	o := e.View().Observations[1]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2, 3, 4}, Fields: []string{"A", "B"}}})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if err != nil || !called || len(v.Gaps) != 0 || len(v.Reviews) != 1 || len(v.Evidence) != 1 || v.Status == "output_ready" {
		t.Fatalf("same original cells were not reused without extra disclosure: called=%t gaps=%+v err=%v", called, v.Gaps, err)
	}
}

func TestAnalysisRejectsUnprovenOrConflictingOriginalCellReuse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*goalwork.Acquired)
		rows   []int
		fields []string
	}{
		{name: "different content", change: func(a *goalwork.Acquired) { a.ContentSHA256 = strings.Repeat("c", 64) }},
		{name: "different contract", change: func(a *goalwork.Acquired) { a.ContractSHA256 = strings.Repeat("c", 64) }},
		{name: "missing hash", change: func(a *goalwork.Acquired) { a.ContentSHA256 = "" }},
		{name: "missing table", change: func(a *goalwork.Acquired) { a.Table = nil }},
		{name: "API observation", change: func(a *goalwork.Acquired) { a.Delivery = "REST" }},
		{name: "CSV observation", change: func(a *goalwork.Acquired) {
			a.Table = nil
			a.CSV = &dataset.CSVProvenance{DataRecords: []int{1, 2, 3, 4}, StartLines: []int{2, 3, 4, 5}}
		}},
		{name: "different sheet", change: func(a *goalwork.Acquired) { a.Table.Sheet = "other" }},
		{name: "different worksheet member", change: func(a *goalwork.Acquired) { a.Table.Member = "xl/worksheets/sheet2.xml" }},
		{name: "missing worksheet member", change: func(a *goalwork.Acquired) { a.Table.Member = "" }},
		{name: "inconsistent range", change: func(a *goalwork.Acquired) { a.Table.Range = "A5:C8" }},
		{name: "duplicate positions", change: func(a *goalwork.Acquired) { a.Table.RowNumbers[2] = 2 }},
		{name: "noncoordinate field", change: func(a *goalwork.Acquired) { a.Rows[0]["renamed"] = "value" }},
		{name: "value conflict", change: func(a *goalwork.Acquired) { a.Rows[1]["A"] = "different" }},
		{name: "type conflict", change: func(a *goalwork.Acquired) { a.Rows[2]["B"] = "0" }},
		{name: "missing versus null", change: func(a *goalwork.Acquired) { delete(a.Rows[1], "B") }},
		{name: "only headings", rows: []int{1, 4}},
		{name: "partial rows", rows: []int{2}},
		{name: "withheld field", fields: []string{"A"}},
		{name: "same values at other addresses", rows: []int{1, 4}, change: func(a *goalwork.Acquired) {
			a.Rows[0]["A"], a.Rows[0]["B"] = "001", nil
			a.Rows[3]["A"], a.Rows[3]["B"] = "002", json.Number("0")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := overlappingReviewEngine(t, func(a *goalwork.Acquired) {
				if a.Table.Range == "A1:C4" && tc.change != nil {
					tc.change(a)
				}
			}, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				t.Fatal("unproven or conflicting reuse reached the reviewer")
				return goalwork.ReviewAssessment{}, nil
			})
			rows, fields := tc.rows, tc.fields
			if rows == nil {
				rows = []int{1, 2, 3, 4}
			}
			if fields == nil {
				fields = []string{"A", "B"}
			}
			o := e.View().Observations[1]
			advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: rows, Fields: fields}})
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if err != nil || len(v.Gaps) != 1 || len(v.Reviews) != 0 || v.Status == "output_ready" || len(v.Artifact.Rows) != 2 {
				t.Fatalf("invalid disclosure altered the result or review budget: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestAnalysisPreservesDisclosedMissingCellsAcrossOriginalObservations(t *testing.T) {
	called := false
	e := overlappingReviewEngine(t, func(a *goalwork.Acquired) {
		for i, row := range a.Table.RowNumbers {
			if row == 2 {
				delete(a.Rows[i], "B")
			}
		}
	}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		called = true
		if _, exists := in.Evidence.Records[0].Values["B"]; exists || len(in.Evidence.Records[0].Missing) != 1 || in.Evidence.Records[0].Missing[0] != "B" {
			t.Fatal("selected missing cell became null or an undisclosed cell")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.Verdict = "insufficient"
		return a, nil
	})
	o := e.View().Observations[1]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2, 3}, Fields: []string{"A", "B"}}})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if !called || len(v.Reviews) != 1 || v.Status == "output_ready" {
		t.Fatal("matching selected absence was not preserved")
	}
}

func TestSameOriginalCellsCannotBuyAnotherReviewThroughAnObservationAlias(t *testing.T) {
	calls := 0
	e := overlappingReviewEngine(t, nil, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		a := supportedAnalysisReview(in)
		a.GoalFit.Verdict = "insufficient"
		return a, nil
	})
	o := e.View().Observations[1]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2, 3}, Fields: []string{"A", "B"}}})
	advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	o = e.View().Observations[0]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2, 1}, Fields: []string{"B", "A"}}})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if err != nil || calls != 1 || len(v.Reviews) != 1 || len(v.Gaps) != 1 || len(v.Evidence) != 2 || v.Budget.EvidencePacketsRemaining != 6 {
		t.Fatalf("repackaged original cells refreshed review or disclosure budget: calls=%d gaps=%+v err=%v", calls, v.Gaps, err)
	}
}

func TestReusedDisclosureRejectsReviewMutationAndCancellation(t *testing.T) {
	for _, scenario := range []string{"packet reference", "packet row", "target row", "field", "context reference", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e := overlappingReviewEngine(t, nil, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				a := supportedAnalysisReview(in)
				use := &in.Analysis.Sources[0].Disclosure[0]
				switch scenario {
				case "packet reference":
					use.PacketID = "invented"
				case "packet row":
					use.PacketRow = 99
				case "target row":
					use.RetainedRow = 99
				case "field":
					use.Fields[0] = "UNSELECTED_PRIVATE"
				case "context reference":
					in.Analysis.SourceContext[0].PacketID = "invented"
				case "cancelled":
					cancel()
				}
				return a, nil
			})
			o := e.View().Observations[1]
			advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2, 3}, Fields: []string{"A", "B"}}})
			v, _ := e.Advance(ctx, e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if v.Status == "output_ready" || len(v.Reviews) != 1 || v.Reviews[0].Status != "failed" || v.Evaluation.Review != nil || len(v.Evidence) != 1 || v.Evidence[0].Records[0].RetainedRow != 2 || v.Artifact.Rows[0]["o1.A"] != "001" {
				t.Fatal("review mutation or cancellation approved a result or changed retained evidence")
			}
		})
	}
}

func TestConflictingDisclosedOriginalCellsCannotBeResolvedByPacketOrder(t *testing.T) {
	for _, order := range [][]int{{0, 1}, {1, 0}} {
		e := overlappingReviewEngine(t, func(a *goalwork.Acquired) {
			if a.Table.Range == "A1:C4" {
				a.Rows[2]["B"] = "0" // Same purported address, different raw JSON type.
			}
		}, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("conflicting disclosed cells reached review")
			return goalwork.ReviewAssessment{}, nil
		})
		for _, index := range order {
			o := e.View().Observations[index]
			rows := []int{1, 2}
			if index == 1 {
				rows = []int{2, 3}
			}
			advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: rows, Fields: []string{"A", "B"}}})
		}
		v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
		if err != nil || len(v.Gaps) != 1 || len(v.Reviews) != 0 || len(v.Evidence) != 2 || v.Artifact.Rows[1]["o1.B"] != json.Number("0") {
			t.Fatal("conflict consumed review authority or overwrote an original value")
		}
	}
}

func TestReusedDisclosureCannotSurviveGoalExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := overlappingReviewEngine(t, nil, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			time.Sleep(time.Hour + time.Second) // Virtual time inside synctest.
			return supportedAnalysisReview(in), nil
		})
		o := e.View().Observations[1]
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2, 3}, Fields: []string{"A", "B"}}})
		v, _ := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
		if v.Status != "expired" || v.Artifact != nil || len(v.Evidence) != 0 || len(v.Reviews) != 1 || v.Reviews[0].Status != "failed" || v.Evaluation.Review != nil {
			t.Fatal("reused original cells or approval survived expiry")
		}
	})
}
