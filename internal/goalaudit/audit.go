// Package goalaudit reports retained execution outcomes, not goal correctness.
// Its readiness floor is only a prerequisite for independent benchmark grading.
package goalaudit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"regexp"
	"strings"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

type manifest struct {
	SchemaVersion int        `json:"schemaVersion"`
	Cases         []caseSpec `json:"cases"`
}

type caseSpec struct {
	ID          string      `json:"id"`
	Goal        string      `json:"goal"`
	Expectation string      `json:"expectation"`
	Trials      []trialSpec `json:"trials"`
}

type trialSpec struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Format string `json:"format"`
}

type Report struct {
	Kind               string       `json:"kind"`
	IndependentGrading string       `json:"independentGrading"`
	ReadinessFloorMet  bool         `json:"readinessFloorMet"`
	UnrunCases         int          `json:"unrunCases"`
	Cases              []CaseReport `json:"cases"`
	Totals             Counts       `json:"totals"`
}

type CaseReport struct {
	ID          string        `json:"id"`
	Expectation string        `json:"expectation"`
	Trials      []TrialReport `json:"trials"`
	Counts      Counts        `json:"counts"`
}

type TrialReport struct {
	Activity
	ID                  string   `json:"id"`
	ArtifactSHA256      string   `json:"artifactSha256,omitempty"`
	State               string   `json:"state"`
	ErrorCode           string   `json:"errorCode,omitempty"`
	ReadyCandidate      bool     `json:"readyCandidate"`
	CommandFailed       *bool    `json:"commandFailed"`
	MissingFields       []string `json:"missingFields"`
	ArtifactSummaryRows *int     `json:"artifactSummaryRows,omitempty"`
}

type Counts struct {
	Activity
	Planned                  int            `json:"planned"`
	Loaded                   int            `json:"loaded"`
	Missing                  int            `json:"missing"`
	Invalid                  int            `json:"invalid"`
	ReadyCandidates          int            `json:"readyCandidates"`
	RecordsWithMissingFields int            `json:"recordsWithMissingFields"`
	SummaryOnlyRows          int            `json:"summaryOnlyRows"`
	States                   map[string]int `json:"states"`
}

// Activity counts only entries actually retained in the view. Absent fields
// are listed per trial; these counts never reconstruct a full interaction trace.
type Activity struct {
	Searches            *int `json:"searches"`
	SemanticSearches    *int `json:"semanticSearches"`
	Candidates          *int `json:"candidates"`
	Observations        *int `json:"observations"`
	AcquisitionAttempts *int `json:"acquisitionAttempts"`
	Executions          *int `json:"executions"`
	ArtifactRows        *int `json:"artifactRows"`
}

// Audit reads a local case manifest and its explicitly referenced artifacts.
// The report contains counts and hashes, never observed values or parser errors.
func Audit(input io.Reader, artifacts fs.FS) (Report, error) {
	b, err := readBounded(input, 1<<20)
	if err != nil {
		return Report{}, errors.New("manifest is unreadable or exceeds 1 MiB")
	}
	var m manifest
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil || d.Decode(new(any)) != io.EOF || !unambiguousJSON(b, true) || !validManifest(m) {
		return Report{}, errors.New("invalid manifest: require v1 cases, unique IDs/paths, exact goals, expectations and explicit artifact formats")
	}
	r := Report{Kind: "goal_trial_audit_not_completion_grade", IndependentGrading: "not_performed", ReadinessFloorMet: true, Totals: emptyCounts()}
	positive := 0
	for _, c := range m.Cases {
		cr := CaseReport{ID: c.ID, Expectation: c.Expectation, Trials: []TrialReport{}, Counts: emptyCounts()}
		if len(c.Trials) == 0 {
			r.UnrunCases++
		}
		for _, t := range c.Trials {
			tr := readTrial(artifacts, t, c.Goal)
			cr.Trials = append(cr.Trials, tr)
			cr.Counts.add(tr)
			r.Totals.add(tr)
		}
		if c.Expectation == "positive" {
			positive++
			if cr.Counts.ReadyCandidates == 0 {
				r.ReadinessFloorMet = false
			}
		}
		r.Cases = append(r.Cases, cr)
	}
	r.ReadinessFloorMet = r.ReadinessFloorMet && positive > 0 && r.Totals.Missing == 0 && r.Totals.Invalid == 0
	return r, nil
}

var auditID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func validManifest(m manifest) bool {
	if m.SchemaVersion != 1 || len(m.Cases) == 0 || len(m.Cases) > 64 {
		return false
	}
	cases, trials, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range m.Cases {
		if !auditID.MatchString(c.ID) || cases[c.ID] || strings.TrimSpace(c.Goal) == "" || len(c.Goal) > 4000 {
			return false
		}
		cases[c.ID] = true
		if c.Expectation != "positive" && c.Expectation != "negative" && c.Expectation != "dependency" {
			return false
		}
		for _, t := range c.Trials {
			if !auditID.MatchString(t.ID) || trials[t.ID] || paths[t.Path] || !fs.ValidPath(t.Path) || t.Path == "." || len(t.Path) > 1024 || strings.Contains(t.Path, `\`) || (t.Format != "view" && t.Format != "diagnostic") {
				return false
			}
			trials[t.ID], paths[t.Path] = true, true
			if len(trials) > 1024 {
				return false
			}
		}
	}
	return true
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("input unreadable or too large")
	}
	return b, nil
}

func readTrial(artifacts fs.FS, t trialSpec, goal string) TrialReport {
	r := TrialReport{ID: t.ID, State: "invalid"}
	f, err := artifacts.Open(t.Path)
	if err != nil {
		r.ErrorCode = "unreadable_artifact"
		if errors.Is(err, fs.ErrNotExist) {
			r.State, r.ErrorCode = "missing", "artifact_not_found"
		}
		return r
	}
	b, err := readBounded(f, 16<<20)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		r.ErrorCode = "unreadable_or_oversized_artifact"
		return r
	}
	h := sha256.Sum256(b)
	r.ArtifactSHA256 = hex.EncodeToString(h[:])
	if !unambiguousJSON(b, false) {
		r.ErrorCode = "ambiguous_or_invalid_artifact_json"
		return r
	}
	if t.Format == "diagnostic" {
		envelopeFields, ok := controlObject(b)
		if !ok {
			r.ErrorCode = "invalid_diagnostic_envelope"
			return r
		}
		if _, ok := controlObject(envelopeFields["run"]); !ok {
			r.ErrorCode = "invalid_diagnostic_run"
			return r
		}
		var envelope struct {
			Run *struct {
				Status        string `json:"status"`
				CommandFailed *bool  `json:"commandFailed"`
			} `json:"run"`
			Execution json.RawMessage `json:"execution"`
		}
		if json.Unmarshal(b, &envelope) != nil || envelope.Run == nil || envelope.Run.CommandFailed == nil {
			r.ErrorCode = "invalid_diagnostic_envelope"
			return r
		}
		var state struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(envelope.Execution, &state) != nil || state.Status != envelope.Run.Status {
			r.ErrorCode = "diagnostic_state_mismatch"
			return r
		}
		r.CommandFailed = envelope.Run.CommandFailed
		b = envelope.Execution
	}
	var v goalwork.View
	if err := json.Unmarshal(b, &v); err != nil {
		r.ErrorCode = "invalid_artifact_json"
		return r
	}
	if v.Goal != goal || v.Revision < 0 {
		r.ErrorCode = "goal_or_revision_mismatch"
		return r
	}
	switch v.Status {
	case "exploring", "review_required", "output_ready", "abstained", "blocked", "expired", "budget_exhausted":
	default:
		r.ErrorCode = "unknown_execution_state"
		return r
	}
	fields, ok := controlObject(b)
	if !ok {
		r.ErrorCode = "invalid_view_object"
		return r
	}
	if !validActivityControls(fields) {
		r.ErrorCode = "invalid_activity_metadata"
		return r
	}
	if v.Artifact != nil {
		if _, ok := controlObject(fields["artifact"]); !ok {
			r.ErrorCode = "invalid_artifact_object"
			return r
		}
	}
	if v.Status == "output_ready" && !readyView(v, fields["evaluation"], fields["artifact"]) {
		r.ErrorCode = "inconsistent_ready_state"
		return r
	}
	r.State = v.Status
	r.MissingFields = []string{}
	for _, field := range []string{"searches", "nodes", "observations", "sampleAttempts", "executions", "artifact"} {
		if raw, ok := fields[field]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			r.MissingFields = append(r.MissingFields, field)
		}
	}
	r.Searches = observedCount(fields, "searches", len(v.Searches))
	r.Candidates = observedCount(fields, "nodes", len(v.Nodes))
	r.Observations = observedCount(fields, "observations", len(v.Observations))
	r.AcquisitionAttempts = observedCount(fields, "sampleAttempts", len(v.SampleAttempts))
	r.Executions = observedCount(fields, "executions", len(v.Executions))
	semantic := 0
	for _, s := range v.Searches {
		if s.Semantic != nil && s.Semantic.Status == "used" {
			semantic++
		}
	}
	r.SemanticSearches = observedCount(fields, "searches", semantic)
	if v.Artifact != nil {
		var artifactFields map[string]json.RawMessage
		// View decoding above already checked the artifact object and row type.
		_ = json.Unmarshal(fields["artifact"], &artifactFields)
		r.ArtifactRows = observedCount(artifactFields, "rows", len(v.Artifact.Rows))
		if r.ArtifactRows == nil {
			r.MissingFields = append(r.MissingFields, "artifact.rows")
		}
	} else if raw, ok := fields["artifactSummary"]; ok {
		var summary struct {
			RowCount *int `json:"rowCount"`
		}
		if json.Unmarshal(raw, &summary) == nil && summary.RowCount != nil && *summary.RowCount >= 0 {
			r.ArtifactSummaryRows = summary.RowCount
		}
	}
	r.ReadyCandidate = v.Status == "output_ready" && (r.CommandFailed == nil || !*r.CommandFailed)
	return r
}

// Check object members before ordinary struct/map decoding can overwrite them.
// Keys inside source rows remain case-sensitive, as they are in the source JSON.
func unambiguousJSON(b []byte, foldKeys bool) bool {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return uniqueValue(d, 0, foldKeys) && d.Decode(new(any)) == io.EOF
}

func uniqueValue(d *json.Decoder, depth int, foldKeys bool) bool {
	if depth > 64 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, composite := token.(json.Delim)
	if !composite {
		return true
	}
	if delim != '{' && delim != '[' {
		return false
	}
	keys := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || keys[name] || (foldKeys && foldedCollision(keys, name)) {
				return false
			}
			keys[name] = true
		}
		if !uniqueValue(d, depth+1, foldKeys) {
			return false
		}
	}
	end, err := d.Token()
	return err == nil && ((delim == '{' && end == json.Delim('}')) || (delim == '[' && end == json.Delim(']')))
}

func foldedCollision(keys map[string]bool, name string) bool {
	for key := range keys {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

// Audit control objects decode into Go structs (case-insensitive field names).
// Apply that collision rule only to metadata, never to arbitrary source rows.
func controlObject(raw []byte) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) > 128 {
		return nil, false
	}
	seen := map[string]bool{}
	for key := range fields {
		if foldedCollision(seen, key) {
			return nil, false
		}
		seen[key] = true
	}
	return fields, true
}

func controlField(fields map[string]json.RawMessage, name string) json.RawMessage {
	for key, raw := range fields {
		if strings.EqualFold(key, name) {
			return raw
		}
	}
	return nil
}

func validActivityControls(fields map[string]json.RawMessage) bool {
	if raw := controlField(fields, "artifactSummary"); raw != nil && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if _, ok := controlObject(raw); !ok {
			return false
		}
	}
	var searches []json.RawMessage
	if raw := controlField(fields, "searches"); raw != nil {
		if json.Unmarshal(raw, &searches) != nil {
			return false
		}
	}
	for _, raw := range searches {
		search, ok := controlObject(raw)
		if !ok {
			return false
		}
		if semantic := controlField(search, "semantic"); semantic != nil && !bytes.Equal(bytes.TrimSpace(semantic), []byte("null")) {
			if _, ok := controlObject(semantic); !ok {
				return false
			}
		}
	}
	return true
}

func observedCount(fields map[string]json.RawMessage, name string, n int) *int {
	if raw, ok := fields[name]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	return &n
}

func emptyCounts() Counts {
	return Counts{Activity: Activity{Searches: new(int), SemanticSearches: new(int), Candidates: new(int), Observations: new(int), AcquisitionAttempts: new(int), Executions: new(int), ArtifactRows: new(int)}, States: map[string]int{}}
}

func addCount(a, b *int) *int {
	if a == nil || b == nil {
		return nil
	}
	n := *a + *b
	return &n
}

func readyView(v goalwork.View, evaluation, artifact json.RawMessage) bool {
	if v.Artifact == nil || len(v.Artifact.Rows) == 0 || (v.Artifact.Status != "sample_executed" && v.Artifact.Status != "sample_joined") {
		return false
	}
	a, ok := controlObject(artifact)
	if !ok || a["rows"] == nil {
		return false
	}
	for _, raw := range []json.RawMessage{evaluation, a["evaluation"]} {
		fields, ok := controlObject(raw)
		if !ok || fields["status"] == nil || fields["needsSemanticReview"] == nil {
			return false
		}
		var e struct {
			Status              string `json:"status"`
			NeedsSemanticReview *bool  `json:"needsSemanticReview"`
		}
		if json.Unmarshal(raw, &e) != nil || e.Status != "requirements_met" || e.NeedsSemanticReview == nil || *e.NeedsSemanticReview {
			return false
		}
	}
	return true
}

func (c *Counts) add(t TrialReport) {
	if c.States == nil {
		c.States = make(map[string]int)
	}
	c.Planned++
	switch t.State {
	case "missing":
		c.Missing++
	case "invalid":
		c.Invalid++
	default:
		c.Loaded++
	}
	c.States[t.State]++
	c.ArtifactRows = addCount(c.ArtifactRows, t.ArtifactRows)
	c.Searches = addCount(c.Searches, t.Searches)
	c.SemanticSearches = addCount(c.SemanticSearches, t.SemanticSearches)
	c.Candidates = addCount(c.Candidates, t.Candidates)
	c.Observations = addCount(c.Observations, t.Observations)
	c.AcquisitionAttempts = addCount(c.AcquisitionAttempts, t.AcquisitionAttempts)
	c.Executions = addCount(c.Executions, t.Executions)
	if len(t.MissingFields) > 0 {
		c.RecordsWithMissingFields++
	}
	if t.ArtifactSummaryRows != nil {
		c.SummaryOnlyRows += *t.ArtifactSummaryRows
	}
	if t.ReadyCandidate {
		c.ReadyCandidates++
	}
}
