package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Exercise the command handler with real Engine execution; only external
// inspection/model calls vary. These tests grade bookkeeping, not model accuracy.
func TestCalibrationCommandPreservesEveryTrialAndNeverOverwrites(t *testing.T) {
	for _, mode := range []string{"pass", "review unavailable", "false positive", "missing reference"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			manifest := fixtureCases(t, dir, mode == "missing reference")
			output := filepath.Join(dir, "trials.jsonl")
			runtime := calibrationServices{
				Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
					return goalwork.Inspection{PK: pk}, nil
				},
				Review: func(_ context.Context, in goalwork.ReviewInput, _ string) (agentplan.GoalReviewResponse, error) {
					if len(in.Artifact.Rows) != 2 || in.Artifact.Rows[0]["o1.value"] != "64" || in.Artifact.Rows[1]["o1.value"] != "62" {
						t.Fatal("did not execute frozen source values")
					}
					if mode == "review unavailable" {
						return agentplan.GoalReviewResponse{RawResponse: "fixture provider failure"}, errors.New("external failure")
					}
					finding := goalwork.ReviewFinding{Verdict: "supported", Reason: "fixture source report", PacketID: in.Evidence.ID}
					a := goalwork.ReviewAssessment{GoalFit: finding, Outputs: []goalwork.OutputReview{{Output: "field1", Finding: finding}}}
					if strings.Contains(in.Goal, "stronger") && mode != "false positive" {
						a.GoalFit.Verdict = "unsupported"
					}
					return agentplan.GoalReviewResponse{Assessment: a, RawResponse: "fixture model response"}, nil
				},
			}
			err := run("codex", output, manifest, runtime)
			if (err == nil) != (mode == "pass") {
				t.Fatalf("mode=%s error=%v", mode, err)
			}
			rows := readTrials(t, output)
			if len(rows) != 12 {
				t.Fatalf("lost planned denominator: %d", len(rows))
			}
			matched := 0
			for _, row := range rows {
				if row.Matched {
					matched++
				}
				if row.Input != nil && row.ProviderResponse == "" {
					t.Fatal("lost raw model response")
				}
			}
			want := map[string]int{"pass": 12, "review unavailable": 0, "false positive": 6, "missing reference": 6}[mode]
			if matched != want {
				t.Fatalf("matched=%d want=%d", matched, want)
			}
			before, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if err := run("codex", output, manifest, runtime); err == nil {
				t.Fatal("overwrote a prior trial")
			}
			after, _ := os.ReadFile(output)
			if string(before) != string(after) {
				t.Fatal("prior trace changed")
			}
		})
	}
}

func fixtureCases(t *testing.T, dir string, missing bool) string {
	t.Helper()
	config := struct {
		Kind  string
		Cases []calibrationCase
	}{Kind: "test_not_calibration"}
	for _, id := range []string{"first", "second"} {
		c := calibrationCase{ID: id, Reference: id + ".json", PK: id, Goal: "report source", NegativeGoal: "stronger claim", Region: "fixture", Period: "source", Records: []int{14, 15}, Fields: []string{"value"}, SourceSHA256: strings.Repeat("a", 64)}
		config.Cases = append(config.Cases, c)
		if missing && id == "second" {
			continue
		}
		body := `{"sources":[{"pk":"` + id + `","contentSha256":"` + c.SourceSHA256 + `","records":[{"csvRecord":14,"values":{"value":"64"}},{"csvRecord":15,"values":{"value":"62"}}]}]}`
		if err := os.WriteFile(filepath.Join(dir, c.Reference), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.Marshal(config)
	path := filepath.Join(dir, "cases.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTrials(t *testing.T, path string) []trial {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1<<20)
	var out []trial
	for s.Scan() {
		var row trial
		if err := json.Unmarshal(s.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
