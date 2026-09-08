package goalwork

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

// Policy is fixed by the trusted caller at start; planner decisions cannot change it.
type Policy struct {
	RequireSemantic   bool   `json:"requireSemantic"`
	MaxRounds         int    `json:"maxRounds,omitempty"`
	EvidenceRecipient string `json:"evidenceRecipient,omitempty"` // empty disables selected value disclosure
	ReviewRecipient   string `json:"reviewRecipient,omitempty"`   // explicit second disclosure/authority; never a planner input
	ReviewAnalyses    bool   `json:"reviewAnalyses,omitempty"`    // additional trusted opt-in; source-report permission alone is insufficient
	ReviewFullScope   bool   `json:"reviewFullScope,omitempty"`   // additional opt-in for source-supported population goals, not population certification
}

const (
	maxSearches   = 12
	maxSearchHits = 8
	maxCandidates = maxSearches * maxSearchHits
)

var ErrActionAlreadyAttempted = errors.New("action already attempted")

// Dependencies are acquisition seams, not planner authority. Implementations
// must use catalogued PKs and official inspected contracts, never arbitrary URLs.
type Dependencies struct {
	Review             func(context.Context, ReviewInput) (ReviewAssessment, error) // trusted startup adapter; separate tool-free context
	Search             func(context.Context, string) (catalog.Result, error)
	Inspect            func(context.Context, string) (Inspection, error)
	InspectFileVersion func(context.Context, string, string) (Inspection, error) // empty edition lists bounded portal history; exact listed edition selects it
	Sample             func(context.Context, SampleRequest, Inspection) (Acquired, error)
	Layout             func(context.Context, LayoutRequest, Inspection) (dataset.FileLayout, error)
	ScanCSV            func(context.Context, SampleRequest, Inspection, func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) // complete stream and inspected contract hash
}

// Inspection separates request parameters from observed response fields. The
// opaque handle is never serialized or supplied by a model.
type Inspection struct {
	FileVersions         []dataset.FileVersion        `json:"fileVersions,omitempty"`
	FileHistoryCount     int                          `json:"fileHistoryCount,omitempty"`
	FileHistoryTruncated bool                         `json:"fileHistoryTruncated,omitempty"`
	SelectedFileVersion  *dataset.FileVersion         `json:"selectedFileVersion,omitempty"`
	PK                   string                       `json:"pk"`
	Deliveries           []string                     `json:"deliveries"`
	Operations           []Operation                  `json:"operations,omitempty"`
	Assets               []string                     `json:"assets,omitempty"`
	Documents            []dataset.DocumentReference  `json:"documents,omitempty"`
	Exports              []dataset.ExportReference    `json:"exports,omitempty"`
	DeclaredColumns      []DeclaredColumn             `json:"declaredColumns,omitempty"`
	Warnings             []string                     `json:"warnings,omitempty"`
	Declarations         map[string]SourceDeclaration `json:"declarations,omitempty"` // keyed by request delivery
	handle               any
}
type DeclaredColumn struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}
type Operation struct {
	Name       string      `json:"name"`
	Title      string      `json:"title,omitempty"`
	Parameters []Parameter `json:"parameters,omitempty"`
}
type Parameter struct {
	Name        string `json:"name"`
	Required    string `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Example     string `json:"example,omitempty"`
}

type SampleRequest struct {
	Compare     *SourceComparison          `json:"compare,omitempty"`     // local support-only comparison of two retained original revisions
	FileVersion string                     `json:"fileVersion,omitempty"` // must equal the selected inspected FILE edition; empty for current files
	Reduce      *SourceReduction           `json:"reduce,omitempty"`      // local pre-join aggregation of an exact retained source revision
	Nearest     *NearestSelection          `json:"nearest,omitempty"`     // local reduction using retained observations, never caller coordinates
	ScanCSV     bool                       `json:"scanCsv,omitempty"`     // complete bounded direct-CSV scan; retain only a prefix
	PK          string                     `json:"pk"`
	Delivery    string                     `json:"delivery"` // api, file, standard or document
	Document    *dataset.DocumentSelection `json:"document,omitempty"`
	Operation   string                     `json:"operation,omitempty"`
	Params      map[string]string          `json:"params,omitempty"`
	Asset       string                     `json:"asset,omitempty"`
	Member      string                     `json:"member,omitempty"`   // exact CSV path inside a ZIP asset
	RowPath     string                     `json:"rowPath,omitempty"`  // API JSON Pointer
	Where       map[string]string          `json:"where,omitempty"`    // FILE only; conjunctive exact string selection before row limit
	WhereIn     map[string][]string        `json:"whereIn,omitempty"`  // direct CSV full scan only; OR within each exact value set, AND across fields
	XLSX        *dataset.XLSXSelection     `json:"xlsx,omitempty"`     // FILE only; exact original worksheet rectangle
	LayoutID    string                     `json:"layoutId,omitempty"` // optional same-source-file pin from layout discovery
}
type Acquired struct {
	Comparison     *ComparisonProvenance
	Reduction      *ReductionProvenance
	Spatial        *SpatialProvenance
	Rows           []Row
	Delivery       string
	Operation      string
	ContentSHA256  string
	ContractSHA256 string
	Warnings       []string
	Selection      *dataset.SelectionReport
	Table          *dataset.TableProvenance
	Archive        *dataset.ArchiveProvenance
	CSV            *dataset.CSVProvenance
	Document       *dataset.DocumentProvenance
}
type Decision struct {
	FileHistory   bool             `json:"fileHistory,omitempty"` // inspect only: list advertised historical FILE editions
	FileVersion   string           `json:"fileVersion,omitempty"` // inspect only: exact edition ID from prior history inspection
	Action        string           `json:"action"`                // define | search | inspect | layout | sample | retry_sample | read_evidence | compose | execute | review_result | abstain
	RetryOf       int              `json:"retryOf,omitempty"`     // latest failed SampleAttempt revision; never a replacement request
	Contract      *GoalContract    `json:"contract,omitempty"`
	Query         string           `json:"query,omitempty"`
	Role          string           `json:"role,omitempty"`
	PK            string           `json:"pk,omitempty"`
	Sample        *SampleRequest   `json:"sample,omitempty"`
	Layout        *LayoutRequest   `json:"layout,omitempty"`
	Composition   *Composition     `json:"composition,omitempty"`
	CompositionID string           `json:"compositionId,omitempty"`
	Reason        string           `json:"reason,omitempty"`
	Evidence      *EvidenceRequest `json:"evidence,omitempty"`
}
type Node struct {
	Hit        catalog.Hit `json:"hit"`
	Roles      []string    `json:"roles"`
	Inspection *Inspection `json:"inspection,omitempty"`
}
type Observation struct {
	Comparison     *ComparisonProvenance       `json:"comparison,omitempty"`
	Reduction      *ReductionProvenance        `json:"reduction,omitempty"`
	Spatial        *SpatialProvenance          `json:"spatial,omitempty"`
	ID             string                      `json:"id"`
	PK             string                      `json:"pk"`
	Delivery       string                      `json:"delivery"`
	Operation      string                      `json:"operation,omitempty"`
	Asset          string                      `json:"asset,omitempty"`
	RowPath        string                      `json:"rowPath,omitempty"`
	RequestSHA256  string                      `json:"requestSha256"`
	ContentSHA256  string                      `json:"contentSha256,omitempty"`
	ContractSHA256 string                      `json:"contractSha256,omitempty"`
	RowsSHA256     string                      `json:"rowsSha256"`
	ObservedAt     string                      `json:"observedAt"`
	Columns        []string                    `json:"columns"`
	ColumnTypes    map[string][]string         `json:"columnTypes"`
	ColumnProfiles map[string]ColumnProfile    `json:"columnProfiles"`
	RowCount       int                         `json:"rowCount"`
	Warnings       []string                    `json:"warnings,omitempty"`
	Selection      *dataset.SelectionReport    `json:"selection,omitempty"`
	Table          *dataset.TableProvenance    `json:"table,omitempty"`
	Archive        *dataset.ArchiveProvenance  `json:"archive,omitempty"`
	CSV            *dataset.CSVProvenance      `json:"csv,omitempty"`
	Document       *dataset.DocumentProvenance `json:"document,omitempty"`
	Declaration    *SourceDeclaration          `json:"declaration,omitempty"`
}
type Gap struct {
	Revision int    `json:"revision"`
	Action   string `json:"action"`
	Target   string `json:"target,omitempty"`
	Detail   string `json:"detail"`
}
type SearchRecord struct {
	Query    string                `json:"query"`
	Role     string                `json:"role"`
	Semantic *catalog.SemanticInfo `json:"semantic,omitempty"`
	Warnings []string              `json:"warnings,omitempty"`
}
type Artifact struct {
	Explanations []ExplanationDraft  `json:"explanations,omitempty"` // proposed source interpretations; verdicts are in Evaluation.Review
	Status       string              `json:"status"`
	Recipe       Composition         `json:"recipe"`
	Sources      []Observation       `json:"sources"`
	Requests     []SampleRequest     `json:"requests"`
	Layouts      []LayoutObservation `json:"layouts,omitempty"`
	Metrics      []JoinMetric        `json:"metrics"`
	Rows         []Row               `json:"rows"`
	Unmatched    []UnmatchedTuple    `json:"unmatched,omitempty"`
	Limitations  []string            `json:"limitations"`
	Evaluation   GoalEvaluation      `json:"evaluation"`
}
type ExecutionRecord struct {
	CompositionID string       `json:"compositionId"`
	Revision      int          `json:"revision"`
	Status        string       `json:"status"` // failed | partial | requirements_met
	Metrics       []JoinMetric `json:"metrics,omitempty"`
	RowCount      int          `json:"rowCount"`
	Error         string       `json:"error,omitempty"`
}

// SampleAttempt records a validated acquisition input, not returned source
// values or credentials injected by the caller. Revision links failures to gaps.
type SampleAttempt struct {
	Revision      int           `json:"revision"`
	StartedAt     string        `json:"startedAt"`
	Request       SampleRequest `json:"request"`
	RequestSHA256 string        `json:"requestSha256"`
	Status        string        `json:"status"` // failed | acquired
	FailureKind   FailureKind   `json:"failureKind,omitempty"`
	RetryOf       int           `json:"retryOf,omitempty"`
	ObservationID string        `json:"observationId,omitempty"`
}
type View struct {
	PlannerFeedback string              `json:"plannerFeedback,omitempty"` // transient caller diagnostic, not source evidence
	Goal            string              `json:"goal"`
	Contract        *GoalContract       `json:"contract,omitempty"`
	Evaluation      *GoalEvaluation     `json:"evaluation,omitempty"`
	Revision        int                 `json:"revision"`
	Status          string              `json:"status"`
	Policy          Policy              `json:"policy"`
	ExpiresAt       time.Time           `json:"expiresAt"`
	Budget          Budget              `json:"budget"`
	Searches        []SearchRecord      `json:"searches,omitempty"`
	Nodes           []Node              `json:"nodes,omitempty"`
	Observations    []Observation       `json:"observations,omitempty"`
	SampleAttempts  []SampleAttempt     `json:"sampleAttempts,omitempty"`
	Layouts         []LayoutObservation `json:"layouts,omitempty"`
	Compositions    []Composition       `json:"compositions,omitempty"`
	Executions      []ExecutionRecord   `json:"executions,omitempty"`
	Gaps            []Gap               `json:"gaps,omitempty"`
	Artifact        *Artifact           `json:"artifact,omitempty"`
	Evidence        []EvidencePacket    `json:"evidence,omitempty"`
	Reviews         []ReviewAttempt     `json:"reviews,omitempty"`
}

type Budget struct {
	SearchesRemaining        int `json:"searchesRemaining"`
	InspectionsRemaining     int `json:"inspectionsRemaining"`
	LayoutsRemaining         int `json:"layoutsRemaining"`
	SamplesRemaining         int `json:"samplesRemaining"`
	CompositionsRemaining    int `json:"compositionsRemaining"`
	CandidatesRemaining      int `json:"candidatesRemaining"`
	SampleBytesRemaining     int `json:"sampleBytesRemaining"`
	EvidencePacketsRemaining int `json:"evidencePacketsRemaining"`
	EvidenceBytesRemaining   int `json:"evidenceBytesRemaining"`
	ReviewsRemaining         int `json:"reviewsRemaining"`
}

// Engine serializes each action and returns detached views. Rows are immutable,
// private, bounded session memory; only explicitly allowed selections enter planning.
type Engine struct {
	mu                          sync.Mutex
	deps                        Dependencies
	state                       View
	rows                        map[string][]Row
	requests                    map[string]SampleRequest
	seen                        map[string]bool
	inspections, samples, bytes int
	layouts                     int
	evidenceBytes               int
	replayCorrectionUsed        bool
}

func Start(goal string, policy Policy, deps Dependencies) (*Engine, error) {
	goal = strings.TrimSpace(goal)
	if len(goal) == 0 || len(goal) > 4000 {
		return nil, fmt.Errorf("goal must contain 1–4000 bytes")
	}
	if policy.MaxRounds == 0 {
		policy.MaxRounds = 32
	}
	if policy.MaxRounds < 1 || policy.MaxRounds > 64 {
		return nil, fmt.Errorf("maxRounds must be 1–64")
	}
	switch policy.EvidenceRecipient {
	case "", "codex", "claude", "gemini", "mcp_host":
	default:
		return nil, fmt.Errorf("evidenceRecipient must name one supported recipient or be empty")
	}
	if policy.ReviewRecipient != "" {
		if (policy.ReviewRecipient != "codex" && policy.ReviewRecipient != "claude" && policy.ReviewRecipient != "gemini") || policy.EvidenceRecipient == "" || (policy.EvidenceRecipient != "mcp_host" && policy.EvidenceRecipient != policy.ReviewRecipient) || deps.Review == nil {
			return nil, fmt.Errorf("review requires a fixed supported CLI recipient, authorized evidence and a trusted reviewer adapter")
		}
	}
	if policy.ReviewAnalyses && policy.ReviewRecipient == "" {
		return nil, fmt.Errorf("analysis review requires an explicitly authorized reviewer")
	}
	if policy.ReviewFullScope && !policy.ReviewAnalyses {
		return nil, fmt.Errorf("full-scope review requires authorized analysis review")
	}
	return &Engine{deps: deps, state: View{Goal: goal, Status: "exploring", Policy: policy, ExpiresAt: time.Now().UTC().Add(time.Hour)}, rows: map[string][]Row{}, requests: map[string]SampleRequest{}, seen: map[string]bool{}}, nil
}

func (e *Engine) View() View { e.mu.Lock(); defer e.mu.Unlock(); e.expire(); return e.snapshot(false) }
func (e *Engine) PlanningView() View {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.expire()
	return e.snapshot(true)
}
func (e *Engine) snapshot(planning bool) View {
	v := e.state
	v.Budget = Budget{LayoutsRemaining: 6 - e.layouts, SearchesRemaining: maxSearches - len(v.Searches), InspectionsRemaining: 12 - e.inspections, SamplesRemaining: 8 - e.samples, CompositionsRemaining: 6 - len(v.Compositions), CandidatesRemaining: maxCandidates - len(v.Nodes), SampleBytesRemaining: (8 << 20) - e.bytes}
	if v.Policy.EvidenceRecipient != "" {
		v.Budget.EvidencePacketsRemaining = maxEvidencePackets - len(v.Evidence)
		v.Budget.EvidenceBytesRemaining = maxEvidenceBytes - e.evidenceBytes
	}
	if v.Policy.ReviewRecipient != "" {
		v.Budget.ReviewsRemaining = 3 - len(v.Reviews)
	}
	if planning {
		v.Artifact = nil
	}
	b, _ := json.Marshal(v)
	var out View
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}
func (e *Engine) expire() {
	if time.Now().After(e.state.ExpiresAt) {
		e.rows = nil
		e.requests = nil
		e.state.Artifact = nil
		e.state.Evidence = nil
		for i := range e.state.Compositions {
			e.state.Compositions[i].Explanations = nil // proposed prose may quote selected source values
		}
		if e.state.Evaluation != nil {
			e.state.Evaluation.Review = nil // reviewer prose can quote selected values
		}
		e.state.Status = "expired"
	}
}

func canAdvance(status string) bool {
	return status == "exploring" || status == "review_required"
}

func (e *Engine) allowReplayCorrection() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.replayCorrectionUsed {
		return false
	}
	e.replayCorrectionUsed = true
	return true
}

// Advance consumes one bounded action. Acquisition/validation failures become
// session-local gaps for replanning. Revision, replay and terminal-state errors do
// not consume another step. Context cancellation is propagated after recording.
func (e *Engine) Advance(ctx context.Context, revision int, d Decision) (View, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.expire()
	if revision != e.state.Revision {
		return e.snapshot(false), fmt.Errorf("stale revision: expected %d", e.state.Revision)
	}
	if !canAdvance(e.state.Status) {
		return e.snapshot(false), fmt.Errorf("goal is %s", e.state.Status)
	}
	if d.Action == "compose" && d.Composition != nil {
		for _, draft := range d.Composition.Explanations {
			if !utf8.ValidString(draft.Text) {
				return e.snapshot(false), fmt.Errorf("source explanation text must be valid UTF-8 before JSON encoding")
			}
		}
	}
	b, err := json.Marshal(d)
	if err != nil || len(b) > 32<<10 {
		return e.snapshot(false), fmt.Errorf("decision byte limit exceeded")
	}
	var detached Decision
	if err := json.Unmarshal(b, &detached); err != nil {
		return e.snapshot(false), err
	}
	d = detached
	if d.Sample != nil {
		for _, values := range d.Sample.WhereIn {
			sort.Strings(values) // a set has no order; reordering cannot buy another acquisition
		}
	}
	if d.Action == "retry_sample" {
		request, err := e.retryRequest(d)
		if err != nil {
			return e.snapshot(false), err
		}
		d.Sample = &request
	} else if d.RetryOf != 0 {
		return e.snapshot(false), fmt.Errorf("retryOf is valid only for retry_sample")
	}
	// Only acquisition/execution inputs identify an action. Irrelevant fields
	// and changed prose must not create a second observation of the same request.
	keyDecision := Decision{Action: d.Action}
	switch d.Action {
	case "define":
		keyDecision.Contract = d.Contract
	case "search":
		keyDecision.Query = strings.TrimSpace(d.Query)
		keyDecision.Role = strings.TrimSpace(d.Role)
	case "inspect":
		keyDecision.PK = d.PK
		keyDecision.FileVersion = d.FileVersion
		keyDecision.FileHistory = d.FileHistory && d.FileVersion == ""
	case "layout":
		keyDecision.Layout = d.Layout
	case "sample":
		keyDecision.Sample = d.Sample
	case "retry_sample":
		keyDecision.RetryOf = d.RetryOf
	case "compose":
		keyDecision.Composition = d.Composition
	case "execute":
		keyDecision.CompositionID = d.CompositionID
	case "read_evidence":
		keyDecision.Evidence = d.Evidence
	case "review_result":
		keyDecision.CompositionID = d.CompositionID
		keyDecision.Reason = digest(e.state.Evidence) // new evidence may justify another bounded review
	}
	key := digest(keyDecision)
	if e.seen[key] {
		return e.snapshot(false), fmt.Errorf("%w; change the request or composition using recorded gaps", ErrActionAlreadyAttempted)
	}
	if err := ctx.Err(); err != nil {
		return e.snapshot(false), err
	}
	e.seen[key] = true
	e.state.Revision++
	e.state.Status = "exploring"
	err = e.act(ctx, d)
	if err != nil {
		target := d.PK
		if d.Action == "search" {
			target = bounded(d.Query, 500)
		}
		if d.Sample != nil {
			target = d.Sample.PK
		}
		if d.Layout != nil {
			target = d.Layout.PK
		}
		if d.CompositionID != "" {
			target = d.CompositionID
		}
		e.state.Gaps = append(e.state.Gaps, Gap{Revision: e.state.Revision, Action: d.Action, Target: target, Detail: bounded(err.Error(), 2000)})
	}
	if canAdvance(e.state.Status) && e.state.Revision >= e.state.Policy.MaxRounds {
		e.state.Status = "budget_exhausted"
	}
	return e.snapshot(false), ctx.Err()
}

func (e *Engine) act(ctx context.Context, d Decision) error {
	if e.state.Contract == nil && d.Action != "define" && d.Action != "abstain" {
		return fmt.Errorf("define the goal contract before searching: preserve required roles, output, region, period and coverage")
	}
	switch d.Action {
	case "define":
		if e.state.Contract != nil {
			return fmt.Errorf("goal contract is immutable; complete missing requirements rather than weakening them")
		}
		if d.Contract == nil {
			return fmt.Errorf("define requires contract")
		}
		if err := validateContract(*d.Contract); err != nil {
			return err
		}
		e.state.Contract = d.Contract
		return nil
	case "search":
		if strings.TrimSpace(d.Query) == "" || len(d.Query) > 500 {
			return fmt.Errorf("search.query must be nonempty and at most 500 UTF-8 bytes; got %d bytes. Use one short Korean catalogue phrase", len(d.Query))
		}
		if strings.TrimSpace(d.Role) == "" || len(d.Role) > 200 {
			return fmt.Errorf("search.role must be a short nonempty role label, at most 200 UTF-8 bytes; got %d bytes. Put any long explanation in reason, not role", len(d.Role))
		}
		if len(e.state.Searches) >= maxSearches {
			return fmt.Errorf("search budget exhausted")
		}
		e.state.Searches = append(e.state.Searches, SearchRecord{Query: d.Query, Role: d.Role})
		if e.deps.Search == nil {
			return fmt.Errorf("search unavailable")
		}
		res, err := e.deps.Search(ctx, d.Query)
		e.state.Searches[len(e.state.Searches)-1].Semantic = res.Semantic
		e.state.Searches[len(e.state.Searches)-1].Warnings = res.Warnings
		if err != nil {
			if e.state.Policy.RequireSemantic && (res.Semantic == nil || res.Semantic.Status != "used") {
				e.state.Status = "blocked"
			}
			return err
		}
		if e.state.Policy.RequireSemantic && (res.Semantic == nil || res.Semantic.Status != "used") {
			e.state.Status = "blocked"
			return fmt.Errorf("required semantic retrieval was not used; no lexical candidates accepted")
		}
		if len(res.Hits) > maxSearchHits {
			res.Hits = res.Hits[:maxSearchHits]
		}
		for _, h := range res.Hits {
			n := e.node(h.PK)
			if n != nil {
				found := false
				for _, r := range n.Roles {
					found = found || r == d.Role
				}
				if !found {
					n.Roles = append(n.Roles, d.Role)
				}
				continue
			}
			if len(e.state.Nodes) >= maxCandidates {
				return fmt.Errorf("candidate budget exhausted")
			}
			h.Preview = bounded(h.Preview, 1000)
			h.Title = bounded(h.Title, 300)
			e.state.Nodes = append(e.state.Nodes, Node{Hit: h, Roles: []string{d.Role}})
		}
		if len(res.Hits) == 0 {
			return fmt.Errorf("no candidate; broaden role or search complementary observations")
		}
		return nil
	case "inspect":
		n := e.node(d.PK)
		if n == nil {
			return fmt.Errorf("unknown PK: select from this goal's search results")
		}
		if e.inspections >= 12 {
			return fmt.Errorf("inspection budget exhausted")
		}
		e.inspections++
		var i Inspection
		var err error
		if d.FileHistory || d.FileVersion != "" {
			if e.deps.InspectFileVersion == nil {
				return fmt.Errorf("historical FILE inspection unavailable; no latest fallback")
			}
			if d.FileVersion != "" {
				found := false
				if n.Inspection != nil {
					for _, version := range n.Inspection.FileVersions {
						found = found || version.ID == d.FileVersion
					}
				}
				if !found {
					return fmt.Errorf("select fileVersion from this goal's prior fileHistory inspection")
				}
			}
			i, err = e.deps.InspectFileVersion(ctx, d.PK, d.FileVersion)
		} else {
			if e.deps.Inspect == nil {
				return fmt.Errorf("inspection unavailable")
			}
			i, err = e.deps.Inspect(ctx, d.PK)
		}
		if err != nil {
			return err
		}
		if i.PK != d.PK {
			return fmt.Errorf("inspection PK mismatch")
		}
		selectedVersion := ""
		if i.SelectedFileVersion != nil {
			selectedVersion = i.SelectedFileVersion.ID
		}
		if selectedVersion != d.FileVersion || (d.FileHistory && d.FileVersion == "" && len(i.Assets) != 0) {
			return fmt.Errorf("inspection substituted the requested FILE version or history listing")
		}
		b, _ := json.Marshal(i)
		if len(b) > 32<<10 {
			return fmt.Errorf("inspection metadata exceeds planning limit")
		}
		n.Inspection = &i
		return nil
	case "layout":
		return e.discoverLayout(ctx, d.Layout)
	case "read_evidence":
		return e.readEvidence(d.Evidence)
	case "sample", "retry_sample":
		if d.Sample == nil {
			return fmt.Errorf("sample request required")
		}
		s := *d.Sample
		if err := validateSampleSelection(s); err != nil {
			return err
		}
		expectedLayoutHash, err := e.sampleLayoutHash(s)
		if err != nil {
			return err
		}
		for k := range s.Params {
			if sensitiveParameter(k) {
				return fmt.Errorf("credential parameters are forbidden")
			}
		}
		n := e.node(s.PK)
		if n == nil || n.Inspection == nil {
			return fmt.Errorf("inspect known PK before sampling")
		}
		if s.Delivery != "api" && s.Delivery != "file" && s.Delivery != "standard" && s.Delivery != "document" {
			return fmt.Errorf("sample delivery must be api, file, standard or document")
		}
		if s.Delivery == "file" && s.Reduce == nil && s.Compare == nil {
			selected := ""
			if n.Inspection.SelectedFileVersion != nil {
				selected = n.Inspection.SelectedFileVersion.ID
			}
			if s.FileVersion != selected {
				return fmt.Errorf("sample fileVersion must equal the selected inspected FILE edition")
			}
		}
		if e.samples >= 8 {
			return fmt.Errorf("sample budget exhausted")
		}
		e.samples++
		attemptIndex := len(e.state.SampleAttempts)
		e.state.SampleAttempts = append(e.state.SampleAttempts, SampleAttempt{Revision: e.state.Revision, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Request: cloneSampleRequest(s), RequestSHA256: digest(s), Status: "failed", FailureKind: FailureUnknown, RetryOf: d.RetryOf})
		if s.Nearest == nil && s.Reduce == nil && s.Compare == nil && e.deps.Sample == nil {
			return fmt.Errorf("sampling unavailable")
		}
		// An adapter may add private request state internally. Preserve only the
		// validated original request, never the adapter's mutable parameter maps.
		var acq Acquired
		if s.Compare != nil {
			acq, err = e.sampleComparison(ctx, cloneSampleRequest(s))
		} else if s.Reduce != nil {
			acq, err = e.sampleReduction(ctx, cloneSampleRequest(s))
		} else if s.Nearest != nil {
			acq, err = e.sampleNearest(ctx, cloneSampleRequest(s), *n.Inspection)
		} else {
			acq, err = e.deps.Sample(ctx, cloneSampleRequest(s), *n.Inspection)
		}
		if err == nil && s.Reduce == nil && acq.Reduction != nil {
			err = fmt.Errorf("external acquisition cannot supply local reduction provenance")
		}
		if err == nil && s.Compare == nil && acq.Comparison != nil {
			err = fmt.Errorf("external acquisition cannot supply local comparison provenance")
		}
		if err == nil {
			err = validateDocumentAcquisition(s, *n.Inspection, acq)
		}
		if err == nil && expectedLayoutHash != "" && acq.ContentSHA256 != expectedLayoutHash {
			err = fmt.Errorf("source file changed since layout discovery; inspect the new layout before choosing another observation")
		}
		if err != nil {
			e.state.SampleAttempts[attemptIndex].FailureKind = acquisitionFailureKind(err)
			return err
		}
		if len(acq.Rows) == 0 || len(acq.Rows) > 1000 {
			return fmt.Errorf("sample must contain 1–1000 rows")
		}
		b, err := json.Marshal(acq.Rows)
		if err != nil {
			return fmt.Errorf("invalid sample rows")
		}
		if len(b) > 2<<20 || e.bytes+len(b) > 8<<20 {
			return fmt.Errorf("sample byte budget exceeded")
		}
		// Normalize/deep-copy JSON so adapters cannot mutate stored observations.
		var raw any
		decoder := json.NewDecoder(strings.NewReader(string(b)))
		decoder.UseNumber()
		if err = decoder.Decode(&raw); err != nil {
			return err
		}
		rows, err := ExtractRows(raw, "", 1000)
		if err != nil {
			return err
		}
		columns := map[string]bool{}
		types := map[string]map[string]bool{}
		for _, r := range rows {
			for k, v := range r {
				columns[k] = true
				if types[k] == nil {
					types[k] = map[string]bool{}
				}
				kind := "null"
				switch v.(type) {
				case string:
					kind = "string"
				case bool:
					kind = "boolean"
				case json.Number, float64:
					kind = "number"
				}
				types[k][kind] = true
			}
		}
		if len(columns) > 256 {
			return fmt.Errorf("column budget exceeded")
		}
		names := make([]string, 0, len(columns))
		for k := range columns {
			names = append(names, k)
		}
		sort.Strings(names)
		columnTypes := map[string][]string{}
		for k, kinds := range types {
			for kind := range kinds {
				columnTypes[k] = append(columnTypes[k], kind)
			}
			sort.Strings(columnTypes[k])
		}
		id := fmt.Sprintf("o%d", len(e.state.Observations)+1)
		e.rows[id] = rows
		e.requests[id] = s
		e.bytes += len(b)
		var declaration *SourceDeclaration
		if declared, ok := n.Inspection.Declarations[s.Delivery]; ok && s.Reduce == nil && s.Compare == nil && s.Document == nil {
			declaration = &declared
		}
		e.state.Observations = append(e.state.Observations, Observation{Comparison: acq.Comparison, Reduction: acq.Reduction, Spatial: acq.Spatial, ID: id, PK: s.PK, Delivery: acq.Delivery, Operation: acq.Operation, Asset: s.Asset, RowPath: s.RowPath, RequestSHA256: digest(s), ContentSHA256: acq.ContentSHA256, ContractSHA256: acq.ContractSHA256, RowsSHA256: digest(rows), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Columns: names, ColumnTypes: columnTypes, ColumnProfiles: profileColumns(rows, names), RowCount: len(rows), Warnings: acq.Warnings, Selection: acq.Selection, Table: cloneTableProvenance(acq.Table), Archive: cloneArchiveProvenance(acq.Archive), CSV: cloneCSVProvenance(acq.CSV), Document: cloneDocumentProvenance(acq.Document), Declaration: declaration})
		e.state.SampleAttempts[attemptIndex].Status = "acquired"
		e.state.SampleAttempts[attemptIndex].FailureKind = ""
		e.state.SampleAttempts[attemptIndex].ObservationID = id
		return nil
	case "compose":
		if d.Composition == nil {
			return fmt.Errorf("composition required")
		}
		p := *d.Composition
		if len(p.Roles) > 8 || len(p.Outputs) > 16 {
			return fmt.Errorf("composition role/output binding limit exceeded")
		}
		if len(p.ReportUnmatched) > 16 {
			return fmt.Errorf("unmatched report supports at most 16 selected fields")
		}
		if p.ID == "" || p.Purpose == "" || len(p.Assumptions) == 0 || len(p.Joins) > 7 || len(p.Select) > 64 || len(p.GroupBy) > 8 || len(p.Aggregates) > 8 || len(p.Measures) > 16 {
			return fmt.Errorf("composition needs id, purpose, explicit namespace/time/coverage assumptions and at most 7 joins")
		}
		if e.state.Contract.TimeWindow != nil && (p.Time == nil || p.Time.Window == nil || *p.Time.Window != *e.state.Contract.TimeWindow) {
			return fmt.Errorf("composition must retain the goal's immutable timeWindow and bind every source's observed time fields")
		}
		if len(e.state.Compositions) >= 6 {
			return fmt.Errorf("alternative composition budget exhausted")
		}
		for _, prior := range e.state.Compositions {
			if prior.ID == p.ID {
				return fmt.Errorf("composition ID already used; preserve failed alternatives")
			}
		}
		if _, ok := e.rows[p.Base]; !ok {
			return fmt.Errorf("unknown base observation")
		}
		for _, j := range p.Joins {
			if _, ok := e.rows[j.Right]; !ok {
				return fmt.Errorf("unknown joined observation")
			}
		}
		if err := validateComputationalSources(p, e.state.Observations); err != nil {
			return err
		}
		if _, err := e.supportContext(p); err != nil {
			return err
		}
		if err := e.validateExplanationDrafts(ctx, p); err != nil {
			return err
		}
		b, _ := json.Marshal(p)
		_ = json.Unmarshal(b, &p)
		e.state.Compositions = append(e.state.Compositions, p)
		return nil
	case "execute":
		for _, p := range e.state.Compositions {
			if p.ID != d.CompositionID {
				continue
			}
			e.state.Artifact = nil
			e.state.Evaluation = nil
			rows, metrics, err := execute(p, e.rows, 1000, e.state.Observations...)
			attempt := len(e.state.Executions)
			e.state.Executions = append(e.state.Executions, ExecutionRecord{CompositionID: p.ID, Revision: e.state.Revision, Status: "failed", Metrics: metrics})
			if err != nil {
				e.state.Executions[attempt].Error = bounded(err.Error(), 2000)
				return err
			}
			unmatched, err := reportUnmatched(p, e.rows, metrics, 1000-len(rows), e.state.Observations...)
			if err != nil {
				e.state.Executions[attempt].Error = bounded(err.Error(), 2000)
				return err
			}
			b, _ := json.Marshal(rows)
			resultBytes := len(b)
			if len(unmatched) > 0 {
				b, _ := json.Marshal(unmatched)
				resultBytes += len(b)
			}
			if resultBytes > 2<<20 {
				e.state.Executions[attempt].Error = "artifact byte limit exceeded; project fewer columns"
				return fmt.Errorf("artifact byte limit exceeded; project fewer columns")
			}
			byID := map[string]Observation{}
			for _, o := range e.state.Observations {
				byID[o.ID] = o
			}
			used := compositionSources(p, byID)
			var sources []Observation
			var requests []SampleRequest
			for _, o := range e.state.Observations {
				if used[o.ID] {
					sources = append(sources, o)
					requests = append(requests, e.requests[o.ID])
				}
			}
			e.state.Artifact = &Artifact{Status: "sample_executed", Recipe: p, Sources: sources, Requests: requests, Layouts: e.state.Layouts, Metrics: metrics, Rows: rows, Limitations: []string{"Bounded acquisition only; full requested coverage needs separately authorized source review.", "Namespace, time, representativeness and usefulness remain declared assumptions, not verified identity or causality.", "Reacquisition may change source bytes; hashes identify the observed revision, not an archived copy.", "Not connection-ledger sample_verified. No automatic claim promotion."}}
			e.state.Artifact.Unmatched = unmatched
			e.state.Artifact.Explanations = p.Explanations
			if len(p.Joins) > 0 {
				e.state.Artifact.Limitations = append(e.state.Artifact.Limitations, "Inner joins omit unmatched rows from the result table; metrics retain stage-local source addresses for selected evidence and replanning, not proof of absence.")
				if len(p.ReportUnmatched) > 0 {
					e.state.Artifact.Limitations = append(e.state.Artifact.Limitations, "Explicitly selected unmatched values are reported separately in unmatched; stage-local tuples do not enter calculations and may repeat sources across stages, not distinct entities or a complete population.")
				}
			}
			evaluation := evaluateRequirements(*e.state.Contract, p, rows, e.state.Observations, e.state.Nodes, e.state.Policy.ReviewFullScope)
			evaluation.ExecutionRevision = e.state.Revision
			for _, o := range sources {
				if o.Spatial != nil {
					e.state.Artifact.Limitations = append(e.state.Artifact.Limitations, "Spatial values are conditional spherical distances among exact-filter source records, not distinct destinations or accessible routes. Anchor selection, common datum, coordinate accuracy and current operation remain unverified.")
					if p.Time != nil {
						e.state.Artifact.Limitations = append(e.state.Artifact.Limitations, "Temporal alignment filters already-selected nearest pairs; ranks are not refilled among all time-eligible candidates. A narrower nearest claim requires another full scan with grounded candidate eligibility.")
					}
					break
				}
			}
			evaluation.Temporal = evaluateTemporal(p, metrics)
			evaluation.Explanations = explainEvidence(*e.state.Contract, p, sources, len(rows), evaluation.Temporal, metrics)
			e.state.Artifact.Evaluation = evaluation
			e.state.Evaluation = &evaluation
			e.state.Executions[attempt].Status = evaluation.Status
			e.state.Executions[attempt].RowCount = len(rows)
			if evaluation.Status == "requirements_met" {
				if evaluation.NeedsSemanticReview {
					e.state.Status = "review_required"
				} else {
					e.state.Status = "output_ready"
				}
			} else {
				return fmt.Errorf("sample executed but goal requirements remain incomplete; inspect evaluation.checks and continue with missing roles or outputs")
			}
			return nil
		}
		return fmt.Errorf("unknown composition ID")
	case "review_result":
		return e.reviewResult(ctx, d.CompositionID)
	case "abstain":
		if strings.TrimSpace(d.Reason) == "" {
			return fmt.Errorf("abstention reason required")
		}
		e.state.Gaps = append(e.state.Gaps, Gap{Revision: e.state.Revision, Action: "abstain", Detail: bounded(d.Reason, 2000)})
		e.state.Status = "abstained"
		return nil
	default:
		return fmt.Errorf("unknown action; use define, search, inspect, layout, sample, retry_sample, read_evidence, compose, execute, review_result or abstain")
	}
}

func (e *Engine) node(pk string) *Node {
	for i := range e.state.Nodes {
		if e.state.Nodes[i].Hit.PK == pk {
			return &e.state.Nodes[i]
		}
	}
	return nil
}
func digest(v any) string { b, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(b)) }

func cloneSampleRequest(s SampleRequest) SampleRequest {
	// Requests contain only typed JSON values and have already passed the
	// decision byte bound. Detach every selector, including nested reductions.
	b, _ := json.Marshal(s)
	var copy SampleRequest
	_ = json.Unmarshal(b, &copy)
	return copy
}

func cloneTableProvenance(table *dataset.TableProvenance) *dataset.TableProvenance {
	if table == nil {
		return nil
	}
	copy := *table
	copy.RowNumbers = append([]int(nil), table.RowNumbers...)
	copy.FormulaCells = append([]string(nil), table.FormulaCells...)
	return &copy
}

func cloneArchiveProvenance(p *dataset.ArchiveProvenance) *dataset.ArchiveProvenance {
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}
func cloneCSVProvenance(p *dataset.CSVProvenance) *dataset.CSVProvenance {
	if p == nil {
		return nil
	}
	copy := *p
	copy.DataRecords = append([]int(nil), p.DataRecords...)
	copy.StartLines = append([]int(nil), p.StartLines...)
	if p.Export != nil {
		export := *p.Export
		export.Columns = append([]string(nil), p.Export.Columns...)
		export.Request.Form = make(url.Values, len(p.Export.Request.Form))
		for name, values := range p.Export.Request.Form {
			export.Request.Form[name] = append([]string(nil), values...)
		}
		copy.Export = &export
	}
	return &copy
}
func bounded(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
