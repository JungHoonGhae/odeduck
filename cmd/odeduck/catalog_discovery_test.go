package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

// This fixture replaces only the installed external planner. The command still
// loads a real catalog and uses the shared discovery and retrieval paths.
type catalogPlannerFixture struct{}

func (catalogPlannerFixture) Generate(context.Context, string, string) (agentplan.Plan, error) {
	return agentplan.Plan{Provider: "codex", Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
		{Role: "anchor", Query: "온비드 공매 물건"},
		{Role: "수요", Query: "상권 점포", Contribution: "수요 비교", Edge: catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
	}}, nil
}

func (catalogPlannerFixture) Expand(context.Context, string, string, agentplan.Plan, []catalog.Hit) (agentplan.Plan, error) {
	return agentplan.Plan{Provider: "codex", Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
		{Role: "위험", Query: "침수 위험", Contribution: "위험조정 판단", Edge: catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
	}}, nil
}

func (catalogPlannerFixture) Compose(context.Context, string, string, []catalog.Hit, []catalog.Hit) (agentplan.SelectionPlan, error) {
	return agentplan.SelectionPlan{Provider: "codex", Status: agentplan.StatusUsed, Selections: []catalog.BridgeSelection{
		{PK: "risk-history", WhyCandidate: "공식 제목의 침수 이력을 공매 위험 비교에 사용할 후보"},
		{PK: "demand", WhyCandidate: "공식 제목의 점포 정보를 수요 비교에 사용할 후보"},
	}}, nil
}

func TestCatalogDiscoverReturnsProgressiveDiagnosticsAndSelectedCandidate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, ".config"))
	t.Setenv("APPDATA", filepath.Join(root, "AppData"))
	cat := &catalog.Catalog{SyncedAt: time.Now(), Entries: []catalog.Entry{
		{PK: "anchor", Title: "온비드 공매 물건", SvcType: catalog.SvcREST},
		{PK: "demand", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
		{PK: "risk", Title: "침수 위험 지도", SvcType: catalog.SvcREST},
		{PK: "risk-history", Title: "침수 위험 이력", SvcType: catalog.SvcREST},
	}}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}
	oldFormat := flagFormat
	t.Cleanup(func() { flagFormat = oldFormat })
	flagFormat = "json"
	cmd := catalogQueryCommand(true, catalogPlannerFixture{})
	cmd.SetArgs([]string{"공매 판단", "--agent=codex", "--semantic=false", "--limit=2", "--max-connections=1"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Intent           string
		Planner          *agentplan.Plan
		BridgePlanner    *agentplan.Plan
		SelectionPlanner *agentplan.SelectionPlan
		Connections      []catalog.ConnectionCandidate
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Intent != "공매 판단" || result.Planner == nil || result.BridgePlanner == nil || result.SelectionPlanner == nil {
		t.Fatalf("missing goal or progressive diagnostics: %s", stdout.String())
	}
	if result.Planner.Provider != "codex" || result.Planner.Status != agentplan.StatusUsed || len(result.Planner.Axes) != 2 || result.Planner.Axes[0].Role != "anchor" ||
		result.BridgePlanner.Provider != "codex" || len(result.BridgePlanner.Axes) != 1 || result.BridgePlanner.Axes[0].Role != "위험" ||
		result.SelectionPlanner.Provider != "codex" || len(result.SelectionPlanner.Selections) != 2 || result.SelectionPlanner.Selections[0].PK != "risk-history" {
		t.Fatalf("planner diagnostics replaced or truncated: %s", stdout.String())
	}
	if len(result.Connections) != 1 || result.Connections[0].Anchor.PK != "anchor" || result.Connections[0].Bridge.PK != "risk-history" || result.Connections[0].Status != "candidate" {
		t.Fatalf("selected candidate or CLI limit lost: %s", stdout.String())
	}
}
