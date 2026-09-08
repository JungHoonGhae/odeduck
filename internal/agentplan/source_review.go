package agentplan

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

//go:embed source-review-guide.md
var sourceReviewGuide string

//go:embed analysis-review-guide.md
var analysisReviewGuide string

// GoalReviewResponse lets an explicit calibration retain provider output even
// when decoding fails. Product adapters pass ONLY Assessment to the Engine;
// RawResponse is untrusted and must never enter a goal, gap, or planning view.
type GoalReviewResponse struct {
	Assessment  goalwork.ReviewAssessment
	RawResponse string
	Truncated   bool
}

// ReviewGoal uses the fixed authorized provider in a new tool-free context. It
// sees neither planner history nor oracle labels. This is a model judgement,
// not an independent source of truth or a human review.
func ReviewGoal(ctx context.Context, in goalwork.ReviewInput, requested string) (GoalReviewResponse, error) {
	if in.Recipient == "" || in.Recipient != requested || (requested != ProviderClaude && requested != ProviderCodex && requested != ProviderGemini) {
		return GoalReviewResponse{}, fmt.Errorf("review recipient requires its exact authorized CLI provider; no fallback")
	}
	b, err := json.Marshal(in)
	if err != nil || len(b) > 96<<10 {
		return GoalReviewResponse{}, fmt.Errorf("bounded source review input required")
	}
	providers, _, err := resolveProviders(requested)
	if err != nil {
		return GoalReviewResponse{}, err
	}
	guide := sourceReviewGuide
	if in.Analysis != nil {
		if in.Analysis.Method != "engine_relational_replay_v2" {
			return GoalReviewResponse{}, fmt.Errorf("unsupported analysis replay contract")
		}
		guide = analysisReviewGuide
	}
	body, err := invokeProvider(ctx, guide+"\nREVIEW_INPUT_JSON:\n"+string(b), providers[0])
	response := GoalReviewResponse{}
	if len(body) > 1<<20 {
		response.RawResponse, response.Truncated = string(body[:1<<20]), true
		return response, fmt.Errorf("source review response exceeds 1 MiB; raw capture is truncated")
	}
	response.RawResponse = string(body)
	if err != nil {
		return response, fmt.Errorf("source review provider failed; no judgement recorded")
	}
	response.Assessment, err = decodeSourceReview(body, in)
	return response, err
}

func decodeSourceReview(body []byte, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
	if len(body) > 1<<20 {
		return goalwork.ReviewAssessment{}, fmt.Errorf("source review response exceeds 1 MiB")
	}
	var candidates [][]byte
	var walk func([]byte, int) error
	walk = func(data []byte, depth int) error {
		if depth > 12 {
			return fmt.Errorf("review response nesting limit exceeded")
		}
		data = bytes.TrimSpace(data)
		if !json.Valid(data) {
			return nil
		}
		if err := uniqueReviewKeys(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
			return err
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) != nil {
			return nil
		}
		if string(obj["is_error"]) == "true" || string(obj["type"]) == `"turn.failed"` || (len(obj["error"]) > 0 && string(obj["error"]) != "null") {
			return fmt.Errorf("review provider returned an error")
		}
		if _, ok := obj["goalFit"]; ok {
			candidates = append(candidates, data)
			return nil
		}
		for _, key := range []string{"structured_output", "result", "response", "text", "message", "item"} {
			value, ok := obj[key]
			if !ok {
				continue
			}
			var text string
			if json.Unmarshal(value, &text) == nil {
				text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(text), "```json"), "```"), "```"))
				value = []byte(text)
			}
			if err := walk(value, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if json.Valid(body) {
		if err := walk(body, 0); err != nil {
			return goalwork.ReviewAssessment{}, err
		}
	} else {
		for _, line := range bytes.Split(body, []byte("\n")) {
			if err := walk(line, 0); err != nil {
				return goalwork.ReviewAssessment{}, err
			}
		}
	}
	if len(candidates) != 1 {
		return goalwork.ReviewAssessment{}, fmt.Errorf("review requires exactly one typed assessment")
	}
	var a goalwork.ReviewAssessment
	d := json.NewDecoder(bytes.NewReader(candidates[0]))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil {
		return a, fmt.Errorf("review assessment schema mismatch")
	}
	return a, goalwork.ValidateReviewAssessment(a, in)
}

// Encoding/json accepts duplicate keys and case-insensitive struct aliases;
// neither may ambiguously overwrite an approval finding.
func uniqueReviewKeys(d *json.Decoder, depth int) error {
	if depth > 20 {
		return fmt.Errorf("review JSON depth exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	opening, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		if opening == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("invalid review JSON key")
			}
			name = reviewKeyFold(name)
			if seen[name] {
				return fmt.Errorf("duplicate review JSON key")
			}
			seen[name] = true
		}
		if err := uniqueReviewKeys(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	if err == io.EOF {
		return fmt.Errorf("unfinished review JSON")
	}
	return err
}

func reviewKeyFold(name string) string {
	var folded strings.Builder
	for _, r := range name {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		folded.WriteRune(minimum)
	}
	return folded.String()
}
