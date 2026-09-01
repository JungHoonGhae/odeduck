package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed testdata/release-golden.json
var releaseGoldenJSON []byte

type ReleaseGoldenSet struct {
	Version    int                 `json:"version"`
	ReviewedAt string              `json:"reviewedAt"`
	Cases      []ReleaseGoldenCase `json:"cases"`
}

type ReleaseGoldenCase struct {
	ID              string `json:"id"`
	Query           string `json:"query"`
	ExpectedPK      string `json:"expectedPk"`
	ExpectedSvcType string `json:"expectedSvcType"`
	MaxRank         int    `json:"maxRank"`
}

type ReleaseQualityCase struct {
	ID         string `json:"id"`
	Query      string `json:"query"`
	ExpectedPK string `json:"expectedPk"`
	Rank       int    `json:"rank,omitempty"`
	Passed     bool   `json:"passed"`
	Failure    string `json:"failure,omitempty"`
}

type ReleaseQualityReport struct {
	GoldenVersion int                  `json:"goldenVersion"`
	ReviewedAt    string               `json:"reviewedAt"`
	Passed        bool                 `json:"passed"`
	Cases         []ReleaseQualityCase `json:"cases"`
}

func DefaultReleaseGoldenSet() (ReleaseGoldenSet, error) {
	var golden ReleaseGoldenSet
	if err := json.Unmarshal(releaseGoldenJSON, &golden); err != nil {
		return ReleaseGoldenSet{}, fmt.Errorf("release golden set 해석 실패: %w", err)
	}
	if golden.Version <= 0 || golden.ReviewedAt == "" || len(golden.Cases) == 0 {
		return ReleaseGoldenSet{}, fmt.Errorf("release golden set metadata가 비어 있습니다")
	}
	return golden, nil
}

// ValidateReleaseQuality protects representative user searches from a
// catalogue/parser regression. It is deliberately deterministic and lexical:
// release runners do not need Ollama, while the optional semantic layer keeps a
// separate evaluation lifecycle.
func ValidateReleaseQuality(candidate *Catalog, golden ReleaseGoldenSet) ReleaseQualityReport {
	report := ReleaseQualityReport{GoldenVersion: golden.Version, ReviewedAt: golden.ReviewedAt, Passed: true}
	for _, expected := range golden.Cases {
		result := ReleaseQualityCase{ID: expected.ID, Query: expected.Query, ExpectedPK: expected.ExpectedPK}
		limit := expected.MaxRank
		if limit <= 0 {
			limit = 10
		}
		search := candidate.Search(expected.Query, limit, false)
		for index, hit := range search.Hits {
			if hit.PK != expected.ExpectedPK {
				continue
			}
			result.Rank = index + 1
			if expected.ExpectedSvcType != "" && hit.SvcType != expected.ExpectedSvcType {
				result.Failure = fmt.Sprintf("svcType=%s, want %s", hit.SvcType, expected.ExpectedSvcType)
			} else {
				result.Passed = true
			}
			break
		}
		if result.Rank == 0 {
			result.Failure = fmt.Sprintf("expected pk %s not found in top %d", expected.ExpectedPK, limit)
		}
		if !result.Passed {
			report.Passed = false
		}
		report.Cases = append(report.Cases, result)
	}
	return report
}
