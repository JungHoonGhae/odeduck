package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeEmbedder struct {
	model string
	fn    func(string) []float32
}

func (f fakeEmbedder) Model() string { return f.model }
func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = f.fn(text)
	}
	return out, nil
}

func TestOllamaEmbedderUsesBatchAPI(t *testing.T) {
	var got struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": got.Model, "embeddings": [][]float32{{1, 0}, {0, 1}},
		})
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "embedding-test")
	vectors, err := e.Embed(context.Background(), []string{"하나", "둘"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if got.Model != "embedding-test" || len(got.Input) != 2 || len(vectors) != 2 {
		t.Fatalf("request=%+v vectors=%v", got, vectors)
	}
}

func TestSemanticIndexRoundTripAndSearch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "auction", Title: "온비드 부동산", Org: "한국자산관리공사", SvcType: SvcREST, Desc: "공공자산 공매 물건"},
		{PK: "weather", Title: "동네 날씨", Org: "기상청", SvcType: SvcREST, Desc: "기온 강수량"},
	}}
	embedder := fakeEmbedder{model: "fake-ko", fn: func(text string) []float32 {
		if strings.Contains(text, "공매") || strings.Contains(text, "싸게 살") {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}}
	idx, err := BuildSemanticIndex(context.Background(), c, embedder, 2, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := idx.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadSemanticIndex(c)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	query, _ := embedder.Embed(context.Background(), []string{semanticQuery("시세보다 싸게 살 물건")})
	hits := loaded.Search(c, query[0], 2, true, true)
	if len(hits) != 2 || hits[0].PK != "auction" || hits[0].SemanticScore <= hits[1].SemanticScore {
		t.Fatalf("semantic hits = %+v", hits)
	}
	if hits[0].Preview == "" {
		t.Fatal("semantic exploration should be able to include a compact description preview")
	}
}

func TestSemanticIndexRejectsChangedCatalog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{{PK: "1", Title: "원본"}}}
	e := fakeEmbedder{model: "fake", fn: func(string) []float32 { return []float32{1} }}
	idx, err := BuildSemanticIndex(context.Background(), c, e, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Save(); err != nil {
		t.Fatal(err)
	}
	c.Entries[0].Title = "변경"
	if _, err := LoadSemanticIndex(c); err != ErrSemanticIndexStale {
		t.Fatalf("changed catalog error = %v, want ErrSemanticIndexStale", err)
	}
}

func TestHybridSearchFindsMeaningWithoutLiteralKeywords(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "auction", Title: "온비드 부동산", SvcType: SvcREST, Desc: "공공자산 공매 물건"},
		{PK: "weather", Title: "동네 날씨", SvcType: SvcREST, Desc: "기온 강수량"},
	}}
	embedder := fakeEmbedder{model: "fake-ko", fn: func(text string) []float32 {
		if strings.Contains(text, "공매") || strings.Contains(text, "싸게 살") || strings.Contains(text, "투자 기회") {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}}
	idx, err := BuildSemanticIndex(context.Background(), c, embedder, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := c.SearchHybrid(context.Background(), QueryPlan{
		Intent: "시세보다 싸게 살 수 있는 투자 기회", Limit: 2, RESTOnly: true,
	}, idx, embedder)
	if r.Mode != SearchModeHybrid || r.Semantic == nil || r.Semantic.Status != SemanticUsed {
		t.Fatalf("hybrid metadata = %+v", r)
	}
	if len(r.Hits) == 0 || r.Hits[0].PK != "auction" {
		t.Fatalf("hybrid hits = %+v, want semantic auction match first", r.Hits)
	}
}

func TestHybridSearchKeepsInferredAxesDiverse(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "auction-1", Title: "온비드 공매 물건", SvcType: SvcREST},
		{PK: "auction-2", Title: "온비드 공매 상세", SvcType: SvcREST},
		{PK: "bid", Title: "나라장터 입찰 공고", SvcType: SvcREST},
		{PK: "market", Title: "도매시장 경매 가격", SvcType: SvcREST},
	}}
	embedder := fakeEmbedder{model: "fake", fn: func(text string) []float32 {
		switch {
		case strings.Contains(text, "온비드") || strings.Contains(text, "공매"):
			return []float32{1, 0, 0}
		case strings.Contains(text, "나라장터") || strings.Contains(text, "입찰"):
			return []float32{0, 1, 0}
		default:
			return []float32{0, 0, 1}
		}
	}}
	idx, err := BuildSemanticIndex(context.Background(), c, embedder, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := c.SearchHybrid(context.Background(), QueryPlan{
		Intent: "돈 될 만한 것", Concepts: []string{"온비드 공매", "나라장터 입찰", "도매시장 경매"},
		Limit: 3, RESTOnly: true,
	}, idx, embedder)
	if len(r.Hits) != 3 {
		t.Fatalf("hits = %+v", r.Hits)
	}
	seen := map[string]bool{}
	for _, hit := range r.Hits {
		seen[hit.MatchedQuery] = true
	}
	if len(seen) != 3 {
		t.Fatalf("first page should cover all inferred axes, got %+v", r.Hits)
	}
}

func TestHybridSearchProtectsOneExactLexicalHitPerAxis(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "exact", Title: "중소기업 지원사업 공고", SvcType: SvcREST, ApplyCount: 2},
		{PK: "similar", Title: "중소기업 수혜기업 연구과제 이력", SvcType: SvcREST, ApplyCount: 9000},
	}}
	embedder := fakeEmbedder{model: "fake", fn: func(text string) []float32 {
		if strings.HasPrefix(text, "task:") {
			return []float32{1, 0}
		}
		if strings.Contains(text, "수혜기업") {
			return []float32{1, 0}
		}
		if strings.Contains(text, "지원사업 공고") {
			return []float32{0.6, 0.8}
		}
		return []float32{1, 0}
	}}
	idx, err := BuildSemanticIndex(context.Background(), c, embedder, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := c.SearchHybrid(context.Background(), QueryPlan{
		Intent: "우리 회사가 받을 수 있는 돈", Concepts: []string{"중소기업 지원사업 공고"},
		Limit: 2, RESTOnly: true,
	}, idx, embedder)
	if len(r.Hits) != 2 || r.Hits[0].PK != "exact" || !r.Hits[0].Exact {
		t.Fatalf("hybrid hits = %+v, want exact actionable result protected first", r.Hits)
	}
}
