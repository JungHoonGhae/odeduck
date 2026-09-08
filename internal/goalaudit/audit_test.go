package goalaudit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalaudit"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestActualEngineReviewIsNotAReadyGoalEvenWithCorrectRows(t *testing.T) {
	const goal = "Report the observed fixture record"
	e, err := goalwork.Start(goal, goalwork.Policy{}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records", Title: "fixture records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"label": "private-observed-value"}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	contract := goalwork.GoalContract{Outcome: "source record", Region: "fixture", Period: "2025", Coverage: "sample",
		Roles:   []goalwork.RoleRequirement{{ID: "records", Description: "observed records"}},
		Outputs: []goalwork.OutputRequirement{{ID: "label", Description: "source label", Role: "records", Type: "string"}},
	}
	recipe := goalwork.Composition{ID: "report", Purpose: "source reporting", Base: "o1", Select: []string{"o1.label"},
		Roles:       []goalwork.RoleBinding{{Role: "records", Observation: "o1"}},
		Outputs:     []goalwork.OutputBinding{{Output: "label", Field: "o1.label"}},
		Assumptions: []string{"fixture source reporting only"},
	}
	for _, d := range []goalwork.Decision{
		{Action: "define", Contract: &contract},
		{Action: "search", Query: "records", Role: "records"},
		{Action: "inspect", PK: "records"},
		{Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}},
		{Action: "compose", Composition: &recipe},
		{Action: "execute", CompositionID: "report"},
	} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if v.Status != "review_required" || v.Artifact == nil || v.Artifact.Rows[0]["o1.label"] != "private-observed-value" {
		t.Fatalf("fixture did not reach the real review path: %+v", v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":1,"cases":[{"id":"report","goal":"` + goal + `","expectation":"positive","trials":[{"id":"first","path":"view.json","format":"view"}]}]}`
	r, err := goalaudit.Audit(strings.NewReader(manifest), fstest.MapFS{"view.json": &fstest.MapFile{Data: b}})
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadinessFloorMet || r.Totals.ReadyCandidates != 0 || r.Totals.ArtifactRows == nil || *r.Totals.ArtifactRows != 1 || r.Totals.States["review_required"] != 1 || r.Totals.Planned != 1 {
		t.Fatalf("all-review counted as completion: %+v", r)
	}
	if r.IndependentGrading != "not_performed" {
		t.Fatal("execution audit claimed independent grading")
	}
	output, err := json.Marshal(r)
	if err != nil || strings.Contains(string(output), "private-observed-value") {
		t.Fatal("audit exposed a source row")
	}
}

func TestMissingInvalidAndUnrunCasesRemainInDenominators(t *testing.T) {
	manifest := `{"schemaVersion":1,"cases":[
		{"id":"run","goal":"goal","expectation":"positive","trials":[
			{"id":"abstained","path":"a.json","format":"view"},
			{"id":"missing","path":"missing.json","format":"view"},
			{"id":"broken","path":"broken.json","format":"view"}]},
		{"id":"unrun","goal":"another goal","expectation":"positive","trials":[]}]}`
	r, err := goalaudit.Audit(strings.NewReader(manifest), fstest.MapFS{
		"a.json":      {Data: []byte(`{"goal":"goal","status":"abstained"}`)},
		"broken.json": {Data: []byte(`{"private-broken-content"`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadinessFloorMet || r.UnrunCases != 1 || len(r.Cases) != 2 || r.Totals.Planned != 3 || r.Totals.Loaded != 1 || r.Totals.Missing != 1 || r.Totals.Invalid != 1 || r.Totals.States["abstained"] != 1 {
		t.Fatalf("incomplete cases disappeared: %+v", r)
	}
	if len(r.Cases[0].Trials) != 3 || r.Cases[1].Counts.Planned != 0 {
		t.Fatal("invented or omitted trial history")
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "private-broken-content") {
		t.Fatal("invalid record content escaped through diagnostics")
	}
}

func TestManifestErrorsCannotSilentlyShrinkTheAudit(t *testing.T) {
	trial := `{"id":"first","path":"a.json","format":"view"}`
	one := `{"id":"one","goal":"goal","expectation":"positive","trials":[` + trial + `]}`
	for name, input := range map[string]string{
		"empty":                 `{}`,
		"null":                  `null`,
		"wrong version":         `{"schemaVersion":2,"cases":[` + one + `]}`,
		"unknown field":         `{"schemaVersion":1,"cases":[` + one + `],"private-unknown":true}`,
		"trailing JSON":         `{"schemaVersion":1,"cases":[` + one + `]} {}`,
		"duplicate case":        `{"schemaVersion":1,"cases":[` + one + `,` + one + `]}`,
		"duplicate JSON field":  `{"schemaVersion":1,"cases":[` + one + `,` + one + `],"cases":[` + one + `]}`,
		"case alias collision":  `{"schemaVersion":1,"cases":[` + one + `,` + one + `],"Cases":[` + one + `]}`,
		"trial alias collision": `{"schemaVersion":1,"cases":[` + strings.Replace(one, `"trials":[`, `"Trials":[],"trials":[`, 1) + `]}`,
		"duplicate trial":       `{"schemaVersion":1,"cases":[` + strings.Replace(one, trial, trial+","+trial, 1) + `]}`,
		"duplicate path":        `{"schemaVersion":1,"cases":[` + strings.Replace(one, trial, trial+","+strings.Replace(trial, "first", "second", 1), 1) + `]}`,
		"outside path":          `{"schemaVersion":1,"cases":[` + strings.Replace(one, "a.json", "../a.json", 1) + `]}`,
		"unknown format":        `{"schemaVersion":1,"cases":[` + strings.Replace(one, "view", "guess", 1) + `]}`,
		"unknown expected":      `{"schemaVersion":1,"cases":[` + strings.Replace(one, "positive", "ignore", 1) + `]}`,
		"empty goal":            `{"schemaVersion":1,"cases":[` + strings.Replace(one, `"goal":"goal"`, `"goal":""`, 1) + `]}`,
		"oversized":             strings.Repeat(" ", 1<<20) + `{"schemaVersion":1,"cases":[` + one + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := goalaudit.Audit(strings.NewReader(input), fstest.MapFS{"a.json": {Data: []byte(`{"goal":"goal","status":"abstained"}`)}})
			if err == nil || strings.Contains(err.Error(), "private-unknown") {
				t.Fatalf("expected safe manifest error, got %v", err)
			}
		})
	}
}

func singleManifest(goal, format string) string {
	return fmt.Sprintf(`{"schemaVersion":1,"cases":[{"id":"case","goal":%q,"expectation":"positive","trials":[{"id":"first","path":"a.json","format":%q}]}]}`, goal, format)
}

// These synthetic views only test the reporting contract. They do not establish
// an actual Engine completion, independent correctness or autonomous execution.
const readyView = `{"goal":"goal","status":"output_ready","evaluation":{"status":"requirements_met","needsSemanticReview":false},"artifact":{"status":"sample_executed","rows":[{"value":"private-result"}],"evaluation":{"status":"requirements_met","needsSemanticReview":false}}}`

func TestReadyCandidateRequiresConsistentObservedState(t *testing.T) {
	for name, input := range map[string]string{
		"different goal":         strings.Replace(readyView, `"goal":"goal"`, `"goal":"other"`, 1),
		"unknown status":         strings.Replace(readyView, "output_ready", "private-unknown-status", 1),
		"missing goal":           strings.Replace(readyView, `"goal":"goal",`, "", 1),
		"null":                   `null`,
		"duplicate state":        strings.Replace(readyView, `"status":"output_ready"`, `"status":"abstained","status":"output_ready"`, 1),
		"state alias collision":  strings.Replace(readyView, `"status":"output_ready"`, `"status":"abstained","Status":"output_ready"`, 1),
		"review alias collision": strings.Replace(readyView, `"needsSemanticReview":false`, `"needsSemanticReview":true,"NeedsSemanticReview":false`, 1),
		"missing review":         strings.ReplaceAll(readyView, `,"needsSemanticReview":false`, ""),
		"requires review":        strings.Replace(readyView, `"needsSemanticReview":false`, `"needsSemanticReview":true`, 1),
		"empty artifact":         strings.Replace(readyView, `[{"value":"private-result"}]`, `[]`, 1),
		"partial artifact":       strings.Replace(readyView, `"status":"sample_executed"`, `"status":"not_executed"`, 1),
		"oversized":              strings.Repeat(" ", 16<<20) + readyView,
	} {
		t.Run(name, func(t *testing.T) {
			r, err := goalaudit.Audit(strings.NewReader(singleManifest("goal", "view")), fstest.MapFS{"a.json": {Data: []byte(input)}})
			if err != nil || r.ReadinessFloorMet || r.Totals.Invalid != 1 || r.Totals.Planned != 1 {
				t.Fatalf("inconsistent view was counted: %+v %v", r, err)
			}
			b, _ := json.Marshal(r)
			if strings.Contains(string(b), "private-") {
				t.Fatal("invalid state or source data escaped")
			}
		})
	}
	r, err := goalaudit.Audit(strings.NewReader(singleManifest("goal", "view")), fstest.MapFS{"a.json": {Data: []byte(readyView)}})
	if err != nil || !r.ReadinessFloorMet || r.Totals.ReadyCandidates != 1 || r.Cases[0].Trials[0].CommandFailed != nil || r.IndependentGrading != "not_performed" {
		t.Fatalf("candidate grade or unknown process status changed: %+v %v", r, err)
	}
}

func TestRetainedDiagnosticsReportActualFailureAndActivity(t *testing.T) {
	const goal = "인천의 공공 문화·관광시설을 휠체어 이용자가 비교할 수 있게, 시설의 접근성 정보와 주변 대중교통 정보를 연결해 표를 만들어줘. 자료별 기준일과 실제 이동 가능 여부를 확인하지 못한 부분도 보여줘."
	manifest := strings.Replace(singleManifest(goal, "diagnostic"), "a.json", "mobility-unseeded-diagnostic-20260907.json", 1)
	r, err := goalaudit.Audit(strings.NewReader(manifest), os.DirFS("../goalwork/testdata/goalbench-v1"))
	if err != nil || r.ReadinessFloorMet || r.Totals.Loaded != 1 || r.Totals.States["abstained"] != 1 {
		t.Fatalf("retained diagnostic failed to load: %+v %v", r, err)
	}
	c := r.Totals
	if c.Searches == nil || *c.Searches != 7 || c.SemanticSearches == nil || *c.SemanticSearches != 7 || c.Candidates != nil || c.Observations == nil || *c.Observations != 7 || c.AcquisitionAttempts == nil || *c.AcquisitionAttempts != 8 || c.Executions == nil || *c.Executions != 1 || c.ArtifactRows != nil {
		t.Fatalf("activity counters differ from retained record: %+v", c)
	}
	tr := r.Cases[0].Trials[0]
	if tr.CommandFailed == nil || !*tr.CommandFailed || len(tr.ArtifactSHA256) != 64 || tr.ReadyCandidate || !slices.Contains(tr.MissingFields, "nodes") {
		t.Fatalf("diagnostic process failure was lost: %+v", tr)
	}
}

func TestDiagnosticCommandFailureAndStatusMismatchCannotPass(t *testing.T) {
	for name, run := range map[string]string{
		"command failed":          `{"status":"output_ready","commandFailed":true}`,
		"status differs":          `{"status":"abstained","commandFailed":false}`,
		"missing process outcome": `{"status":"output_ready"}`,
	} {
		t.Run(name, func(t *testing.T) {
			input := `{"run":` + run + `,"execution":` + readyView + `}`
			r, err := goalaudit.Audit(strings.NewReader(singleManifest("goal", "diagnostic")), fstest.MapFS{"a.json": {Data: []byte(input)}})
			if err != nil || r.ReadinessFloorMet || r.Totals.ReadyCandidates != 0 {
				t.Fatalf("failed/inconsistent diagnostic passed: %+v %v", r, err)
			}
		})
	}
}

func TestMissingArtifactRowsAreUnknownButExplicitEmptyRowsAreZero(t *testing.T) {
	for name, artifact := range map[string]string{
		"missing": `{}`,
		"null":    `{"rows":null}`,
		"empty":   `{"rows":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			input := `{"goal":"goal","status":"abstained","artifact":` + artifact + `}`
			r, err := goalaudit.Audit(strings.NewReader(singleManifest("goal", "view")), fstest.MapFS{"a.json": {Data: []byte(input)}})
			if err != nil || r.Totals.Loaded != 1 {
				t.Fatalf("cannot audit incomplete artifact: %+v %v", r, err)
			}
			tr := r.Cases[0].Trials[0]
			if name == "empty" {
				if tr.ArtifactRows == nil || *tr.ArtifactRows != 0 || r.Totals.ArtifactRows == nil || *r.Totals.ArtifactRows != 0 || slices.Contains(tr.MissingFields, "artifact.rows") {
					t.Fatal("explicit empty rows were lost")
				}
			} else if tr.ArtifactRows != nil || r.Totals.ArtifactRows != nil || !slices.Contains(tr.MissingFields, "artifact.rows") {
				t.Fatal("missing rows were counted as zero")
			}
		})
	}
}

func TestSourceRowKeysRemainCaseSensitive(t *testing.T) {
	input := strings.Replace(readyView, `{"value":"private-result"}`, `{"Model":"private-upper","model":"private-lower"}`, 1)
	r, err := goalaudit.Audit(strings.NewReader(singleManifest("goal", "view")), fstest.MapFS{"a.json": {Data: []byte(input)}})
	if err != nil || !r.ReadinessFloorMet || r.Totals.ReadyCandidates != 1 || r.Totals.ArtifactRows == nil || *r.Totals.ArtifactRows != 1 {
		t.Fatalf("control-field rule changed valid source keys: %+v %v", r, err)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "private-") {
		t.Fatal("source keys or values escaped into the audit")
	}
}

func TestNestedActivityControlAliasesCannotChangeCounts(t *testing.T) {
	for name, activity := range map[string]string{
		"summary":         `"artifactSummary":{"rowCount":6,"RowCount":999}`,
		"semantic status": `"searches":[{"semantic":{"status":"unavailable","Status":"used"}}]`,
		"semantic object": `"searches":[{"semantic":{"status":"unavailable"},"Semantic":{"status":"used"}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			input := `{"goal":"goal","status":"abstained",` + activity + `}`
			r, err := goalaudit.Audit(strings.NewReader(singleManifest("goal", "view")), fstest.MapFS{"a.json": {Data: []byte(input)}})
			if err != nil || r.Totals.Invalid != 1 || r.Totals.Loaded != 0 || r.ReadinessFloorMet || r.Totals.SummaryOnlyRows != 0 || r.Totals.SemanticSearches != nil {
				t.Fatalf("aliased activity was counted: %+v %v", r, err)
			}
		})
	}
}
