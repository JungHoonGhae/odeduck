package goalwork_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestComparisonCanReportUnmatchedSourceValuesWithoutJoiningThem(t *testing.T) {
	e := unmatchedEngine(t, [][]goalwork.Row{
		{{"code": "a", "label": "matched"}, {"code": "b", "label": "UNMATCHED_LEFT"}},
		{{"code": "a", "n": "8"}, {"code": "c", "n": "0"}, {"code": "d", "n": nil}, {"code": "e"}},
	}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, nil)
	p := e.View().Compositions[0]
	p.ID = "whole-comparison"
	p.ReportUnmatched = []string{"o1.label", "o2.code", "o2.n"}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	want := []goalwork.UnmatchedTuple{
		{JoinIndex: 1, Side: "left", Positions: map[string]int{"o1": 2}, Values: goalwork.Row{"o1.label": "UNMATCHED_LEFT"}},
		{JoinIndex: 1, Side: "right", Positions: map[string]int{"o2": 2}, Values: goalwork.Row{"o2.code": "c", "o2.n": "0"}},
		{JoinIndex: 1, Side: "right", Positions: map[string]int{"o2": 3}, Values: goalwork.Row{"o2.code": "d", "o2.n": nil}},
		{JoinIndex: 1, Side: "right", Positions: map[string]int{"o2": 4}, Values: goalwork.Row{"o2.code": "e"}, Missing: []string{"o2.n"}},
	}
	if !reflect.DeepEqual(v.Artifact.Unmatched, want) || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o1.label"] != "matched" || v.Status != "review_required" {
		t.Fatalf("unmatched records missing, coerced or counted as successful joins: %+v", v.Artifact)
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "UNMATCHED_LEFT") {
		t.Fatal("result projection widened planner disclosure")
	}
	v.Artifact.Unmatched[0].Values["o1.label"] = "forged"
	v.Artifact.Unmatched[0].Positions["o1"] = 1
	if e.View().Artifact.Unmatched[0].Values["o1.label"] != "UNMATCHED_LEFT" || e.View().Artifact.Unmatched[0].Positions["o1"] != 2 {
		t.Fatal("returned result mutates the retained report")
	}
}

func TestUnmatchedReportPreservesLaterExcludedTuples(t *testing.T) {
	e := unmatchedEngine(t, [][]goalwork.Row{
		{{"code": "a", "label": "first"}, {"code": "b", "label": "second"}},
		{{"code": "a", "next": "x"}, {"code": "a", "next": "y"}, {"code": "z", "next": "z"}},
		{{"next": "x"}},
	}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}, {Right: "o3", LeftKeys: []string{"o2.next"}, RightKeys: []string{"next"}}}, nil)
	p := e.View().Compositions[0]
	p.ID, p.ReportUnmatched = "report-stages", []string{"o1.label", "o2.next", "o3.next"}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	last := v.Artifact.Unmatched[2]
	if len(v.Artifact.Unmatched) != 3 || last.JoinIndex != 2 || !reflect.DeepEqual(last.Positions, map[string]int{"o1": 1, "o2": 2}) || !reflect.DeepEqual(last.Values, goalwork.Row{"o1.label": "first", "o2.next": "y"}) || v.Artifact.Rows[0]["o1.label"] != "first" {
		t.Fatal("stage-local tuple was reduced to unique entities or confused with final-only exclusions")
	}
}

func TestUnmatchedReportRejectsUnknownFieldsAndCombinedOutputOverflow(t *testing.T) {
	for _, variant := range []string{"unknown", "alias", "duplicate", "rows", "bytes"} {
		t.Run(variant, func(t *testing.T) {
			left := []goalwork.Row{{"code": "a", "label": "matched"}, {"code": "b", "label": "excluded"}}
			if variant == "rows" {
				for len(left) < 1000 {
					left = append(left, goalwork.Row{"code": strconv.Itoa(len(left)), "label": "excluded"})
				}
			}
			if variant == "bytes" {
				left[1]["label"] = strings.Repeat("l", 1100000)
			}
			right := []goalwork.Row{{"code": "a", "n": "1"}, {"code": "c", "n": "0"}}
			if variant == "bytes" {
				right[1]["n"] = strings.Repeat("r", 1100000)
			}
			e := unmatchedEngine(t, [][]goalwork.Row{left, right}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, nil)
			advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: "compare"})
			p := e.View().Compositions[0]
			p.ID, p.ReportUnmatched = "with-report", []string{"o1.label", "o2.n"}
			switch variant {
			case "unknown":
				p.ReportUnmatched = []string{"o3.label"}
			case "alias":
				p.ReportUnmatched = []string{"computed"}
			case "duplicate":
				p.ReportUnmatched = []string{"o1.label", "o1.label"}
			}
			advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "execute", CompositionID: p.ID})
			if err != nil || len(v.Gaps) != 1 || v.Artifact != nil || v.Evaluation != nil || v.Executions[1].Status != "failed" {
				t.Fatal("invalid or oversized unmatched report became a current artifact")
			}
		})
	}
}

func TestUnmatchedReportFieldBudgetIsCheckedBeforeStoringComposition(t *testing.T) {
	e := unmatchedEngine(t, [][]goalwork.Row{{{"code": "a", "label": "a"}}, {{"code": "a"}}}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, nil)
	p := e.View().Compositions[0]
	p.ID, p.ReportUnmatched = "too-many", make([]string, 17)
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "compose", Composition: &p})
	if err != nil || len(v.Gaps) != 1 || len(v.Compositions) != 1 {
		t.Fatal("oversized report recipe was retained")
	}
}

func TestUnmatchedResultValuesRequireDisclosureBeforeAnalysisReview(t *testing.T) {
	calls := 0
	e := unmatchedEngine(t, [][]goalwork.Row{
		{{"code": "a", "label": "matched"}, {"code": "b", "label": "UNSELECTED_LEFT"}},
		{{"code": "a", "n": "UNSELECTED_MATCH"}, {"code": "c", "n": "REPORTED_RIGHT"}},
	}, []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.code"}, RightKeys: []string{"code"}}}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		b, _ := json.Marshal(in)
		if strings.Contains(string(b), "UNSELECTED_") || len(in.Artifact.Unmatched) != 2 || in.Artifact.Unmatched[1].Values["o2.n"] != "REPORTED_RIGHT" {
			t.Fatal("unmatched result was missing or disclosed unrelated values")
		}
		return supportedAnalysisReview(in), nil
	})
	p := e.View().Compositions[0]
	p.ID, p.ReportUnmatched = "with-report", []string{"o2.n"}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	for _, selection := range []goalwork.EvidenceRequest{
		{Observation: "o1", Rows: []int{1}, Fields: []string{"code", "label"}},
		{Observation: "o1", Rows: []int{2}, Fields: []string{"code"}},
		{Observation: "o2", Rows: []int{1, 2}, Fields: []string{"code"}},
	} {
		for _, o := range e.View().Observations {
			if o.ID == selection.Observation {
				selection.RowsSHA256 = o.RowsSHA256
			}
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &selection})
	}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || calls != 0 || len(v.Reviews) != 0 || len(v.Gaps) != 1 || v.Status == "output_ready" {
		t.Fatal("unmatched report bypassed selected evidence authority")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.Observations[1].RowsSHA256, Rows: []int{2}, Fields: []string{"n"}}})
	if err != nil || len(v.Gaps) != 1 {
		t.Fatal("report evidence unavailable")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || calls != 1 || v.Status != "output_ready" {
		t.Fatal("disclosed unmatched report could not reach bounded review")
	}
}
