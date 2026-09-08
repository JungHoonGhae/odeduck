package goalwork_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func comparisonFixture(t *testing.T, left, right []goalwork.Row, reviewers ...func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	acquisitions := 0
	e, err := goalwork.Start("두 원천의 측정값을 대조하고 다른 기록을 보존한다", goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, goalwork.Dependencies{
		Review: func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			if len(reviewers) > 0 {
				return reviewers[0](ctx, in)
			}
			return goalwork.ReviewAssessment{}, fmt.Errorf("no reviewer configured by this fixture")
		},
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "left"}, {PK: "right"}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			acquisitions++
			if acquisitions > 2 {
				t.Fatal("local comparison attempted another external acquisition")
			}
			rows, hash := left, strings.Repeat("a", 64)
			if s.PK == "right" {
				rows, hash = right, strings.Repeat("b", 64)
			}
			return goalwork.Acquired{Rows: rows, Delivery: "FILE", ContentSHA256: hash, ContractSHA256: strings.Repeat("c", 64)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "compare source measurements", Region: "source records", Period: "declared source dates", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source records"}}, Outputs: []goalwork.OutputRequirement{{ID: "value", Role: "r", Type: "string", Description: "original value"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "measurements", Role: "r"})
	for _, pk := range []string{"left", "right"} {
		advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: pk})
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "file", Asset: pk + ".csv"}})
	}
	return e
}

func comparisonRequest(t *testing.T, e *goalwork.Engine) goalwork.SampleRequest {
	t.Helper()
	v := e.View()
	wire := fmt.Sprintf(`{"pk":"left","delivery":"file","compare":{"left":{"observation":"o1","rowsSha256":%q,"keys":[{"field":"code","rule":"exact"}]},"right":{"observation":"o2","rowsSha256":%q,"keys":[{"field":"label","rule":"trailing_parenthesized_digits_v1","digits":3}]},"checks":[{"id":"total","left":{"op":"sum_fields","fields":["first","second"],"format":"decimal_v1","unit":"units"},"right":{"field":"total","format":"grouped_decimal_v1","unit":"units"}}]}}`, v.Observations[0].RowsSHA256, v.Observations[1].RowsSHA256)
	var s goalwork.SampleRequest
	if err := json.Unmarshal([]byte(wire), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func readComparisonSummary(t *testing.T, e *goalwork.Engine) map[string]any {
	t.Helper()
	o := e.View().Observations[2]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, Fields: []string{"metric", "value"}}})
	v := e.View()
	values := map[string]any{}
	for _, record := range v.Evidence[len(v.Evidence)-1].Records {
		values[record.Values["metric"].(string)] = record.Values["value"]
	}
	return values
}

func TestCompleteComparisonSummaryFitsOneExistingEvidencePacket(t *testing.T) {
	e := comparisonFixture(t, []goalwork.Row{{"code": "001", "first": "1", "second": "2"}}, []goalwork.Row{{"label": "A (001)", "total": "4"}})
	s := comparisonRequest(t, e)
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	values := readComparisonSummary(t, e)
	if len(values) != 13 || values["different"] != json.Number("1") || values["equal"] != json.Number("0") || len(e.View().Evidence) != 1 {
		t.Fatalf("complete mismatch accounting did not fit one packet: %+v", values)
	}
}

func TestComparisonSupportRequiresCompleteAccountingForItsTarget(t *testing.T) {
	for _, variant := range []string{"equality only", "metric only", "missing discrepancy"} {
		t.Run(variant, func(t *testing.T) {
			e := comparisonFixture(t, []goalwork.Row{{"code": "001", "first": "1", "second": "2"}}, []goalwork.Row{{"label": "A (001)", "total": "4"}})
			s := comparisonRequest(t, e)
			advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
			o := e.View().Observations[2]
			selection := goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, Fields: []string{"metric", "value"}}
			if variant == "equality only" {
				selection.Rows = []int{10}
			}
			if variant == "metric only" {
				selection.Fields = []string{"metric"}
			}
			if variant == "missing discrepancy" {
				selection.Rows = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13}
			}
			advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &selection})
			p := goalwork.Composition{ID: "selective", Base: "o1", Purpose: "report original codes", Select: []string{"o1.code"}, Assumptions: []string{"comparison alone is not applicability"}, Support: []goalwork.SupportBinding{{PacketID: e.View().Evidence[0].ID, Targets: []string{"o1"}, Purpose: "Check applicability"}}}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "compose", Composition: &p})
			if err != nil || len(v.Gaps) != 1 || len(v.Compositions) != 0 {
				t.Fatalf("selective comparison accepted as context: %s %+v %v", variant, v.Gaps, err)
			}
		})
	}
}

func TestSourceComparisonAccountsForEveryAmbiguousMissingInvalidAndUnmatchedRecord(t *testing.T) {
	e := comparisonFixture(t, []goalwork.Row{
		{"code": "001", "first": "1", "second": "2"},
		{"code": "002", "first": nil, "second": "2"},
		{"code": "003", "first": "bad", "second": "2"},
		{"code": "004", "first": "1", "second": "2"},
		{"code": "004", "first": "1", "second": "2"},
		{"code": nil, "first": "1", "second": "2"},
		{"code": json.Number("7"), "first": "1", "second": "2"},
		{"code": "008", "first": "1", "second": "2"},
	}, []goalwork.Row{
		{"label": "A (001)", "total": "4"}, {"label": "B (002)", "total": "0"}, {"label": "C (003)", "total": "3"},
		{"label": "D (004)", "total": "3"}, {"label": "Zero (006)", "total": "0"}, {"label": "malformed", "total": "0"}, {"label": "Not numeric identity (007)", "total": "3"},
	})
	s := comparisonRequest(t, e)
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	values := readComparisonSummary(t, e)
	want := map[string]string{"leftRows": "8", "rightRows": "7", "matchedPairs": "3", "leftOnly": "1", "rightOnly": "2", "leftUnresolved": "4", "rightUnresolved": "2", "checks": "1", "comparisons": "3", "equal": "0", "different": "1", "missing": "1", "invalid": "1"}
	for metric, n := range want {
		if values[metric] != json.Number(n) {
			t.Errorf("%s=%v want %s", metric, values[metric], n)
		}
	}
	p := e.View().Observations[2].Comparison
	if !reflect.DeepEqual(p.Pairs, [][2]int{{1, 1}, {2, 2}, {3, 3}}) {
		t.Fatalf("duplicate keys formed pairs: %+v", p.Pairs)
	}
	found := false
	for _, origin := range p.Records {
		if reflect.DeepEqual(origin.Left, []int{4, 5}) && reflect.DeepEqual(origin.Right, []int{4}) {
			found = true
		}
	}
	if !found {
		t.Fatal("ambiguous key lost a duplicate member or its opposite source row")
	}
}

func TestSourceComparisonKeepsCompositeKeysAndLeadingZeroes(t *testing.T) {
	e := comparisonFixture(t, []goalwork.Row{{"code": "001", "period": "2025", "first": "1", "second": "2"}, {"code": "001", "period": "2026", "first": "2", "second": "3"}, {"code": "01", "period": "2026", "first": "1", "second": "2"}}, []goalwork.Row{{"label": "B (001)", "period": "2026", "total": "5"}, {"label": "A (001)", "period": "2025", "total": "3"}})
	s := comparisonRequest(t, e)
	s.Compare.Left.Keys = append(s.Compare.Left.Keys, goalwork.ComparisonKey{Field: "period", Rule: "exact"})
	s.Compare.Right.Keys = append(s.Compare.Right.Keys, goalwork.ComparisonKey{Field: "period", Rule: "exact"})
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	values := readComparisonSummary(t, e)
	if values["matchedPairs"] != json.Number("2") || values["equal"] != json.Number("2") || values["leftOnly"] != json.Number("1") || values["leftUnresolved"] != json.Number("0") {
		t.Fatalf("tuple or leading zeroes changed: %+v", values)
	}
	if got := e.View().Observations[2].Comparison.Pairs; !reflect.DeepEqual(got, [][2]int{{1, 2}, {2, 1}}) {
		t.Fatalf("periods not aligned by actual tuples: %+v", got)
	}
}

func TestSourceComparisonRejectsInvalidRecipesWithoutPartialObservations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*goalwork.SampleRequest)
	}{
		{"stale", func(s *goalwork.SampleRequest) { s.Compare.Left.RowsSHA256 = strings.Repeat("0", 64) }},
		{"same source", func(s *goalwork.SampleRequest) { s.Compare.Right = s.Compare.Left }},
		{"unknown field", func(s *goalwork.SampleRequest) { s.Compare.Left.Keys[0].Field = "absent" }},
		{"numeric identifier coercion", func(s *goalwork.SampleRequest) { s.Compare.Left.Keys[0].Rule = "number" }},
		{"digit width", func(s *goalwork.SampleRequest) { s.Compare.Right.Keys[0].Digits = 0 }},
		{"arity", func(s *goalwork.SampleRequest) { s.Compare.Right.Keys = nil }},
		{"unit mismatch", func(s *goalwork.SampleRequest) { s.Compare.Checks[0].Right.Unit = "other unit" }},
		{"duplicate check", func(s *goalwork.SampleRequest) { s.Compare.Checks = append(s.Compare.Checks, s.Compare.Checks[0]) }},
		{"missing check", func(s *goalwork.SampleRequest) { s.Compare.Checks = nil }},
		{"alias", func(s *goalwork.SampleRequest) { s.Compare.Checks[0].Left.As = "first" }},
		{"unknown format", func(s *goalwork.SampleRequest) { s.Compare.Checks[0].Right.Format = "guessed_locale" }},
		{"unknown measurement", func(s *goalwork.SampleRequest) { s.Compare.Checks[0].Right.Field = "absent" }},
		{"duplicate terms", func(s *goalwork.SampleRequest) { s.Compare.Checks[0].Left.Fields = []string{"first", "first"} }},
		{"mixed acquisition", func(s *goalwork.SampleRequest) { s.Asset = "guessed.csv" }},
		{"mixed operation", func(s *goalwork.SampleRequest) { s.Operation = "mois-monthly-age-csv" }},
		{"changed delivery", func(s *goalwork.SampleRequest) { s.Delivery = "api" }},
		{"other owner", func(s *goalwork.SampleRequest) { s.PK = "right" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := comparisonFixture(t, []goalwork.Row{{"code": "001", "first": "1", "second": "2"}}, []goalwork.Row{{"label": "A (001)", "total": "3"}})
			s := comparisonRequest(t, e)
			tc.change(&s)
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 || len(v.Evidence) != 0 {
				t.Fatalf("invalid comparison retained partial evidence: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestSourceComparisonBoundsCompleteReportWithoutDroppingDiscrepancies(t *testing.T) {
	for _, variant := range []string{"row limit", "byte limit"} {
		t.Run(variant, func(t *testing.T) {
			var left, right []goalwork.Row
			count, padding := 500, ""
			if variant == "byte limit" {
				count, padding = 450, strings.Repeat("x", 2000)
			}
			for i := 0; i < count; i++ {
				left = append(left, goalwork.Row{"code": fmt.Sprintf("L%03d%s", i, padding), "secondKey": padding + "left", "first": "1", "second": "2"})
				right = append(right, goalwork.Row{"label": fmt.Sprintf("R%03d%s", i, padding), "secondKey": padding + "right", "total": "3"})
			}
			e := comparisonFixture(t, left, right)
			s := comparisonRequest(t, e)
			s.Compare.Right.Keys = []goalwork.ComparisonKey{{Field: "label", Rule: "exact"}}
			if variant == "byte limit" {
				s.Compare.Left.Keys = append(s.Compare.Left.Keys, goalwork.ComparisonKey{Field: "secondKey", Rule: "exact"})
				s.Compare.Right.Keys = append(s.Compare.Right.Keys, goalwork.ComparisonKey{Field: "secondKey", Rule: "exact"})
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 || v.SampleAttempts[len(v.SampleAttempts)-1].Status != "failed" {
				t.Fatalf("oversized report became a truncated observation: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestSourceComparisonRejectsNumericExpansionWithoutPartialObservation(t *testing.T) {
	for _, variant := range []string{"sum carry", "left exponent", "right exponent", "missing peer", "invalid peer"} {
		t.Run(variant, func(t *testing.T) {
			left := []goalwork.Row{{"code": "001", "first": strings.Repeat("9", 256), "second": "1"}}
			right := []goalwork.Row{{"label": "A (001)", "total": "1"}}
			if variant == "left exponent" {
				left[0]["first"] = json.Number("1e256")
			}
			if variant == "right exponent" {
				left[0]["first"], right[0]["total"] = "1", json.Number("1e256")
			}
			if variant == "missing peer" {
				right[0]["total"] = nil
			}
			if variant == "invalid peer" {
				right[0]["total"] = "not a number"
			}
			e := comparisonFixture(t, left, right)
			s := comparisonRequest(t, e)
			if variant == "left exponent" {
				s.Compare.Checks[0].Left = goalwork.Measure{Field: "first", Format: "decimal_v1", Unit: "units"}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 || len(v.Evidence) != 0 || v.SampleAttempts[2].Status != "failed" {
				t.Fatalf("expanded numeric comparison did not fail atomically: gaps=%+v err=%v", v.Gaps, err)
			}
		})
	}
}

func TestSourceComparisonUsesExactDecimalsAndDisclosesOnlySelectedComputedEvidence(t *testing.T) {
	e := comparisonFixture(t,
		[]goalwork.Row{{"code": "001", "first": "9007199254740993.1", "second": "0.2", "private": "UNSELECTED_PRIVATE"}, {"code": "002", "first": "12.5", "second": "7.5"}, {"code": "003", "first": "5", "second": "5"}},
		[]goalwork.Row{{"label": "Different name (002)", "total": "20"}, {"label": "Other name (001)", "total": "9,007,199,254,740,993.3"}, {"label": "Extra zero record (004)", "total": "0"}},
	)
	before, _ := json.Marshal(e.View().Observations)
	s := comparisonRequest(t, e)
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	v := e.View()
	if len(v.Observations) != 3 || v.Observations[2].Delivery != "DERIVED" || v.Artifact != nil || len(v.Evidence) != 0 {
		t.Fatalf("comparison became a source/result/disclosure: %+v", v)
	}
	after, _ := json.Marshal(v.Observations[:2])
	if string(before) != string(after) {
		t.Fatal("comparison rewrote original observations")
	}
	wire, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(wire), "UNSELECTED_PRIVATE") || strings.Contains(string(wire), "9007199254740993.3") || strings.Contains(string(wire), "Other name") {
		t.Fatal("unselected source or comparison values leaked")
	}
	o := v.Observations[2]
	values := readComparisonSummary(t, e)
	v = e.View()
	for field, want := range map[string]string{"leftRows": "3", "rightRows": "3", "matchedPairs": "2", "leftOnly": "1", "rightOnly": "1", "leftUnresolved": "0", "rightUnresolved": "0"} {
		if got := values[field]; got != json.Number(want) {
			t.Errorf("alignment %s=%v want %s", field, got, want)
		}
	}
	for field, want := range map[string]string{"checks": "1", "comparisons": "2", "equal": "2", "different": "0", "missing": "0", "invalid": "0"} {
		if got := values[field]; got != json.Number(want) {
			t.Errorf("numeric %s=%v want %s", field, got, want)
		}
	}
	for i, record := range v.Evidence[0].Records {
		if origin := record.Origins["value"]; origin.Kind != "computed_comparison" || origin.Observation != o.ID || origin.Ordinal != i+1 {
			t.Fatalf("computed count attributed to a publisher cell: %+v", origin)
		}
	}
}

func TestSourceComparisonCannotBecomeAComputationalSource(t *testing.T) {
	for _, variant := range []string{"base", "join", "reduce"} {
		t.Run(variant, func(t *testing.T) {
			e := comparisonFixture(t, []goalwork.Row{{"code": "001", "first": "1", "second": "2"}}, []goalwork.Row{{"label": "A (001)", "total": "3"}})
			s := comparisonRequest(t, e)
			advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
			p := goalwork.Composition{ID: "misuse", Base: "o3", Purpose: "misuse support as publisher rows", Select: []string{"o3.kind"}, Assumptions: []string{"support is not publisher data"}}
			d := goalwork.Decision{Action: "compose", Composition: &p}
			if variant == "join" {
				p.Base = "o1"
				p.Joins = []goalwork.Join{{Right: "o3", LeftKeys: []string{"o1.code"}, RightKeys: []string{"kind"}}}
			}
			if variant == "reduce" {
				d = goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "left", Delivery: "file", Reduce: &goalwork.SourceReduction{Observation: "o3", RowsSHA256: e.View().Observations[2].RowsSHA256, GroupBy: []string{"kind"}, Aggregates: []goalwork.Aggregate{{As: "count", Op: "count"}}}}}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, d)
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 3 || len(v.Compositions) != 0 {
				t.Fatalf("computed support entered %s: observations=%d compositions=%d gaps=%+v err=%v", variant, len(v.Observations), len(v.Compositions), v.Gaps, err)
			}
		})
	}
}

func TestSourceComparisonSupportCarriesBothRevisionsWithoutChangingTheResult(t *testing.T) {
	calls := 0
	e := comparisonFixture(t, []goalwork.Row{{"code": "001", "first": "1", "second": "2", "private": "UNSELECTED_PRIVATE"}}, []goalwork.Row{{"label": "PRIVATE_LABEL (001)", "total": "3"}}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		if in.Analysis == nil || len(in.Analysis.SourceContext) != 1 || len(in.Artifact.Sources) != 1 || len(in.Artifact.Rows) != 1 || in.Artifact.Rows[0]["o1.code"] != "001" {
			t.Fatalf("support altered computational participation: %+v", in)
		}
		c := in.Analysis.SourceContext[0]
		wire, _ := json.Marshal(c)
		var decoded struct {
			ComparisonSources []struct {
				Source  goalwork.Observation
				Request goalwork.SampleRequest
			}
		}
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.ComparisonSources) != 2 {
			t.Fatalf("computed support omitted its two original revisions: %s", wire)
		}
		for i, source := range decoded.ComparisonSources {
			wantID, wantPK, wantHash := "o1", "left", strings.Repeat("a", 64)
			if i == 1 {
				wantID, wantPK, wantHash = "o2", "right", strings.Repeat("b", 64)
			}
			if source.Source.ID != wantID || source.Source.ContentSHA256 != wantHash || source.Request.PK != wantPK || source.Request.Asset != wantPK+".csv" {
				t.Fatalf("comparison provenance lost: %+v", source)
			}
		}
		if !c.Proposed || len(c.Targets) != 1 || c.Targets[0] != "o1" || c.Source.Delivery != "DERIVED" || c.Source.Comparison == nil || c.Evidence.Records[9].Values["metric"] != "equal" || c.Evidence.Records[9].Values["value"] != json.Number("1") {
			t.Fatalf("lost computed/proposed separation: %+v", c)
		}
		wire, _ = json.Marshal(in)
		if strings.Contains(string(wire), "UNSELECTED_PRIVATE") || strings.Contains(string(wire), "PRIVATE_LABEL") {
			t.Fatal("comparison replay disclosed unselected original cells")
		}
		return goalwork.ReviewAssessment{}, fmt.Errorf("captured input, not semantic approval")
	})
	s := comparisonRequest(t, e)
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	v := e.View()
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1}, Fields: []string{"code"}}})
	readComparisonSummary(t, e)
	p := goalwork.Composition{ID: "report", Base: "o1", Purpose: "report the original identifier", Select: []string{"o1.code"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "value", Field: "o1.code"}}, Assumptions: []string{"comparison supports interpretation but does not establish it"}, Support: []goalwork.SupportBinding{{PacketID: e.View().Evidence[1].ID, Targets: []string{"o1"}, Purpose: "Check this source's relationship to the other observed distribution"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || calls != 1 || len(v.Reviews) != 1 || v.Status == "output_ready" {
		t.Fatalf("comparison context did not reach separate review: calls=%d gaps=%+v err=%v", calls, v.Gaps, err)
	}
}
