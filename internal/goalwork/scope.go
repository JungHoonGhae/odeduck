package goalwork

import (
	"fmt"
	"strings"
)

// JoinScope compares row text or explicitly cited source-scope hypotheses after
// key equality, before expansion. It does not certify geographic/field meaning.
type JoinScope struct {
	LeftParts     []string       `json:"leftParts,omitempty"`  // qualified fields from the accumulated left relation
	RightParts    []string       `json:"rightParts,omitempty"` // original fields from the new right observation
	LeftCitation  *ScopeCitation `json:"leftCitation,omitempty"`
	RightCitation *ScopeCitation `json:"rightCitation,omitempty"`
	Rule          string         `json:"rule"`                 // equal_tokens_v1, left_prefix_v1, right_prefix_v1
	Vocabulary    string         `json:"vocabulary,omitempty"` // explicit published first-token label correspondence
}

// ScopeCheck counts candidate pairs, not unique records or verified entities.
// Counts across multiple checks can overlap; every configured check must pass.
type ScopeCheck struct {
	Binding         JoinScope                `json:"binding"`
	LeftClaim       *CitedScopeClaim         `json:"leftClaim,omitempty"`
	RightClaim      *CitedScopeClaim         `json:"rightClaim,omitempty"`
	Vocabulary      *ScopeVocabularyEvidence `json:"vocabulary,omitempty"`
	MeaningVerified bool                     `json:"meaningVerified"`
	CandidatePairs  int                      `json:"candidatePairs"`
	MatchedPairs    int                      `json:"matchedPairs"`
	ConflictPairs   int                      `json:"conflictPairs"`
	UnknownPairs    int                      `json:"unknownPairs"`
}

type preparedScope struct {
	rule        string
	left, right [][]string
}
type scopeContext []preparedScope

func prepareScopes(specs []JoinScope, left, right []Row, sources scopeSources) (scopeContext, []ScopeCheck, error) {
	if len(specs) > 4 {
		return nil, nil, fmt.Errorf("at most 4 row-bound scope checks per join")
	}
	var prepared scopeContext
	var checks []ScopeCheck
	for _, spec := range specs {
		if spec.Rule != "equal_tokens_v1" && spec.Rule != "left_prefix_v1" && spec.Rule != "right_prefix_v1" {
			return nil, nil, fmt.Errorf("unsupported row-bound scope rule")
		}
		l, leftClaim, err := prepareScopeSide(left, spec.LeftParts, spec.LeftCitation, sources, true)
		if err != nil {
			return nil, nil, err
		}
		r, rightClaim, err := prepareScopeSide(right, spec.RightParts, spec.RightCitation, sources, false)
		if err != nil {
			return nil, nil, err
		}
		var vocabulary *ScopeVocabularyEvidence
		if spec.Vocabulary != "" {
			if spec.LeftCitation != nil || spec.RightCitation != nil {
				return nil, nil, fmt.Errorf("scope vocabulary requires row-bound fields on both sides, not source-scope citations")
			}
			labels, evidence, err := scopeVocabulary(spec.Vocabulary)
			if err != nil {
				return nil, nil, err
			}
			l, r = normalizeScopeTokens(l, labels), normalizeScopeTokens(r, labels)
			vocabulary = evidence
		}
		prepared = append(prepared, preparedScope{rule: spec.Rule, left: l, right: r})
		checks = append(checks, ScopeCheck{Binding: spec, LeftClaim: leftClaim, RightClaim: rightClaim, Vocabulary: vocabulary})
	}
	return prepared, checks, nil
}

func scopeTokens(rows []Row, parts []string) ([][]string, error) {
	if len(parts) < 1 || len(parts) > 4 {
		return nil, fmt.Errorf("scope needs 1–4 observed text fields on each side")
	}
	seen := map[string]bool{}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || seen[part] {
			return nil, fmt.Errorf("scope fields must be nonblank and distinct")
		}
		seen[part] = true
	}
	if err := requireFields(rows, parts); err != nil {
		return nil, err
	}
	out := make([][]string, len(rows))
	for i, row := range rows {
		known := true
		var tokens []string
		for _, part := range parts {
			v := row[part]
			if v == nil {
				known = false
				continue
			}
			s, ok := v.(string)
			if !ok || len(s) > 2048 {
				return nil, fmt.Errorf("scope field %q requires text of at most 2048 UTF-8 bytes; no value coercion", part)
			}
			words := strings.Fields(s)
			if len(words) == 0 {
				known = false
			}
			tokens = append(tokens, words...)
			if len(tokens) > 64 {
				return nil, fmt.Errorf("scope side exceeds 64 text tokens")
			}
		}
		if known {
			out[i] = tokens
		}
	}
	return out, nil
}

func (s scopeContext) match(left, right int, checks []ScopeCheck) bool {
	pass := true
	for i, scope := range s {
		l, r := scope.left[left], scope.right[right]
		known := len(l) > 0 && len(r) > 0
		if scope.rule == "right_prefix_v1" {
			l, r = r, l
		}
		match := known && len(l) <= len(r)
		if scope.rule == "equal_tokens_v1" {
			match = match && len(l) == len(r)
		}
		if match {
			for j := range l {
				if l[j] != r[j] {
					match = false
					break
				}
			}
		}
		if checks != nil {
			checks[i].CandidatePairs++
			switch {
			case !known:
				checks[i].UnknownPairs++
			case match:
				checks[i].MatchedPairs++
			default:
				checks[i].ConflictPairs++
			}
		}
		pass = pass && match
	}
	return pass
}
