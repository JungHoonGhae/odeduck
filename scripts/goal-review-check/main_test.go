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

func TestAnalysisCalibrationExecutesBeforeReviewAndPreservesItsDenominator(t *testing.T) {
	for _, badOracle := range []bool{false, true} {
		dir := t.TempDir()
		manifest := fixtureCases(t, dir, false)
		body, _ := os.ReadFile(manifest)
		var config struct {
			Kind  string
			Cases []calibrationCase
		}
		if err := json.Unmarshal(body, &config); err != nil {
			t.Fatal(err)
		}
		for i := range config.Cases {
			c := &config.Cases[i]
			want := json.Number("126")
			if badOracle {
				want = json.Number("125")
			}
			contract := goalwork.GoalContract{Outcome: c.Goal, Region: "fixture", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Role: "r", Description: "sum", Type: "number"}}}
			p := goalwork.Composition{ID: "sum", Base: "o1", Purpose: "sum reference values", Select: []string{"total"}, Measures: []goalwork.Measure{{As: "n", Field: "o1.value", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "total"}}, Assumptions: []string{"fixture"}}
			c.Analysis = &analysisPlan{Sources: []analysisSource{{Reference: c.Reference, PK: c.PK, SourceSHA256: c.SourceSHA256, Records: c.Records, Role: "r"}}, Contract: contract, Decisions: []goalwork.Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "sum"}}, Evidence: []goalwork.EvidenceRequest{{Observation: "o1", Rows: []int{1, 2}, Fields: []string{"value"}}}, ExpectedRows: []goalwork.Row{{"total": want}}}
		}
		body, _ = json.Marshal(config)
		if err := os.WriteFile(manifest, body, 0600); err != nil {
			t.Fatal(err)
		}
		calls := 0
		runtime := calibrationServices{Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		}, Review: func(_ context.Context, in goalwork.ReviewInput, _ string) (agentplan.GoalReviewResponse, error) {
			calls++
			if in.Analysis == nil || in.Artifact.Rows[0]["total"] != json.Number("126") {
				t.Fatal("calibration bypassed actual computation")
			}
			f := goalwork.ReviewFinding{Verdict: "supported", Reason: "fixture", PacketIDs: []string{in.Evidence.ID}}
			a := goalwork.ReviewAssessment{GoalFit: f, Outputs: []goalwork.OutputReview{{Output: "total", Finding: f}}}
			for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
				a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: f})
			}
			if strings.Contains(in.Goal, "stronger") {
				a.GoalFit.Verdict = "unsupported"
			}
			return agentplan.GoalReviewResponse{Assessment: a, RawResponse: "fixture raw analysis"}, nil
		}}
		output := filepath.Join(dir, "analysis.jsonl")
		err := run("codex", output, manifest, runtime)
		if (err != nil) != badOracle || len(readTrials(t, output)) != 12 {
			t.Fatalf("bad oracle=%t: %v", badOracle, err)
		}
		if (badOracle && calls != 0) || (!badOracle && calls != 12) {
			t.Fatal("wrong arithmetic reached reviewer or lost a scheduled trial")
		}
	}
}

// Replaying these independently frozen source values grades current execution
// and disclosure plumbing, not a new model trial or semantic approval.
func TestAnalysisManifestReplaysIndependentExpectedRows(t *testing.T) {
	manifest := "../../internal/goalwork/testdata/goalbench-v1/analysis-review-cases.json"
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var config struct{ Cases []calibrationCase }
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	negativeGoals := map[string]bool{}
	for _, c := range config.Cases {
		negativeGoals[c.NegativeGoal] = true
	}
	calls := 0
	runtime := calibrationServices{
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Review: func(_ context.Context, in goalwork.ReviewInput, _ string) (agentplan.GoalReviewResponse, error) {
			calls++
			if in.Analysis == nil || len(in.Artifact.Rows) == 0 {
				t.Fatal("frozen analysis did not reach the real review seam")
			}
			finding := goalwork.ReviewFinding{Verdict: "supported", Reason: "offline bookkeeping fixture, not a semantic judgement", PacketIDs: []string{in.Evidence.ID}}
			a := goalwork.ReviewAssessment{GoalFit: finding}
			for _, output := range in.Contract.Outputs {
				a.Outputs = append(a.Outputs, goalwork.OutputReview{Output: output.ID, Finding: finding})
			}
			for _, topic := range []string{"relations", "periods", "measurements", "coverage"} {
				a.AnalysisChecks = append(a.AnalysisChecks, goalwork.AnalysisCheck{Topic: topic, Finding: finding})
			}
			if negativeGoals[in.Goal] {
				a.GoalFit.Verdict = "unsupported"
			}
			return agentplan.GoalReviewResponse{Assessment: a, RawResponse: "offline fixture"}, nil
		},
	}
	output := filepath.Join(t.TempDir(), "replay.jsonl")
	if err := run("codex", output, manifest, runtime); err != nil {
		t.Fatal(err)
	}
	if calls != 12 || len(readTrials(t, output)) != 12 {
		t.Fatal("frozen reference replay lost a case")
	}
}
