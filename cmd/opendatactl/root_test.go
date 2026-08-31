package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/opendatactl/internal/agentplan"
	"github.com/JungHoonGhae/opendatactl/internal/catalog"
)

func TestRootCommandPresentsOpenDataCTLBrand(t *testing.T) {
	if rootCmd.Use != "opendatactl" {
		t.Fatalf("root command use = %q", rootCmd.Use)
	}
	if !strings.Contains(rootCmd.Long, "OpenDataCTL") {
		t.Fatalf("root command does not present the product brand: %q", rootCmd.Long)
	}
}

func TestMergeBridgeAxesPrioritizesPostRetrievalAndDeduplicatesRoles(t *testing.T) {
	initial := []catalog.DiscoveryAxis{
		{Role: "수요", Query: "인구 이동"},
		{Role: "위험", Query: "침수 위험"},
	}
	expanded := []catalog.DiscoveryAxis{
		{Role: "상권", Query: "점포 개폐업"},
		{Role: "수요", Query: "전월세 거래"},
	}
	got := mergeBridgeAxes(expanded, initial, 3)
	if len(got) != 3 {
		t.Fatalf("axes = %+v", got)
	}
	if got[0].Role != "상권" || got[1].Query != "전월세 거래" || got[2].Role != "위험" {
		t.Fatalf("post-retrieval priority/dedup = %+v", got)
	}
}

func TestSelectAnchorHitsDoesNotGuessFromUnstructuredResults(t *testing.T) {
	hits := []catalog.Hit{{PK: "popular"}, {PK: "anchor", Role: "anchor"}}
	got := selectAnchorHits(hits, 1)
	if len(got) != 1 || got[0].PK != "anchor" {
		t.Fatalf("anchors = %+v", got)
	}
}

func TestCatalogDiscoverRunsProgressiveThreeStageFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	cat := &catalog.Catalog{SyncedAt: time.Now(), Type: "API", Entries: []catalog.Entry{
		{PK: "anchor", Title: "온비드 공매 물건", SvcType: catalog.SvcREST},
		{PK: "demand", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
		{PK: "risk", Title: "침수 위험 지도", SvcType: catalog.SvcREST},
	}}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}

	oldGenerate, oldExpand, oldCompose := generateDiscoveryPlan, expandDiscoveryPlan, composeDiscoveryPlan
	oldFormat := flagFormat
	t.Cleanup(func() {
		generateDiscoveryPlan, expandDiscoveryPlan, composeDiscoveryPlan = oldGenerate, oldExpand, oldCompose
		flagFormat = oldFormat
	})
	flagFormat = "json"
	spatial := catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}
	generateDiscoveryPlan = func(context.Context, string, string) (agentplan.Plan, error) {
		return agentplan.Plan{Provider: agentplan.ProviderCodex, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
			{Role: "anchor", Query: "온비드 공매 물건"},
			{Role: "상권 수요", Query: "상권 점포 개폐업", Contribution: "수요를 비교", Edge: spatial},
		}}, nil
	}
	expandDiscoveryPlan = func(_ context.Context, _ string, provider string, _ agentplan.Plan, observed []catalog.Hit) (agentplan.Plan, error) {
		if provider != agentplan.ProviderCodex || len(observed) == 0 {
			t.Fatalf("expand provider=%q observed=%+v", provider, observed)
		}
		return agentplan.Plan{Provider: provider, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{{
			Role: "재난 위험", Query: "침수 위험 지도", Contribution: "위험조정 판단", Edge: spatial,
		}}}, nil
	}
	composeDiscoveryPlan = func(_ context.Context, _ string, provider string, anchors, candidates []catalog.Hit) (agentplan.SelectionPlan, error) {
		if provider != agentplan.ProviderCodex || len(anchors) != 1 || anchors[0].PK != "anchor" {
			t.Fatalf("compose provider=%q anchors=%+v", provider, anchors)
		}
		for _, hit := range candidates {
			if hit.PK == "risk" {
				return agentplan.SelectionPlan{Provider: provider, Status: agentplan.StatusUsed, Selections: []catalog.BridgeSelection{{
					PK: hit.PK, WhyCandidate: "침수 위험 지도라는 제목이 위험 역할을 뒷받침",
				}}}, nil
			}
		}
		t.Fatalf("risk candidate missing: %+v", candidates)
		return agentplan.SelectionPlan{}, nil
	}

	cmd := catalogQueryCmd(true)
	cmd.SetArgs([]string{"공매 투자 판단", "--semantic=false"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("discover: %v\nstderr: %s", err, stderr.String())
	}
	var result struct {
		Connections []catalog.ConnectionCandidate `json:"connections"`
		Planner     agentplan.Plan                `json:"planner"`
		Bridge      agentplan.Plan                `json:"bridgePlanner"`
		Selection   agentplan.SelectionPlan       `json:"selectionPlanner"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.Planner.Status != agentplan.StatusUsed || result.Bridge.Status != agentplan.StatusUsed ||
		result.Selection.Status != agentplan.StatusUsed || len(result.Connections) != 1 {
		t.Fatalf("progressive result = %+v\nstderr: %s", result, stderr.String())
	}
	connection := result.Connections[0]
	if connection.Anchor.PK != "anchor" || connection.Bridge.PK != "risk" || connection.BridgeRole != "재난 위험" {
		t.Fatalf("connection = %+v", connection)
	}
}

func TestCatalogDiscoverAbstainsWhenInitialSearchHasNoAnchor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	cat := &catalog.Catalog{SyncedAt: time.Now(), Type: "API", Entries: []catalog.Entry{
		{PK: "bridge", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
	}}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}

	oldGenerate, oldFormat := generateDiscoveryPlan, flagFormat
	t.Cleanup(func() {
		generateDiscoveryPlan = oldGenerate
		flagFormat = oldFormat
	})
	flagFormat = "json"
	generateDiscoveryPlan = func(context.Context, string, string) (agentplan.Plan, error) {
		return agentplan.Plan{Provider: agentplan.ProviderCodex, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
			{Role: "anchor", Query: "존재하지 않는 직접 대상"},
			{Role: "수요", Query: "상권 점포", Contribution: "수요 비교", Edge: catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
			{Role: "위험", Query: "침수 위험", Contribution: "위험 비교", Edge: catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		}}, nil
	}

	cmd := catalogQueryCmd(true)
	cmd.SetArgs([]string{"사업 기회", "--semantic=false"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Connections []catalog.ConnectionCandidate `json:"connections"`
		Abstention  *catalog.Abstention           `json:"abstention"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Connections) != 0 || result.Abstention == nil {
		t.Fatalf("missing anchor must abstain: %+v", result)
	}
}
