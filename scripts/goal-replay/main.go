// goal-replay re-acquires a previously discovered goal's exact sources and
// replays its last composition with explicit caller context. This is a development
// diagnostic, not autonomous discovery or resumption of the original session.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/JungHoonGhae/odeduck/internal/providerauth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input", "", "previous actual goal JSON")
	userContext := flag.String("context", "", "relevant user conversation, not source evidence")
	provider := flag.String("review-with", "", "explicit fixed reviewer; selected source values are sent")
	output := flag.String("output", "", "new diagnostic JSON file")
	flag.Parse()
	if *input == "" || *output == "" || (*provider != "codex" && *provider != "claude" && *provider != "gemini") {
		return fmt.Errorf("input, output and explicit review-with required")
	}
	body, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var old goalwork.View
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&old); err != nil {
		return err
	}
	if old.Contract == nil || old.Artifact == nil || len(old.Compositions) == 0 {
		return fmt.Errorf("replay requires retained contract and artifact")
	}
	f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	policy := goalwork.Policy{RequireSemantic: old.Policy.RequireSemantic, MaxRounds: 64, EvidenceRecipient: *provider, ReviewRecipient: *provider, ReviewAnalyses: true, ReviewFullScope: true}
	client := fetch.New()
	deps := goalwork.LiveDependencies(client, "", apicall.NewDatasetCaller(client, "", providerauth.Source{}), catalog.Searcher{}, policy)
	var reviewInput *goalwork.ReviewInput
	var rawReview string
	deps.Review = func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		reviewInput = &in
		r, err := agentplan.ReviewGoal(ctx, in, *provider)
		b, _ := json.Marshal(r)
		rawReview = string(b)
		return r.Assessment, err
	}
	e, err := goalwork.StartRequest(goalwork.Request{Goal: old.Goal, Context: *userContext}, policy, deps)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	advance := func(d goalwork.Decision) error {
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			return err
		}
		if n := len(v.Gaps); n > 0 && v.Gaps[n-1].Revision == v.Revision {
			return fmt.Errorf("%s: %s", d.Action, v.Gaps[n-1].Detail)
		}
		return nil
	}
	perform := func() error {
		if err := advance(goalwork.Decision{Action: "define", Contract: old.Contract}); err != nil {
			return err
		}
		for _, s := range old.Searches {
			if err := advance(goalwork.Decision{Action: "search", Query: s.Query, Role: s.Role}); err != nil {
				return err
			}
		}
		inspected := map[string]bool{}
		for _, a := range old.SampleAttempts {
			if a.Status != "acquired" {
				continue
			}
			if !inspected[a.Request.PK] {
				if err := advance(goalwork.Decision{Action: "inspect", PK: a.Request.PK}); err != nil {
					return err
				}
				inspected[a.Request.PK] = true
			}
			req := a.Request
			if err := advance(goalwork.Decision{Action: "sample", Sample: &req}); err != nil {
				return err
			}
			current := e.View().Observations
			got := current[len(current)-1]
			matched := false
			for _, want := range old.Observations {
				if want.ID == a.ObservationID {
					matched = got.ID == want.ID && got.RowsSHA256 == want.RowsSHA256 && got.ContentSHA256 == want.ContentSHA256
					break
				}
			}
			if !matched {
				return fmt.Errorf("source changed; no substitution accepted: %s", a.ObservationID)
			}
		}
		ids := map[string]string{}
		for _, p := range old.Evidence {
			selection := p.Selection
			if err := advance(goalwork.Decision{Action: "read_evidence", Evidence: &selection}); err != nil {
				return err
			}
			packets := e.View().Evidence
			ids[p.ID] = packets[len(packets)-1].ID
		}
		// Remap only engine-issued packet IDs, never source cells or prior judgements.
		recipeJSON, _ := json.Marshal(old.Artifact.Recipe)
		text := string(recipeJSON)
		for from, to := range ids {
			text = strings.ReplaceAll(text, from, to)
		}
		var recipe goalwork.Composition
		if err := json.Unmarshal([]byte(text), &recipe); err != nil {
			return err
		}
		recipe.ID += "_context_replay"
		if err := advance(goalwork.Decision{Action: "compose", Composition: &recipe}); err != nil {
			return err
		}
		if err := advance(goalwork.Decision{Action: "execute", CompositionID: recipe.ID}); err != nil {
			return err
		}
		return advance(goalwork.Decision{Action: "review_result", CompositionID: recipe.ID})
	}
	runErr := perform()
	result := struct {
		Kind           string                `json:"kind"`
		OriginalStatus string                `json:"originalStatus"`
		Input          *goalwork.ReviewInput `json:"input,omitempty"`
		Result         goalwork.View         `json:"result"`
		ReviewResponse string                `json:"reviewResponse,omitempty"`
		Error          string                `json:"error,omitempty"`
	}{Kind: "explicit_context_source_replay_not_autonomous_discovery", OriginalStatus: old.Status, Input: reviewInput, Result: e.View(), ReviewResponse: rawReview}
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if err := json.NewEncoder(f).Encode(result); err != nil {
		return err
	}
	return runErr
}
