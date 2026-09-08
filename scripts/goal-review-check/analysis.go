package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Development manifests reuse Engine decisions, not a second query language.
// PKs/recipes/oracles are deliberate calibration inputs, never planner hints.
type analysisPlan struct {
	Sources      []analysisSource           `json:"sources"`
	Contract     goalwork.GoalContract      `json:"contract"`
	Decisions    []goalwork.Decision        `json:"decisions"`
	Evidence     []goalwork.EvidenceRequest `json:"evidence"`
	ExpectedRows []goalwork.Row             `json:"expectedRows"`
}

type analysisSource struct {
	Reference    string `json:"reference"`
	PK           string `json:"pk"`
	SourceSHA256 string `json:"sourceSha256"`
	Records      []int  `json:"records"`
	Role         string `json:"role"`
}

func runAnalysisTrial(provider, kind string, c calibrationCase, prepared []preparedSource, runtime calibrationServices, repeat int, positive bool) (t trial) {
	t = trial{Kind: kind, Case: c.ID, Variant: "positive", Repeat: repeat, StartedAt: time.Now().UTC(), ExpectedReady: positive}
	defer func() { t.ElapsedSeconds = time.Since(t.StartedAt).Seconds() }()
	goal := c.Goal
	if !positive {
		goal, t.Variant = c.NegativeGoal, "stronger_goal_negative"
	}
	t.Result = goalwork.View{Goal: goal, Status: "not_run"}
	if len(prepared) == 0 || len(c.Analysis.ExpectedRows) == 0 {
		t.Error = "analysis needs frozen sources and independent expected rows"
		return
	}
	sources := map[string]preparedSource{}
	for _, p := range prepared {
		if p.err != nil {
			t.Error = p.err.Error()
			return
		}
		if _, exists := sources[p.source.PK]; exists {
			t.Error = "duplicate source PK in development fixture"
			return
		}
		sources[p.source.PK] = p
	}
	t.ReferenceObservedAt = prepared[0].source.ObservedAt
	deps := goalwork.Dependencies{
		Search: func(_ context.Context, pk string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: pk}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) { return sources[pk].inspection, nil },
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			p, ok := sources[s.PK]
			if !ok {
				return goalwork.Acquired{}, fmt.Errorf("unprepared reference")
			}
			return goalwork.Acquired{Delivery: "FILE", Rows: p.rows, ContentSHA256: p.source.ContentSHA256, Warnings: []string{"Development replay of independently retained reference records, not a new download. Original URL: " + p.source.URL, "Original observedAt: " + p.source.ObservedAt}}, nil
		},
		Review: func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			t.Input = &in
			response, err := runtime.Review(ctx, in, provider)
			t.ProviderResponse, t.ProviderResponseTruncated = response.RawResponse, response.Truncated
			return response.Assessment, err
		},
	}
	e, err := goalwork.Start(goal, goalwork.Policy{EvidenceRecipient: provider, ReviewRecipient: provider, ReviewAnalyses: true}, deps)
	if err != nil {
		t.Error = err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	step := func(d goalwork.Decision) bool {
		t.Result, err = e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(t.Result.Gaps) != 0 {
			t.Error = fmt.Sprintf("calibration: %v %v", err, t.Result.Gaps)
			return false
		}
		return true
	}
	if !step(goalwork.Decision{Action: "define", Contract: &c.Analysis.Contract}) {
		return
	}
	for _, source := range c.Analysis.Sources {
		for _, d := range []goalwork.Decision{{Action: "search", Query: source.PK, Role: source.Role}, {Action: "inspect", PK: source.PK}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: source.PK, Delivery: "file"}}} {
			if !step(d) {
				return
			}
		}
	}
	// Detach before filling current observation hashes; prior trial configuration
	// stays immutable, and the manifest cannot inject source values or approval.
	b, _ := json.Marshal(c.Analysis.Decisions)
	var decisions []goalwork.Decision
	if err := json.Unmarshal(b, &decisions); err != nil {
		t.Error = err.Error()
		return
	}
	for _, d := range decisions {
		if d.Action != "sample" && d.Action != "compose" && d.Action != "execute" {
			t.Error = "analysis calibration supports only local calculation decisions"
			return
		}
		if d.Action == "sample" {
			if d.Sample == nil || d.Sample.Reduce == nil {
				t.Error = "analysis sample must reduce an already-retained source"
				return
			}
			for _, o := range e.View().Observations {
				if o.ID == d.Sample.Reduce.Observation {
					d.Sample.Reduce.RowsSHA256 = o.RowsSHA256
				}
			}
		}
		if !step(d) {
			return
		}
	}
	want, _ := json.Marshal(c.Analysis.ExpectedRows)
	if t.Result.Artifact == nil {
		t.Error = "no actual calibration artifact"
		return
	}
	got, _ := json.Marshal(t.Result.Artifact.Rows)
	if string(got) != string(want) {
		t.Error = "calculated rows differ from independent frozen expectations; reviewer not called"
		return
	}
	for _, selection := range c.Analysis.Evidence {
		for _, o := range e.View().Observations {
			if o.ID == selection.Observation {
				selection.RowsSHA256 = o.RowsSHA256
			}
		}
		if !step(goalwork.Decision{Action: "read_evidence", Evidence: &selection}) {
			return
		}
	}
	if !step(goalwork.Decision{Action: "review_result", CompositionID: t.Result.Artifact.Recipe.ID}) {
		return
	}
	t.Matched = (t.Result.Status == "output_ready") == positive && t.Result.Evaluation.Review != nil
	return
}
