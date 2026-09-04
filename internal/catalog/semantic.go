package catalog

import (
	"bytes"
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/portal"
)

const (
	// Ollama is optional. The QAT model is only about 239 MB while retaining
	// almost all of the full model's multilingual retrieval quality.
	DefaultOllamaURL      = "http://127.0.0.1:11434"
	DefaultEmbeddingModel = "embeddinggemma:300m-qat-q4_0"
	semanticIndexVersion  = 2
	semanticRecipeVersion = 1
	maxSemanticIndexBytes = int64(1 << 30)
	maxSemanticIndexRows  = 1_000_000
	maxSemanticDimensions = 8_192
)

// OllamaURLFromEnv returns the configured Ollama endpoint.
func OllamaURLFromEnv() string {
	return strings.TrimSpace(os.Getenv("ODEDUCK_OLLAMA_URL"))
}

var (
	ErrSemanticIndexNotBuilt = errors.New("semantic index is not built")
	ErrSemanticIndexStale    = errors.New("semantic index does not match the current catalog")
)

const (
	SemanticUsed        = "used"
	SemanticNotIndexed  = "not-indexed"
	SemanticUnavailable = "unavailable"
)

// SemanticInfo makes fallback visible. Search still returns lexical/planned
// results when Ollama is absent, but an agent can tell whether vector recall was
// actually part of this answer.
type SemanticInfo struct {
	Status string `json:"status"`
	Model  string `json:"model,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// RecordSemanticOutcome keeps degraded retrieval visible in both human and
// machine-readable outputs. A caller may still choose lexical/planned recall,
// but it cannot mistake those candidates for a completed hybrid search.
func RecordSemanticOutcome(result *Result, info SemanticInfo) {
	result.Semantic = &info
	if info.Status == SemanticUsed {
		return
	}
	warning := fmt.Sprintf("semantic 폴백: status=%s", info.Status)
	if strings.TrimSpace(info.Detail) != "" {
		warning += " · " + info.Detail
	}
	warning += " · 결과는 의미 벡터 없이 lexical/planned 검색으로 생성됨"
	for _, existing := range result.Warnings {
		if existing == warning {
			return
		}
	}
	result.Warnings = append(result.Warnings, warning)
}

// RequireSemantic rejects a degraded result for research where vector recall
// is part of the requested evidence standard.
func RequireSemantic(result Result) error {
	if result.Semantic != nil && result.Semantic.Status == SemanticUsed {
		return nil
	}
	status, detail := "disabled", "semantic 검색이 비활성화됨"
	if result.Semantic != nil {
		status, detail = result.Semantic.Status, result.Semantic.Detail
	}
	if strings.TrimSpace(detail) != "" {
		detail = ": " + detail
	}
	return fmt.Errorf("semantic 검색이 필수지만 status=%s%s; `odeduck catalog semantic-build` 후 다시 시도하세요", status, detail)
}

// Embedder is the deliberately small provider boundary. Ollama is the built-in
// free/local provider; a hosted service or another local runtime can implement
// the same two methods without changing catalogue storage or ranking.
type Embedder interface {
	Model() string
	Embed(context.Context, []string) ([][]float32, error)
}

// OllamaEmbedder calls Ollama's stable local HTTP API directly, avoiding an SDK
// dependency. The API accepts batches and returns unit-length vectors.
type OllamaEmbedder struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultOllamaURL
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultEmbeddingModel
	}
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"), model: model,
		// Pulling even the small default model can exceed ten minutes on a slow
		// first-time connection. Embedding calls remain bounded by their caller's
		// context, while this ceiling prevents a permanently wedged daemon.
		http: &http.Client{Timeout: 30 * time.Minute},
	}
}

func (e *OllamaEmbedder) Model() string { return e.model }

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{
		"model": e.model, "input": texts, "truncate": true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama 연결 실패 (%s): %w", e.baseURL, err)
	}
	defer resp.Body.Close()
	var body struct {
		Embeddings [][]float32 `json:"embeddings"`
		Error      string      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("Ollama embed 응답 해석 실패: %w", err)
	}
	if resp.StatusCode/100 != 2 || body.Error != "" {
		if body.Error == "" {
			body.Error = resp.Status
		}
		return nil, fmt.Errorf("Ollama embed 실패: %s", body.Error)
	}
	if len(body.Embeddings) != len(texts) {
		return nil, fmt.Errorf("Ollama embed 벡터 수 %d, 입력 수 %d", len(body.Embeddings), len(texts))
	}
	for i := range body.Embeddings {
		normalize(body.Embeddings[i])
	}
	return body.Embeddings, nil
}

// Pull downloads the configured model through Ollama. stream=false keeps the
// command implementation simple; its caller prints a clear pre-download notice.
func (e *OllamaEmbedder) Pull(ctx context.Context) error {
	payload, _ := json.Marshal(map[string]any{"model": e.model, "stream": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/pull", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("Ollama 연결 실패 (%s): %w", e.baseURL, err)
	}
	defer resp.Body.Close()
	var body struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("Ollama pull 응답 해석 실패: %w", err)
	}
	if resp.StatusCode/100 != 2 || body.Error != "" {
		if body.Error == "" {
			body.Error = resp.Status
		}
		return fmt.Errorf("Ollama 모델 다운로드 실패: %s", body.Error)
	}
	return nil
}

// SemanticIndex is intentionally a flat local matrix. At the current 96k-row
// scale brute-force cosine search remains operationally simpler than requiring
// a vector database. DocumentHashes let refreshes reuse vectors whose actual
// embedding input did not change, even when catalogue metadata or SyncedAt did.
type SemanticIndex struct {
	Version        int
	RecipeVersion  int
	Model          string
	Dimensions     int
	CatalogDigest  string
	CreatedAt      time.Time
	PKs            []string
	DocumentHashes []string
	Vectors        [][]float32
}

// SemanticBuildStats exposes whether a refresh avoided the expensive document
// embedding work. Query embedding remains necessary when semantic search runs.
type SemanticBuildStats struct {
	Total    int
	Reused   int
	Embedded int
}

type semanticCandidate struct {
	entry *Entry
	score float32
}

// semanticCandidateHeap keeps the worst retained candidate at index zero so a
// full catalogue scan only materializes the requested top K results.
type semanticCandidateHeap []semanticCandidate

func (h semanticCandidateHeap) Len() int { return len(h) }
func (h semanticCandidateHeap) Less(i, j int) bool {
	if h[i].score != h[j].score {
		return h[i].score < h[j].score
	}
	return moreDemandedEntry(h[j].entry, h[i].entry)
}
func (h semanticCandidateHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *semanticCandidateHeap) Push(value any) {
	*h = append(*h, value.(semanticCandidate))
}
func (h *semanticCandidateHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

func moreDemandedEntry(left, right *Entry) bool {
	if left.ApplyCount != right.ApplyCount {
		return left.ApplyCount > right.ApplyCount
	}
	if left.ViewCount != right.ViewCount {
		return left.ViewCount > right.ViewCount
	}
	return left.PK < right.PK
}

func betterSemanticCandidate(left, right semanticCandidate) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	return moreDemandedEntry(left.entry, right.entry)
}

// BuildSemanticIndex embeds the complete catalogue in bounded batches. It is a
// pure full-build interface used by tests and custom callers; the CLI uses
// RefreshSemanticIndex so unchanged vectors survive normal catalogue refreshes.
func BuildSemanticIndex(ctx context.Context, c *Catalog, embedder Embedder, batchSize int, progress func(done, total int)) (*SemanticIndex, error) {
	idx, _, err := buildSemanticIndex(ctx, c, embedder, batchSize, nil, progress)
	return idx, err
}

// RefreshSemanticIndex loads the previous local index, reuses vectors with the
// same model, recipe and document hash, and embeds only changed or new entries.
// A version-1 index can be migrated without re-embedding when it exactly matches
// the current catalogue digest.
func RefreshSemanticIndex(ctx context.Context, c *Catalog, embedder Embedder, batchSize int, progress func(done, total int)) (*SemanticIndex, SemanticBuildStats, error) {
	previous, err := loadSemanticIndexFile()
	if errors.Is(err, ErrSemanticIndexNotBuilt) || errors.Is(err, ErrSemanticIndexStale) {
		previous = nil
	} else if err != nil {
		return nil, SemanticBuildStats{}, err
	}
	return buildSemanticIndex(ctx, c, embedder, batchSize, previous, progress)
}

func buildSemanticIndex(ctx context.Context, c *Catalog, embedder Embedder, batchSize int, previous *SemanticIndex, progress func(done, total int)) (*SemanticIndex, SemanticBuildStats, error) {
	if batchSize <= 0 {
		batchSize = 32
	}
	total := len(c.Entries)
	currentDigest := catalogDigest(c)
	idx := &SemanticIndex{
		Version: semanticIndexVersion, RecipeVersion: semanticRecipeVersion, Model: embedder.Model(),
		CatalogDigest: currentDigest, CreatedAt: time.Now().UTC(),
		PKs: make([]string, total), DocumentHashes: make([]string, total), Vectors: make([][]float32, total),
	}

	type reusableVector struct {
		hash   string
		vector []float32
	}
	reusable := make(map[string]reusableVector)
	legacyExactMatch := previous != nil && previous.Version == 1 && previous.CatalogDigest == currentDigest
	if previous != nil && previous.Model == embedder.Model() && (legacyExactMatch ||
		(previous.Version == semanticIndexVersion && previous.RecipeVersion == semanticRecipeVersion && len(previous.DocumentHashes) == len(previous.PKs))) {
		for i, pk := range previous.PKs {
			if i >= len(previous.Vectors) || len(previous.Vectors[i]) == 0 {
				continue
			}
			hash := ""
			if !legacyExactMatch {
				hash = previous.DocumentHashes[i]
			}
			reusable[pk] = reusableVector{hash: hash, vector: previous.Vectors[i]}
		}
	}

	pending := make([]int, 0, total)
	stats := SemanticBuildStats{Total: total}
	for i, entry := range c.Entries {
		docHash := semanticDocumentHash(entry)
		idx.PKs[i] = entry.PK
		idx.DocumentHashes[i] = docHash
		if old, ok := reusable[entry.PK]; ok && (legacyExactMatch || old.hash == docHash) {
			if idx.Dimensions != 0 && len(old.vector) != idx.Dimensions {
				pending = append(pending, i)
				continue
			}
			idx.Vectors[i] = old.vector
			if idx.Dimensions == 0 {
				idx.Dimensions = len(old.vector)
			}
			stats.Reused++
			continue
		}
		pending = append(pending, i)
	}
	if progress != nil && stats.Reused > 0 {
		progress(stats.Reused, total)
	}

	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		docs := make([]string, end-start)
		for i, position := range pending[start:end] {
			docs[i] = semanticDocument(c.Entries[position])
		}
		vectors, err := embedder.Embed(ctx, docs)
		if err != nil {
			return nil, SemanticBuildStats{}, fmt.Errorf("semantic index %d-%d/%d changed documents: %w", start+1, end, len(pending), err)
		}
		if len(vectors) != len(docs) {
			return nil, SemanticBuildStats{}, fmt.Errorf("semantic index 벡터 수 %d, 문서 수 %d", len(vectors), len(docs))
		}
		for i, vector := range vectors {
			if !validSemanticVector(vector, idx.Dimensions) {
				return nil, SemanticBuildStats{}, fmt.Errorf("semantic index 벡터 차원 %d, 기대값 %d", len(vector), idx.Dimensions)
			}
			if idx.Dimensions == 0 {
				idx.Dimensions = len(vector)
			}
			idx.Vectors[pending[start+i]] = vector
		}
		stats.Embedded += len(vectors)
		if progress != nil {
			progress(stats.Reused+stats.Embedded, total)
		}
	}
	if progress != nil && len(pending) == 0 {
		progress(total, total)
	}
	return idx, stats, nil
}

func semanticDocument(e Entry) string {
	text := strings.Join(strings.Fields(strings.Join([]string{
		"제공기관: " + e.Org, "분류: " + e.Category, e.Desc,
	}, " | ")), " ")
	return "title: " + e.Title + " | text: " + text
}

func semanticQuery(query string) string {
	return "task: search result | query: " + strings.TrimSpace(query)
}

func semanticDocumentHash(e Entry) string {
	sum := sha256.Sum256([]byte(semanticDocument(e)))
	return hex.EncodeToString(sum[:])
}

func catalogDigest(c *Catalog) string {
	data, _ := json.Marshal(struct {
		SyncedAt time.Time
		Entries  []Entry
	}{c.SyncedAt, c.Entries})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func semanticIndexPath() (string, error) {
	dir, err := portal.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "catalog-semantic.gob"), nil
}

func (s *SemanticIndex) Save() error {
	path, err := semanticIndexPath()
	if err != nil {
		return err
	}
	return atomicWrite(path, "catalog-semantic-*.tmp", 0o600, func(w io.Writer) error {
		return gob.NewEncoder(w).Encode(s)
	})
}

func loadSemanticIndexFile() (*SemanticIndex, error) {
	path, err := semanticIndexPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, ErrSemanticIndexNotBuilt
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 || info.Size() > maxSemanticIndexBytes {
		return nil, ErrSemanticIndexStale
	}
	var idx SemanticIndex
	if err := gob.NewDecoder(io.LimitReader(f, maxSemanticIndexBytes)).Decode(&idx); err != nil {
		return nil, ErrSemanticIndexStale
	}
	if err := validateSemanticIndex(&idx); err != nil {
		return nil, ErrSemanticIndexStale
	}
	if idx.Version == semanticIndexVersion && (idx.RecipeVersion != semanticRecipeVersion || len(idx.DocumentHashes) != len(idx.PKs)) {
		return nil, ErrSemanticIndexStale
	}
	return &idx, nil
}

func validateSemanticIndex(idx *SemanticIndex) error {
	if idx == nil || (idx.Version != 1 && idx.Version != semanticIndexVersion) ||
		len(idx.PKs) != len(idx.Vectors) || len(idx.PKs) > maxSemanticIndexRows {
		return ErrSemanticIndexStale
	}
	expectedDimensions := idx.Dimensions
	for _, vector := range idx.Vectors {
		if len(vector) == 0 || len(vector) > maxSemanticDimensions {
			return ErrSemanticIndexStale
		}
		if expectedDimensions == 0 {
			expectedDimensions = len(vector)
		}
		if len(vector) != expectedDimensions {
			return ErrSemanticIndexStale
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return ErrSemanticIndexStale
			}
		}
	}
	if len(idx.PKs) > 0 && (expectedDimensions == 0 || expectedDimensions > maxSemanticDimensions) {
		return ErrSemanticIndexStale
	}
	return nil
}

func LoadSemanticIndex(c *Catalog) (*SemanticIndex, error) {
	idx, err := loadSemanticIndexFile()
	if err != nil {
		return nil, err
	}
	if idx.CatalogDigest != catalogDigest(c) {
		return nil, ErrSemanticIndexStale
	}
	return idx, nil
}

// Search performs a flat cosine scan. Vectors are normalized at build/query
// time, so cosine similarity is a dot product.
func (s *SemanticIndex) Search(c *Catalog, query []float32, limit int, restOnly, includePreviews bool) []Hit {
	if limit <= 0 {
		limit = 20
	}
	entries := make(map[string]*Entry, len(c.Entries))
	for i := range c.Entries {
		entries[c.Entries[i].PK] = &c.Entries[i]
	}
	return s.search(entries, query, limit, restOnly, includePreviews)
}

func (s *SemanticIndex) search(entries map[string]*Entry, query []float32, limit int, restOnly, includePreviews bool) []Hit {
	if limit <= 0 {
		limit = 20
	}
	normalize(query)
	candidates := make(semanticCandidateHeap, 0, limit)
	heap.Init(&candidates)
	for i, vector := range s.Vectors {
		if i >= len(s.PKs) || len(vector) != len(query) {
			continue
		}
		e := entries[s.PKs[i]]
		if e == nil || (restOnly && e.SvcType != SvcREST) {
			continue
		}
		candidate := semanticCandidate{entry: e, score: dot(query, vector)}
		if candidates.Len() < limit {
			heap.Push(&candidates, candidate)
			continue
		}
		if betterSemanticCandidate(candidate, candidates[0]) {
			candidates[0] = candidate
			heap.Fix(&candidates, 0)
		}
	}
	selected := make([]semanticCandidate, len(candidates))
	copy(selected, candidates)
	sort.SliceStable(selected, func(i, j int) bool { return betterSemanticCandidate(selected[i], selected[j]) })
	hits := make([]Hit, 0, len(selected))
	for _, candidate := range selected {
		h := hitFromEntry(candidate.entry)
		h.SemanticScore = candidate.score
		if includePreviews {
			h.Preview = descriptionPreview(candidate.entry, 180)
		}
		hits = append(hits, h)
	}
	return hits
}

// SearchHybrid fuses deterministic lexical/planned retrieval with semantic
// similarity using reciprocal-rank fusion. Semantic-only candidates receive a
// slightly larger weight so a bad literal parse (for example "돈/벌/수") cannot
// drown the intent signal; agreement between both retrievers ranks highest.
func (c *Catalog) SearchHybrid(ctx context.Context, plan QueryPlan, index *SemanticIndex, embedder Embedder) Result {
	want := normalizeSearchLimit(plan.Limit)
	normalizedPlan := plan
	normalizedPlan.Axes, _ = normalizeDiscoveryAxes(plan.Axes, 8)
	expanded := plan
	expanded.Limit = want * 4
	if expanded.Limit < 40 {
		expanded.Limit = 40
	}
	base := c.searchPlan(expanded)
	entries := make(map[string]*Entry, len(c.Entries))
	for i := range c.Entries {
		entries[c.Entries[i].PK] = &c.Entries[i]
	}
	if index == nil || embedder == nil {
		base.Hits = trimHits(base.Hits, want)
		finalizeConnectionCandidates(normalizedPlan, &base, entries)
		RecordSemanticOutcome(&base, SemanticInfo{Status: SemanticNotIndexed, Detail: "의미 인덱스가 준비되지 않음"})
		return base
	}

	// Concrete model-inferred axes come first. The original broad intent remains
	// a final semantic safety net, but must not crowd out the diverse retrieval
	// plan the host model deliberately supplied.
	semanticConcepts := append([]string{}, plan.Concepts...)
	if len(normalizedPlan.Axes) > 0 {
		semanticConcepts = semanticConcepts[:0]
		for _, axis := range normalizedPlan.Axes {
			semanticConcepts = append(semanticConcepts, axis.Query)
		}
	}
	queries := uniqueQueries(append(semanticConcepts, plan.Intent), 8)
	if len(queries) == 0 {
		base.Hits = trimHits(base.Hits, want)
		finalizeConnectionCandidates(normalizedPlan, &base, entries)
		RecordSemanticOutcome(&base, SemanticInfo{Status: SemanticUnavailable, Model: index.Model, Detail: "검색할 문장이 비어 있음"})
		return base
	}
	inputs := make([]string, len(queries))
	for i, query := range queries {
		inputs[i] = semanticQuery(query)
	}
	vectors, err := embedder.Embed(ctx, inputs)
	if err != nil {
		base.Hits = trimHits(base.Hits, want)
		finalizeConnectionCandidates(normalizedPlan, &base, entries)
		RecordSemanticOutcome(&base, SemanticInfo{Status: SemanticUnavailable, Model: index.Model, Detail: err.Error()})
		return base
	}
	expectedDimensions := index.Dimensions
	if expectedDimensions == 0 {
		for _, vector := range index.Vectors {
			if len(vector) > 0 {
				expectedDimensions = len(vector)
				break
			}
		}
	}
	if len(vectors) != len(inputs) {
		base.Hits = trimHits(base.Hits, want)
		finalizeConnectionCandidates(normalizedPlan, &base, entries)
		RecordSemanticOutcome(&base, SemanticInfo{Status: SemanticUnavailable, Model: index.Model, Detail: "query embedding count does not match inputs"})
		return base
	}
	for _, vector := range vectors {
		if !validSemanticVector(vector, expectedDimensions) {
			base.Hits = trimHits(base.Hits, want)
			finalizeConnectionCandidates(normalizedPlan, &base, entries)
			RecordSemanticOutcome(&base, SemanticInfo{Status: SemanticUnavailable, Model: index.Model, Detail: "query embedding dimensions or values are invalid"})
			return base
		}
	}

	perQuery := expanded.Limit
	buckets := make([][]Hit, len(vectors))
	axisByQuery := map[string]DiscoveryAxis{}
	for _, axis := range normalizedPlan.Axes {
		axisByQuery[axis.Query] = axis
	}
	for i, vector := range vectors {
		buckets[i] = index.search(entries, vector, perQuery, plan.RESTOnly, plan.IncludePreviews)
		for j := range buckets[i] {
			buckets[i][j].MatchedQuery = queries[i]
			if axis, ok := axisByQuery[queries[i]]; ok {
				buckets[i][j].Role = axis.Role
				buckets[i][j].Contribution = axis.Contribution
				if len(axis.Edge.Kinds) > 0 || len(axis.Edge.ExpectedKeys) > 0 {
					edge := axis.Edge
					buckets[i][j].EdgeHypothesis = &edge
				}
			}
		}
	}
	semantic := roundRobinHits(buckets, expanded.Limit)
	diversityOrder := queries
	if concepts := uniqueQueries(semanticConcepts, 8); len(concepts) > 0 {
		// The original intent is a semantic safety net, not a seventh product
		// axis. Fill concept coverage first; intent-only surprises can enter as
		// remainder when the requested page has room.
		diversityOrder = concepts
	}
	fused := fuseHits(base.Hits, semantic, expanded.Limit, diversityOrder)
	base.Hits = trimHits(fused, want)
	base.ConnectionOptions = buildConnectionOptionsFromHits(fused, normalizedPlan.AnchorPKs, MaxOptionsPerRole)
	finalizeConnectionCandidates(normalizedPlan, &base, entries)
	base.Mode = SearchModeHybrid
	RecordSemanticOutcome(&base, SemanticInfo{Status: SemanticUsed, Model: index.Model})
	if len(base.Queries) == 0 {
		base.Intent = strings.TrimSpace(plan.Intent)
	}
	if len(base.Hits) > base.Total {
		base.Total = len(base.Hits)
	}
	return base
}

func roundRobinHits(buckets [][]Hit, limit int) []Hit {
	var out []Hit
	seen := map[string]bool{}
	for row := 0; len(out) < limit; row++ {
		progress := false
		for _, hits := range buckets {
			if row >= len(hits) {
				continue
			}
			progress = true
			if seen[hits[row].PK] {
				continue
			}
			seen[hits[row].PK] = true
			out = append(out, hits[row])
			if len(out) == limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return out
}

func fuseHits(lexical, semantic []Hit, limit int, queryOrder []string) []Hit {
	type fused struct {
		hit   Hit
		score float64
	}
	byPK := map[string]*fused{}
	add := func(hits []Hit, weight float64, semantic bool) {
		for rank, hit := range hits {
			item := byPK[hit.PK]
			if item == nil {
				copy := hit
				item = &fused{hit: copy}
				byPK[hit.PK] = item
			} else {
				if item.hit.MatchedQuery == "" {
					item.hit.MatchedQuery = hit.MatchedQuery
				}
				if item.hit.Preview == "" {
					item.hit.Preview = hit.Preview
				}
				if hit.SemanticScore != 0 {
					item.hit.SemanticScore = hit.SemanticScore
				}
			}
			contribution := weight / float64(60+rank+1)
			if semantic {
				// Similarity carries information that rank alone discards. A
				// zero/negative tail candidate must not gain enough RRF weight to
				// beat a genuinely similar semantic-only result.
				if hit.SemanticScore <= 0 {
					continue
				}
				contribution *= float64(hit.SemanticScore)
			}
			item.score += contribution
		}
	}
	add(lexical, 1, false)
	// Semantic retrieval is the recall engine for natural-language intent. Its
	// cosine values are commonly around 0.3-0.6, so a 4x rank weight brings a
	// strong semantic-only candidate onto the same scale as a literal match;
	// entries found by both retrievers still win.
	add(semantic, 4, true)
	items := make([]fused, 0, len(byPK))
	for _, item := range byPK {
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return moreDemandedHit(items[i].hit, items[j].hit)
	})
	out := make([]Hit, len(items))
	for i := range items {
		out[i] = items[i].hit
	}
	if len(queryOrder) == 0 {
		return trimHits(out, limit)
	}
	// RRF gives relevance within each inferred axis. Round-robin across axes is
	// a separate responsibility: exploration fails if a dense API family (for
	// example dozens of Onbid operations) consumes the whole first page.
	buckets := make([][]Hit, 0, len(queryOrder)+1)
	assigned := map[string]bool{}
	for _, query := range queryOrder {
		var bucket []Hit
		// Preserve one strict lexical result per explicit axis before semantic
		// expansion. The benchmark found cases where broadly similar company or
		// project history displaced an exact "지원사업 공고"/"입찰 공고" hit.
		// This is a ranking constraint derived from the query, not a domain
		// keyword dictionary.
		for _, hit := range lexical {
			if hit.MatchedQuery == query && hit.Exact && !assigned[hit.PK] {
				assigned[hit.PK] = true
				for _, fusedHit := range out {
					if fusedHit.PK == hit.PK {
						bucket = append(bucket, fusedHit)
						break
					}
				}
				break
			}
		}
		for _, hit := range out {
			if !assigned[hit.PK] && hit.MatchedQuery == query {
				assigned[hit.PK] = true
				bucket = append(bucket, hit)
			}
		}
		buckets = append(buckets, bucket)
	}
	var remainder []Hit
	for _, hit := range out {
		if !assigned[hit.PK] {
			remainder = append(remainder, hit)
		}
	}
	if len(remainder) > 0 {
		buckets = append(buckets, remainder)
	}
	return roundRobinHits(buckets, limit)
}

func trimHits(hits []Hit, limit int) []Hit {
	if len(hits) <= limit {
		return hits
	}
	return hits[:limit]
}

func normalize(v []float32) {
	var sum float64
	for _, n := range v {
		sum += float64(n * n)
	}
	if sum == 0 {
		return
	}
	inv := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

func validSemanticVector(vector []float32, expectedDimensions int) bool {
	if len(vector) == 0 || len(vector) > maxSemanticDimensions || (expectedDimensions > 0 && len(vector) != expectedDimensions) {
		return false
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

func dot(a, b []float32) float32 {
	var out float32
	for i := range a {
		out += a[i] * b[i]
	}
	return out
}
