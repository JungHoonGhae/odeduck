package goalwork_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Offline retention audit only. These historical engine outputs are neither an
// independent oracle nor a replay of the live planner/provider/source revision.
func TestRetainedGoalDiagnosticsDoNotBecomeCompletionEvidence(t *testing.T) {
	for _, tc := range []struct {
		file                                      string
		searches, revision, executions, finalRows int
	}{
		{"mobility-unseeded-diagnostic-20260907.json", 7, 27, 1, 0},
		{"mobility-unseeded-vocabulary-diagnostic-20260907.json", 5, 26, 2, 6},
	} {
		t.Run(tc.file, func(t *testing.T) {
			var record struct {
				SchemaVersion int `json:"schemaVersion"`
				Run           struct {
					Kind          string    `json:"kind"`
					Status        string    `json:"status"`
					Revision      int       `json:"revision"`
					CommandFailed bool      `json:"commandFailed"`
					StartedAt     time.Time `json:"startedAt"`
					FinishedAt    time.Time `json:"finishedAt"`
				} `json:"run"`
				Execution struct {
					goalwork.View
					ArtifactSummary *struct {
						Status    string   `json:"status"`
						RowCount  int      `json:"rowCount"`
						SourceIDs []string `json:"sourceIds"`
					} `json:"artifactSummary"`
				} `json:"execution"`
			}
			readReference(t, tc.file, &record)
			r, e := record.Run, record.Execution
			if record.SchemaVersion != 1 || r.Kind != "unseeded_diagnostic_not_completion_gate" || !r.CommandFailed || r.Status != "abstained" || e.Status != r.Status || r.Revision != tc.revision || e.Revision != r.Revision || !r.FinishedAt.After(r.StartedAt) {
				t.Fatal("diagnostic capture/command failure distinction or run identity changed")
			}
			if e.Contract == nil || e.Contract.Coverage != "population" || len(e.Searches) != tc.searches || len(e.Observations) != 7 || len(e.Executions) != tc.executions || e.Artifact != nil {
				t.Fatal("original interpretation, acquisition history or value-free retention boundary changed")
			}
			for _, search := range e.Searches {
				if search.Semantic == nil || search.Semantic.Status != "used" || search.Semantic.Model != "embeddinggemma:300m-qat-q4_0" {
					t.Fatal("recorded semantic use lost; it is not a goal-level ablation")
				}
			}
			observed := map[string]goalwork.Observation{}
			for _, o := range e.Observations {
				if _, exists := observed[o.ID]; exists {
					t.Fatal("duplicate observation")
				}
				observed[o.ID] = o
				if _, err := time.Parse(time.RFC3339Nano, o.ObservedAt); err != nil {
					t.Fatal("original observation time missing")
				}
				for _, hash := range []string{o.ContentSHA256, o.ContractSHA256, o.RequestSHA256, o.RowsSHA256} {
					b, err := hex.DecodeString(hash)
					if err != nil || len(b) != sha256.Size {
						t.Fatal("source revision/address evidence missing")
					}
				}
			}
			for _, attempt := range e.SampleAttempts {
				b, err := json.Marshal(attempt.Request)
				if err != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != attempt.RequestSHA256 {
					t.Fatal("recorded request/hash disagreement")
				}
				if attempt.Status == "acquired" {
					o, ok := observed[attempt.ObservationID]
					if !ok || o.RequestSHA256 != attempt.RequestSHA256 || o.PK != attempt.Request.PK {
						t.Fatal("successful request lost observation binding")
					}
					if attempt.Request.LayoutID != "" {
						pinned := false
						for _, layout := range e.Layouts {
							pinned = pinned || (layout.ID == attempt.Request.LayoutID && layout.Request.PK == o.PK && layout.Layout.SHA256 == o.ContentSHA256)
						}
						if !pinned {
							t.Fatal("sample lost its exact layout/source revision")
						}
					}
				}
			}
			last := e.Executions[len(e.Executions)-1]
			if last.RowCount != tc.finalRows {
				t.Fatal("historical candidate count changed")
			}
			if tc.finalRows == 0 {
				if last.Status != "failed" || e.ArtifactSummary != nil || e.Evaluation != nil {
					t.Fatal("failed join became an artifact")
				}
				return
			}
			if e.ArtifactSummary == nil || e.ArtifactSummary.Status != "sample_joined" || e.ArtifactSummary.RowCount != tc.finalRows || e.Evaluation == nil || e.Evaluation.Status != "partial" || !e.Evaluation.NeedsSemanticReview || e.Evaluation.Temporal.Status != "not_checked" {
				t.Fatal("partial candidate artifact became goal success or semantic/time approval")
			}
			for _, id := range e.ArtifactSummary.SourceIDs {
				if _, ok := observed[id]; !ok {
					t.Fatal("artifact lost source reference")
				}
			}
			failed := 0
			for _, check := range e.Evaluation.Checks {
				if !check.Passed {
					failed++
					if check.Kind != "coverage" || check.ID != "population" {
						t.Fatal("historical structural failure changed")
					}
				}
			}
			if failed != 1 {
				t.Fatal("population failure was silently weakened")
			}
			vocabulary, err := os.ReadFile("vocabularies/kr-sido-labels-20260907.json")
			if err != nil {
				t.Fatal(err)
			}
			wantHash := fmt.Sprintf("%x", sha256.Sum256(vocabulary))
			for _, execution := range e.Executions {
				uses := 0
				for _, metric := range execution.Metrics {
					for _, scope := range metric.ScopeChecks {
						if scope.Vocabulary != nil {
							uses++
							if scope.Vocabulary.ID != "kr_sido_labels_20260907_v1" || scope.Vocabulary.SHA256 != wantHash || scope.MeaningVerified {
								t.Fatal("vocabulary evidence changed or became identity approval")
							}
						}
					}
				}
				if uses == 0 {
					t.Fatal("autonomous vocabulary-use evidence missing")
				}
			}
		})
	}
}
