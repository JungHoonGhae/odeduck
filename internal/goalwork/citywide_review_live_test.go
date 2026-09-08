package goalwork_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

type citywideModelReview struct {
	recipient  string
	branches   bool
	historical bool
	call       func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)
	observe    func(goalwork.View)
}

func TestCitywideComparisonPreservesSeparateBranchCounts(t *testing.T) {
	reviewer := &citywideModelReview{recipient: "codex", branches: true, call: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		if len(in.Contract.Outputs) != 5 {
			t.Fatal("school and branch outputs are not independently represented")
		}
		return supportedAnalysisReview(in), nil // State transition fixture, not semantic approval.
	}}
	v := checkCitywideReduction(t, false, reviewer)
	branches, pupils := 0, 0
	rows := append([]goalwork.Row(nil), v.Artifact.Rows...)
	for _, tuple := range v.Artifact.Unmatched {
		if tuple.Side == "right" {
			rows = append(rows, tuple.Values)
		}
	}
	for _, row := range rows {
		b, err := strconv.Atoi(row["o2.AH"].(string))
		if err != nil {
			t.Fatal(err)
		}
		p, err := strconv.Atoi(row["o2.AL"].(string))
		if err != nil {
			t.Fatal(err)
		}
		branches, pupils = branches+b, pupils+p
	}
	if branches != 7 || pupils != 63 {
		t.Fatal("separate school branches differ from the independent publisher totals")
	}
}

// This is one explicitly selected DEVELOPMENT diagnostic, not autonomous search,
// held-out grading or a replacement for the unchanged G4 question and oracle.
func TestLiveCitywideGoalAnalysisReview(t *testing.T) {
	provider := os.Getenv("ODEDUCK_CITYWIDE_REVIEW")
	if provider == "" {
		t.Skip("set ODEDUCK_CITYWIDE_REVIEW=codex|claude|gemini and ODEDUCK_CITYWIDE_REVIEW_OUTPUT to a new JSON file; sends selected public aggregates and headers")
	}
	if !slices.Contains([]string{"codex", "claude", "gemini"}, provider) {
		t.Fatal("explicit supported review recipient required; no fallback")
	}
	mode := os.Getenv("ODEDUCK_CITYWIDE_ACQUISITION")
	if mode == "" {
		mode = "live"
	}
	if mode != "live" && mode != "reference" && mode != "historical" {
		t.Fatal("acquisition must be live, explicit historical portal selection or reference replay; no automatic fallback")
	}
	variant := os.Getenv("ODEDUCK_CITYWIDE_RESULT")
	if variant == "" {
		variant = "baseline"
	}
	if variant != "baseline" && variant != "with-branches" {
		t.Fatal("result variant must be baseline or with-branches")
	}
	f, err := os.OpenFile(os.Getenv("ODEDUCK_CITYWIDE_REVIEW_OUTPUT"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	record := struct {
		Kind          string                        `json:"kind"`
		StartedAt     time.Time                     `json:"startedAt"`
		Elapsed       float64                       `json:"elapsedSeconds"`
		Provider      string                        `json:"provider"`
		Acquisition   string                        `json:"acquisition"`
		Variant       string                        `json:"variant"`
		PlannedTrials int                           `json:"plannedTrials"`
		ExpectedReady bool                          `json:"expectedReady"`
		Input         *goalwork.ReviewInput         `json:"input,omitempty"`
		Response      *agentplan.GoalReviewResponse `json:"response,omitempty"`
		Result        goalwork.View                 `json:"result"`
		Error         string                        `json:"error,omitempty"`
	}{Kind: "G4_scripted_actual_model_diagnostic", Acquisition: mode, Variant: variant, StartedAt: time.Now().UTC(), Provider: provider, PlannedTrials: 1, Result: goalwork.View{Status: "not_run"}}
	t.Cleanup(func() {
		record.Elapsed = time.Since(record.StartedAt).Seconds()
		if t.Failed() && record.Error == "" {
			record.Error = "diagnostic assertion or pre-review acquisition failed; see go test output"
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(record); err != nil {
			t.Error(err)
		}
		if err := f.Sync(); err != nil {
			t.Error(err)
		}
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	reviewer := &citywideModelReview{recipient: provider, historical: mode == "historical", branches: variant == "with-branches", observe: func(v goalwork.View) { record.Result = v }, call: func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		record.Input = &in
		response, err := agentplan.ReviewGoal(ctx, in, provider)
		record.Response = &response
		if err != nil {
			record.Error = err.Error()
		}
		return response.Assessment, err
	}}
	record.Result = checkCitywideReduction(t, mode != "reference", reviewer)
	if record.Input == nil || record.Response == nil || record.Error != "" {
		t.Fatal("actual model review was not completed; retain this failed attempt")
	}
	if record.Result.Evaluation == nil || record.Result.Evaluation.Review == nil {
		t.Fatal("the actual model finding was not attached to the Engine result")
	}
	// Independent G4 contract requires source census dates and table interpretation;
	// the current result does not yet present all of those requested explanations.
	if record.Result.Status == "output_ready" {
		t.Error("model approved the incomplete original G4 comparison; retain as a false approval, not goal completion")
	}
	t.Logf("actual %s review: %s; raw input/response and result retained; not autonomous discovery or G4 completion", provider, record.Result.Status)
}
