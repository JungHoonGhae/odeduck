package goalwork

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// This is a frozen reference vocabulary, not a runtime SGIS data adapter or a
// numeric-code crosswalk. Revisions coexist; the caller selects one explicitly.
//
//go:embed vocabularies/kr-sido-labels-20260907.json
var sidoVocabularyJSON []byte

type VocabularySource struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Bytes    int    `json:"bytes"`
	Selector string `json:"selector"`
}

// ScopeVocabularyEvidence certifies which published label table was used, not
// the geographic meaning of the selected row fields or any facility identity.
type ScopeVocabularyEvidence struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	SHA256      string             `json:"sha256"`
	ObservedAt  string             `json:"observedAt"`
	Sources     []VocabularySource `json:"sources"`
	Limitations []string           `json:"limitations"`
}

func scopeVocabulary(id string) (map[string]string, *ScopeVocabularyEvidence, error) {
	if id != "kr_sido_labels_20260907_v1" {
		return nil, nil, fmt.Errorf("unknown scope vocabulary; supported revision: kr_sido_labels_20260907_v1")
	}
	var data struct {
		ID         string             `json:"id"`
		Kind       string             `json:"kind"`
		ObservedAt string             `json:"observedAt"`
		Sources    []VocabularySource `json:"sources"`
		Labels     []struct {
			Code  string `json:"sourceCode"`
			Full  string `json:"full"`
			Short string `json:"short"`
		} `json:"labels"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal(sidoVocabularyJSON, &data); err != nil || data.ID != id || len(data.Labels) != 17 || len(data.Sources) != 2 {
		return nil, nil, fmt.Errorf("invalid embedded scope vocabulary")
	}
	labels := map[string]string{}
	codes := map[string]bool{}
	for _, entry := range data.Labels {
		if entry.Full == "" || entry.Short == "" || entry.Code == "" || codes[entry.Code] {
			return nil, nil, fmt.Errorf("invalid or duplicate published label correspondence")
		}
		codes[entry.Code] = true
		for _, label := range []string{entry.Full, entry.Short} {
			if prior, exists := labels[label]; exists && prior != entry.Full {
				return nil, nil, fmt.Errorf("ambiguous published label correspondence")
			}
			labels[label] = entry.Full
		}
	}
	sha := sha256.Sum256(sidoVocabularyJSON)
	evidence := &ScopeVocabularyEvidence{ID: data.ID, Kind: data.Kind, SHA256: hex.EncodeToString(sha[:]), ObservedAt: data.ObservedAt, Sources: data.Sources, Limitations: data.Limitations}
	return labels, evidence, nil
}

func normalizeScopeTokens(rows [][]string, labels map[string]string) [][]string {
	out := make([][]string, len(rows))
	for i, tokens := range rows {
		if len(tokens) == 0 {
			continue
		}
		label, known := labels[tokens[0]]
		if !known {
			continue // Unknown vocabulary membership is not geographic conflict.
		}
		out[i] = append([]string(nil), tokens...)
		out[i][0] = label // Only the comparison token; original fields stay unchanged.
	}
	return out
}
