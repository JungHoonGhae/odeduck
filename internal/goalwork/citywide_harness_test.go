package goalwork_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Exercise the actual go-test entrypoint, including fatal/cleanup behavior.
// Only the external coding-agent CLI is replaced; no live account is needed.
func TestCitywideReviewDiagnosticPreservesFailures(t *testing.T) {
	var falseApproval goalwork.ReviewAssessment
	checkCitywideReduction(t, false, &citywideModelReview{recipient: "codex", call: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		falseApproval = supportedAnalysisReview(in)
		return falseApproval, nil
	}})
	approved, err := json.Marshal(falseApproval)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"existing-output", "provider-error", "false-approval", "missing-reference", "unproven-full-scope", "unproven-table-context", "invalid-provider", "invalid-mode"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "diagnostic.json")
			body, exit := approved, "0"
			if kind == "provider-error" {
				body, exit = []byte("provider failure fixture"), "1"
			}
			response := filepath.Join(dir, "response.json")
			if err := os.WriteFile(response, body, 0600); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\n/bin/cat '" + strings.ReplaceAll(response, "'", "'\\''") + "'\nexit " + exit + "\n"
			if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "existing-output" {
				if err := os.WriteFile(output, []byte("retained previous trial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			provider, mode, variant := "codex", "reference", "baseline"
			if kind == "unproven-full-scope" {
				variant = "with-branches-full-scope"
			}
			if kind == "unproven-table-context" {
				variant = "with-table-context"
			}
			if kind == "invalid-provider" {
				provider = "auto"
			}
			if kind == "invalid-mode" {
				mode = "automatic-fallback"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLiveCitywideGoalAnalysisReview$", "-test.count=1")
			for _, env := range os.Environ() {
				if !strings.HasPrefix(env, "ODEDUCK_CITYWIDE_") && !strings.HasPrefix(env, "PATH=") {
					cmd.Env = append(cmd.Env, env)
				}
			}
			cmd.Env = append(cmd.Env, "PATH="+dir, "ODEDUCK_CITYWIDE_REVIEW="+provider, "ODEDUCK_CITYWIDE_ACQUISITION="+mode, "ODEDUCK_CITYWIDE_RESULT="+variant, "ODEDUCK_CITYWIDE_REVIEW_OUTPUT="+output)
			if kind == "missing-reference" {
				cmd.Dir = dir // The declared source reference is genuinely unavailable.
			}
			log, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("diagnostic should fail for %s: %s", kind, log)
			}
			saved, err := os.ReadFile(output)
			if kind == "invalid-provider" || kind == "invalid-mode" {
				if !os.IsNotExist(err) {
					t.Fatal("invalid setup created an execution record")
				}
				return
			}
			if err != nil {
				t.Fatalf("failed attempt was not preserved: %v\n%s", err, log)
			}
			if kind == "existing-output" {
				if string(saved) != "retained previous trial" {
					t.Fatal("earlier trial was overwritten")
				}
				return
			}
			var record struct {
				PlannedTrials int
				Input         *goalwork.ReviewInput
				Response      *agentplan.GoalReviewResponse
				Result        goalwork.View
				Error         string
			}
			if err := json.Unmarshal(saved, &record); err != nil || record.PlannedTrials != 1 || record.Error == "" {
				t.Fatalf("failed trial lost its denominator/error: %v", err)
			}
			if kind == "missing-reference" {
				if record.Input != nil || record.Response != nil || record.Result.Status != "not_run" {
					t.Fatal("preparation failure invented a model execution")
				}
				return
			}
			if kind == "unproven-full-scope" || kind == "unproven-table-context" {
				if record.Input != nil || record.Response != nil || record.Result.Contract == nil || record.Result.Contract.Coverage != "population" || record.Result.Evaluation == nil || record.Result.Evaluation.FullScope == nil || record.Result.Evaluation.FullScope.Eligible {
					t.Fatal("reference replay manufactured acquisition evidence or called the reviewer")
				}
				return
			}
			if record.Input == nil || record.Response == nil || record.Response.RawResponse != string(body) {
				t.Fatal("model failure or false approval lost its actual input/response")
			}
			if (record.Result.Status == "output_ready") != (kind == "false-approval") {
				t.Fatal("diagnostic hid the false approval or approved a provider error")
			}
		})
	}
}
