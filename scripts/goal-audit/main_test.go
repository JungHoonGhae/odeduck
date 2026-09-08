package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalaudit"
)

func TestRetainedSuiteFailsReadinessWithoutLosingUnrunCases(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := run([]string{"../../internal/goalwork/testdata/goalbench-v1/trial-audit.json"}, &out, &diagnostic)
	if code != 1 || diagnostic.Len() != 0 {
		t.Fatalf("want readiness failure, got %d %s", code, diagnostic.String())
	}
	var r goalaudit.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.ReadinessFloorMet || len(r.Cases) != 5 || r.UnrunCases != 4 || r.Totals.Planned != 2 || r.Totals.Loaded != 2 || r.Totals.States["abstained"] != 2 || r.Totals.Searches == nil || *r.Totals.Searches != 12 || r.Totals.Executions == nil || *r.Totals.Executions != 3 || r.Totals.SummaryOnlyRows != 6 || r.Totals.ArtifactRows != nil {
		t.Fatalf("retained suite changed meaning: %+v", r)
	}
	if r.IndependentGrading != "not_performed" {
		t.Fatal("audit became an independent goal grade")
	}
}

func TestCommandSeparatesMalformedManifestFromReadinessFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(`{"private-content":`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {path}, {filepath.Join(dir, "missing.json")}} {
		var out, diagnostic bytes.Buffer
		if code := run(args, &out, &diagnostic); code != 2 || out.Len() != 0 || bytes.Contains(diagnostic.Bytes(), []byte("private-content")) {
			t.Fatalf("unsafe/ambiguous command error: %d %s %s", code, out.String(), diagnostic.String())
		}
	}
}

func TestCommandReadyFloorStillDoesNotClaimIndependentGrading(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"schemaVersion":1,"cases":[{"id":"synthetic","goal":"goal","expectation":"positive","trials":[{"id":"first","path":"view.json","format":"view"}]}]}`
	view := `{"goal":"goal","status":"output_ready","evaluation":{"status":"requirements_met","needsSemanticReview":false},"artifact":{"status":"sample_executed","rows":[{"value":"private-fixture"}],"evaluation":{"status":"requirements_met","needsSemanticReview":false}}}`
	for name, data := range map[string]string{"manifest.json": manifest, "view.json": view} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out, diagnostic bytes.Buffer
	if code := run([]string{filepath.Join(dir, "manifest.json")}, &out, &diagnostic); code != 0 || diagnostic.Len() != 0 || bytes.Contains(out.Bytes(), []byte("private-fixture")) {
		t.Fatalf("candidate reporting failed: %d %s %s", code, out.String(), diagnostic.String())
	}
	var r goalaudit.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || !r.ReadinessFloorMet || r.IndependentGrading != "not_performed" {
		t.Fatal("minimum ready floor was confused with independent grading")
	}
}
