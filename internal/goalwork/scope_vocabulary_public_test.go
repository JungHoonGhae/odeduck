package goalwork

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

// Seeded replay of the source revisions found by the recorded unseeded run.
// It tests the new interpretation step, not autonomous source selection or M5.
func TestLivePublishedScopeVocabularyRecoversRecordedMobilityPairs(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_SCOPE") != "1" {
		t.Skip("opt-in public G2 source replay; no planner or login")
	}
	b, err := os.ReadFile("testdata/goalbench-v1/mobility-unseeded-diagnostic-20260907.json")
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct{ Execution View }
	if err := json.Unmarshal(b, &recorded); err != nil {
		t.Fatal(err)
	}
	ref := recorded.Execution
	if len(ref.Compositions) != 1 || ref.Compositions[0].Base != "o6" {
		t.Fatal("recorded failure contract changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	deps := LiveDependencies(fetch.New(fetch.WithDelay(0)), "", nil, catalog.Searcher{}, Policy{})
	// Scripted replay starts at the already discovered source PKs. Acquisition,
	// source-contract resolution, ZIP reading and nearest scan are production code.
	deps.Search = func(_ context.Context, pk string) (catalog.Result, error) {
		return catalog.Result{Hits: []catalog.Hit{{PK: pk}}}, nil
	}
	e, err := Start(ref.Goal, Policy{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	step(Decision{Action: "define", Contract: ref.Contract}) // Preserve the original population interpretation.
	selected := map[string]string{"o1": "o1", "o4": "o2", "o5": "o3", "o6": "o4"}
	observations := map[string]Observation{}
	for _, o := range ref.Observations {
		observations[o.ID] = o
	}
	for _, attempt := range ref.SampleAttempts {
		id, wanted := selected[attempt.ObservationID]
		if !wanted {
			continue
		}
		s := attempt.Request
		if s.Nearest == nil {
			role := "facilities"
			if attempt.ObservationID == "o5" {
				role = "transit"
			}
			step(Decision{Action: "search", Query: s.PK, Role: role})
			step(Decision{Action: "inspect", PK: s.PK})
			if s.LayoutID != "" {
				v := step(Decision{Action: "layout", Layout: &LayoutRequest{PK: s.PK, Asset: s.Asset}})
				if len(v.Layouts) != 1 {
					t.Fatalf("layout unavailable: %+v", v.Gaps)
				}
				s.LayoutID = v.Layouts[0].ID
			}
		} else {
			s.Nearest.Anchor, s.Nearest.Candidate = selected[s.Nearest.Anchor], selected[s.Nearest.Candidate]
		}
		v := step(Decision{Action: "sample", Sample: &s})
		if len(v.Gaps) > 0 {
			t.Fatalf("live acquisition gap: %+v", v.Gaps)
		}
		o := v.Observations[len(v.Observations)-1]
		if o.ID != id || o.ContentSHA256 != observations[attempt.ObservationID].ContentSHA256 || o.RowCount != observations[attempt.ObservationID].RowCount {
			t.Fatal("live source/selection changed; preserve recorded failure rather than rewriting expectations")
		}
	}
	// Only observation addresses change because unused acquisitions were omitted.
	b, _ = json.Marshal(ref.Compositions[0])
	replace := strings.NewReplacer("o6", "o4", "o5", "o3", "o4", "o2", "o1", "o1")
	var p Composition
	if err := json.Unmarshal([]byte(replace.Replace(string(b))), &p); err != nil {
		t.Fatal(err)
	}
	p.ID = "literal-replay"
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Artifact != nil || v.Executions[0].Metrics[0].ScopeChecks[0].ConflictPairs != 9 {
		t.Fatal("original nine-pair failure did not reproduce")
	}
	p.ID = "published-sido-labels"
	p.Joins[0].Scopes[0].Vocabulary = "kr_sido_labels_20260907_v1"
	step(Decision{Action: "compose", Composition: &p})
	v = step(Decision{Action: "execute", CompositionID: p.ID})
	if v.Artifact == nil || len(v.Artifact.Rows) != 9 || len(v.Artifact.Sources) != 4 || v.Status == "output_ready" || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("expected nine candidate rows, not goal completion: %+v", v.Gaps)
	}
	counts := map[string]int{}
	for _, row := range v.Artifact.Rows {
		name, _ := row["o4.anchor.시설명"].(string)
		counts[name]++
		if row["o1.시도명"] != "인천" || row["o1.출입구 경사로 유무"] != "Y" || row["o1.장애인화장실 유무"] != "Y" || row["o4.anchor.데이터기준일자"] != "2025-03-31" {
			t.Fatal("original source values changed or independent accessibility reference disagrees")
		}
	}
	for _, name := range []string{"인천광역시립박물관", "인천도시역사관", "한국이민사박물관"} {
		if counts[name] != 3 {
			t.Fatalf("unexpected facility pairs: %+v", counts)
		}
	}
	c := v.Artifact.Metrics[0].ScopeChecks[0]
	if c.CandidatePairs != 9 || c.MatchedPairs != 9 || c.Vocabulary == nil || c.MeaningVerified {
		t.Fatal("missing or promoted label evidence")
	}
	t.Logf("literal conflicts 9 → published-label pairs 9; 3 facilities × 3 rail records; original population contract retained; status=%s evaluation=%s", v.Status, v.Evaluation.Status)
}
