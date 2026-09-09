// Package discovery runs standalone Connection Hypothesis discovery. The MCP
// host supplies its own plans and continues to use catalog directly.
package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

// Planner is the seam for installed agent CLIs and scripted test adapters.
type Planner interface {
	Generate(context.Context, string, string) (agentplan.Plan, error)
	Expand(context.Context, string, string, agentplan.Plan, []catalog.Hit) (agentplan.Plan, error)
	Compose(context.Context, string, string, []catalog.Hit, []catalog.Hit) (agentplan.SelectionPlan, error)
}

type agentPlanner struct{}

func (agentPlanner) Generate(ctx context.Context, goal, provider string) (agentplan.Plan, error) {
	return agentplan.Generate(ctx, goal, provider)
}
func (agentPlanner) Expand(ctx context.Context, goal, provider string, prior agentplan.Plan, hits []catalog.Hit) (agentplan.Plan, error) {
	return agentplan.Expand(ctx, goal, provider, prior, hits)
}
func (agentPlanner) Compose(ctx context.Context, goal, provider string, anchors, hits []catalog.Hit) (agentplan.SelectionPlan, error) {
	return agentplan.Compose(ctx, goal, provider, anchors, hits)
}

// Request preserves search policy across all retrieval passes. Explicit Concepts
// skip model planning. Connections enables progressive selection after planning.
type Request struct {
	Plan        catalog.QueryPlan
	Provider    string
	Connections bool
}

type Result struct {
	catalog.Result
	Planner          *agentplan.Plan
	BridgePlanner    *agentplan.Plan
	SelectionPlanner *agentplan.SelectionPlan
}

// Runner owns progression, fallback and Abstention. Search supplies the same
// retrieval policy at each pass; errors stop progression before another planner
// runs. A nil Planner uses installed agent CLIs. Progress is optional display text.
// Per-run state is local, so tests and independent runs need no global overrides.
type Runner struct {
	Planner  Planner
	Search   func(context.Context, catalog.QueryPlan) (catalog.Result, error)
	Progress func(string)
}

func (r Runner) progress(format string, args ...any) {
	if r.Progress != nil {
		r.Progress(fmt.Sprintf(format, args...))
	}
}

func (r Runner) Run(ctx context.Context, request Request) (Result, error) {
	var out Result
	if r.Search == nil {
		return out, fmt.Errorf("discovery search가 초기화되지 않았습니다")
	}
	planner := r.Planner
	if planner == nil {
		planner = agentPlanner{}
	}
	plan := request.Plan
	goal := plan.Intent
	if len(plan.Concepts) == 0 && request.Provider != "none" {
		generated, err := planner.Generate(ctx, goal, request.Provider)
		if err != nil {
			if request.Provider != agentplan.ProviderAuto {
				return out, err
			}
			out.Planner = &agentplan.Plan{Status: agentplan.StatusUnavailable, Detail: err.Error()}
			r.progress("⚠ AI 검색 계획을 만들지 못해 원문 검색으로 폴백합니다: %v", err)
		} else {
			out.Planner = &generated
			plan.Concepts = generated.Concepts
			if len(generated.Axes) > 0 {
				plan.Axes = generated.Axes
			}
			r.progress("검색 계획 · %s: %s", generated.Provider, strings.Join(generated.Concepts, " · "))
		}
	}
	var err error
	out.Result, err = r.Search(ctx, plan)
	if err != nil {
		return out, err
	}
	if !request.Connections || out.Planner == nil || len(out.Planner.Axes) == 0 {
		return out, nil
	}
	anchors := selectAnchorHits(out.Hits, 1)
	if len(anchors) == 0 {
		out.Abstention = &catalog.Abstention{Reason: "초기 검색 결과에서 Anchor를 회수하지 못해 연결 후보를 만들지 않았습니다"}
		return out, nil
	}
	expanded, expandErr := planner.Expand(ctx, goal, out.Planner.Provider, *out.Planner, out.Hits)
	bridgeAxes := mergeBridgeAxes(nil, nonAnchorAxes(out.Planner.Axes), 7)
	if expandErr != nil {
		r.progress("⚠ 결과 기반 Bridge 재계획에 실패해 1차 역할축으로 계속합니다: %v", expandErr)
	} else {
		out.BridgePlanner = &expanded
		bridgeAxes = mergeBridgeAxes(expanded.Axes, nonAnchorAxes(out.Planner.Axes), 7)
		if len(expanded.Axes) > 0 {
			r.progress("Bridge 재계획 · %s: %s", expanded.Provider, strings.Join(expanded.Concepts, " · "))
		}
	}
	if len(bridgeAxes) == 0 {
		return out, nil
	}
	bridgePlan := catalog.QueryPlan{
		Intent: goal, Axes: append(anchorAxes(out.Planner.Axes), bridgeAxes...),
		Limit: plan.Limit, RESTOnly: plan.RESTOnly, IncludePreviews: plan.IncludePreviews,
		Ranking: plan.Ranking, MaxConnections: plan.MaxConnections,
	}
	for _, anchor := range anchors {
		bridgePlan.AnchorPKs = append(bridgePlan.AnchorPKs, anchor.PK)
	}
	out.Result, err = r.Search(ctx, bridgePlan)
	if err != nil {
		return out, err
	}
	options := connectionOptionHits(out.ConnectionOptions)
	if len(options) == 0 {
		options = out.Hits
	}
	selected, selectErr := planner.Compose(ctx, goal, out.Planner.Provider, anchors, options)
	if selectErr != nil {
		out.Abstention = &catalog.Abstention{Reason: "실제 Bridge PK 선택에 실패해 연결 카드를 만들지 않았습니다: " + selectErr.Error()}
		r.progress("⚠ Bridge 후보 선택에 실패해 카드 생성을 중단합니다: %v", selectErr)
		return out, nil
	}
	out.SelectionPlanner = &selected
	if len(selected.Selections) == 0 {
		out.Abstention = &catalog.Abstention{Reason: selected.AbstentionReason}
		return out, nil
	}
	bridgePlan.BridgeSelections = selected.Selections
	out.Result, err = r.Search(ctx, bridgePlan)
	return out, err
}

func selectAnchorHits(hits []catalog.Hit, limit int) []catalog.Hit {
	var anchors []catalog.Hit
	for _, hit := range hits {
		if !strings.EqualFold(hit.Role, "anchor") {
			continue
		}
		anchors = append(anchors, hit)
		if len(anchors) == limit {
			break
		}
	}
	return anchors
}

func nonAnchorAxes(axes []catalog.DiscoveryAxis) []catalog.DiscoveryAxis {
	out := make([]catalog.DiscoveryAxis, 0, len(axes))
	for _, axis := range axes {
		if !strings.EqualFold(axis.Role, "anchor") {
			out = append(out, axis)
		}
	}
	return out
}

func anchorAxes(axes []catalog.DiscoveryAxis) []catalog.DiscoveryAxis {
	for _, axis := range axes {
		if strings.EqualFold(strings.TrimSpace(axis.Role), "anchor") {
			return []catalog.DiscoveryAxis{axis}
		}
	}
	return nil
}

func connectionOptionHits(groups []catalog.ConnectionOptionGroup) []catalog.Hit {
	var out []catalog.Hit
	seen := map[string]bool{}
	for _, group := range groups {
		for _, hit := range group.Nodes {
			if seen[hit.PK] {
				continue
			}
			seen[hit.PK] = true
			out = append(out, hit)
		}
	}
	return out
}

// mergeBridgeAxes gives post-retrieval roles first, then fills unused slots
// from the initial plan. Repeating a role or query cannot buy another candidate.
func mergeBridgeAxes(primary, fallback []catalog.DiscoveryAxis, limit int) []catalog.DiscoveryAxis {
	seenRoles := map[string]bool{}
	seenQueries := map[string]bool{}
	var out []catalog.DiscoveryAxis
	for _, axes := range [][]catalog.DiscoveryAxis{primary, fallback} {
		for _, axis := range axes {
			role := strings.ToLower(strings.TrimSpace(axis.Role))
			query := strings.ToLower(strings.TrimSpace(axis.Query))
			if role == "" || query == "" || role == "anchor" || seenRoles[role] || seenQueries[query] {
				continue
			}
			seenRoles[role], seenQueries[query] = true, true
			out = append(out, axis)
			if len(out) == limit {
				return out
			}
		}
	}
	return out
}
