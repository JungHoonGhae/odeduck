package agentplan

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Replay the retained real-provider transcripts through the public adapter.
// This is an offline decoder regression, not another model accuracy trial.
func TestReviewGoalReplaysArchivedCodexCalibration(t *testing.T) {
	for _, archive := range []struct {
		folder string
		file   string
		ready  int
		trials int
	}{{"source-report-review-20260908", "codex-raw.jsonl.gz", 6, 12}, {"analysis-review-20260908", "codex-raw.jsonl.gz", 3, 12}, {"analysis-review-20260908", "live-codex-raw.jsonl.gz", 6, 12}, {"citywide-review-20260908", "baseline-codex.json.gz", 0, 1}, {"citywide-review-20260908", "with-branches-codex.json.gz", 0, 1}, {"citywide-review-20260908", "historical-codex.json.gz", 0, 1}, {"citywide-review-20260908", "full-scope-codex.json.gz", 0, 1}, {"citywide-review-20260908", "table-context-codex.json.gz", 0, 1}, {"citywide-review-20260908", "age-definition-codex.json.gz", 0, 1}} {
		t.Run(archive.folder+"/"+archive.file, func(t *testing.T) {
			replayArchivedReview(t, archive.folder, archive.file, archive.ready, archive.trials)
		})
	}
}

func replayArchivedReview(t *testing.T, folder, file string, wantReady, wantTrials int) {
	t.Helper()
	f, err := os.Open(filepath.Join("../goalwork/testdata/goalbench-v1", folder, file))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	d := json.NewDecoder(z)
	d.UseNumber()
	count, ready := 0, 0
	for {
		var trial struct {
			Case, Variant, ProviderResponse string
			Repeat                          int
			Input                           json.RawMessage
			Result                          goalwork.View
			Response                        GoalReviewResponse // single-diagnostic envelope
		}
		if err := d.Decode(&trial); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		count++
		if trial.ProviderResponse == "" {
			trial.ProviderResponse = trial.Response.RawResponse
		}
		if trial.Result.Status == "output_ready" {
			ready++
		}
		t.Run(fmt.Sprintf("%s/%s/%d", trial.Case, trial.Variant, trial.Repeat), func(t *testing.T) {
			if trial.ProviderResponse == "" || trial.Result.Evaluation == nil || trial.Result.Evaluation.Review == nil {
				t.Fatal("archive is missing its provider transcript or typed assessment")
			}
			dir := t.TempDir()
			responsePath := filepath.Join(dir, "response.jsonl")
			if err := os.WriteFile(responsePath, []byte(trial.ProviderResponse), 0600); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nexec /bin/cat '" + strings.ReplaceAll(responsePath, "'", "'\\''") + "'\n"
			if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			response, err := ReviewGoal(context.Background(), decodeLegacyReviewInput(t, trial.Input), "codex")
			if err != nil || response.Truncated || response.RawResponse != trial.ProviderResponse || !reflect.DeepEqual(response.Assessment, trial.Result.Evaluation.Review.Assessment) {
				t.Fatalf("archived response no longer decodes to its recorded findings: %v", err)
			}
		})
	}
	if count != wantTrials || ready != wantReady {
		t.Fatalf("archive denominator changed: %d trials, %d ready", count, ready)
	}
}

// Adapt historical v1 input in memory solely to replay its recorded response
// through the current public decoder. The original model saw v1, not v3; this
// conversion is neither a new model trial nor Engine-produced v3 attribution.
// Archives and their recorded findings remain unchanged.
func decodeLegacyReviewInput(t *testing.T, raw json.RawMessage) goalwork.ReviewInput {
	t.Helper()
	decode := func(target any) {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(target); err != nil {
			t.Fatal(err)
		}
	}
	var in goalwork.ReviewInput
	decode(&in)
	if in.Analysis == nil {
		return in // The source-report v1 contract has not changed.
	}
	var legacy struct {
		Analysis struct {
			Method        string
			SourceContext []struct {
				goalwork.SourceContext
				Evidence goalwork.EvidencePacket
			}
		}
	}
	decode(&legacy)
	if legacy.Analysis.Method != "engine_relational_replay_v1" {
		t.Fatal("expected an explicitly archived v1 analysis")
	}
	in.Analysis.Method = "engine_relational_replay_v3"
	in.Analysis.SourceContext = nil
	for _, context := range legacy.Analysis.SourceContext {
		if context.Evidence.ID == "" {
			t.Fatal("legacy context lost its embedded packet")
		}
		found := false
		for _, packet := range in.EvidencePackets() {
			if packet.ID == context.Evidence.ID {
				if !reflect.DeepEqual(packet, context.Evidence) {
					t.Fatal("legacy archive contains conflicting packet bodies")
				}
				found = true
			}
		}
		if !found {
			in.Analysis.AdditionalEvidence = append(in.Analysis.AdditionalEvidence, context.Evidence)
		}
		context.SourceContext.PacketID = context.Evidence.ID
		in.Analysis.SourceContext = append(in.Analysis.SourceContext, context.SourceContext)
	}
	return in
}

func TestReviewGoalUsesAnIsolatedContextAndReturnsTypedFindings(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s' '{\"result\":\"{\\\"goalFit\\\":{\\\"verdict\\\":\\\"supported\\\",\\\"reason\\\":\\\"source report only\\\",\\\"packetId\\\":\\\"ep_actual\\\"},\\\"outputs\\\":[{\\\"output\\\":\\\"record\\\",\\\"finding\\\":{\\\"verdict\\\":\\\"supported\\\",\\\"reason\\\":\\\"observed label\\\",\\\"packetId\\\":\\\"ep_actual\\\"}}]}\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	in := goalwork.ReviewInput{Recipient: "claude", Goal: "recorded labels", Contract: goalwork.GoalContract{Outputs: []goalwork.OutputRequirement{{ID: "record"}}}, Evidence: goalwork.EvidencePacket{ID: "ep_actual"}}
	response, err := ReviewGoal(context.Background(), in, "claude")
	if err != nil || response.Assessment.GoalFit.Verdict != "supported" || response.RawResponse == "" {
		t.Fatalf("review: %+v %v", response, err)
	}
	_, err = ReviewGoal(context.Background(), in, "auto")
	if err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("recipient fallback allowed: %v", err)
	}
}

func TestReviewGoalRejectsInventedOrMalformedVerdicts(t *testing.T) {
	in := goalwork.ReviewInput{Recipient: "claude", Contract: goalwork.GoalContract{Outputs: []goalwork.OutputRequirement{{ID: "record"}}}, Evidence: goalwork.EvidencePacket{ID: "ep_actual"}}
	for _, body := range []string{
		`{"goalFit":{"verdict":"supported","reason":"yes","packetId":"ep_actual"},"outputs":[]}`,
		`{"goalFit":{"verdict":"supported","reason":"yes","packetId":"ep_fake"},"outputs":[{"output":"record","finding":{"verdict":"supported","reason":"yes","packetId":"ep_fake"}}]}`,
		`{"goalFit":{"verdict":"supported","verdict":"unsupported","reason":"yes","packetId":"ep_actual"},"outputs":[]}`,
		`{"approved":true}`,
		`{"goalFit":{"verdict":"supported","reason":"yes","packetId":"ep_actual"},"outputs":[{"output":"record","finding":{"verdict":"supported","reason":"yes","packetId":"ep_actual"}}],"outputſ":[{"output":"record","finding":{"verdict":"supported","reason":"yes","packetId":"ep_actual"}}]}`,
		"{\"goalFit\":{\"verdict\":\"supported\",\"reason\":\"yes\",\"packetId\":\"ep_actual\"},\"outputs\":[{\"output\":\"record\",\"finding\":{\"verdict\":\"supported\",\"reason\":\"yes\",\"packetId\":\"ep_actual\"}}]}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"provider failure\"}}",
	} {
		dir := t.TempDir()
		script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", strings.ReplaceAll(body, "'", "'\\''"))
		if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		response, err := ReviewGoal(context.Background(), in, "claude")
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
		if response.RawResponse != body {
			t.Fatal("malformed provider response was not preserved for explicit calibration")
		}
	}
}

func TestReviewGoalSelectsAnalysisContractWithoutChangingSourceReportGuide(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.txt")
	f := goalwork.ReviewFinding{Verdict: "supported", Reason: "typed fixture", PacketIDs: []string{"ep_one", "ep_two", "ep_context"}}
	a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "total", Finding: f}}}
	for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
		a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
	}
	body, _ := json.Marshal(a)
	script := "#!/bin/sh\n/bin/cat > '" + strings.ReplaceAll(promptPath, "'", "'\\''") + "'\nprintf '%s' '" + strings.ReplaceAll(string(body), "'", "'\\''") + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	in := goalwork.ReviewInput{Recipient: "claude", Goal: "compare recorded totals", Contract: goalwork.GoalContract{Outputs: []goalwork.OutputRequirement{{ID: "total"}}}, Evidence: goalwork.EvidencePacket{ID: "ep_one"}, Analysis: &goalwork.AnalysisReviewContext{Method: "engine_relational_replay_v3", AdditionalEvidence: []goalwork.EvidencePacket{{ID: "ep_two"}, {ID: "ep_context", Records: []goalwork.EvidenceRecord{{RetainedRow: 1, Values: goalwork.Row{"A": "SOURCE_HEADER_FIXTURE"}}}}, {ID: "ep_definitions", Records: []goalwork.EvidenceRecord{{RetainedRow: 1, Values: goalwork.Row{"term": "SEPARATE_DEFINITION_FIXTURE"}}}}}}}
	in.Analysis.SourceContext = []goalwork.SourceContext{{Targets: []string{"o1"}, PacketID: "ep_context"}}
	in.Analysis.SourceContext = append(in.Analysis.SourceContext, goalwork.SourceContext{Targets: []string{"o1"}, Proposed: true, Purpose: "Check definition applicability", Source: goalwork.Observation{ID: "o3", PK: "definitions"}, PacketID: "ep_definitions"})
	in.Artifact.Sources = []goalwork.Observation{{ID: "o1", PK: "left"}}
	in.Artifact.Requests = []goalwork.SampleRequest{{PK: "left", Delivery: "file", Asset: "left.csv"}}
	in.Analysis.AdditionalEvidence = append(in.Analysis.AdditionalEvidence, goalwork.EvidencePacket{ID: "ep_comparison"})
	in.Analysis.SourceContext = append(in.Analysis.SourceContext, goalwork.SourceContext{
		Targets: []string{"o1"}, PacketID: "ep_comparison", Proposed: true, Purpose: "Check comparison applicability",
		Source:            goalwork.Observation{ID: "o4", Delivery: "DERIVED"},
		Request:           goalwork.SampleRequest{PK: "left", Delivery: "file", Compare: &goalwork.SourceComparison{Checks: []goalwork.NumericComparison{{ID: "COMPARISON_RECIPE_FIXTURE"}}}},
		Comparison:        &goalwork.ComparisonReviewTrace{Method: "source_comparison_v1", Pairs: [][2]int{{1, 2}}},
		ComparisonSources: []goalwork.ComparisonContextSource{{ArtifactSource: "o1"}, {Source: &goalwork.Observation{ID: "o5", PK: "right"}, Request: &goalwork.SampleRequest{PK: "right", Delivery: "file", Asset: "right.csv"}}},
	})
	response, err := ReviewGoal(context.Background(), in, "claude")
	if err != nil || len(response.Assessment.AnalysisChecks) != 4 {
		t.Fatalf("analysis response: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil || !strings.Contains(string(prompt), "RELATIONAL_ANALYSIS_V3") || strings.Contains(string(prompt), "No semantic approval of joins") {
		t.Fatal("analysis sent to source-only reviewer instructions")
	}
	if !strings.Contains(string(prompt), "SOURCE_HEADER_FIXTURE") || !strings.Contains(string(prompt), "same-file association") {
		t.Fatal("selected context or its association-only interpretation was lost in the adapter")
	}
	if strings.Count(string(prompt), "SOURCE_HEADER_FIXTURE") != 1 || strings.Count(string(prompt), "SEPARATE_DEFINITION_FIXTURE") != 1 {
		t.Fatal("analysis prompt duplicates selected context packet bodies")
	}
	_, inputJSON, found := strings.Cut(string(prompt), "REVIEW_INPUT_JSON:\n")
	var delivered goalwork.ReviewInput
	if !found || json.Unmarshal([]byte(inputJSON), &delivered) != nil || len(delivered.Analysis.SourceContext) != 3 {
		t.Fatal("review adapter did not deliver a self-contained v3 input")
	}
	c := delivered.Analysis.SourceContext[2]
	if c.Comparison == nil || c.Source.Comparison != nil || c.Request.Compare == nil || c.ComparisonSources[0].ArtifactSource != "o1" || c.ComparisonSources[1].Request.Asset != "right.csv" || strings.Count(inputJSON, "COMPARISON_RECIPE_FIXTURE") != 1 {
		t.Fatal("adapter expanded or lost comparison references, recipe or original request")
	}
	for _, required := range []string{"SEPARATE_DEFINITION_FIXTURE", `"proposed":true`, `"purpose":"Check definition applicability"`, "UNTRUSTED PROPOSALS"} {
		if !strings.Contains(string(prompt), required) {
			t.Fatalf("proposed support or its interpretation was lost: %s", required)
		}
	}
	in.Analysis.Method = "engine_relational_replay_v2"
	if _, err := ReviewGoal(context.Background(), in, "claude"); err == nil || !strings.Contains(err.Error(), "unsupported analysis replay contract") {
		t.Fatal("legacy analysis input was sent under current instructions")
	}
	in.Analysis.Method = "engine_relational_replay_v3"
	// The same adapter must require per-source findings for the additional
	// full-scope contract, not accept its ordinary analysis response unchanged.
	in.Contract.Coverage = "population"
	in.Evidence.Selection.Observation = "o1"
	in.Analysis.FullScope = &goalwork.FullScopeContext{Method: "bounded_source_extent_v1", Eligible: true, Sources: []goalwork.SourceExtent{{Observation: "o1", Complete: true, Kind: "complete_csv_selection"}}}
	if _, err := ReviewGoal(context.Background(), in, "claude"); err == nil {
		t.Fatal("full-scope adapter accepted missing original-source coverage")
	}
	a.SourceCoverage = []goalwork.SourceCoverageReview{{Observation: "o1", Finding: f}}
	body, _ = json.Marshal(a)
	script = "#!/bin/sh\n/bin/cat > '" + strings.ReplaceAll(promptPath, "'", "'\\''") + "'\nprintf '%s' '" + strings.ReplaceAll(string(body), "'", "'\\''") + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	response, err = ReviewGoal(context.Background(), in, "claude")
	if err != nil || len(response.Assessment.SourceCoverage) != 1 {
		t.Fatalf("full-scope provider contract lost: %v", err)
	}
	prompt, err = os.ReadFile(promptPath)
	if err != nil || !strings.Contains(string(prompt), `"fullScope":`) || !strings.Contains(string(prompt), "EVERY fullScope.sources") {
		t.Fatal("full-scope evidence or review contract not sent")
	}
	// A source-specific explanation is a separately judged deliverable; neither
	// a successful field nor the older all-supported assessment may approve it.
	in.Contract.Explanations = []goalwork.ExplanationRequirement{{ID: "reference_date", Topic: "temporal", Basis: "source", Description: "Explain the original reference date"}}
	in.Artifact.Explanations = []goalwork.ExplanationDraft{{ID: "reference_date", Text: "The header states this source's reference date, not its retrieval time.", Citations: []goalwork.EvidenceCitation{{PacketID: "ep_context", PacketRow: 1, Field: "A"}}}}
	in.Artifact.Recipe.Explanations = in.Artifact.Explanations
	in.Analysis.AdditionalEvidence[1].Selection.Fields = []string{"A"}
	if _, err := ReviewGoal(context.Background(), in, "claude"); err == nil {
		t.Fatal("review adapter accepted a missing source explanation finding")
	}
	a.Explanations = []goalwork.ExplanationReview{{Explanation: "reference_date", Finding: f}}
	body, _ = json.Marshal(a)
	script = "#!/bin/sh\n/bin/cat > '" + strings.ReplaceAll(promptPath, "'", "'\\''") + "'\nprintf '%s' '" + strings.ReplaceAll(string(body), "'", "'\\''") + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	response, err = ReviewGoal(context.Background(), in, "claude")
	if err != nil || len(response.Assessment.Explanations) != 1 || response.Assessment.Explanations[0].Explanation != "reference_date" {
		t.Fatalf("source explanation judgement lost: %v", err)
	}
	prompt, err = os.ReadFile(promptPath)
	_, inputJSON, found = strings.Cut(string(prompt), "REVIEW_INPUT_JSON:\n")
	if err != nil || !found || !strings.Contains(string(prompt), "SOURCE_CITED_EXPLANATIONS_V1") || json.Unmarshal([]byte(inputJSON), &delivered) != nil || len(delivered.Artifact.Explanations) != 1 || delivered.Artifact.Explanations[0].Citations[0].PacketID != "ep_context" {
		t.Fatal("source explanation contract, draft or original citation did not reach the isolated reviewer")
	}
}
