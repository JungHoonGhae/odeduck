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
	f, err := os.Open("../goalwork/testdata/goalbench-v1/source-report-review-20260908/codex-raw.jsonl.gz")
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
		}
		if err := d.Decode(&trial); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		count++
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
	if count != 12 || ready != 6 {
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
