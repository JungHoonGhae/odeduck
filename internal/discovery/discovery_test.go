package discovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

type scriptedPlanner struct {
	generate func(context.Context, string, string) (agentplan.Plan, error)
	expand   func(context.Context, string, string, agentplan.Plan, []catalog.Hit) (agentplan.Plan, error)
	compose  func(context.Context, string, string, []catalog.Hit, []catalog.Hit) (agentplan.SelectionPlan, error)
}

func (p scriptedPlanner) Generate(ctx context.Context, goal, provider string) (agentplan.Plan, error) {
	return p.generate(ctx, goal, provider)
}
func (p scriptedPlanner) Expand(ctx context.Context, goal, provider string, prior agentplan.Plan, hits []catalog.Hit) (agentplan.Plan, error) {
	return p.expand(ctx, goal, provider, prior, hits)
}
func (p scriptedPlanner) Compose(ctx context.Context, goal, provider string, anchors, hits []catalog.Hit) (agentplan.SelectionPlan, error) {
	return p.compose(ctx, goal, provider, anchors, hits)
}

func TestRunProgressiveDiscovery(t *testing.T) {
	for _, scenario := range []string{"success", "no-anchor", "expand-failure", "select-failure", "empty-selection", "search-1-failure", "search-2-failure", "search-3-failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			cat := &catalog.Catalog{Entries: []catalog.Entry{
				{PK: "anchor", Title: "온비드 공매 물건", SvcType: catalog.SvcREST},
				{PK: "demand", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
				{PK: "risk", Title: "침수 위험 지도", SvcType: catalog.SvcREST},
				{PK: "risk-history", Title: "침수 위험 이력", SvcType: catalog.SvcREST},
			}}
			edge := catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}
			axis := catalog.DiscoveryAxis{Role: "anchor", Query: "온비드 공매 물건"}
			if scenario == "no-anchor" {
				axis.Query = "존재하지않는질의"
			}
			var events []string
			searches := 0
			failure := errors.New("required semantic retrieval failed")
			planner := scriptedPlanner{
				generate: func(_ context.Context, goal, provider string) (agentplan.Plan, error) {
					events = append(events, "generate")
					if goal != "공매 판단" || provider != "auto" {
						t.Fatalf("goal=%s provider=%s", goal, provider)
					}
					return agentplan.Plan{Provider: "codex", Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{axis,
						{Role: "수요", Query: "상권 점포", Contribution: "수요 비교", Edge: edge},
					}}, nil
				},
				expand: func(_ context.Context, _ string, provider string, prior agentplan.Plan, hits []catalog.Hit) (agentplan.Plan, error) {
					events = append(events, "expand")
					if provider != "codex" || prior.Provider != provider || len(hits) == 0 {
						t.Fatalf("expand lost context")
					}
					if scenario == "expand-failure" {
						return agentplan.Plan{}, errors.New("planner offline")
					}
					return agentplan.Plan{Provider: provider, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
						{Role: "위험", Query: "침수 위험", Contribution: "위험조정 판단", Edge: edge},
					}}, nil
				},
				compose: func(_ context.Context, _ string, provider string, anchors, hits []catalog.Hit) (agentplan.SelectionPlan, error) {
					events = append(events, "compose")
					if provider != "codex" || len(anchors) != 1 || anchors[0].PK != "anchor" {
						t.Fatalf("compose lost anchor/provider")
					}
					if scenario == "select-failure" {
						return agentplan.SelectionPlan{}, errors.New("selection failed")
					}
					if scenario == "empty-selection" {
						return agentplan.SelectionPlan{AbstentionReason: "근거 부족"}, nil
					}
					pk := "risk-history"
					if scenario == "expand-failure" {
						pk = "demand"
					}
					for _, hit := range hits {
						if hit.PK == pk {
							return agentplan.SelectionPlan{Provider: provider, Status: agentplan.StatusUsed, Selections: []catalog.BridgeSelection{{PK: pk, WhyCandidate: "공식 제목이 역할을 뒷받침"}}}, nil
						}
					}
					t.Fatalf("full option pool missing %s: %+v", pk, hits)
					return agentplan.SelectionPlan{}, nil
				},
			}
			runner := Runner{Planner: planner, Search: func(_ context.Context, plan catalog.QueryPlan) (catalog.Result, error) {
				searches++
				events = append(events, "search")
				if plan.Limit != 2 || plan.Ranking != catalog.RankRecent || !plan.IncludePreviews || !plan.RESTOnly {
					t.Fatalf("lost search policy: %+v", plan)
				}
				if searches > 1 && (len(plan.AnchorPKs) != 1 || plan.AnchorPKs[0] != "anchor" || !reflect.DeepEqual(plan.Axes[0], axis)) {
					t.Fatalf("lost anchor: %+v", plan)
				}
				if scenario == fmt.Sprintf("search-%d-failure", searches) {
					return catalog.Result{}, failure
				}
				return cat.SearchPlan(plan), nil
			}}
			result, err := runner.Run(context.Background(), Request{Plan: catalog.QueryPlan{Intent: "공매 판단", Limit: 2, Ranking: catalog.RankRecent, IncludePreviews: true, RESTOnly: true, MaxConnections: 3}, Provider: "auto", Connections: true})
			wantEvents := []string{"generate", "search", "expand", "search", "compose", "search"}
			switch scenario {
			case "no-anchor", "search-1-failure":
				wantEvents = wantEvents[:2]
			case "search-2-failure":
				wantEvents = wantEvents[:4]
			case "select-failure", "empty-selection":
				wantEvents = wantEvents[:5]
			}
			if !reflect.DeepEqual(events, wantEvents) {
				t.Fatalf("events=%v want=%v", events, wantEvents)
			}
			if scenario == "search-1-failure" || scenario == "search-2-failure" || scenario == "search-3-failure" {
				if !errors.Is(err, failure) {
					t.Fatalf("error swallowed: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "no-anchor" || scenario == "select-failure" || scenario == "empty-selection" {
				if result.Abstention == nil || len(result.Connections) != 0 {
					t.Fatalf("must abstain: %+v", result)
				}
				return
			}
			if result.Planner == nil || result.SelectionPlanner == nil || len(result.Connections) != 1 {
				t.Fatalf("result=%+v", result)
			}
			wantPK := "risk-history"
			if scenario == "expand-failure" {
				wantPK = "demand"
			}
			if result.Connections[0].Anchor.PK != "anchor" || result.Connections[0].Bridge.PK != wantPK || result.Connections[0].Status != "candidate" {
				t.Fatalf("connection=%+v", result.Connections[0])
			}
		})
	}
}

func TestRunInitialPlanningPolicy(t *testing.T) {
	for _, tc := range []struct {
		provider                 string
		concepts                 []string
		wantGenerate, wantSearch bool
	}{
		{"auto", nil, true, true}, {"codex", nil, true, false}, {"", nil, true, false}, {"none", nil, false, true}, {"auto", []string{"제주"}, false, true},
	} {
		t.Run(tc.provider+fmt.Sprint(tc.concepts), func(t *testing.T) {
			t.Parallel()
			generated, searched := false, false
			runner := Runner{Planner: scriptedPlanner{generate: func(context.Context, string, string) (agentplan.Plan, error) {
				generated = true
				return agentplan.Plan{}, errors.New("offline")
			}}, Search: func(_ context.Context, plan catalog.QueryPlan) (catalog.Result, error) {
				searched = true
				if plan.Intent != "원래 목표" || !reflect.DeepEqual(plan.Concepts, tc.concepts) {
					t.Fatalf("lost intent: %+v", plan)
				}
				return catalog.Result{}, nil
			}}
			result, err := runner.Run(context.Background(), Request{Plan: catalog.QueryPlan{Intent: "원래 목표", Concepts: tc.concepts}, Provider: tc.provider, Connections: true})
			if generated != tc.wantGenerate || searched != tc.wantSearch || (err != nil) == tc.wantSearch {
				t.Fatalf("generated=%v searched=%v err=%v", generated, searched, err)
			}
			if tc.provider == "auto" && tc.concepts == nil && (result.Planner == nil || result.Planner.Status != agentplan.StatusUnavailable) {
				t.Fatalf("missing fallback diagnostic: %+v", result)
			}
		})
	}
}

func TestRunRespectsQueryPlanConnectionLimit(t *testing.T) {
	t.Parallel()
	cat := &catalog.Catalog{Entries: []catalog.Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: catalog.SvcREST},
		{PK: "flood", Title: "침수 위험", SvcType: catalog.SvcFILE},
		{PK: "fire", Title: "화재 위험", SvcType: catalog.SvcFILE},
	}}
	edge := catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"주소"}}
	axes := []catalog.DiscoveryAxis{
		{Role: "anchor", Query: "공매 물건"},
		{Role: "침수", Query: "침수 위험", Contribution: "침수 위험 비교", Edge: edge},
		{Role: "화재", Query: "화재 위험", Contribution: "화재 위험 비교", Edge: edge},
	}
	runner := Runner{Planner: scriptedPlanner{
		generate: func(context.Context, string, string) (agentplan.Plan, error) {
			return agentplan.Plan{Provider: "codex", Axes: axes}, nil
		},
		expand: func(context.Context, string, string, agentplan.Plan, []catalog.Hit) (agentplan.Plan, error) {
			return agentplan.Plan{Provider: "codex", Axes: axes[1:]}, nil
		},
		compose: func(context.Context, string, string, []catalog.Hit, []catalog.Hit) (agentplan.SelectionPlan, error) {
			return agentplan.SelectionPlan{Selections: []catalog.BridgeSelection{
				{PK: "flood", WhyCandidate: "공식 제목의 침수 위험"},
				{PK: "fire", WhyCandidate: "공식 제목의 화재 위험"},
			}}, nil
		},
	}, Search: func(ctx context.Context, plan catalog.QueryPlan) (catalog.Result, error) {
		return (catalog.Searcher{}).Search(ctx, cat, plan, catalog.SearchOptions{})
	}}
	result, err := runner.Run(context.Background(), Request{Plan: catalog.QueryPlan{Intent: "공매 위험 비교", MaxConnections: 1}, Provider: "codex", Connections: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Connections) != 1 || result.Connections[0].Bridge.PK != "flood" || result.Connections[0].Status != "candidate" {
		t.Fatalf("QueryPlan connection limit lost: %+v", result.Connections)
	}
}

func TestRunPrioritizesExpandedRolesAndBoundsBridgeBudget(t *testing.T) {
	t.Parallel()
	initial := []catalog.DiscoveryAxis{{Role: "anchor", Query: "anchor-query"}}
	for i := 1; i <= 7; i++ {
		initial = append(initial, catalog.DiscoveryAxis{Role: fmt.Sprint(i), Query: fmt.Sprintf("query-%d", i)})
	}
	expanded := []catalog.DiscoveryAxis{
		{Role: "1", Query: "new-query"},
		{Role: "1", Query: "duplicate-role"},
		{Role: "duplicate-query", Query: "new-query"},
		{Role: "8", Query: "query-8"},
		{Role: "anchor", Query: "replacement-anchor"},
	}
	searches := 0
	runner := Runner{Planner: scriptedPlanner{
		generate: func(context.Context, string, string) (agentplan.Plan, error) {
			return agentplan.Plan{Provider: "codex", Axes: initial}, nil
		},
		expand: func(context.Context, string, string, agentplan.Plan, []catalog.Hit) (agentplan.Plan, error) {
			return agentplan.Plan{Axes: expanded}, nil
		},
		compose: func(context.Context, string, string, []catalog.Hit, []catalog.Hit) (agentplan.SelectionPlan, error) {
			return agentplan.SelectionPlan{AbstentionReason: "no selection"}, nil
		},
	}, Search: func(_ context.Context, plan catalog.QueryPlan) (catalog.Result, error) {
		searches++
		if searches == 2 {
			want := append([]catalog.DiscoveryAxis{initial[0], expanded[0], expanded[3]}, initial[2:7]...)
			if !reflect.DeepEqual(plan.Axes, want) {
				t.Fatalf("axes=%+v want=%+v", plan.Axes, want)
			}
		}
		return catalog.Result{Hits: []catalog.Hit{{PK: "anchor", Role: "anchor"}}}, nil
	}}
	_, err := runner.Run(context.Background(), Request{Provider: "auto", Connections: true})
	if err != nil || searches != 2 {
		t.Fatalf("searches=%d err=%v", searches, err)
	}
	if initial[1].Query != "query-1" {
		t.Fatal("initial plan was mutated")
	}
}
