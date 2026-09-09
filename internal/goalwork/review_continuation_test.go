package goalwork_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestReviewCanExploreWithoutResettingTheGoal(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	before := executeResult(t, e, resultRecipe())
	if before.Status != "review_required" {
		t.Fatalf("real execution did not reach review: %+v", before)
	}
	after, err := e.Advance(context.Background(), before.Revision, goalwork.Decision{Action: "search", Query: "official scope mapping", Role: "scope"})
	if err != nil || after.Status != "exploring" || after.Revision != before.Revision+1 {
		t.Fatalf("cannot seek missing evidence after review: %s revision=%d err=%v", after.Status, after.Revision, err)
	}
	if after.Goal != before.Goal || !reflect.DeepEqual(after.Contract, before.Contract) || after.Policy != before.Policy || !after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatal("review continuation changed the original goal, policy or expiry")
	}
	if after.Budget.SearchesRemaining != before.Budget.SearchesRemaining-1 || after.Budget.SamplesRemaining != before.Budget.SamplesRemaining || !reflect.DeepEqual(after.Observations, before.Observations) {
		t.Fatal("continuation reset acquisition history or budget")
	}
	if !reflect.DeepEqual(after.Artifact, before.Artifact) || !after.Evaluation.NeedsSemanticReview {
		t.Fatal("search changed or approved the previous result")
	}
	if before.Evaluation.ExecutionRevision != 6 || after.Evaluation.ExecutionRevision != 6 || after.Artifact.Evaluation.ExecutionRevision != 6 {
		t.Fatal("later exploration relabeled the result as the latest revision")
	}
}

func TestReviewContinuationRetainsRoundLimits(t *testing.T) {
	for _, limit := range []int{6, 7, 8} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			e := observedResultEnginePolicy(t, resultContract(), goalwork.Policy{MaxRounds: limit})
			v := executeResult(t, e, resultRecipe())
			wantExecution := 6
			if limit == 7 {
				var err error
				v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "search", Query: "official context", Role: "scope"})
				if err != nil {
					t.Fatal(err)
				}
			} else if limit == 8 {
				p := resultRecipe()
				p.ID = "alternative"
				v = executeResult(t, e, p)
				wantExecution = 8
			}
			if v.Status != "budget_exhausted" || v.Revision != limit || v.Artifact == nil || !v.Evaluation.NeedsSemanticReview || v.Evaluation.ExecutionRevision != wantExecution {
				t.Fatalf("review escaped the round limit or lost its unapproved result: %+v", v)
			}
			after, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "search", Query: "one more", Role: "scope"})
			if err == nil || !reflect.DeepEqual(after, v) {
				t.Fatal("exhausted goal resumed or discarded its result")
			}
		})
	}
}

func TestReviewRejectsStaleReplayAndCancellationWithoutChangingResult(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	before := executeResult(t, e, resultRecipe())
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx      context.Context
		revision int
		decision goalwork.Decision
	}{
		{context.Background(), 5, goalwork.Decision{Action: "search", Query: "scope", Role: "scope"}},
		{context.Background(), 6, goalwork.Decision{Action: "execute", CompositionID: "records", Reason: "new rationale cannot bypass replay"}},
		{cancelled, 6, goalwork.Decision{Action: "search", Query: "scope", Role: "scope"}},
	} {
		after, err := e.Advance(tc.ctx, tc.revision, tc.decision)
		if err == nil || !reflect.DeepEqual(after, before) {
			t.Fatal("pre-action rejection mutated the review state or its result")
		}
	}
}

func TestReviewCannotWeakenItsContractOrApproveAPartialCorrection(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	before := executeResult(t, e, resultRecipe())
	weakened := resultContract()
	weakened.Region = "a smaller region"
	after, err := e.Advance(context.Background(), before.Revision, goalwork.Decision{Action: "define", Contract: &weakened})
	if err != nil || !reflect.DeepEqual(after.Contract, before.Contract) || len(after.Gaps) != 1 || !strings.Contains(after.Gaps[0].Detail, "immutable") {
		t.Fatal("review allowed goal reinterpretation")
	}
	p := resultRecipe()
	p.ID = "missing output"
	p.Outputs = nil
	after = executeResult(t, e, p)
	if after.Status != "exploring" || after.Evaluation.Status != "partial" || after.Evaluation.ExecutionRevision != 9 || after.Artifact.Evaluation.CompositionID != p.ID || !after.Evaluation.NeedsSemanticReview {
		t.Fatalf("prior review hid the new partial result: %+v", after)
	}
}

func TestReviewEvidenceStillNeedsDisclosurePermissionAndExpires(t *testing.T) {
	for _, recipient := range []string{"", "claude"} {
		t.Run("recipient="+recipient, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				e := observedResultEnginePolicy(t, resultContract(), goalwork.Policy{EvidenceRecipient: recipient})
				before := executeResult(t, e, resultRecipe())
				time.Sleep(55 * time.Minute)
				after := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": before.Observations[0].RowsSHA256, "rows": []int{1}, "fields": []string{"record"}})
				if after.Status != "exploring" || after.Revision != 7 || !after.Evaluation.NeedsSemanticReview || !after.ExpiresAt.Equal(before.ExpiresAt) {
					t.Fatal("evidence read approved the result or extended expiry")
				}
				planning := e.PlanningView()
				if recipient == "" {
					if len(planning.Evidence) != 0 || len(after.Gaps) != 1 || !strings.Contains(after.Gaps[0].Detail, "disabled") {
						t.Fatal("review enabled unauthorized disclosure")
					}
				} else if len(planning.Evidence) != 1 || planning.Evidence[0].Records[0].Values["record"] != "private-A" || planning.Budget.EvidencePacketsRemaining != 7 {
					t.Fatal("review lost selected evidence or cumulative disclosure cost")
				}
				if planning.Artifact != nil || strings.Contains(evidenceJSON(t, planning), "private-B") {
					t.Fatal("review disclosed unselected source values")
				}
				time.Sleep(6 * time.Minute)
				expired, err := e.Advance(context.Background(), after.Revision, goalwork.Decision{Action: "search", Query: "more", Role: "scope"})
				if err == nil || expired.Status != "expired" || expired.Revision != 7 || expired.Artifact != nil || len(expired.Evidence) != 0 || strings.Contains(evidenceJSON(t, e.PlanningView()), "private-A") {
					t.Fatal("review continuation revived expired source evidence")
				}
			})
		})
	}
}

func TestRepeatedRunDoesNotRefillReplayCorrection(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	before := executeResult(t, e, resultRecipe())
	for run, wantPlans := range []int{2, 1} {
		plans := 0
		after, err := goalwork.Run(context.Background(), e, func(context.Context, goalwork.View) (goalwork.Decision, error) {
			plans++
			return goalwork.Decision{Action: "execute", CompositionID: "records"}, nil
		}, nil)
		if !errors.Is(err, goalwork.ErrActionAlreadyAttempted) || plans != wantPlans || !reflect.DeepEqual(after, before) {
			t.Fatalf("run %d refilled replay repair or changed result: plans=%d want=%d err=%v", run, plans, wantPlans, err)
		}
	}
}

func TestStandaloneContinuesFromReviewToAnAlternative(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	before := executeResult(t, e, resultRecipe())
	steps := []goalwork.Decision{
		{Action: "search", Query: "missing official definition", Role: "scope"},
		{Action: "abstain", Reason: "no official definition available in this fixture"},
	}
	n := 0
	after, err := goalwork.Run(context.Background(), e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
		if n >= len(steps) {
			t.Fatal("run continued after abstention")
		}
		if n == 0 && v.Status != "review_required" {
			t.Fatal("planner did not receive the actual review state")
		}
		b, _ := json.Marshal(v)
		if v.Artifact != nil || strings.Contains(string(b), "private-A") {
			t.Fatal("review continuation exposed the private artifact to the planner")
		}
		d := steps[n]
		n++
		return d, nil
	}, nil)
	if err != nil || n != 2 || after.Status != "abstained" || after.Revision != before.Revision+2 || after.Artifact == nil || !after.Evaluation.NeedsSemanticReview {
		t.Fatalf("standalone did not continue review honestly: steps=%d status=%s err=%v", n, after.Status, err)
	}
}

func TestReviewCorrectionReplacesCurrentResultButKeepsFailedHistory(t *testing.T) {
	e := observedResultEngine(t, resultContract())
	first := executeResult(t, e, resultRecipe())
	// This recipe is accepted, but its invalid observed-field selection fails
	// during execution. A prior successful projection must not stand in for it.
	broken := resultRecipe()
	broken.ID = "bad correction"
	broken.Select = []string{"o1.not_observed"}
	failed := executeResult(t, e, broken)
	if failed.Status != "exploring" || failed.Artifact != nil || failed.Evaluation != nil {
		t.Fatalf("failed correction retained a stale current result: %+v", failed)
	}
	if len(failed.Executions) != 2 || failed.Executions[0].Revision != 6 || failed.Executions[1].Revision != 8 || failed.Executions[1].Status != "failed" || failed.Executions[1].Error == "" {
		t.Fatalf("correction lost execution history: %+v", failed.Executions)
	}
	if len(first.Artifact.Rows) != 2 || first.Artifact.Rows[0]["o1.record"] != "private-A" || first.Evaluation.Status != "requirements_met" {
		t.Fatal("new execution mutated the prior detached result")
	}
	corrected := resultRecipe()
	corrected.ID = "corrected"
	corrected.Time = &goalwork.TemporalAlignment{Window: &goalwork.DateWindow{From: "2025-01-01", Through: "2025-12-31"}, Bindings: []goalwork.TimeBinding{{Observation: "o1", FromField: "year", Format: "year_v1", Meaning: "reference_period"}}}
	after := executeResult(t, e, corrected)
	if after.Status != "review_required" || after.Artifact == nil || len(after.Artifact.Rows) != 1 || after.Artifact.Rows[0]["o1.record"] != "private-A" || after.Evaluation.CompositionID != "corrected" || !after.Evaluation.NeedsSemanticReview {
		t.Fatalf("corrected result was lost or self-approved: %+v", after)
	}
	if len(after.Observations) != 1 || len(after.Compositions) != 3 || len(after.Executions) != 3 || after.Budget.CompositionsRemaining != 3 {
		t.Fatal("correction replaced original observations, recipes or budget")
	}
	if after.Evaluation.ExecutionRevision != 10 || after.Artifact.Evaluation.ExecutionRevision != 10 || first.Evaluation.ExecutionRevision != 6 {
		t.Fatal("evaluation lost its actual execution revision")
	}
}
