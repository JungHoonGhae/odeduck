package goalwork_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSourceExplanationCannotBeReplacedByAnExecutionDisclaimer(t *testing.T) {
	c := resultContract()
	// Decode the caller's contract so the regression also detects silently lost
	// source-grounding requirements, rather than a private explanation helper.
	if err := json.Unmarshal([]byte(`{"explanations":[{"id":"source_dates","topic":"temporal","basis":"source","description":"Report the actual source reference date."}]}`), &c); err != nil {
		t.Fatal(err)
	}
	e := observedResultEngine(t, c)
	v := executeResult(t, e, resultRecipe())
	if v.Artifact == nil || v.Evaluation.Status != "partial" || v.Status == "output_ready" {
		t.Fatal("generic execution disclaimer satisfied a required source-specific explanation")
	}
	found := false
	for _, check := range v.Evaluation.Checks {
		if check.Kind == "explanation" && check.ID == "source_dates" && !check.Passed {
			found = true
		}
	}
	if !found || v.Contract.Explanations[0].Description != "Report the actual source reference date." || len(v.Artifact.Rows) != 2 {
		t.Fatal("missing explanation did not preserve the requirement and partial data output")
	}
}

func TestSourceExplanationProseIsDiscardedWhenTheGoalExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, p := sourceExplanationFixture(t, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("expiry test must not call a reviewer")
			return goalwork.ReviewAssessment{}, nil
		})
		executeResult(t, e, p)
		time.Sleep(time.Hour + time.Second)
		v := e.View()
		wire, _ := json.Marshal(v)
		if v.Status != "expired" || v.Artifact != nil || len(v.Evidence) != 0 || strings.Contains(string(wire), p.Explanations[0].Text) {
			t.Fatal("source-derived explanation prose survived selected-evidence expiry")
		}
	})
}

func TestSourceExplanationRejectsInventedOrUnselectedCitations(t *testing.T) {
	for _, name := range []string{"unknown requirement", "execution requirement", "duplicate requirement", "empty text", "long text", "credential text", "invalid UTF-8", "no citations", "duplicate citation", "too many citations", "unknown packet", "undisclosed field", "undisclosed row"} {
		t.Run(name, func(t *testing.T) {
			e, p := sourceExplanationFixture(t, nil)
			d := &p.Explanations[0]
			switch name {
			case "unknown requirement":
				d.ID = "invented"
			case "execution requirement":
				d.ID = "execution_limits"
			case "duplicate requirement":
				p.Explanations = append(p.Explanations, *d)
			case "empty text":
				d.Text = " "
			case "long text":
				d.Text = strings.Repeat("a", 2001)
			case "credential text":
				d.Text = "serviceKey=fixture-secret-never-return"
			case "invalid UTF-8":
				d.Text = string([]byte{0xff})
			case "no citations":
				d.Citations = nil
			case "duplicate citation":
				d.Citations = append(d.Citations, d.Citations[0])
			case "too many citations":
				d.Citations = make([]goalwork.EvidenceCitation, 17)
			case "unknown packet":
				d.Citations[0].PacketID = "ep_another_goal"
			case "undisclosed field":
				d.Citations[0].Field = "private"
			case "undisclosed row":
				d.Citations[0].PacketRow = 2
			}
			priorRevision := e.View().Revision
			v, err := e.Advance(context.Background(), priorRevision, goalwork.Decision{Action: "compose", Composition: &p})
			if name == "invalid UTF-8" {
				if err == nil || len(v.Compositions) != 0 || v.Revision != priorRevision {
					t.Fatal("JSON encoding silently replaced invalid explanation text")
				}
				return
			}
			if err != nil || len(v.Gaps) != 1 || len(v.Compositions) != 0 || v.Artifact != nil || v.Budget.EvidencePacketsRemaining != 6 || strings.Contains(fmt.Sprint(v.Gaps), "fixture-secret-never-return") {
				t.Fatalf("invalid explanation was retained or disclosed: %s %v", name, err)
			}
		})
	}
}

func TestSourceExplanationNeedsItsBasisInTheActualReviewContext(t *testing.T) {
	e, p := sourceExplanationFixture(t, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		t.Fatal("unrelated packet was silently added to the review")
		return goalwork.ReviewAssessment{}, nil
	})
	p.Support = nil
	executeResult(t, e, p)
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || len(v.Gaps) != 1 || len(v.Reviews) != 0 || v.Status == "output_ready" || len(v.Evidence) != 2 {
		t.Fatal("missing applicability context widened review disclosure or lost evidence")
	}
}

func sourceExplanationAssessment(in goalwork.ReviewInput) goalwork.ReviewAssessment {
	a := supportedAnalysisReview(in)
	a.Explanations = []goalwork.ExplanationReview{{Explanation: "source_dates", Finding: a.GoalFit}}
	return a
}

func TestSourceExplanationCannotBypassItsOwnFindingOrChangeReviewedInput(t *testing.T) {
	for _, name := range []string{"missing", "duplicate", "unknown", "unsupported", "insufficient", "unknown packet", "unrelated support", "mutated text", "mutated citation", "weakened requirement"} {
		t.Run(name, func(t *testing.T) {
			e, p := sourceExplanationFixture(t, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				a := sourceExplanationAssessment(in)
				switch name {
				case "missing":
					a.Explanations = nil
				case "duplicate":
					a.Explanations = append(a.Explanations, a.Explanations[0])
				case "unknown":
					a.Explanations[0].Explanation = "invented"
				case "unsupported", "insufficient":
					a.Explanations[0].Finding.Verdict = name
				case "unknown packet":
					a.Explanations[0].Finding.PacketIDs = []string{"ep_invented"}
				case "unrelated support":
					a.Explanations[0].Finding.PacketIDs = []string{in.Evidence.ID}
				case "mutated text":
					in.Artifact.Explanations[0].Text = "fabricated date"
				case "mutated citation":
					in.Artifact.Explanations[0].Citations[0].Field = "note"
				case "weakened requirement":
					in.Contract.Explanations[0].Basis = "execution"
				}
				return a, nil
			})
			executeResult(t, e, p)
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
			if err != nil || v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview || len(v.Reviews) != 1 || v.Contract.Explanations[0].Basis != "source" || v.Artifact.Explanations[0].Text != p.Explanations[0].Text || v.Artifact.Explanations[0].Citations[0].Field != "date" {
				t.Fatalf("invalid explanation review approved or mutated the result: %s", name)
			}
		})
	}
}

func TestSourceExplanationCorrectionRequiresANewExecutionAndKeepsReviewHistory(t *testing.T) {
	calls := 0
	e, p := sourceExplanationFixture(t, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		a := sourceExplanationAssessment(in)
		if calls == 1 {
			a.Explanations[0].Finding.Verdict = "insufficient"
		}
		return a, nil
	})
	executeResult(t, e, p)
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	first := v.Evaluation.ExecutionRevision
	if v.Status != "review_required" {
		t.Fatal("insufficient explanation ended the goal")
	}
	p.ID = "clarified"
	p.Explanations[0].Text = "기록에 명시된 기준일은 2025-06-30이며 현재 상태를 뜻하지 않습니다."
	v = executeResult(t, e, p)
	if v.Evaluation.Review != nil || v.Evaluation.ExecutionRevision == first || len(v.Reviews) != 1 {
		t.Fatal("explanation correction carried over an earlier review")
	}
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if v.Status != "output_ready" || calls != 2 || len(v.Reviews) != 2 || v.Reviews[0].ExecutionRevision != first || v.Budget.ReviewsRemaining != 1 || v.Budget.EvidencePacketsRemaining != 6 || len(v.Executions) != 2 {
		t.Fatal("explanation correction changed the goal, disclosed more values or reset review history")
	}
}

func TestSourceExplanationCarriesItsSelectedCitationsIntoSeparateReview(t *testing.T) {
	e, p := sourceExplanationFixture(t, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		wire, _ := json.Marshal(in)
		if len(in.Artifact.Explanations) != 1 {
			t.Fatal("source-specific explanation is absent from the actual review artifact")
		}
		note := in.Artifact.Explanations[0]
		if note.ID != "source_dates" || note.Text != "이 관측의 기준일은 2025-06-30입니다. 조회일이나 현재 날씨를 뜻하지 않습니다." || len(note.Citations) != 1 || note.Citations[0].PacketRow != 1 || note.Citations[0].Field != "date" || in.Analysis == nil || len(in.Analysis.SourceContext) != 1 {
			t.Fatal("explanation lost its exact proposal, selection or applicability context")
		}
		if len(in.Artifact.Evaluation.Explanations) != 1 || in.Artifact.Evaluation.Explanations[0].ID != "execution_limits" {
			t.Fatal("execution notes were confused with the source-specific answer")
		}
		packet := reviewPacket(t, in, note.Citations[0].PacketID)
		if packet.Records[0].Values["date"] != "2025-06-30" || packet.Records[0].Origins["date"].Observation != "o2" || strings.Contains(string(wire), "PRIVATE_UNSELECTED") || len(in.Artifact.Sources) != 1 || len(in.Artifact.Rows) != 1 {
			t.Fatal("explanation changed the computation, exposed unrelated values or lost its source")
		}
		return sourceExplanationAssessment(in), nil
	})
	v := executeResult(t, e, p)
	if v.Status != "review_required" || v.Evaluation.Status != "requirements_met" || !v.Evaluation.NeedsSemanticReview {
		t.Fatal("cited explanation must be executable but not self-approved")
	}
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if v.Status != "output_ready" || len(v.Reviews) != 1 || v.Budget.EvidencePacketsRemaining != 6 || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod {
		t.Fatal("separately reviewed source explanation did not reach the common result contract")
	}
}

func sourceExplanationFixture(t *testing.T, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) (*goalwork.Engine, goalwork.Composition) {
	t.Helper()
	if review == nil {
		review = func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Fatal("unexpected reviewer call")
			return goalwork.ReviewAssessment{}, nil
		}
	}
	e, err := goalwork.Start("관측소의 원천 기록과 실제 기준일을 설명해줘", goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, r goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			rows := []goalwork.Row{{"station": "A", "private": "PRIVATE_UNSELECTED"}}
			if r.PK == "definition" {
				rows = []goalwork.Row{{"date": "2025-06-30", "note": "reference date for station A", "private": "PRIVATE_UNSELECTED"}}
			}
			return goalwork.Acquired{Delivery: "REST", Rows: rows}, nil
		},
		Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "관측 기록과 기준일", Region: "관측소 A", Period: "원천 기준일", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "weather", Description: "기상 관측"}}, Outputs: []goalwork.OutputRequirement{{ID: "station", Role: "weather", Type: "string", Description: "관측소"}}, Explanations: []goalwork.ExplanationRequirement{{ID: "source_dates", Topic: "temporal", Basis: "source", Description: "실제 원천 기준일과 조회 시점의 차이"}, {ID: "execution_limits", Topic: "temporal", Description: "실행된 시간 검사의 한계"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	for _, pk := range []string{"weather", "definition"} {
		advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: pk, Role: "weather"})
		advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: pk})
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: pk, Delivery: "api"}})
	}
	for i, fields := range [][]string{{"station"}, {"date", "note"}} {
		o := e.View().Observations[i]
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: fields}})
	}
	packetID := e.View().Evidence[1].ID
	p := goalwork.Composition{
		ID: "dated", Purpose: "source date reporting", Base: "o1", Select: []string{"o1.station"},
		Roles: []goalwork.RoleBinding{{Role: "weather", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "station", Field: "o1.station"}},
		Assumptions:  []string{"fixture source interpretation, not current weather"},
		Support:      []goalwork.SupportBinding{{PacketID: packetID, Targets: []string{"o1"}, Purpose: "Check whether the stated reference date applies to these records"}},
		Explanations: []goalwork.ExplanationDraft{{ID: "source_dates", Text: "이 관측의 기준일은 2025-06-30입니다. 조회일이나 현재 날씨를 뜻하지 않습니다.", Citations: []goalwork.EvidenceCitation{{PacketID: packetID, PacketRow: 1, Field: "date"}}}},
	}
	return e, p
}
