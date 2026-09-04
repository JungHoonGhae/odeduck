// Package catalog keeps a local copy of data.go.kr's OpenAPI and file catalogue so an
// agent can find out what exists without guessing keywords at the portal one
// request at a time.
//
// Discovery was the real bottleneck: the portal only answers keyword queries, so
// finding a dataset meant inventing search terms and paging through results,
// never knowing whether a miss meant "doesn't exist" or "wrong word". A synced
// catalogue turns that into a local lookup over every dataset at once, ranked by
// how many people actually applied for each.
//
// The catalogue is deliberately split from what a caller sees: descriptions are
// stored (they make matching much better) but never returned, because ten of them
// is three thousand characters of an agent's context spent on prose it did not
// ask for. Search returns compact rows.
package catalog

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/JungHoonGhae/odeduck/internal/portal"
)

// Entry is one catalogued dataset. Desc is used for matching and is not part of
// what Search returns.
type Entry struct {
	PK          string               `json:"pk"`
	Title       string               `json:"title"`
	Org         string               `json:"org,omitempty"`
	OrgType     string               `json:"orgType,omitempty"`
	Category    string               `json:"category,omitempty"`
	ApplyCount  int                  `json:"applyCount,omitempty"`
	ViewCount   int                  `json:"viewCount,omitempty"`
	ModifiedAt  string               `json:"modifiedAt,omitempty"`
	SvcType     string               `json:"svcType,omitempty"`   // REST | LINK | FILE | "" (unknown)
	DataTypes   []string             `json:"dataTypes,omitempty"` // API | FILE; both when the portal exposes both representations
	Formats     []string             `json:"formats,omitempty"`   // publisher-declared delivery formats such as CSV, JSON, XML
	Desc        string               `json:"desc,omitempty"`
	OfficialAPI *OfficialAPIContract `json:"officialApi,omitempty"` // documented bulk API facts; never a credential or direct call authorization
}

// OfficialAPIContract is the operation-level metadata published by data.go.kr's
// documented catalogue API. It is retained in the release snapshot so ordinary
// users can inspect these facts without applying for the bulk-listing API.
// Publisher URLs remain evidence, not trusted invocation instructions.
type OfficialAPIContract struct {
	APIType      string                 `json:"apiType,omitempty"`
	GuideURL     string                 `json:"guideUrl,omitempty"`
	EndpointURL  string                 `json:"endpointUrl,omitempty"`
	LinkURL      string                 `json:"linkUrl,omitempty"`
	MetaURL      string                 `json:"metaUrl,omitempty"`
	DevApproval  string                 `json:"devApproval,omitempty"`
	ProdApproval string                 `json:"prodApproval,omitempty"`
	EvidenceKind string                 `json:"evidenceKind,omitempty"`
	EvidenceURL  string                 `json:"evidenceUrl,omitempty"`
	Operations   []OfficialAPIOperation `json:"operations,omitempty"`
}

type OfficialAPIOperation struct {
	Sequence     string   `json:"sequence,omitempty"`
	Name         string   `json:"name,omitempty"`
	URL          string   `json:"url,omitempty"`
	RequestNames []string `json:"requestNames,omitempty"`
}

// Service types worth labelling. REST means the portal publishes a spec, so
// describe → call can be driven from it; LINK means the portal only points at the
// publisher, so an agent that applies for one gets a key it cannot use here. Two
// in five OpenAPI datasets are LINK, which is far too many to leave unmarked.
const (
	SvcREST = "REST"
	SvcLINK = "LINK"
	SvcFILE = "FILE"
)

// Catalog is the synced snapshot.
type Catalog struct {
	SyncedAt time.Time `json:"syncedAt"`
	Type     string    `json:"type"` // dataset type swept ("API")
	Source   string    `json:"source,omitempty"`
	Entries  []Entry   `json:"entries"`
}

const (
	SourceOfficial     = "official"
	SourceCombined     = "official-file+web"
	SourceOfficialFile = "official-file"
	SourceWeb          = "web"
)

// Find returns the catalogued Data Node for an exact publicDataPk. The returned
// value is a copy so callers cannot mutate the loaded snapshot.
func (c *Catalog) Find(pk string) (Entry, bool) {
	for _, entry := range c.Entries {
		if entry.PK == pk {
			return entry, true
		}
	}
	return Entry{}, false
}

// Hit is one search result — the compact shape callers get. No description.
type Hit struct {
	PK         string   `json:"pk"`
	Title      string   `json:"title"`
	Org        string   `json:"org,omitempty"`
	ApplyCount int      `json:"applyCount,omitempty"`
	ViewCount  int      `json:"viewCount,omitempty"`
	ModifiedAt string   `json:"modifiedAt,omitempty"`
	SvcType    string   `json:"svcType,omitempty"` // REST/LINK/FILE are all inspected through inspect_dataset
	DataTypes  []string `json:"dataTypes,omitempty"`
	Formats    []string `json:"formats,omitempty"`
	DetailURL  string   `json:"detailUrl,omitempty"`  // official page for FILE-only discovery nodes
	NextAction string   `json:"nextAction,omitempty"` // inspect_dataset
	Matched    int      `json:"matched,omitempty"`    // terms hit — only meaningful when Result.Relaxed
	// Planned searches explain which model-inferred data axis surfaced the row.
	// Preview is deliberately short and opt-in: useful for ideation without
	// returning every full catalogue description to the model context.
	MatchedQuery  string  `json:"matchedQuery,omitempty"`
	Preview       string  `json:"preview,omitempty"`
	SemanticScore float32 `json:"semanticScore,omitempty"`
	Exact         bool    `json:"exact,omitempty"` // all lexical terms for MatchedQuery matched
	// Discovery fields are present only for a structured role axis. They let a
	// caller keep "why this node exists in the plan" separate from lexical or
	// semantic similarity, without expanding the catalogue entry itself.
	Role           string          `json:"role,omitempty"`
	Contribution   string          `json:"contribution,omitempty"`
	EdgeHypothesis *EdgeHypothesis `json:"edgeHypothesis,omitempty"`
}

// EdgeHypothesis is a metadata-only claim about how an Anchor Node and Bridge
// Node might join. Search never upgrades it beyond candidate: official fields
// and sampled values must be inspected through inspect_dataset and call_api.
type EdgeHypothesis struct {
	Kinds        []string `json:"kinds"`               // entity | spatial | temporal | proxy
	ExpectedKeys []string `json:"expectedKeys"`        // e.g. 법정동코드, 기준연월
	Transform    string   `json:"transform,omitempty"` // required when Kinds contains proxy
}

// DiscoveryAxis is one distinct role in a goal-driven search. Query retrieves
// nodes; the remaining fields explain why a result could complement an anchor.
// The anchor role deliberately has no edge or contribution requirement.
type DiscoveryAxis struct {
	Role         string         `json:"role"`
	Query        string         `json:"query"`
	Contribution string         `json:"contribution,omitempty"`
	Edge         EdgeHypothesis `json:"edge,omitempty"`
}

// ConnectionCandidate is a bounded, explicitly unverified pair. It is safe to
// produce from catalogue metadata because Status can only be "candidate" here.
// EvidenceRequired tells the host what must be learned before it may use the
// stronger Verified Connection term.
type ConnectionCandidate struct {
	Anchor            Hit            `json:"anchor"`
	Bridge            Hit            `json:"bridge"`
	BridgeRole        string         `json:"bridgeRole"`
	Edge              EdgeHypothesis `json:"edge"`
	IncrementalValue  string         `json:"incrementalValue"`
	Status            string         `json:"status"`
	EvidenceRequired  []string       `json:"evidenceRequired"`
	CandidateEvidence string         `json:"candidateEvidence,omitempty"`
	ClaimBoundary     string         `json:"claimBoundary"`
}

// ConnectionOptionGroup is the broad, role-diverse pool from which a caller may
// compose connection cards. A group is not a relationship claim: its nodes only
// matched the role query and still need explicit selection. Keeping this pool
// separate from Connections preserves a small high-precision final surface while
// allowing a caller to revisit different combinations without re-running a
// narrower search.
type ConnectionOptionGroup struct {
	Role         string         `json:"role"`
	Contribution string         `json:"contribution"`
	Edge         EdgeHypothesis `json:"edge"`
	Nodes        []Hit          `json:"nodes"`
}

// BridgeSelection is a host/model choice made after it has inspected real
// catalogue hits. PK and WhyCandidate are the only trusted choices: Role,
// IncrementalValue and Edge are echoed for compatibility but the catalogue
// derives the connection contract from the selected hit so it cannot be
// relabelled. Search never turns the top-ranked row into a connection
// automatically; an explicit PK selection is the precision gate.
type BridgeSelection struct {
	PK               string         `json:"pk"`
	Role             string         `json:"role,omitempty"`
	IncrementalValue string         `json:"incrementalValue,omitempty"`
	Edge             EdgeHypothesis `json:"edge,omitempty"`
	WhyCandidate     string         `json:"whyCandidate"`
}

// Abstention is a normal discovery result when structured axes do not produce
// a connection candidate that satisfies the metadata contract.
type Abstention struct {
	Reason string `json:"reason"`
}

// Result is what a search returns. Terms and Relaxed exist so a caller can tell
// what was actually searched for: a query is not always used as written.
type Result struct {
	Mode              string                  `json:"mode,omitempty"`    // lexical | planned
	Intent            string                  `json:"intent,omitempty"`  // original natural-language goal for a planned search
	Queries           []string                `json:"queries,omitempty"` // concrete semantic axes inferred by the caller model
	Terms             []string                `json:"terms,omitempty"`   // what a single lexical query was reduced to
	Total             int                     `json:"total"`             // entries matched
	Relaxed           bool                    `json:"relaxed,omitempty"` // true = at least one query had to OR its terms
	Hits              []Hit                   `json:"hits"`
	Semantic          *SemanticInfo           `json:"semantic,omitempty"`
	Anchors           []Hit                   `json:"anchors,omitempty"`
	ConnectionOptions []ConnectionOptionGroup `json:"connectionOptions,omitempty"`
	Connections       []ConnectionCandidate   `json:"connections,omitempty"`
	Warnings          []string                `json:"warnings,omitempty"`
	Abstention        *Abstention             `json:"abstention,omitempty"`
}

const (
	SearchModeLexical = "lexical"
	SearchModePlanned = "planned"
	SearchModeHybrid  = "hybrid"

	RankDemand   = "demand"
	RankRecent   = "recent"
	RankBalanced = "balanced"

	// MaxSearchLimit preserves progressive disclosure: search returns a compact
	// candidate set and inspect_dataset expands one selection. It also bounds model
	// context when tools are invoked directly with untrusted arguments.
	MaxSearchLimit = 100
)

// QueryPlan is the boundary between semantic interpretation and deterministic
// retrieval. The MCP host model turns an open-ended user goal into a few
// concrete Concepts; the catalogue executes, merges, explains and diversifies
// them without embedding-provider credentials or a hard-coded synonym tree.
//
// Ranking controls candidates within each concept. Balanced alternates proven
// demand with recently modified data so exploratory searches can surface both
// usable incumbents and underused new possibilities.
type QueryPlan struct {
	Intent          string
	Concepts        []string
	Limit           int
	RESTOnly        bool
	IncludePreviews bool
	Ranking         string // balanced (default) | demand | recent
	// Axes is the structured form used for cross-domain connection discovery.
	// Concepts remains supported for existing callers and ordinary broad search.
	Axes             []DiscoveryAxis
	AnchorPKs        []string
	BridgeSelections []BridgeSelection
	MaxConnections   int
}

type lexicalScored struct {
	entry   *Entry
	matched int
	inTitle int
}

type lexicalTopHeap struct {
	rows   []lexicalScored
	better func(lexicalScored, lexicalScored) bool
}

func (h lexicalTopHeap) Len() int { return len(h.rows) }
func (h lexicalTopHeap) Less(i, j int) bool {
	return h.better(h.rows[j], h.rows[i])
}
func (h lexicalTopHeap) Swap(i, j int) { h.rows[i], h.rows[j] = h.rows[j], h.rows[i] }
func (h *lexicalTopHeap) Push(value any) {
	h.rows = append(h.rows, value.(lexicalScored))
}
func (h *lexicalTopHeap) Pop() any {
	old := h.rows
	last := old[len(old)-1]
	h.rows = old[:len(old)-1]
	return last
}

func retainLexicalTop(h *lexicalTopHeap, candidate lexicalScored, limit int) {
	if h.Len() < limit {
		heap.Push(h, candidate)
		return
	}
	if h.better(candidate, h.rows[0]) {
		h.rows[0] = candidate
		heap.Fix(h, 0)
	}
}

const (
	ConnectionStatusCandidate = "candidate"
	// MaxConnectionCandidates keeps exploratory discovery reviewable. A larger
	// list encourages the host model to pass weak, merely novel pairs instead of
	// abstaining when the join contract is not supported by metadata.
	MaxConnectionCandidates = 3
	// MaxOptionsPerRole widens the composition space without creating an
	// all-pairs graph. With at most seven Bridge roles this is at most 21 nodes.
	MaxOptionsPerRole = 3
	maxAnchorNodes    = 3
)

// StaleAfter is when a synced catalogue should be refreshed. The portal adds and
// retires datasets continuously, so a month-old snapshot is still useful for
// orientation but should not be trusted as complete.
const StaleAfter = 14 * 24 * time.Hour

func path() (string, error) {
	dir, err := portal.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "catalog.json"), nil
}

// Load reads the synced catalogue. Returns ErrNotSynced when there is none.
func Load() (*Catalog, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, ErrNotSynced
	}
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ErrNotSynced means no catalogue has been synced yet.
var ErrNotSynced = fmt.Errorf("카탈로그가 아직 없습니다 — `odeduck catalog sync` 를 먼저 실행하세요")

// Save writes the catalogue.
func (c *Catalog) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return atomicWrite(p, "catalog-*.tmp", 0o644, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// Stale reports whether the snapshot is old enough to warrant a re-sync.
func (c *Catalog) Stale() bool { return time.Since(c.SyncedAt) > StaleAfter }

// Age is how long ago the catalogue was synced.
func (c *Catalog) Age() time.Duration { return time.Since(c.SyncedAt) }

// CoversType reports whether this snapshot already contains the requested sync
// scope. It keeps --if-stale from treating a fresh legacy API-only snapshot as a
// reason to skip the first ALL sync after upgrading.
func (c *Catalog) CoversType(raw string) bool {
	_, requested, err := catalogTypes(raw)
	if err != nil {
		return false
	}
	current := strings.ToUpper(strings.TrimSpace(c.Type))
	if current == "ALL" {
		return true
	}
	return current != "" && current == requested
}

// PreserveOnAutoSync reports whether an automatic refresh would replace a
// broader local catalogue with a materially weaker fallback. Explicit --source
// requests remain authoritative; this guard exists only for the unattended
// default refresh path.
func PreserveOnAutoSync(current, candidate *Catalog) bool {
	if current == nil || candidate == nil {
		return false
	}
	if current.CoversType("ALL") && !candidate.CoversType("ALL") {
		return true
	}
	if sourceRank(current.Source) > sourceRank(candidate.Source) {
		return true
	}
	currentCoverage := catalogueCoverage(current)
	candidateCoverage := catalogueCoverage(candidate)
	for _, pair := range [][2]int{
		{currentCoverage.entries, candidateCoverage.entries},
		{currentCoverage.api, candidateCoverage.api},
		{currentCoverage.file, candidateCoverage.file},
		{currentCoverage.officialContracts, candidateCoverage.officialContracts},
	} {
		// Monthly removals are legitimate. A loss above 10% in one automatic
		// refresh is more likely a source/parser regression and requires an
		// explicit source choice instead of silently replacing local state.
		if pair[0] > 0 && pair[1]*10 < pair[0]*9 {
			return true
		}
	}
	return false
}

type coverageCounts struct {
	entries, api, file, officialContracts int
}

func catalogueCoverage(candidate *Catalog) coverageCounts {
	var coverage coverageCounts
	if candidate == nil {
		return coverage
	}
	coverage.entries = len(candidate.Entries)
	for _, entry := range candidate.Entries {
		for _, delivery := range entry.DataTypes {
			switch strings.ToUpper(strings.TrimSpace(delivery)) {
			case "API":
				coverage.api++
			case "FILE":
				coverage.file++
			}
		}
		if entry.OfficialAPI != nil {
			coverage.officialContracts++
		}
	}
	return coverage
}

// Sync sweeps the portal's dataset list into a catalogue. perPage is honoured by
// the portal, so a large page size turns thousands of datasets into tens of
// requests. progress, when non-nil, is called with the running total.
//
// API sync uses one unfiltered sweep followed by REST/LINK labelling sweeps. FILE
// uses one unfiltered sweep and may merge into the same PK when the portal exposes
// more than one representation. The unfiltered API sweep defines membership even
// if the portal introduces a service type nobody here has heard of; filtered
// sweeps only label it. Unknown is preserved rather than guessed, because a guess
// could send an agent to apply for something it cannot call.
func Sync(ctx context.Context, pc *portal.Client, dType string, perPage int, progress func(int)) (*Catalog, error) {
	return NewWebSource(pc).Sync(ctx, dType, perPage, progress)
}

// WebSource is the unauthenticated compatibility Adapter for the public portal
// pages. It remains useful when the official catalogue operation has not been
// approved for a user's account.
type WebSource struct {
	portal *portal.Client
}

func NewWebSource(client *portal.Client) *WebSource { return &WebSource{portal: client} }

func (s *WebSource) Sync(ctx context.Context, dType string, perPage int, progress func(int)) (*Catalog, error) {
	pc := s.portal
	if pc == nil {
		return nil, fmt.Errorf("web 카탈로그 source가 설정되지 않았습니다")
	}
	if perPage <= 0 {
		perPage = 200
	}
	types, snapshotType, err := catalogTypes(dType)
	if err != nil {
		return nil, err
	}
	seen := map[string]Entry{}
	// sweep pages until the portal stops producing datasets it has not already
	// returned in this sweep. Termination is decided by pagination progress alone —
	// never by whether visit did anything with a row — so a labelling pass cannot
	// cut itself short on a page whose rows were all handled already.
	sweep := func(dataType, svcType string, visit func(portal.Dataset)) error {
		inSweep := map[string]bool{}
		for page := 1; ; page++ {
			batch, err := pc.SearchDatasets(ctx, portal.SearchOptions{
				Type: dataType, SvcType: svcType, Page: page, PerPage: perPage,
			})
			if err != nil {
				return fmt.Errorf("%s %d페이지 수집 실패 (svcType=%q): %w", dataType, page, svcType, err)
			}
			fresh := 0
			for _, d := range batch {
				if inSweep[d.PublicDataPk] {
					continue
				}
				inSweep[d.PublicDataPk] = true
				fresh++
				visit(d)
			}
			if progress != nil {
				progress(len(seen))
			}
			if fresh == 0 {
				return nil
			}
		}
	}

	for _, dataType := range types {
		switch dataType {
		case "API":
			if err := sweep(dataType, "", func(d portal.Dataset) {
				seen[d.PublicDataPk] = mergeEntry(seen[d.PublicDataPk], entryFromDataset(d, dataType, ""))
			}); err != nil {
				return nil, err
			}
			for _, svc := range []string{SvcREST, SvcLINK} {
				if err := sweep(dataType, svc, func(d portal.Dataset) {
					entry := entryFromDataset(d, dataType, svc)
					seen[d.PublicDataPk] = mergeEntry(seen[d.PublicDataPk], entry)
				}); err != nil {
					return nil, err
				}
			}
		case "FILE":
			if err := sweep(dataType, "", func(d portal.Dataset) {
				seen[d.PublicDataPk] = mergeEntry(seen[d.PublicDataPk], entryFromDataset(d, dataType, SvcFILE))
			}); err != nil {
				return nil, err
			}
		}
	}

	c := &Catalog{SyncedAt: time.Now().UTC(), Type: snapshotType, Source: SourceWeb}
	for _, e := range seen {
		c.Entries = append(c.Entries, e)
	}
	sortEntries(c.Entries)
	return c, nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ApplyCount != entries[j].ApplyCount {
			return entries[i].ApplyCount > entries[j].ApplyCount
		}
		if entries[i].ViewCount != entries[j].ViewCount {
			return entries[i].ViewCount > entries[j].ViewCount
		}
		return entries[i].PK < entries[j].PK
	})
}

func catalogTypes(raw string) ([]string, string, error) {
	switch value := strings.ToUpper(strings.TrimSpace(raw)); value {
	case "", "ALL":
		return []string{"API", "FILE"}, "ALL", nil
	case "API", "FILE":
		return []string{value}, value, nil
	default:
		return nil, "", fmt.Errorf("지원하지 않는 카탈로그 유형 %q — API, FILE, ALL 중 하나를 사용하세요", raw)
	}
}

func entryFromDataset(d portal.Dataset, dataType, svcType string) Entry {
	dataTypes := []string{dataType}
	if dataType == "FILE" && d.HasOpenAPI {
		dataTypes = append(dataTypes, "API")
	}
	return Entry{
		PK: d.PublicDataPk, Title: d.Title, Org: d.Org, OrgType: d.OrgType,
		Category: d.Category, ApplyCount: d.ApplyCount, ViewCount: d.ViewCount,
		ModifiedAt: d.ModifiedAt, SvcType: svcType, DataTypes: dataTypes,
		Formats: append([]string(nil), d.Formats...), Desc: d.Description,
	}
}

func mergeEntry(current, incoming Entry) Entry {
	if current.PK == "" {
		incoming.DataTypes = appendUniqueFold(nil, incoming.DataTypes...)
		incoming.Formats = appendUniqueFold(nil, incoming.Formats...)
		return incoming
	}
	current.DataTypes = appendUniqueFold(current.DataTypes, incoming.DataTypes...)
	current.Formats = appendUniqueFold(current.Formats, incoming.Formats...)
	current.OfficialAPI = mergeOfficialAPI(current.OfficialAPI, incoming.OfficialAPI)
	if current.Title == "" {
		current.Title = incoming.Title
	}
	if current.Org == "" {
		current.Org = incoming.Org
	}
	if current.OrgType == "" {
		current.OrgType = incoming.OrgType
	}
	if current.Category == "" {
		current.Category = incoming.Category
	}
	if len([]rune(incoming.Desc)) > len([]rune(current.Desc)) {
		current.Desc = incoming.Desc
	}
	if incoming.ApplyCount > current.ApplyCount {
		current.ApplyCount = incoming.ApplyCount
	}
	if incoming.ViewCount > current.ViewCount {
		current.ViewCount = incoming.ViewCount
	}
	if incoming.ModifiedAt > current.ModifiedAt {
		current.ModifiedAt = incoming.ModifiedAt
	}
	// A callable/inspectable API contract is more specific than FILE. FILE stays
	// in DataTypes and Formats, so the download representation is not lost.
	if serviceTypeRank(incoming.SvcType) > serviceTypeRank(current.SvcType) {
		current.SvcType = incoming.SvcType
	}
	return current
}

func serviceTypeRank(value string) int {
	switch value {
	case SvcREST:
		return 3
	case SvcLINK:
		return 2
	case SvcFILE:
		return 1
	default:
		return 0
	}
}

func mergeOfficialAPI(current, incoming *OfficialAPIContract) *OfficialAPIContract {
	if incoming == nil {
		return current
	}
	if current == nil {
		copy := *incoming
		copy.Operations = append([]OfficialAPIOperation(nil), incoming.Operations...)
		return &copy
	}
	for target, value := range map[*string]string{
		&current.APIType: incoming.APIType, &current.GuideURL: incoming.GuideURL,
		&current.EndpointURL: incoming.EndpointURL, &current.LinkURL: incoming.LinkURL,
		&current.MetaURL: incoming.MetaURL, &current.DevApproval: incoming.DevApproval,
		&current.ProdApproval: incoming.ProdApproval, &current.EvidenceKind: incoming.EvidenceKind,
		&current.EvidenceURL: incoming.EvidenceURL,
	} {
		if *target == "" && value != "" {
			*target = value
		}
	}
	seen := make(map[string]bool, len(current.Operations)+len(incoming.Operations))
	for _, operation := range current.Operations {
		seen[officialOperationKey(operation)] = true
	}
	for _, operation := range incoming.Operations {
		key := officialOperationKey(operation)
		if key == "\x00\x00" || seen[key] {
			continue
		}
		seen[key] = true
		operation.RequestNames = appendUniqueExact(nil, operation.RequestNames...)
		current.Operations = append(current.Operations, operation)
	}
	return current
}

func officialOperationKey(operation OfficialAPIOperation) string {
	return operation.Sequence + "\x00" + operation.Name + "\x00" + operation.URL
}

func appendUniqueExact(dst []string, values ...string) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	out := make([]string, 0, len(dst)+len(values))
	for _, value := range append(append([]string(nil), dst...), values...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func appendUniqueFold(dst []string, values ...string) []string {
	seen := make(map[string]bool, len(dst)+len(values))
	out := make([]string, 0, len(dst)+len(values))
	for _, value := range append(append([]string(nil), dst...), values...) {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// particles are Korean grammatical endings that a natural-sounding query carries
// and a catalogue title never does: nobody publishes "폭염에" or "광진구에서".
// Longest first, so 에서 is tried before 에.
var particles = []string{"에서는", "에서", "에게", "으로", "부터", "까지", "이나", "라도",
	"보다", "만큼", "처럼", "및", "의", "에", "을", "를", "은", "는", "이", "가", "와", "과", "도", "로"}

// fillers carry no domain meaning but appear in how people ask. Left in, they
// would rank titles by whether they happen to contain the word "데이터". Matching
// is on the whole token, so 대한 here never touches 대한민국.
var fillers = map[string]bool{
	"데이터": true, "자료": true, "정보": true, "정보를": true, "좋은": true, "관련": true, "관한": true,
	"있는": true, "찾아줘": true, "알려줘": true, "무엇": true, "어떤": true, "필요한": true,
	"목록": true, "리스트": true, "api": true, "그리고": true, "또는": true,
	// connectives — "폭염으로 인한", "청소년을 위한", "고령화에 따른"
	"인한": true, "대한": true, "위한": true, "통한": true, "따른": true, "관하여": true,
	"있나": true, "없나": true, "싶어": true, "주세요": true, "해줘": true, "좀": true,
}

// queryTerms reduces a query to the words worth matching. It is deliberately not
// a morphological analyser: stripping a trailing particle and dropping fillers is
// enough to make "폭염에 취약한 고령자" behave like "폭염 취약 고령자", and anything
// cleverer would need a dictionary this tool has no reason to carry.
func queryTerms(query string) []string {
	var out []string
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		// Agent plans commonly join related government terms with ·, / or -.
		// Treat punctuation as a separator so "공매·압류재산" is not one
		// impossible literal token. Symbols are separators for the same reason.
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	for _, w := range words {
		if w == "" || fillers[w] {
			continue
		}
		for _, p := range particles {
			// Keep at least two characters: trimming 가 off 고가 leaves nothing usable.
			if trimmed := strings.TrimSuffix(w, p); trimmed != w && len([]rune(trimmed)) >= 2 {
				w = trimmed
				break
			}
		}
		if fillers[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// Search finds datasets for a query written the way people and agents actually
// write one — "폭염에 취약한 고령자", not "폭염 고령". Terms are reduced (particles
// trimmed, fillers dropped), then matched against title, publisher, category and
// description.
//
// Precision first, recall as a fallback: entries matching *every* term win, and
// only when that finds nothing are the terms ORed and ranked by how many hit.
// The fallback is reported rather than hidden — a relaxed result answers a
// different question from the one asked, and the caller has to know that.
// Within a tier, ranking uses utilization applications first and portal views
// second. Views preserve a useful demand signal for FILE datasets, which do not
// have application counts.
//
// restOnly drops everything the portal does not report as REST. It is an explicit
// precision filter; product defaults include REST, LINK and FILE so discovery is
// broad, while each hit's next action preserves the invocation boundary.
func (c *Catalog) Search(query string, limit int, restOnly bool) Result {
	return c.search(query, normalizeSearchLimit(limit), restOnly)
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > MaxSearchLimit {
		return MaxSearchLimit
	}
	return limit
}

// search is the demand-ranked internal candidate retriever. Planned search uses
// searchRanked to inspect every match while materializing only bounded results.
func (c *Catalog) search(query string, limit int, restOnly bool) Result {
	return c.searchRanked(query, limit, restOnly, RankDemand, false, nil)
}

func (c *Catalog) searchRanked(query string, limit int, restOnly bool, ranking string, plannedPrecision bool, observe func(string)) Result {
	if limit <= 0 {
		limit = 20
	}
	terms := queryTerms(query)
	res := Result{Mode: SearchModeLexical, Terms: terms}
	scoreEntry := func(entry *Entry) lexicalScored {
		name := strings.ToLower(entry.Title + " " + entry.Org + " " + entry.Category)
		desc := strings.ToLower(entry.Desc)
		s := lexicalScored{entry: entry}
		for _, term := range terms {
			switch {
			case strings.Contains(name, term):
				s.matched++
				s.inTitle++
			case strings.Contains(desc, term):
				s.matched++
			}
		}
		return s
	}
	eligible := func(entry *Entry) bool {
		return !restOnly || entry.SvcType == SvcREST
	}

	strictExists := len(terms) == 0
	if !strictExists {
		for i := range c.Entries {
			e := &c.Entries[i]
			if eligible(e) && scoreEntry(e).matched == len(terms) {
				strictExists = true
				break
			}
		}
	}
	if !strictExists && len(terms) > 1 {
		res.Relaxed = true
	}

	betterDemand := func(left, right lexicalScored) bool {
		// A match in the compact identity fields (title, publisher, category)
		// carries more intent than prose in a long description. In particular,
		// this keeps a named place or agency from being displaced by another
		// region whose description happens to repeat generic workflow words.
		leftRelevance := left.matched + 2*left.inTitle
		rightRelevance := right.matched + 2*right.inTitle
		if leftRelevance != rightRelevance {
			return leftRelevance > rightRelevance
		}
		if left.inTitle != right.inTitle {
			return left.inTitle > right.inTitle
		}
		if left.matched != right.matched {
			return left.matched > right.matched
		}
		return moreDemandedEntry(left.entry, right.entry)
	}
	betterRecent := func(left, right lexicalScored) bool {
		leftRelevance := left.matched + 2*left.inTitle
		rightRelevance := right.matched + 2*right.inTitle
		if leftRelevance != rightRelevance {
			return leftRelevance > rightRelevance
		}
		if left.inTitle != right.inTitle {
			return left.inTitle > right.inTitle
		}
		if left.matched != right.matched {
			return left.matched > right.matched
		}
		if left.entry.ModifiedAt != right.entry.ModifiedAt {
			return left.entry.ModifiedAt > right.entry.ModifiedAt
		}
		return moreDemandedEntry(left.entry, right.entry)
	}
	demand := &lexicalTopHeap{better: betterDemand}
	recent := &lexicalTopHeap{better: betterRecent}
	heap.Init(demand)
	heap.Init(recent)
	for i := range c.Entries {
		e := &c.Entries[i]
		if restOnly && e.SvcType != SvcREST {
			continue
		}
		s := scoreEntry(e)
		if strictExists {
			if s.matched != len(terms) {
				continue
			}
		} else if s.matched == 0 {
			continue
		}
		if plannedPrecision && res.Relaxed && len(terms) >= 3 && s.matched < (len(terms)+1)/2 {
			continue
		}
		res.Total++
		if observe != nil {
			observe(s.entry.PK)
		}
		if ranking != RankRecent {
			retainLexicalTop(demand, s, limit)
		}
		if ranking == RankRecent || ranking == RankBalanced {
			retainLexicalTop(recent, s, limit)
		}
	}
	makeHits := func(rows []lexicalScored, better func(lexicalScored, lexicalScored) bool) []Hit {
		sort.SliceStable(rows, func(i, j int) bool { return better(rows[i], rows[j]) })
		hits := make([]Hit, 0, len(rows))
		for _, s := range rows {
			h := hitFromEntry(s.entry)
			h.Exact = !res.Relaxed && len(terms) > 0
			if res.Relaxed {
				h.Matched = s.matched
			}
			hits = append(hits, h)
		}
		return hits
	}

	switch ranking {
	case RankRecent:
		res.Hits = makeHits(recent.rows, betterRecent)
	case RankBalanced:
		res.Hits = trimHits(interleaveHits(makeHits(demand.rows, betterDemand), makeHits(recent.rows, betterRecent)), limit)
	default:
		res.Hits = makeHits(demand.rows, betterDemand)
	}
	return res
}

// SearchPlan executes up to eight model-inferred concept queries and returns a
// round-robin merge. This is intentionally different from silently expanding a
// phrase with a synonym map: the language model can reason about new industries,
// causal indicators and adjacent opportunities we did not anticipate, while the
// local catalogue remains cheap, complete and reproducible.
func (c *Catalog) SearchPlan(plan QueryPlan) Result {
	plan.Limit = normalizeSearchLimit(plan.Limit)
	return c.searchPlan(plan)
}

func (c *Catalog) searchPlan(plan QueryPlan) Result {
	limit := plan.Limit
	if limit <= 0 {
		limit = 20
	}
	axes, axisWarnings := normalizeDiscoveryAxes(plan.Axes, 8)
	concepts := uniqueQueries(plan.Concepts, 8)
	if len(axes) > 0 {
		concepts = make([]string, 0, len(axes))
		for _, axis := range axes {
			concepts = append(concepts, axis.Query)
		}
	}
	if len(concepts) == 0 {
		return c.search(plan.Intent, limit, plan.RESTOnly)
	}
	ranking := plan.Ranking
	if ranking != RankDemand && ranking != RankRecent {
		ranking = RankBalanced
	}

	entries := make(map[string]*Entry, len(c.Entries))
	for i := range c.Entries {
		entries[c.Entries[i].PK] = &c.Entries[i]
	}

	buckets := make([][]Hit, 0, len(concepts))
	all := make(map[string]bool)
	anyRelaxed := false
	candidateLimit := limit
	if candidateLimit < MaxOptionsPerRole {
		candidateLimit = MaxOptionsPerRole
	}
	for queryIndex, query := range concepts {
		found := c.searchRanked(query, candidateLimit, plan.RESTOnly, ranking, true, func(pk string) {
			all[pk] = true
		})
		anyRelaxed = anyRelaxed || found.Relaxed
		hits := append([]Hit(nil), found.Hits...)
		for i := range hits {
			hits[i].MatchedQuery = query
			if queryIndex < len(axes) {
				hits[i].Role = axes[queryIndex].Role
				hits[i].Contribution = axes[queryIndex].Contribution
				if len(axes[queryIndex].Edge.Kinds) > 0 || len(axes[queryIndex].Edge.ExpectedKeys) > 0 {
					edge := axes[queryIndex].Edge
					hits[i].EdgeHypothesis = &edge
				}
			}
			if plan.IncludePreviews {
				hits[i].Preview = descriptionPreview(entries[hits[i].PK], 180)
			}
		}
		buckets = append(buckets, hits)
	}

	res := Result{
		Mode: SearchModePlanned, Intent: strings.TrimSpace(plan.Intent),
		Queries: concepts, Total: len(all), Relaxed: anyRelaxed, Warnings: axisWarnings,
	}
	var optionHits []Hit
	for _, hits := range buckets {
		optionHits = append(optionHits, hits...)
	}
	res.ConnectionOptions = buildConnectionOptionsFromHits(optionHits, plan.AnchorPKs, MaxOptionsPerRole)
	res.Hits = roundRobinHits(buckets, limit)
	finalizeConnectionCandidates(plan, &res, entries)
	return res
}

func entryDataTypes(entry *Entry) []string {
	if entry == nil {
		return nil
	}
	if len(entry.DataTypes) > 0 {
		return append([]string(nil), entry.DataTypes...)
	}
	// Old saved catalogues predate the per-entry field. Infer only what their
	// service label proves so an upgrade does not require an immediate re-sync.
	switch entry.SvcType {
	case SvcREST, SvcLINK:
		return []string{"API"}
	case SvcFILE:
		return []string{"FILE"}
	default:
		return nil
	}
}

func hitFromEntry(entry *Entry) Hit {
	if entry == nil {
		return Hit{}
	}
	return Hit{
		PK: entry.PK, Title: entry.Title, Org: entry.Org,
		ApplyCount: entry.ApplyCount, ViewCount: entry.ViewCount,
		ModifiedAt: entry.ModifiedAt, SvcType: entry.SvcType,
		DataTypes: entryDataTypes(entry), Formats: append([]string(nil), entry.Formats...),
		DetailURL: entryDetailURL(entry), NextAction: entryNextAction(entry),
	}
}

func entryDetailURL(entry *Entry) string {
	if entry != nil && entry.PK != "" && (entry.SvcType == SvcFILE || containsFold(entry.DataTypes, "FILE")) {
		return portal.BaseURL + "/data/" + entry.PK + "/fileData.do"
	}
	return ""
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func entryNextAction(entry *Entry) string {
	if entry == nil {
		return ""
	}
	return "inspect_dataset"
}

func buildConnectionOptionsFromHits(hits []Hit, anchorPKs []string, perRole int) []ConnectionOptionGroup {
	if len(anchorPKs) == 0 || perRole <= 0 {
		return nil
	}
	anchors := map[string]bool{}
	for _, pk := range anchorPKs {
		anchors[strings.TrimSpace(pk)] = true
	}
	groupIndex := map[string]int{}
	seenNodes := map[string]bool{}
	var groups []ConnectionOptionGroup
	for _, hit := range hits {
		role := compactText(hit.Role, 40)
		if anchors[hit.PK] || seenNodes[hit.PK] || role == "" || strings.EqualFold(role, "anchor") || hit.EdgeHypothesis == nil {
			continue
		}
		axis := DiscoveryAxis{Role: role, Query: hit.MatchedQuery, Contribution: hit.Contribution, Edge: *hit.EdgeHypothesis}
		if !validCandidateAxis(axis) {
			continue
		}
		key := strings.ToLower(role)
		index, exists := groupIndex[key]
		if !exists {
			index = len(groups)
			groupIndex[key] = index
			groups = append(groups, ConnectionOptionGroup{Role: role, Contribution: hit.Contribution, Edge: *hit.EdgeHypothesis})
		}
		if len(groups[index].Nodes) == perRole {
			continue
		}
		seenNodes[hit.PK] = true
		groups[index].Nodes = append(groups[index].Nodes, hit)
	}
	return groups
}

func normalizeDiscoveryAxes(raw []DiscoveryAxis, max int) ([]DiscoveryAxis, []string) {
	if max <= 0 {
		max = 8
	}
	seen := map[string]bool{}
	seenRoles := map[string]bool{}
	var axes []DiscoveryAxis
	var warnings []string
	for _, item := range raw {
		axis := DiscoveryAxis{
			Role:         compactText(item.Role, 40),
			Query:        compactText(item.Query, 80),
			Contribution: compactText(item.Contribution, 240),
			Edge: EdgeHypothesis{
				Kinds:        normalizeEdgeKinds(item.Edge.Kinds),
				ExpectedKeys: uniqueCompact(item.Edge.ExpectedKeys, 4, 80),
				Transform:    compactText(item.Edge.Transform, 240),
			},
		}
		if axis.Query == "" || axis.Role == "" {
			warnings = append(warnings, "role 또는 query가 비어 있는 discovery axis를 제외했습니다")
			continue
		}
		key := strings.ToLower(axis.Role + "\x00" + axis.Query)
		roleKey := strings.ToLower(axis.Role)
		if seen[key] || seenRoles[roleKey] {
			warnings = append(warnings, fmt.Sprintf("중복 discovery role을 제외했습니다: %s", axis.Role))
			continue
		}
		seen[key] = true
		seenRoles[roleKey] = true
		axes = append(axes, axis)
		if len(axes) == max {
			break
		}
	}
	return axes, warnings
}

func normalizeEdgeKinds(raw []string) []string {
	allowed := map[string]bool{"entity": true, "spatial": true, "temporal": true, "proxy": true}
	var out []string
	seen := map[string]bool{}
	for _, kind := range raw {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if !allowed[kind] || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}

func uniqueCompact(raw []string, maxItems, maxRunes int) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range raw {
		value = compactText(value, maxRunes)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if len(out) == maxItems {
			break
		}
	}
	return out
}

func compactText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func anchorHits(current []Hit, rawPKs []string) []Hit {
	eligible := make(map[string]Hit, len(current))
	for _, hit := range current {
		if strings.EqualFold(strings.TrimSpace(hit.Role), "anchor") {
			eligible[hit.PK] = hit
		}
	}
	seen := map[string]bool{}
	anchors := make([]Hit, 0, maxAnchorNodes)
	for _, raw := range rawPKs {
		pk := strings.TrimSpace(raw)
		hit, exists := eligible[pk]
		if !exists || seen[pk] {
			continue
		}
		seen[pk] = true
		hit.Role = "anchor"
		anchors = append(anchors, hit)
		if len(anchors) == maxAnchorNodes {
			break
		}
	}
	return anchors
}

func finalizeConnectionCandidates(plan QueryPlan, res *Result, entries map[string]*Entry) {
	res.Anchors = nil
	res.Connections = nil
	res.Abstention = nil
	if len(plan.AnchorPKs) == 0 {
		return
	}
	res.Anchors = anchorHits(res.Hits, plan.AnchorPKs)
	if len(res.Anchors) == 0 {
		res.ConnectionOptions = nil
		res.Abstention = &Abstention{Reason: "요청한 Anchor PK가 현재 hits에서 anchor 역할로 회수되지 않았습니다"}
		return
	}
	if len(plan.BridgeSelections) == 0 {
		return
	}
	allowed := make(map[string]Hit, len(res.Hits)+len(res.ConnectionOptions)*MaxOptionsPerRole)
	for _, group := range res.ConnectionOptions {
		for _, hit := range group.Nodes {
			if _, exists := allowed[hit.PK]; !exists {
				allowed[hit.PK] = hit
			}
		}
	}
	for _, hit := range res.Hits {
		if _, exists := allowed[hit.PK]; !exists {
			allowed[hit.PK] = hit
		}
	}
	maxConnections := plan.MaxConnections
	if maxConnections <= 0 || maxConnections > MaxConnectionCandidates {
		maxConnections = MaxConnectionCandidates
	}
	res.Connections = buildSelectedConnections(res.Anchors, entries, allowed, plan.BridgeSelections, maxConnections)
	if len(res.Connections) == 0 {
		res.Abstention = &Abstention{Reason: "선택된 Bridge가 현재 hits에 없거나 검색 hit의 역할·후보 근거·예상 결합키·Incremental Value 계약을 만족하지 않습니다"}
	}
}

func buildSelectedConnections(anchors []Hit, entries map[string]*Entry, allowed map[string]Hit, selections []BridgeSelection, limit int) []ConnectionCandidate {
	if len(anchors) == 0 || limit <= 0 {
		return nil
	}
	seenPKs, seenRoles := map[string]bool{}, map[string]bool{}
	var out []ConnectionCandidate
	for _, raw := range selections {
		pk := strings.TrimSpace(raw.PK)
		hit, visible := allowed[pk]
		entry := entries[pk]
		whyCandidate := compactText(raw.WhyCandidate, 240)
		if entry == nil || !visible || seenPKs[pk] || whyCandidate == "" || hit.EdgeHypothesis == nil {
			continue
		}
		edge := EdgeHypothesis{
			Kinds:        normalizeEdgeKinds(hit.EdgeHypothesis.Kinds),
			ExpectedKeys: uniqueCompact(hit.EdgeHypothesis.ExpectedKeys, 4, 80),
			Transform:    compactText(hit.EdgeHypothesis.Transform, 240),
		}
		role := compactText(hit.Role, 40)
		incrementalValue := compactText(hit.Contribution, 240)
		axis := DiscoveryAxis{Role: role, Query: entry.Title, Contribution: incrementalValue, Edge: edge}
		roleKey := strings.ToLower(role)
		if roleKey == "anchor" || seenRoles[roleKey] || !validCandidateAxis(axis) {
			continue
		}
		seenPKs[pk], seenRoles[roleKey] = true, true
		bridge := hitFromEntry(entry)
		bridge.Role = role
		bridge.Contribution = incrementalValue
		bridge.EdgeHypothesis = &edge
		for _, anchor := range anchors {
			if anchor.PK == bridge.PK {
				continue
			}
			out = append(out, ConnectionCandidate{
				Anchor: anchor, Bridge: bridge, BridgeRole: role, Edge: edge,
				IncrementalValue: incrementalValue, Status: ConnectionStatusCandidate,
				EvidenceRequired: evidenceFor(edge, anchor, bridge), CandidateEvidence: whyCandidate,
				ClaimBoundary: "실제 검색 결과에서 선택했지만 명세·값 교집합·사업 또는 연구 성과는 아직 검증하지 않았습니다",
			})
			break
		}
		if len(out) == limit {
			break
		}
	}
	return out
}

func validCandidateAxis(axis DiscoveryAxis) bool {
	if axis.Role == "" || axis.Query == "" || axis.Contribution == "" || len(axis.Edge.Kinds) == 0 || len(axis.Edge.ExpectedKeys) == 0 {
		return false
	}
	for _, kind := range axis.Edge.Kinds {
		if kind == "proxy" && axis.Edge.Transform == "" {
			return false
		}
	}
	return true
}

func evidenceFor(edge EdgeHypothesis, nodes ...Hit) []string {
	hasFile := false
	for _, node := range nodes {
		if node.SvcType == SvcFILE || containsFold(node.DataTypes, "FILE") {
			hasFile = true
			break
		}
	}
	var evidence []string
	if hasFile {
		evidence = []string{
			"FILE 노드의 detailUrl에서 공식 컬럼·갱신일·coverage·다운로드 조건 확인",
			"API는 inspect_dataset/call_api, FILE은 bounded 표본으로 실제 결합키와 양방향 match rate 확인",
			"uniqueness·null rate·join cardinality·duplicate expansion 확인",
		}
	} else {
		evidence = []string{
			"inspect_dataset에서 실제 출력 field 이름·type·code namespace 확인",
			"공통 지역·기간 slice의 call_api 표본에서 matched keys와 양방향 match rate 확인",
			"uniqueness·null rate·join cardinality·duplicate expansion 확인",
		}
	}
	for _, kind := range edge.Kinds {
		switch kind {
		case "spatial":
			evidence = append(evidence, "공간 coverage·좌표계 또는 행정구역 code version·grain 확인")
		case "temporal":
			evidence = append(evidence, "시간 coverage·timestamp 의미·timezone·resolution 확인")
		case "entity":
			evidence = append(evidence, "식별자 namespace·version·정규화 규칙 확인")
		case "proxy":
			evidence = append(evidence, "proxy 변환식의 손실률과 오매칭률 확인")
		}
	}
	return evidence
}

func uniqueQueries(queries []string, max int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		key := strings.ToLower(query)
		if query == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, query)
		if len(out) == max {
			break
		}
	}
	return out
}

func moreDemandedHit(left, right Hit) bool {
	if left.ApplyCount != right.ApplyCount {
		return left.ApplyCount > right.ApplyCount
	}
	if left.ViewCount != right.ViewCount {
		return left.ViewCount > right.ViewCount
	}
	return left.PK < right.PK
}

func interleaveHits(primary, secondary []Hit) []Hit {
	out := make([]Hit, 0, len(primary))
	seen := make(map[string]bool, len(primary))
	for i := 0; i < len(primary) || i < len(secondary); i++ {
		for _, hits := range [][]Hit{primary, secondary} {
			if i >= len(hits) || seen[hits[i].PK] {
				continue
			}
			seen[hits[i].PK] = true
			out = append(out, hits[i])
		}
	}
	return out
}

func descriptionPreview(entry *Entry, maxRunes int) string {
	if entry == nil {
		return ""
	}
	preview := strings.Join(strings.Fields(entry.Desc), " ")
	runes := []rune(preview)
	if len(runes) <= maxRunes {
		return preview
	}
	return string(runes[:maxRunes]) + "…"
}

// SvcTypes counts each delivery/service type so `info` can explain the mix of
// callable portal APIs, provider handoffs, file datasets and unknown entries.
func (c *Catalog) SvcTypes() map[string]int {
	out := map[string]int{}
	for _, e := range c.Entries {
		k := e.SvcType
		if k == "" {
			k = "미확인"
		}
		out[k]++
	}
	return out
}

// Orgs counts datasets per publisher, most first — the fastest way to see who
// publishes in an area once a search has narrowed it down.
func (c *Catalog) Orgs(query string, limit int) []OrgCount {
	terms := queryTerms(query)
	counts := map[string]*OrgCount{}
	for _, e := range c.Entries {
		hay := strings.ToLower(e.Title + " " + e.Org + " " + e.Category + " " + e.Desc)
		ok := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		c, exists := counts[e.Org]
		if !exists {
			c = &OrgCount{Org: e.Org}
			counts[e.Org] = c
		}
		c.Count++
		c.ApplySum += e.ApplyCount
	}
	out := make([]OrgCount, 0, len(counts))
	for _, v := range counts {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// OrgCount is one publisher's share of a query's matches.
type OrgCount struct {
	Org      string `json:"org"`
	Count    int    `json:"count"`
	ApplySum int    `json:"applySum"`
}
