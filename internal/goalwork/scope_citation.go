package goalwork

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ScopeCitation proposes an interpretation of a retained source declaration.
// It cannot supply a source URL, an invented constant or a verification result.
type ScopeCitation struct {
	Observation string `json:"observation"`
	Field       string `json:"field"` // description or spatialCoverage
	Quote       string `json:"quote"`
}

// CitedScopeClaim proves only where the quoted text occurred. A quote may be
// negated, partial, historical or about a different subject; meaning is unverified.
type CitedScopeClaim struct {
	ClaimSHA256          string        `json:"claimSha256,omitempty"`
	Status               string        `json:"status"` // always proposed_scope
	Citation             ScopeCitation `json:"citation"`
	PK                   string        `json:"pk"`
	SourceURL            string        `json:"sourceUrl,omitempty"`
	ObservedAt           string        `json:"observedAt"`
	ObservationSHA256    string        `json:"observationSha256"`
	DeclarationSHA256    string        `json:"declarationSha256"`
	EvidenceSHA256       string        `json:"evidenceSha256"`
	FirstMatchByte       int           `json:"firstMatchByte"`
	Occurrences          int           `json:"occurrences"`
	DeclarationTruncated bool          `json:"declarationTruncated"`
}

type scopeSources struct {
	observations map[string]Observation
	left         map[string]bool
	right        string
}

func prepareScopeSide(rows []Row, parts []string, citation *ScopeCitation, sources scopeSources, left bool) ([][]string, *CitedScopeClaim, error) {
	if citation == nil {
		tokens, err := scopeTokens(rows, parts)
		return tokens, nil, err
	}
	if len(parts) > 0 {
		return nil, nil, fmt.Errorf("scope side selects either observed field parts or one source citation, never both")
	}
	allowed := citation.Observation == sources.right
	if left {
		allowed = sources.left[citation.Observation]
	}
	observation, exists := sources.observations[citation.Observation]
	if !allowed || !exists {
		return nil, nil, fmt.Errorf("scope citation must belong to an observation on this join side")
	}
	declaration := observation.Declaration
	if declaration == nil || declaration.Status != "publisher_declared" {
		return nil, nil, fmt.Errorf("scope citation requires the sampled observation's retained publisher declaration")
	}
	var evidence string
	switch citation.Field {
	case "description":
		evidence = declaration.Description
	case "spatialCoverage":
		evidence = declaration.SpatialCoverage
	default:
		return nil, nil, fmt.Errorf("scope citation may reference description or spatialCoverage only; titles, provider identity and catalogue dates are not record scope")
	}
	if !utf8.ValidString(citation.Quote) || strings.TrimSpace(citation.Quote) == "" || len(citation.Quote) > 256 {
		return nil, nil, fmt.Errorf("scope citation quote needs 1–256 UTF-8 bytes of nonblank source text")
	}
	offset := strings.Index(evidence, citation.Quote)
	if offset < 0 {
		return nil, nil, fmt.Errorf("scope quote does not occur verbatim in the sampled observation's declaration field")
	}
	words := strings.Fields(citation.Quote)
	if len(words) > 64 {
		return nil, nil, fmt.Errorf("scope citation exceeds 64 text tokens")
	}
	claim := &CitedScopeClaim{Status: "proposed_scope", Citation: *citation, PK: observation.PK, SourceURL: declaration.SourceURL, ObservedAt: observation.ObservedAt, ObservationSHA256: digest(observation), DeclarationSHA256: digest(declaration), EvidenceSHA256: digest(evidence), FirstMatchByte: offset, Occurrences: strings.Count(evidence, citation.Quote), DeclarationTruncated: declaration.Truncated}
	claim.ClaimSHA256 = digest(claim)
	tokens := make([][]string, len(rows))
	for i := range rows {
		tokens[i] = words
	}
	return tokens, claim, nil
}
