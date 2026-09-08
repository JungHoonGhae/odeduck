// goal-review-check runs a DEVELOPMENT calibration, not autonomous search or
// held-out goal grading. Expected source records are frozen independently of
// goalwork. Only selected public aggregate fields go to the explicitly chosen CLI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

type calibrationCase struct {
	ID, Reference, PK, Goal, NegativeGoal, NegativeReason, Region, Period, SourceSHA256 string
	Records                                                                             []int
	Fields                                                                              []string
	Analysis                                                                            *analysisPlan `json:"analysis,omitempty"`
}
type referenceSource struct {
	PK, URL, ObservedAt, ContentSHA256 string
	Records                            []struct {
		CSVRecord int
		Values    goalwork.Row
	}
}
type trial struct {
	Kind                      string                `json:"kind"`
	Case                      string                `json:"case"`
	Variant                   string                `json:"variant"`
	Repeat                    int                   `json:"repeat"`
	StartedAt                 time.Time             `json:"startedAt"`
	ElapsedSeconds            float64               `json:"elapsedSeconds"`
	ExpectedReady             bool                  `json:"expectedReady"`
	Matched                   bool                  `json:"matched"`
	ReferenceObservedAt       string                `json:"referenceObservedAt"`
	Input                     *goalwork.ReviewInput `json:"input,omitempty"`
	Result                    goalwork.View         `json:"result"`
	Error                     string                `json:"error,omitempty"`
	ProviderResponse          string                `json:"providerResponse,omitempty"`
	ProviderResponseTruncated bool                  `json:"providerResponseTruncated,omitempty"`
}

type calibrationServices struct {
	Inspect func(context.Context, string) (goalwork.Inspection, error)
	Review  func(context.Context, goalwork.ReviewInput, string) (agentplan.GoalReviewResponse, error)
}

type preparedSource struct {
	source     referenceSource
	rows       []goalwork.Row
	inspection goalwork.Inspection
	err        error
}

func main() {
	provider := flag.String("agent", "", "explicit codex|claude|gemini; sends selected public aggregate records")
	out := flag.String("output", "", "new JSONL file; existing files are never overwritten")
	manifest := flag.String("cases", "internal/goalwork/testdata/goalbench-v1/source-report-cases.json", "frozen development cases")
	flag.Parse()
	if !slices.Contains([]string{"codex", "claude", "gemini"}, *provider) || *out == "" {
		fmt.Fprintln(os.Stderr, "explicit --agent and --output required")
		os.Exit(2)
	}
	live := goalwork.LiveDependencies(fetch.New(), "https://www.data.go.kr", nil, catalog.Searcher{}, goalwork.Policy{})
	if err := run(*provider, *out, *manifest, calibrationServices{Inspect: live.Inspect, Review: agentplan.ReviewGoal}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(provider, output, manifest string, runtime calibrationServices) error {
	body, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	var config struct {
		Kind  string
		Cases []calibrationCase
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	if len(config.Cases) != 2 {
		return fmt.Errorf("expected two frozen development cases")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	matched, count := 0, 0
	for _, c := range config.Cases {
		var evaluate func(int, bool) trial
		if c.Analysis == nil {
			prepared := prepareSource(c, filepath.Dir(manifest), runtime)
			evaluate = func(repeat int, positive bool) trial {
				return runTrial(provider, config.Kind, c, prepared, runtime, repeat, positive)
			}
		} else {
			var prepared []preparedSource
			for _, source := range c.Analysis.Sources {
				prepared = append(prepared, prepareSource(calibrationCase{ID: c.ID, Reference: source.Reference, PK: source.PK, SourceSHA256: source.SourceSHA256, Records: source.Records}, filepath.Dir(manifest), runtime))
			}
			evaluate = func(repeat int, positive bool) trial {
				return runAnalysisTrial(provider, config.Kind, c, prepared, runtime, repeat, positive)
			}
		}
		for repeat := 1; repeat <= 3; repeat++ {
			for _, positive := range []bool{true, false} {
				t := evaluate(repeat, positive)
				if err := encoder.Encode(t); err != nil {
					return err
				}
				if err := f.Sync(); err != nil {
					return err
				}
				count++
				if t.Matched {
					matched++
				}
				fmt.Fprintf(os.Stderr, "%s %s %d: status=%s matched=%t error=%s\n", c.ID, t.Variant, repeat, t.Result.Status, t.Matched, t.Error)
			}
		}
	}
	if matched != count {
		return fmt.Errorf("development calibration matched %d/%d; not a passing completion claim", matched, count)
	}
	fmt.Fprintf(os.Stderr, "development calibration matched %d/%d; not autonomous/held-out/G1–G5 completion\n", matched, count)
	return nil
}

func prepareSource(c calibrationCase, root string, runtime calibrationServices) (p preparedSource) {
	body, err := os.ReadFile(filepath.Join(root, c.Reference))
	if err != nil {
		p.err = err
		return
	}
	var reference struct{ Sources []referenceSource }
	if err := json.Unmarshal(body, &reference); err != nil {
		p.err = err
		return
	}
	for _, source := range reference.Sources {
		if source.PK == c.PK {
			p.source = source
		}
	}
	if p.source.ContentSHA256 != c.SourceSHA256 || c.SourceSHA256 == "" {
		p.err = fmt.Errorf("reference source changed: %s", c.ID)
		return
	}
	for _, ordinal := range c.Records {
		for _, r := range p.source.Records {
			if r.CSVRecord == ordinal {
				p.rows = append(p.rows, r.Values)
			}
		}
	}
	if len(p.rows) != len(c.Records) || len(p.rows) == 0 {
		p.err = fmt.Errorf("missing independent source records: %s", c.ID)
		return
	}
	// Current metadata is read through odeduck, separately from historical rows.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	p.inspection, p.err = runtime.Inspect(ctx, c.PK)
	return
}

func runTrial(provider, kind string, c calibrationCase, prepared preparedSource, runtime calibrationServices, repeat int, positive bool) (t trial) {
	source, rows := prepared.source, prepared.rows
	t = trial{Kind: kind, Case: c.ID, Variant: "positive", Repeat: repeat, StartedAt: time.Now().UTC(), ExpectedReady: positive, ReferenceObservedAt: source.ObservedAt}
	defer func() { t.ElapsedSeconds = time.Since(t.StartedAt).Seconds() }()
	goal := c.Goal
	if !positive {
		goal, t.Variant = c.NegativeGoal, "stronger_goal_negative"
	}
	if prepared.err != nil {
		t.Result = goalwork.View{Goal: goal, Status: "not_run"}
		t.Error = prepared.err.Error()
		return
	}
	policy := goalwork.Policy{EvidenceRecipient: provider, ReviewRecipient: provider}
	// This deliberately replays previously verified raw rows. It does not claim
	// a new source download or invent an inspected source declaration.
	deps := goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: c.PK, Title: c.ID}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) { return prepared.inspection, nil },
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Delivery: "FILE", Rows: rows, ContentSHA256: source.ContentSHA256, Warnings: []string{"Development replay of independently recorded CSV values. Original URL: " + source.URL, fmt.Sprintf("Original CSV data record positions: %v; observedAt: %s. No new download or fabricated physical line positions.", c.Records, source.ObservedAt)}}, nil
		},
		Review: func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Input = &in
			response, err := runtime.Review(ctx, in, provider)
			t.ProviderResponse, t.ProviderResponseTruncated = response.RawResponse, response.Truncated
			return response.Assessment, err
		},
	}
	e, err := goalwork.Start(goal, policy, deps)
	if err != nil {
		t.Error = err.Error()
		return
	}
	cx := goalwork.GoalContract{Outcome: c.Goal, Region: c.Region, Period: c.Period, Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "records", Description: "requested historical source records"}}}
	p := goalwork.Composition{ID: "source_report", Base: "o1", Purpose: "copy requested original fields only", Roles: []goalwork.RoleBinding{{Role: "records", Observation: "o1"}}, Assumptions: []string{"Report of identified historical source records only, not field verification, population estimate or a newly computed value."}}
	for n, field := range c.Fields {
		id := fmt.Sprintf("field%d", n+1)
		cx.Outputs = append(cx.Outputs, goalwork.OutputRequirement{ID: id, Description: field + " 원문 값", Role: "records", Type: "string"})
		p.Select = append(p.Select, "o1."+field)
		p.Outputs = append(p.Outputs, goalwork.OutputBinding{Output: id, Field: "o1." + field})
	}
	decisions := []goalwork.Decision{{Action: "define", Contract: &cx}, {Action: "search", Query: "development reference replay", Role: "records"}, {Action: "inspect", PK: c.PK}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: c.PK, Delivery: "file"}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	for _, d := range decisions {
		t.Result, err = e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(t.Result.Gaps) != 0 {
			t.Error = fmt.Sprintf("setup: %v %v", err, t.Result.Gaps)
			return
		}
	}
	positions := make([]int, len(rows))
	for i := range positions {
		positions[i] = i + 1
	}
	for _, d := range []goalwork.Decision{{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: t.Result.Observations[0].RowsSHA256, Rows: positions, Fields: c.Fields}}, {Action: "review_result", CompositionID: p.ID}} {
		t.Result, err = e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(t.Result.Gaps) != 0 {
			t.Error = fmt.Sprintf("review: %v %v", err, t.Result.Gaps)
			return
		}
	}
	t.Matched = (t.Result.Status == "output_ready") == positive && t.Result.Evaluation.Review != nil
	return
}
