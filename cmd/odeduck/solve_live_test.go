package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// This is an opt-in diagnostic, not a goal-completion assertion. It exercises
// the real command, tool-free planner and live acquisition without source IDs,
// reference rows or a scripted action sequence. Normal tests never invoke it.
func TestLiveSolveUnseededMobilityDiagnostic(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_SOLVE") != "1" {
		t.Skip("opt-in live planner diagnostic; incurs installed agent usage")
	}
	dir, err := os.MkdirTemp("", "odeduck-unseeded-mobility-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("diagnostic artifacts retained at %s (not a benchmark pass)", dir)
	const goal = "인천의 공공 문화·관광시설을 휠체어 이용자가 비교할 수 있게, 시설의 접근성 정보와 주변 대중교통 정보를 연결해 표를 만들어줘. 자료별 기준일과 실제 이동 가능 여부를 확인하지 못한 부분도 보여줘."
	var out bytes.Buffer
	cmd := solveCmd()
	cmd.SetOut(&out)
	cmd.SetErr(os.Stderr)
	cmd.SetArgs([]string{goal, "--agent", "codex", "--max-rounds", "32"})
	started := time.Now().UTC()
	runErr := cmd.Execute()
	if err := os.WriteFile(filepath.Join(dir, "view.json"), out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var view goalwork.View
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("command did not produce a goal view: %v (command error: %v)", err, runErr)
	}
	summary := struct {
		Kind        string    `json:"kind"`
		StartedAt   time.Time `json:"startedAt"`
		FinishedAt  time.Time `json:"finishedAt"`
		Provider    string    `json:"provider"`
		Status      string    `json:"status"`
		Revision    int       `json:"revision"`
		CommandFail bool      `json:"commandFailed"`
	}{"unseeded_diagnostic_not_completion_gate", started, time.Now().UTC(), "codex", view.Status, view.Revision, runErr != nil}
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%s revision=%d observations=%d compositions=%d command_error=%v", view.Status, view.Revision, len(view.Observations), len(view.Compositions), runErr)
}

// Replay the discovered queries, not the planner, against the same local search
// interface. Differences establish retrieval contribution, not goal causality.
func TestLiveSolveRetrievalReplay(t *testing.T) {
	path := os.Getenv("ODEDUCK_SOLVE_DIAGNOSTIC_VIEW")
	if os.Getenv("ODEDUCK_LIVE_SOLVE") != "1" || path == "" {
		t.Skip("opt-in post-hoc retrieval replay of a retained diagnostic view")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var view goalwork.View
	if err := json.Unmarshal(b, &view); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	index, err := catalog.LoadSemanticIndex(cat)
	if err != nil {
		t.Fatal(err)
	}
	searcher := catalog.Searcher{Index: index}
	type comparison struct {
		Query            string                `json:"query"`
		Role             string                `json:"role"`
		RecordedSemantic *catalog.SemanticInfo `json:"recordedSemantic"`
		ReplayedSemantic *catalog.SemanticInfo `json:"replayedSemantic"`
		Lexical          []string              `json:"lexicalTop8"`
		Hybrid           []string              `json:"hybridTop8"`
	}
	report := struct {
		Kind            string       `json:"kind"`
		SourceView      string       `json:"sourceView"`
		ReplayedAt      time.Time    `json:"replayedAt"`
		CatalogSyncedAt time.Time    `json:"catalogSyncedAt"`
		CatalogEntries  int          `json:"catalogEntries"`
		Comparisons     []comparison `json:"comparisons"`
	}{Kind: "posthoc_retrieval_replay_not_goal_ablation", SourceView: path, ReplayedAt: time.Now().UTC(), CatalogSyncedAt: cat.SyncedAt, CatalogEntries: len(cat.Entries)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for _, search := range view.Searches {
		plan := catalog.QueryPlan{Intent: search.Query, Limit: 8, IncludePreviews: true}
		lexical := cat.SearchPlan(plan)
		hybrid, err := searcher.Search(ctx, cat, plan, catalog.SearchOptions{Semantic: true, RequireSemantic: true})
		if err != nil {
			t.Fatal(err)
		}
		row := comparison{Query: search.Query, Role: search.Role, RecordedSemantic: search.Semantic, ReplayedSemantic: hybrid.Semantic}
		for _, hit := range lexical.Hits {
			row.Lexical = append(row.Lexical, hit.PK)
		}
		for _, hit := range hybrid.Hits {
			row.Hybrid = append(row.Hybrid, hit.PK)
		}
		report.Comparisons = append(report.Comparisons, row)
		t.Logf("%q lexical=%v hybrid=%v", search.Query, row.Lexical, row.Hybrid)
	}
	b, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "odeduck-retrieval-replay-")
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "comparison.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("retrieval replay retained at %s (not a goal-ablation result)", path)
}
