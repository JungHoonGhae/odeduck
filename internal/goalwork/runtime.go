package goalwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/version"
)

// Runtime describes this executable's contract and trusted startup authority,
// not the latest release's capabilities or proof of a completed user goal.
type Runtime struct {
	Build         string   `json:"build"`
	Executable    string   `json:"executable,omitempty"`
	BinarySHA256  string   `json:"binarySha256,omitempty"`
	IdentityError string   `json:"identityError,omitempty"`
	GuideSHA256   string   `json:"guideSha256"`
	Actions       []string `json:"actions"`
	Status        string   `json:"status"`
	CheckedAt     string   `json:"checkedAt,omitempty"`
	Environment
}

// Fingerprint during package initialization, before any CLI/MCP request. A
// normal upgrade may replace this path before the first goal. This identifies
// the file at startup; it is not release authenticity or protection against a
// concurrent in-place replacement while startup is reading the file.
type binaryIdentity struct{ path, hash string }

var processIdentity, processIdentityError = identifyExecutable()

func identifyExecutable() (binaryIdentity, error) {
	p, err := os.Executable()
	if err != nil {
		return binaryIdentity{}, err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return binaryIdentity{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return binaryIdentity{path: p}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return binaryIdentity{path: p}, err
	}
	return binaryIdentity{p, hex.EncodeToString(h.Sum(nil))}, nil
}

type RuntimeCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // checked, blocked, not_required
	Detail string `json:"detail,omitempty"`
}

// Environment records what was checked, separately from goal observations.
// A semantic probe is not a goal search or evidence of incremental discovery.
type Environment struct {
	CatalogRevision string         `json:"catalogRevision,omitempty"`
	CatalogEntries  int            `json:"catalogEntries,omitempty"`
	Checks          []RuntimeCheck `json:"checks,omitempty"`
}

func runtimeFor(policy Policy, deps Dependencies) Runtime {
	hash := sha256.Sum256([]byte(PlanningGuide()))
	r := Runtime{Build: version.String(), GuideSHA256: hex.EncodeToString(hash[:]), Status: "not_checked", Actions: []string{"define", "compose", "execute", "abstain"}}
	identity, err := processIdentity, processIdentityError
	r.Executable, r.BinarySHA256 = identity.path, identity.hash
	if err != nil {
		r.IdentityError = err.Error()
	}
	if deps.Search != nil {
		r.Actions = append(r.Actions, "search", "retry_search")
	}
	if deps.Inspect != nil {
		r.Actions = append(r.Actions, "inspect")
	}
	if deps.Layout != nil {
		r.Actions = append(r.Actions, "layout")
	}
	if deps.Sample != nil {
		r.Actions = append(r.Actions, "sample", "retry_sample")
	}
	if policy.EvidenceRecipient != "" {
		r.Actions = append(r.Actions, "read_evidence")
	}
	if policy.ReviewRecipient != "" && deps.Review != nil {
		r.Actions = append(r.Actions, "review_result")
	}
	return r
}

// Preflight checks the same acquisition adapter the goal will use, without
// calling the planner, consuming actions or authorizing data disclosure. Missing
// startup prerequisites block live execution before expensive planning begins.
func (e *Engine) Preflight(ctx context.Context) (View, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.expire()
	if e.state.Runtime.IdentityError != "" {
		e.state.Runtime.Status = "blocked"
		return e.snapshot(false), fmt.Errorf("cannot identify executing binary: %s", e.state.Runtime.IdentityError)
	}

	if e.state.Runtime.Status == "ready" || e.deps.Preflight == nil {
		return e.snapshot(false), nil
	}
	if e.state.Revision != 0 || e.state.Status != "exploring" {
		return e.snapshot(false), fmt.Errorf("preflight requires an unstarted goal")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	environment, err := e.deps.Preflight(ctx)
	if err == nil {
		err = ctx.Err()
	}
	for _, check := range environment.Checks {
		if err == nil && check.Status == "blocked" {
			err = fmt.Errorf("%s: %s", check.Name, check.Detail)
		}
	}
	e.state.Runtime.Environment = environment
	e.state.Runtime.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		e.state.Runtime.Status = "blocked"
		return e.snapshot(false), fmt.Errorf("goal preflight blocked: %w", err)
	}
	e.state.Runtime.Status = "ready"
	return e.snapshot(false), nil
}
