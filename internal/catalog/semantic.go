package catalog

import (
	"bytes"
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

	"github.com/JungHoonGhae/gongctl/internal/portal"
)

const (
	// Ollama is optional. The QAT model is only about 239 MB while retaining
	// almost all of the full model's multilingual retrieval quality.
	DefaultOllamaURL      = "http://127.0.0.1:11434"
	DefaultEmbeddingModel = "embeddinggemma:300m-qat-q4_0"
	semanticIndexVersion  = 1
)

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

// SemanticIndex is intentionally a flat local matrix. At the current 11k-row
// scale it is roughly 35 MB at 768 dimensions and brute-force cosine search is
// faster and operationally simpler than running a vector database. The type is
// isolated so an ANN/vector-DB backend can replace it if the corpus grows by
// orders of magnitude or becomes a concurrent service.
type SemanticIndex struct {
	Version       int
	Model         string
	CatalogDigest string
	CreatedAt     time.Time
	PKs           []string
	Vectors       [][]float32
}

// BuildSemanticIndex embeds the complete catalogue in bounded batches.
func BuildSemanticIndex(ctx context.Context, c *Catalog, embedder Embedder, batchSize int, progress func(done, total int)) (*SemanticIndex, error) {
	if batchSize <= 0 {
		batchSize = 32
	}
	idx := &SemanticIndex{
		Version: semanticIndexVersion, Model: embedder.Model(),
		CatalogDigest: catalogDigest(c), CreatedAt: time.Now().UTC(),
		PKs: make([]string, len(c.Entries)), Vectors: make([][]float32, len(c.Entries)),
	}
	for start := 0; start < len(c.Entries); start += batchSize {
		end := start + batchSize
		if end > len(c.Entries) {
			end = len(c.Entries)
		}
		docs := make([]string, end-start)
		for i := start; i < end; i++ {
			idx.PKs[i] = c.Entries[i].PK
			docs[i-start] = semanticDocument(c.Entries[i])
		}
		vectors, err := embedder.Embed(ctx, docs)
		if err != nil {
			return nil, fmt.Errorf("semantic index %d-%d/%d: %w", start+1, end, len(c.Entries), err)
		}
		if len(vectors) != len(docs) {
			return nil, fmt.Errorf("semantic index 벡터 수 %d, 문서 수 %d", len(vectors), len(docs))
		}
		copy(idx.Vectors[start:end], vectors)
		if progress != nil {
			progress(end, len(c.Entries))
		}
	}
	return idx, nil
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

func LoadSemanticIndex(c *Catalog) (*SemanticIndex, error) {
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
	var idx SemanticIndex
	if err := gob.NewDecoder(f).Decode(&idx); err != nil {
		return nil, err
	}
	if idx.Version != semanticIndexVersion || idx.CatalogDigest != catalogDigest(c) || len(idx.PKs) != len(idx.Vectors) {
		return nil, ErrSemanticIndexStale
	}
	return &idx, nil
}

// Search performs a flat cosine scan. Vectors are normalized at build/query
// time, so cosine similarity is a dot product.
func (s *SemanticIndex) Search(c *Catalog, query []float32, limit int, restOnly, includePreviews bool) []Hit {
	if limit <= 0 {
		limit = 20
	}
	normalize(query)
	entries := make(map[string]*Entry, len(c.Entries))
	for i := range c.Entries {
		entries[c.Entries[i].PK] = &c.Entries[i]
	}
	var hits []Hit
	for i, vector := range s.Vectors {
		if i >= len(s.PKs) || len(vector) != len(query) {
			continue
		}
		e := entries[s.PKs[i]]
		if e == nil || (restOnly && e.SvcType != SvcREST) {
			continue
		}
		score := dot(query, vector)
		h := Hit{PK: e.PK, Title: e.Title, Org: e.Org, ApplyCount: e.ApplyCount,
			ModifiedAt: e.ModifiedAt, SvcType: e.SvcType, SemanticScore: score}
		if includePreviews {
			h.Preview = descriptionPreview(e, 180)
		}
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].SemanticScore != hits[j].SemanticScore {
			return hits[i].SemanticScore > hits[j].SemanticScore
		}
		return hits[i].ApplyCount > hits[j].ApplyCount
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// SearchHybrid fuses deterministic lexical/planned retrieval with semantic
// similarity using reciprocal-rank fusion. Semantic-only candidates receive a
// slightly larger weight so a bad literal parse (for example "돈/벌/수") cannot
// drown the intent signal; agreement between both retrievers ranks highest.
func (c *Catalog) SearchHybrid(ctx context.Context, plan QueryPlan, index *SemanticIndex, embedder Embedder) Result {
	want := normalizeSearchLimit(plan.Limit)
	expanded := plan
	expanded.Limit = want * 4
	if expanded.Limit < 40 {
		expanded.Limit = 40
	}
	base := c.searchPlan(expanded)
	if index == nil || embedder == nil {
		base.Hits = trimHits(base.Hits, want)
		base.Semantic = &SemanticInfo{Status: SemanticNotIndexed}
		return base
	}

	// Concrete model-inferred axes come first. The original broad intent remains
	// a final semantic safety net, but must not crowd out the diverse retrieval
	// plan the host model deliberately supplied.
	queries := uniqueQueries(append(append([]string{}, plan.Concepts...), plan.Intent), 8)
	if len(queries) == 0 {
		base.Hits = trimHits(base.Hits, want)
		base.Semantic = &SemanticInfo{Status: SemanticUnavailable, Model: index.Model, Detail: "검색할 문장이 비어 있음"}
		return base
	}
	inputs := make([]string, len(queries))
	for i, query := range queries {
		inputs[i] = semanticQuery(query)
	}
	vectors, err := embedder.Embed(ctx, inputs)
	if err != nil {
		base.Hits = trimHits(base.Hits, want)
		base.Semantic = &SemanticInfo{Status: SemanticUnavailable, Model: index.Model, Detail: err.Error()}
		return base
	}

	perQuery := expanded.Limit
	buckets := make([][]Hit, len(vectors))
	for i, vector := range vectors {
		buckets[i] = index.Search(c, vector, perQuery, plan.RESTOnly, plan.IncludePreviews)
		for j := range buckets[i] {
			buckets[i][j].MatchedQuery = queries[i]
		}
	}
	semantic := roundRobinHits(buckets, expanded.Limit)
	diversityOrder := queries
	if concepts := uniqueQueries(plan.Concepts, 8); len(concepts) > 0 {
		// The original intent is a semantic safety net, not a seventh product
		// axis. Fill concept coverage first; intent-only surprises can enter as
		// remainder when the requested page has room.
		diversityOrder = concepts
	}
	base.Hits = fuseHits(base.Hits, semantic, want, diversityOrder)
	base.Mode = SearchModeHybrid
	base.Semantic = &SemanticInfo{Status: SemanticUsed, Model: index.Model}
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
		return items[i].hit.ApplyCount > items[j].hit.ApplyCount
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

func dot(a, b []float32) float32 {
	var out float32
	for i := range a {
		out += a[i] * b[i]
	}
	return out
}
