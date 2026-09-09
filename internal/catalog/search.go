package catalog

import (
	"context"
	"errors"
)

// SearchOptions controls retrieval policy independently of CLI/MCP presentation.
type SearchOptions struct {
	Semantic        bool
	RequireSemantic bool
}

// Searcher owns optional index loading, embedding and degraded-search policy.
// Its zero value loads the local index and uses its model with Ollama. Supplied
// dependencies allow an embedding adapter and an already loaded index to be reused.
// It does not cache disk state: subsequent requests can see a refreshed index.
type Searcher struct {
	Index    *SemanticIndex
	Embedder Embedder
}

// Search returns diagnostics even when required semantic retrieval fails. Callers
// must handle the error before publishing any candidates from the result.
func (s Searcher) Search(ctx context.Context, cat *Catalog, plan QueryPlan, options SearchOptions) (Result, error) {
	var result Result
	if !options.Semantic {
		result = cat.SearchPlan(plan)
	} else {
		index, embedder := s.Index, s.Embedder
		if index == nil {
			loaded, err := LoadSemanticIndex(cat)
			switch {
			case err == nil:
				index = loaded
			case errors.Is(err, ErrSemanticIndexNotBuilt):
				result = cat.SearchHybrid(ctx, plan, nil, nil)
			case errors.Is(err, ErrSemanticIndexStale):
				result = cat.SearchPlan(plan)
				recordSemanticOutcome(&result, SemanticInfo{Status: SemanticUnavailable, Detail: "카탈로그 갱신 후 semantic-build 가 필요함"})
			default:
				result = cat.SearchPlan(plan)
				recordSemanticOutcome(&result, SemanticInfo{Status: SemanticUnavailable, Detail: "의미 인덱스 로드 실패: " + err.Error()})
			}
		}
		if index != nil {
			if embedder == nil {
				embedder = NewOllamaEmbedder(OllamaURLFromEnv(), index.Model)
			}
			result = cat.SearchHybrid(ctx, plan, index, embedder)
		}
	}
	if options.RequireSemantic {
		return result, requireSemantic(result)
	}
	return result, nil
}
