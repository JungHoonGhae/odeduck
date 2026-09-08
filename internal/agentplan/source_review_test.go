package agentplan

import (
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
	}{{"source-report-review-20260908", "codex-raw.jsonl.gz", 6, 12}, {"analysis-review-20260908", "codex-raw.jsonl.gz", 3, 12}, {"analysis-review-20260908", "live-codex-raw.jsonl.gz", 6, 12}, {"citywide-review-20260908", "baseline-codex.json.gz", 0, 1}, {"citywide-review-20260908", "with-branches-codex.json.gz", 0, 1}, {"citywide-review-20260908", "historical-codex.json.gz", 0, 1}} {
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
			Input                           goalwork.ReviewInput
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
			response, err := ReviewGoal(context.Background(), trial.Input, "codex")
			if err != nil || response.Truncated || response.RawResponse != trial.ProviderResponse || !reflect.DeepEqual(response.Assessment, trial.Result.Evaluation.Review.Assessment) {
				t.Fatalf("archived response no longer decodes to its recorded findings: %v", err)
			}
		})
	}
	if count != wantTrials || ready != wantReady {
		t.Fatalf("archive denominator changed: %d trials, %d ready", count, ready)
	}
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
	in := goalwork.ReviewInput{Recipient: "claude", Goal: "compare recorded totals", Contract: goalwork.GoalContract{Outputs: []goalwork.OutputRequirement{{ID: "total"}}}, Evidence: goalwork.EvidencePacket{ID: "ep_one"}, Analysis: &goalwork.AnalysisReviewContext{Method: "engine_relational_replay_v1", AdditionalEvidence: []goalwork.EvidencePacket{{ID: "ep_two"}}}}
	in.Analysis.SourceContext = []goalwork.SourceContext{{Targets: []string{"o1"}, Evidence: goalwork.EvidencePacket{ID: "ep_context", Records: []goalwork.EvidenceRecord{{RetainedRow: 1, Values: goalwork.Row{"A": "SOURCE_HEADER_FIXTURE"}}}}}}
	response, err := ReviewGoal(context.Background(), in, "claude")
	if err != nil || len(response.Assessment.AnalysisChecks) != 4 {
		t.Fatalf("analysis response: %v", err)
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil || !strings.Contains(string(prompt), "RELATIONAL_ANALYSIS_V1") || strings.Contains(string(prompt), "No semantic approval of joins") {
		t.Fatal("analysis sent to source-only reviewer instructions")
	}
	if !strings.Contains(string(prompt), "SOURCE_HEADER_FIXTURE") || !strings.Contains(string(prompt), "same-file association") {
		t.Fatal("selected context or its association-only interpretation was lost in the adapter")
	}
}
