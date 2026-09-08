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
	recipient     string
	branches      bool
	historical    bool
	fullScope     bool
	tableContext  bool
	ageDefinition bool
	call          func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)
	observe       func(goalwork.View)
}

func TestCitywideReviewReceivesAgeDefinitionWithoutLosingEvidence(t *testing.T) {
	var before, after goalwork.ReviewInput
	checkCitywideReduction(t, false, &citywideModelReview{recipient: "codex", branches: true, tableContext: true, call: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		before = in
		return supportedAnalysisReview(in), nil
	}})
	v := checkCitywideReduction(t, false, &citywideModelReview{recipient: "codex", branches: true, tableContext: true, ageDefinition: true, call: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		after = in
		return supportedAnalysisReview(in), nil // Delivery fixture, not definition applicability.
	}})
	if after.Analysis == nil || len(after.Analysis.SourceContext) != 2 {
		t.Fatal("registered age definition is missing from the existing citywide review")
	}
	doc := after.Analysis.SourceContext[1]
	if !doc.Proposed || !slices.Equal(doc.Targets, []string{"o1"}) || doc.Source.Document == nil || doc.Source.Document.Reference.Basis != "adapter_reference" || doc.Request.Document == nil || doc.Request.Document.ReferenceID != "kosis-answer-22124" || len(doc.Evidence.Records) != 2 {
		t.Fatal("definition lost its proposed target, actual request or registered origin")
	}
	for n, record := range doc.Evidence.Records {
		address := record.Origins["text"]
		if address.Kind != "document_block" || address.Ordinal != n+1 || address.Locator == "" || record.Values["text"] == nil {
			t.Fatal("definition lost its selected original block")
		}
	}
	// Compare disclosed cells, not packet IDs/order. The old duplicate is redundant;
	// every original value, missing/null distinction and address must survive.
	dataEvidence := func(in goalwork.ReviewInput) string {
		cells := map[string]any{}
		packets := append([]goalwork.EvidencePacket{in.Evidence}, in.Analysis.AdditionalEvidence...)
		packets = append(packets, in.Analysis.SourceContext[0].Evidence)
		for _, packet := range packets {
			for _, record := range packet.Records {
				for _, field := range packet.Selection.Fields {
					key := packet.Selection.Observation + ":" + strconv.Itoa(record.RetainedRow) + ":" + field
					value, present := record.Values[field]
					cells[key] = []any{present, value, record.Origins[field], slices.Contains(record.Missing, field)}
				}
			}
		}
		body, err := json.Marshal(cells)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	if dataEvidence(before) != dataEvidence(after) {
		t.Fatal("document context displaced original calculation or school context evidence")
	}
	if len(v.Evidence) != 8 || v.Budget.EvidencePacketsRemaining != 0 || len(v.Observations) != 5 || len(v.Artifact.Sources) != 3 || len(v.Artifact.Rows) != 10 || len(v.Artifact.Unmatched) != 2 || before.Analysis.OutputSHA256 != after.Analysis.OutputSHA256 {
		t.Fatal("document context widened budgets or changed the original citywide comparison")
	}
}

func TestCitywideReviewReceivesTableContextWithoutChangingComparison(t *testing.T) {
	var ref struct {
		ContentSHA256 string
		Records       []struct {
			Sheet string
			Row   int
			Cells goalwork.Row
		}
	}
	readReference(t, "education-table-context-reference.json", &ref)
	called := false
	reviewer := &citywideModelReview{recipient: "codex", branches: true, tableContext: true, call: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		called = true
		c := in.Analysis.SourceContext[0]
		if c.Request.XLSX.Range != "A22:AM40" || c.Source.ContentSHA256 != ref.ContentSHA256 || len(c.Evidence.Records) != 8 {
			t.Fatal("review is missing independently observed table footer context")
		}
		for i, want := range ref.Records {
			got := c.Evidence.Records[5+i]
			if got.RetainedRow != want.Row-21 || len(got.Values) != 5 || len(got.Missing) != 0 {
				t.Fatal("context did not preserve selected blank cells and original row positions")
			}
			for field, value := range want.Cells {
				address := got.Origins[field]
				if got.Values[field] != value || address.Sheet != want.Sheet || address.Ordinal != want.Row {
					t.Fatalf("context differs from independent original cell %s!%s%d", want.Sheet, field, want.Row)
				}
			}
		}
		return supportedAnalysisReview(in), nil // Delivery test, not a source interpretation.
	}}
	v := checkCitywideReduction(t, false, reviewer)
	if !called || len(v.Evidence) != 8 || len(v.Artifact.Rows) != 10 || len(v.Artifact.Unmatched) != 2 || len(v.Artifact.Sources) != 3 {
		t.Fatal("table context changed the calculation, disclosure budget or review path")
	}
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
	if !slices.Contains([]string{"baseline", "with-branches", "with-branches-full-scope", "with-table-context", "with-age-definition"}, variant) {
		t.Fatal("result variant must be baseline, with-branches, with-branches-full-scope, with-table-context or with-age-definition")
	}
	if variant == "with-age-definition" && mode == "reference" {
		t.Fatal("age-definition diagnostic requires actual acquisition; offline document fixtures are not methodology evidence")
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
	reviewer := &citywideModelReview{recipient: provider, historical: mode == "historical", branches: variant != "baseline", fullScope: variant == "with-branches-full-scope" || variant == "with-table-context" || variant == "with-age-definition", tableContext: variant == "with-table-context" || variant == "with-age-definition", ageDefinition: variant == "with-age-definition", observe: func(v goalwork.View) { record.Result = v }, call: func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
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
