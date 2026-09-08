package goalwork_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSourceReportRejectsUnsupportedGoalOrAnyOutput(t *testing.T) {
	for _, tc := range []string{"stronger original goal", "unsupported output", "missing output", "forged citation", "invented output", "credential prose", "reviewer failure", "mutated reviewer input"} {
		t.Run(tc, func(t *testing.T) {
			e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				a := supportedSourceReview(in)
				switch tc {
				case "stronger original goal":
					a.GoalFit.Verdict = "unsupported"
				case "unsupported output":
					a.Outputs[0].Finding.Verdict = "insufficient"
				case "missing output":
					a.Outputs = nil
				case "forged citation":
					a.Outputs[0].Finding.PacketID = "ep_invented"
				case "invented output":
					a.Outputs[0].Output = "invention"
				case "credential prose":
					a.GoalFit.Reason = "serviceKey=fixture-secret-never-return"
				case "reviewer failure":
					return a, errors.New("source-value-never-log")
				case "mutated reviewer input":
					in.Artifact.Rows[0]["o1.record"] = "invented record"
					in.Contract.Outputs[0].ID = "weakened"
					a.GoalFit.Verdict = "unsupported"
				}
				return a, nil
			})
			executeResult(t, e, resultRecipe())
			readSourceReviewEvidence(t, e)
			v := advanceSourceReview(t, e)
			if v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview || v.Artifact.Rows[0]["o1.record"] != "museum A" || v.Contract.Outputs[0].ID != "record" || strings.Contains(fmt.Sprint(v.Gaps), "source-value-never-log") {
				t.Fatalf("unsafe review accepted or mutated source: %+v", v)
			}
		})
	}
}

func TestSourceReportRequiresExplicitAuthorityAndCompleteEvidence(t *testing.T) {
	for _, tc := range []string{"disabled", "unread", "partial rows", "derived calculation", "wrong composition"} {
		t.Run(tc, func(t *testing.T) {
			policy := goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}
			if tc == "disabled" {
				policy.ReviewRecipient = ""
			}
			e := sourceReviewEngine(t, policy, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				t.Fatal("unauthorized/incomplete source input reached reviewer")
				return goalwork.ReviewAssessment{}, nil
			})
			p := resultRecipe()
			if tc == "derived calculation" {
				p.Measures = []goalwork.Measure{{As: "parsed_year", Field: "o1.year", Format: "decimal_v1", Unit: "declared"}}
			}
			executeResult(t, e, p)
			if tc != "unread" && tc != "partial rows" {
				readSourceReviewEvidence(t, e)
			}
			if tc == "partial rows" {
				v := e.View()
				_, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1}, Fields: []string{"record"}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			id := "records"
			if tc == "wrong composition" {
				id = "old"
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: id})
			if err != nil || len(v.Gaps) == 0 || v.Status == "output_ready" || len(v.Reviews) != 0 {
				t.Fatalf("preflight: %+v %v", v, err)
			}
		})
	}
}

func TestSourceReportCannotCompleteAfterReviewExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			time.Sleep(time.Hour + time.Second)
			return supportedSourceReview(in), nil
		})
		executeResult(t, e, resultRecipe())
		readSourceReviewEvidence(t, e)
		v := advanceSourceReview(t, e)
		if v.Status != "expired" || v.Artifact != nil || len(v.Evidence) != 0 || v.Evaluation.Review != nil || v.Reviews[0].Status != "failed" {
			t.Fatalf("expired approval: %+v", v)
		}
	})
}

func TestSourceReportCancellationCannotRecordApproval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		cancel()
		return supportedSourceReview(in), nil
	})
	executeResult(t, e, resultRecipe())
	readSourceReviewEvidence(t, e)
	v, err := e.Advance(ctx, e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "records"})
	if !errors.Is(err, context.Canceled) || v.Status == "output_ready" || v.Evaluation.Review != nil || v.Reviews[0].Status != "failed" {
		t.Fatal("cancelled review approved")
	}
}

func TestSourceReportReviewBudgetAndRevisionSurviveCorrections(t *testing.T) {
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		a := supportedSourceReview(in)
		a.GoalFit.Verdict = "insufficient"
		return a, nil
	})
	executeResult(t, e, resultRecipe())
	readSourceReviewEvidence(t, e)
	v := advanceSourceReview(t, e)
	if v.Reviews[0].ExecutionRevision != 6 {
		t.Fatal("review not pinned")
	}
	_, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: "records"})
	if !errors.Is(err, goalwork.ErrActionAlreadyAttempted) {
		t.Fatal("same review replay allowed")
	}
	for n := 2; n <= 4; n++ {
		p := resultRecipe()
		p.ID = fmt.Sprintf("corrected%d", n)
		v = executeResult(t, e, p)
		if v.Evaluation.Review != nil {
			t.Fatal("old review attached to a new result")
		}
		v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(v.Reviews) != 3 || v.Budget.ReviewsRemaining != 0 || v.Status == "output_ready" || !strings.Contains(v.Gaps[len(v.Gaps)-1].Detail, "budget") {
		t.Fatalf("review budget refilled: %+v", v)
	}
}

func TestSourceReportDoesNotReviewTheSameEvidenceInAnotherOrder(t *testing.T) {
	calls := 0
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		a := supportedSourceReview(in)
		a.GoalFit.Verdict = "insufficient"
		return a, nil
	})
	executeResult(t, e, resultRecipe())
	readSourceReviewEvidence(t, e)
	advanceSourceReview(t, e)
	v := e.View()
	v, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{2, 1}, Fields: []string{"year", "record"}}})
	if err != nil {
		t.Fatal(err)
	}
	v = advanceSourceReview(t, e)
	if calls != 1 || len(v.Reviews) != 1 || !strings.Contains(v.Gaps[len(v.Gaps)-1].Detail, "already reviewed") {
		t.Fatal("reordered evidence bought another approval attempt")
	}
}

func TestSourceReportReviewInputCannotChangeItsOwnApprovalRequirements(t *testing.T) {
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		in.Contract.Outputs[0].ID = "weakened"
		a := supportedSourceReview(in)
		a.Outputs[0].Output = "weakened"
		return a, nil
	})
	executeResult(t, e, resultRecipe())
	readSourceReviewEvidence(t, e)
	v := advanceSourceReview(t, e)
	if v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview {
		t.Fatal("review callback weakened its approval requirements")
	}
}

// This fixture checks the real Engine transition, not model accuracy. Actual
// reviewer calibration against independent source facts is recorded separately.
func TestSourceReportSeparatelyReviewsOriginalGoalAndEveryOutput(t *testing.T) {
	e := sourceReviewEngine(t, goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude"}, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		if strings.Contains(fmt.Sprint(in), "never-disclosed-value") {
			t.Fatal("review received an unselected value")
		}
		if in.Goal != "Report the labels recorded in this source snapshot, not current operating status" || in.Artifact.Rows[0]["o1.record"] != "museum A" || in.Artifact.Evaluation.ExecutionRevision != 6 || in.Evidence.Selection.RowsSHA256 != in.Artifact.Sources[0].RowsSHA256 {
			t.Fatalf("review lost original goal or actual evidence: %+v", in)
		}
		return supportedSourceReview(in), nil
	})
	v := executeResult(t, e, resultRecipe())
	if v.Status != "review_required" {
		t.Fatalf("execution self-approved: %+v", v)
	}
	readSourceReviewEvidence(t, e)
	v = advanceSourceReview(t, e)
	if v.Status != "output_ready" || v.Evaluation.NeedsSemanticReview || v.Evaluation.Review == nil || v.Evaluation.Review.Method != "independent_model_source_report_v1" || v.Evaluation.Review.ExecutionRevision != 6 || len(v.Reviews) != 1 {
		t.Fatalf("reviewed report unavailable: %+v", v)
	}
	if v.Artifact.Evaluation.Review.InputSHA256 != v.Evaluation.Review.InputSHA256 || v.Evaluation.Review.Provider != "claude" {
		t.Fatal("review did not bind the exact current artifact")
	}
}

func supportedSourceReview(in goalwork.ReviewInput) goalwork.ReviewAssessment {
	finding := goalwork.ReviewFinding{Verdict: "supported", Reason: "Only the source's recorded labels are requested and reported", PacketID: in.Evidence.ID}
	return goalwork.ReviewAssessment{GoalFit: finding, Outputs: []goalwork.OutputReview{{Output: "record", Finding: finding}}}
}

func sourceReviewEngine(t *testing.T, policy goalwork.Policy, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	e, err := goalwork.Start("Report the labels recorded in this source snapshot, not current operating status", policy, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records", Title: "source records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"record": "museum A", "year": "2025", "unselected": "never-disclosed-value"}, {"record": "museum B", "year": "2024", "unselected": "never-disclosed-value"}}}, nil
		},
		Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := resultContract()
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "records"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}} {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("setup: %+v %v", v, err)
		}
	}
	return e
}

func readSourceReviewEvidence(t *testing.T, e *goalwork.Engine) {
	t.Helper()
	v := e.View()
	v, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1, 2}, Fields: []string{"record", "year"}}})
	if err != nil || len(v.Gaps) != 0 {
		t.Fatalf("read: %+v %v", v, err)
	}
}

func advanceSourceReview(t *testing.T, e *goalwork.Engine) goalwork.View {
	t.Helper()
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "records"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
