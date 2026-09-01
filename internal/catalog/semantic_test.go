package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type fakeEmbedder struct {
	model string
	fn    func(string) []float32
	err   error
}

type recordingEmbedder struct {
	model  string
	inputs []string
}

type fixedVectorsEmbedder struct {
	model   string
	vectors [][]float32
}

func (e fixedVectorsEmbedder) Model() string { return e.model }
func (e fixedVectorsEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return e.vectors, nil
}

func (e *recordingEmbedder) Model() string { return e.model }
func (e *recordingEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.inputs = append(e.inputs, texts...)
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = []float32{float32(len(text)), 1}
	}
	return vectors, nil
}

func (f fakeEmbedder) Model() string { return f.model }
func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = f.fn(text)
	}
	return out, nil
}

func TestHybridSearchPropagatesStructuredAxisContractToSemanticHit(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "온비드 공매 물건", SvcType: SvcREST},
		{PK: "bridge", Title: "지역 점포 개폐업 이력", SvcType: SvcREST},
	}}
	embedder := fakeEmbedder{model: "fake", fn: func(text string) []float32 {
		if strings.Contains(text, "상권 생존 신호") || strings.Contains(text, "점포 개폐업") {
			return []float32{1, 0}
		}
		return []float32{0, 1}
	}}
	idx, err := BuildSemanticIndex(context.Background(), c, embedder, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	edge := EdgeHypothesis{Kinds: []string{"spatial", "temporal"}, ExpectedKeys: []string{"법정동코드", "기준연월"}}
	plan := QueryPlan{
		Intent: "공매 투자 판단",
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "온비드 공매 물건"},
			{Role: "상권 수요", Query: "상권 생존 신호", Contribution: "쇠퇴와 가격 기회를 구분", Edge: edge},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{{
			PK: "bridge", WhyCandidate: "점포 개폐업 이력 제목 근거",
		}},
		Limit: 2, RESTOnly: true,
	}
	r := c.SearchHybrid(context.Background(), plan, idx, embedder)
	if len(r.ConnectionOptions) != 1 || len(r.ConnectionOptions[0].Nodes) == 0 || r.ConnectionOptions[0].Nodes[0].PK != "bridge" {
		t.Fatalf("semantic-only bridge missing from option pool: %+v", r.ConnectionOptions)
	}
	if len(r.Connections) != 1 {
		t.Fatalf("connections = %+v; hits=%+v", r.Connections, r.Hits)
	}
	got := r.Connections[0]
	if got.BridgeRole != "상권 수요" || got.IncrementalValue != "쇠퇴와 가격 기회를 구분" ||
		len(got.Edge.ExpectedKeys) != 2 {
		t.Fatalf("semantic hit lost structured contract: %+v", got)
	}
}

func TestHybridSearchDoesNotRestoreRejectedDuplicateRoleAxis(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "anchor term", SvcType: SvcREST},
		{PK: "first", Title: "first term", SvcType: SvcREST},
		{PK: "duplicate", Title: "duplicate term", SvcType: SvcREST},
	}}
	embedder := fakeEmbedder{model: "fake", fn: func(text string) []float32 {
		switch {
		case strings.Contains(text, "anchor"):
			return []float32{1, 0, 0}
		case strings.Contains(text, "first"):
			return []float32{0, 1, 0}
		default:
			return []float32{0, 0, 1}
		}
	}}
	idx, err := BuildSemanticIndex(context.Background(), c, embedder, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	edge := EdgeHypothesis{Kinds: []string{"entity"}, ExpectedKeys: []string{"id"}}
	result := c.SearchHybrid(context.Background(), QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "anchor term"},
			{Role: "risk", Query: "first term", Contribution: "first contribution", Edge: edge},
			{Role: "risk", Query: "duplicate term", Contribution: "duplicate contribution", Edge: edge},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{{
			PK: "duplicate", WhyCandidate: "duplicate evidence",
		}},
		Limit: 3, RESTOnly: true,
	}, idx, embedder)
	if len(result.Connections) != 0 {
		t.Fatalf("semantic retrieval restored a rejected duplicate-role axis: %+v", result.Connections)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("duplicate-role normalization warning missing: %+v", result)
	}
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

func TestOllamaURLFromEnvPrefersOpenDataCTLAndFallsBackToLegacy(t *testing.T) {
	t.Setenv("OPENDATACTL_OLLAMA_URL", "")
	t.Setenv("GONGCTL_OLLAMA_URL", "http://legacy.example")
	if got := OllamaURLFromEnv(); got != "http://legacy.example" {
		t.Fatalf("legacy Ollama URL = %q", got)
	}

	t.Setenv("OPENDATACTL_OLLAMA_URL", "http://current.example")
	if got := OllamaURLFromEnv(); got != "http://current.example" {
		t.Fatalf("current Ollama URL = %q", got)
	}
}

func TestOllamaURLFromEnvUsesDefaultWhenNeitherVariableIsSet(t *testing.T) {
	t.Setenv("OPENDATACTL_OLLAMA_URL", "")
	t.Setenv("GONGCTL_OLLAMA_URL", "")

	embedder := NewOllamaEmbedder(OllamaURLFromEnv(), "")
	if embedder.baseURL != DefaultOllamaURL {
		t.Fatalf("Ollama base URL = %q, want %q", embedder.baseURL, DefaultOllamaURL)
	}
	if embedder.model != DefaultEmbeddingModel {
		t.Fatalf("Ollama model = %q, want %q", embedder.model, DefaultEmbeddingModel)
	}
}

func TestSemanticIndexRoundTripAndSearch(t *testing.T) {
	isolateConfigHome(t)
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

func TestSemanticTopKMatchesExhaustiveOrdering(t *testing.T) {
	const rows = 257
	c := &Catalog{Entries: make([]Entry, rows)}
	idx := &SemanticIndex{PKs: make([]string, rows), Vectors: make([][]float32, rows)}
	for i := 0; i < rows; i++ {
		pk := fmt.Sprintf("%03d", i)
		c.Entries[i] = Entry{PK: pk, Title: "후보", SvcType: SvcREST, ApplyCount: i % 9, ViewCount: i % 17}
		idx.PKs[i] = pk
		idx.Vectors[i] = []float32{float32((i*37)%101 - 50), float32((i*19)%67 - 33), float32(i%7 - 3)}
		normalize(idx.Vectors[i])
	}
	query := []float32{0.7, -0.2, 0.5}
	got := idx.Search(c, append([]float32(nil), query...), 31, false, false)

	type expectedRow struct {
		entry *Entry
		score float32
	}
	want := make([]expectedRow, rows)
	normalize(query)
	for i := range c.Entries {
		want[i] = expectedRow{entry: &c.Entries[i], score: dot(query, idx.Vectors[i])}
	}
	sort.SliceStable(want, func(i, j int) bool {
		if want[i].score != want[j].score {
			return want[i].score > want[j].score
		}
		return moreDemandedEntry(want[i].entry, want[j].entry)
	})
	for i := range got {
		if got[i].PK != want[i].entry.PK || got[i].SemanticScore != want[i].score {
			t.Fatalf("rank %d got=%+v want=%s score=%f", i, got[i], want[i].entry.PK, want[i].score)
		}
	}
}

func TestSemanticIndexRejectsChangedCatalog(t *testing.T) {
	isolateConfigHome(t)
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

func TestRefreshSemanticIndexReusesOnlyUnchangedDocuments(t *testing.T) {
	isolateConfigHome(t)
	initial := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "keep", Title: "같은 제목", Desc: "같은 설명", ApplyCount: 1},
		{PK: "change", Title: "바뀔 제목", Desc: "이전 설명"},
		{PK: "remove", Title: "삭제될 데이터"},
	}}
	firstEmbedder := &recordingEmbedder{model: "incremental-test"}
	first, err := BuildSemanticIndex(context.Background(), initial, firstEmbedder, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}

	updated := &Catalog{SyncedAt: initial.SyncedAt.Add(time.Hour), Entries: []Entry{
		{PK: "keep", Title: "같은 제목", Desc: "같은 설명", ApplyCount: 999, ViewCount: 1234},
		{PK: "change", Title: "바뀔 제목", Desc: "새 설명"},
		{PK: "new", Title: "새 데이터"},
	}}
	secondEmbedder := &recordingEmbedder{model: "incremental-test"}
	refreshed, stats, err := RefreshSemanticIndex(context.Background(), updated, secondEmbedder, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 3 || stats.Reused != 1 || stats.Embedded != 2 {
		t.Fatalf("stats = %+v", stats)
	}
	if len(secondEmbedder.inputs) != 2 {
		t.Fatalf("embedded inputs = %d, want 2", len(secondEmbedder.inputs))
	}
	if refreshed.Version != semanticIndexVersion || refreshed.RecipeVersion != semanticRecipeVersion ||
		refreshed.Dimensions != 2 || len(refreshed.DocumentHashes) != 3 {
		t.Fatalf("refreshed index metadata = %+v", refreshed)
	}
	if refreshed.PKs[0] != "keep" || refreshed.PKs[2] != "new" {
		t.Fatalf("refreshed PKs = %v", refreshed.PKs)
	}
	if err := refreshed.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSemanticIndex(updated); err != nil {
		t.Fatalf("load refreshed index: %v", err)
	}
}

func TestRefreshSemanticIndexMigratesExactVersionOneIndexWithoutEmbedding(t *testing.T) {
	isolateConfigHome(t)
	cat := &Catalog{SyncedAt: time.Now(), Entries: []Entry{{PK: "legacy", Title: "기존 데이터"}}}
	legacy := &SemanticIndex{
		Version: 1, Model: "legacy-test", CatalogDigest: catalogDigest(cat), CreatedAt: time.Now(),
		PKs: []string{"legacy"}, Vectors: [][]float32{{1, 0}},
	}
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}
	embedder := &recordingEmbedder{model: "legacy-test"}
	refreshed, stats, err := RefreshSemanticIndex(context.Background(), cat, embedder, 32, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reused != 1 || stats.Embedded != 0 || len(embedder.inputs) != 0 {
		t.Fatalf("stats=%+v inputs=%d", stats, len(embedder.inputs))
	}
	if refreshed.Version != semanticIndexVersion || len(refreshed.DocumentHashes) != 1 || refreshed.DocumentHashes[0] == "" {
		t.Fatalf("migrated index = %+v", refreshed)
	}
}

func TestRefreshSemanticIndexRebuildsChangedVersionOneCatalog(t *testing.T) {
	isolateConfigHome(t)
	original := &Catalog{SyncedAt: time.Now(), Entries: []Entry{{PK: "legacy", Title: "기존 데이터"}}}
	legacy := &SemanticIndex{
		Version: 1, Model: "legacy-changed", CatalogDigest: catalogDigest(original), CreatedAt: time.Now(),
		PKs: []string{"legacy"}, Vectors: [][]float32{{1, 0}},
	}
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}
	changed := &Catalog{SyncedAt: original.SyncedAt.Add(time.Hour), Entries: []Entry{
		{PK: "legacy", Title: "변경된 데이터"},
		{PK: "new", Title: "새 데이터"},
	}}
	embedder := &recordingEmbedder{model: "legacy-changed"}
	refreshed, stats, err := RefreshSemanticIndex(context.Background(), changed, embedder, 32, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reused != 0 || stats.Embedded != len(changed.Entries) || len(embedder.inputs) != len(changed.Entries) {
		t.Fatalf("stats=%+v inputs=%d", stats, len(embedder.inputs))
	}
	if err := refreshed.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSemanticIndex(changed); err != nil {
		t.Fatalf("load rebuilt index: %v", err)
	}
}

func TestRefreshSemanticIndexRebuildsIncompatibleRecipe(t *testing.T) {
	isolateConfigHome(t)
	cat := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "first", Title: "첫 데이터"},
		{PK: "second", Title: "둘째 데이터"},
	}}
	old := &SemanticIndex{
		Version: semanticIndexVersion, RecipeVersion: semanticRecipeVersion + 1,
		Model: "recipe-test", CatalogDigest: catalogDigest(cat), CreatedAt: time.Now(),
		PKs: []string{"first", "second"}, DocumentHashes: []string{"old-a", "old-b"},
		Vectors: [][]float32{{1, 0}, {0, 1}},
	}
	if err := old.Save(); err != nil {
		t.Fatal(err)
	}
	embedder := &recordingEmbedder{model: "recipe-test"}
	refreshed, stats, err := RefreshSemanticIndex(context.Background(), cat, embedder, 32, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reused != 0 || stats.Embedded != len(cat.Entries) || len(embedder.inputs) != len(cat.Entries) {
		t.Fatalf("stats=%+v inputs=%d", stats, len(embedder.inputs))
	}
	if refreshed.RecipeVersion != semanticRecipeVersion || len(refreshed.DocumentHashes) != len(cat.Entries) {
		t.Fatalf("rebuilt index = %+v", refreshed)
	}
}

func TestRefreshSemanticIndexRebuildsUnsafeVectorCache(t *testing.T) {
	isolateConfigHome(t)
	cat := &Catalog{SyncedAt: time.Now(), Entries: []Entry{{PK: "safe", Title: "안전 데이터"}}}
	unsafe := &SemanticIndex{
		Version: semanticIndexVersion, RecipeVersion: semanticRecipeVersion, Model: "unsafe-test",
		CatalogDigest: catalogDigest(cat), Dimensions: 2, CreatedAt: time.Now(),
		PKs: []string{"safe"}, DocumentHashes: []string{semanticDocumentHash(cat.Entries[0])},
		Vectors: [][]float32{{float32(math.NaN()), 0}},
	}
	if err := unsafe.Save(); err != nil {
		t.Fatal(err)
	}
	embedder := &recordingEmbedder{model: "unsafe-test"}
	_, stats, err := RefreshSemanticIndex(context.Background(), cat, embedder, 32, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reused != 0 || stats.Embedded != 1 || len(embedder.inputs) != 1 {
		t.Fatalf("stats=%+v inputs=%d", stats, len(embedder.inputs))
	}
}

func TestRefreshSemanticIndexRebuildsTruncatedCache(t *testing.T) {
	isolateConfigHome(t)
	path, err := semanticIndexPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0x7f, 0x01, 0x02}, 0o600); err != nil {
		t.Fatal(err)
	}
	cat := &Catalog{SyncedAt: time.Now(), Entries: []Entry{{PK: "safe", Title: "안전 데이터"}}}
	embedder := &recordingEmbedder{model: "truncated-test"}
	_, stats, err := RefreshSemanticIndex(context.Background(), cat, embedder, 32, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reused != 0 || stats.Embedded != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestHybridSearchRejectsInvalidQueryEmbeddings(t *testing.T) {
	c := &Catalog{Entries: []Entry{{PK: "one", Title: "상권 데이터", SvcType: SvcREST}}}
	index, err := BuildSemanticIndex(context.Background(), c, &recordingEmbedder{model: "valid"}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		vectors [][]float32
	}{
		{name: "wrong count", vectors: [][]float32{{1, 0}, {0, 1}}},
		{name: "wrong dimensions", vectors: [][]float32{{1}}},
		{name: "non finite", vectors: [][]float32{{float32(math.Inf(1)), 0}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := c.SearchHybrid(context.Background(), QueryPlan{Intent: "상권", Limit: 1}, index,
				fixedVectorsEmbedder{model: "invalid", vectors: test.vectors})
			if result.Semantic == nil || result.Semantic.Status != SemanticUnavailable {
				t.Fatalf("semantic = %+v", result.Semantic)
			}
		})
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

func TestHybridSearchAcceptsSelectionFromBoundedOptionPoolBeyondDisplayedHits(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "visible", Title: "상권 점포 현황", SvcType: SvcREST, ApplyCount: 100},
		{PK: "trimmed", Title: "상권 점포 이력", SvcType: SvcREST, ApplyCount: 1},
	}}
	plan := QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "상권 수요", Query: "상권 점포", Contribution: "쇠퇴 상권을 구분",
				Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{{
			PK: "trimmed", Role: "상권 수요", IncrementalValue: "쇠퇴 상권을 구분", WhyCandidate: "점포 이력 제목",
			Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}},
		}},
		Limit: 2, RESTOnly: true,
	}
	r := c.SearchHybrid(context.Background(), plan, nil, nil)
	if len(r.Hits) != 2 || r.Hits[0].PK != "anchor" || r.Hits[1].PK != "visible" {
		t.Fatalf("hits = %+v", r.Hits)
	}
	if len(r.ConnectionOptions) != 1 || len(r.ConnectionOptions[0].Nodes) != 2 {
		t.Fatalf("option pool = %+v", r.ConnectionOptions)
	}
	if len(r.Connections) != 1 || r.Connections[0].Bridge.PK != "trimmed" || r.Abstention != nil {
		t.Fatalf("bounded option outside display page should remain selectable: %+v", r)
	}
}

func BenchmarkSearchPlan95K(b *testing.B) {
	const rows = 95_951
	entries := make([]Entry, rows)
	for i := range entries {
		entries[i] = Entry{
			PK: fmt.Sprintf("%06d", i), Title: fmt.Sprintf("지역 상권 매출 데이터 %d", i),
			Desc: "시군구 업종별 월간 매출과 점포 현황", ModifiedAt: fmt.Sprintf("2026-%02d-%02d", i%12+1, i%28+1),
			SvcType: SvcFILE, ViewCount: i,
		}
	}
	c := &Catalog{Entries: entries}
	plan := QueryPlan{
		Intent: "지역 사업 기회", Concepts: []string{
			"지역 상권", "업종 매출", "점포 현황", "월간 매출",
			"시군구 업종", "상권 데이터", "지역 매출", "점포 매출",
		},
		Limit: 100, Ranking: RankBalanced, IncludePreviews: true,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := c.SearchPlan(plan)
		if len(result.Hits) != 100 {
			b.Fatalf("hits = %d", len(result.Hits))
		}
	}
}

func BenchmarkSemanticSearch95K(b *testing.B) {
	const rows = 95_951
	entries := make([]Entry, rows)
	index := &SemanticIndex{PKs: make([]string, rows), Vectors: make([][]float32, rows)}
	for i := range entries {
		pk := fmt.Sprintf("%06d", i)
		entries[i] = Entry{PK: pk, Title: "지역 상권 데이터", SvcType: SvcFILE, ViewCount: i}
		index.PKs[i] = pk
		index.Vectors[i] = []float32{float32(i % 11), 1, 2, 3, 4, 5, 6, 7}
		normalize(index.Vectors[i])
	}
	c := &Catalog{Entries: entries}
	query := []float32{1, 1, 1, 1, 1, 1, 1, 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits := index.Search(c, append([]float32(nil), query...), 400, false, true)
		if len(hits) != 400 {
			b.Fatalf("hits = %d", len(hits))
		}
	}
}
