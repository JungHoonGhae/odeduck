package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JungHoonGhae/opendatactl/internal/agentplan"
	"github.com/JungHoonGhae/opendatactl/internal/catalog"
	"github.com/JungHoonGhae/opendatactl/internal/output"
	"github.com/spf13/cobra"
)

var (
	generateDiscoveryPlan = agentplan.Generate
	expandDiscoveryPlan   = agentplan.Expand
	composeDiscoveryPlan  = agentplan.Compose
)

func catalogCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "catalog",
		Short: "API 카탈로그 — 무엇이 존재하는지 로컬에서 즉시 검색",
		Long: `data.go.kr 의 오픈API 목록을 로컬에 한 번 받아두고, 이후 검색을 네트워크 없이
즉시 처리합니다. 포털은 키워드 검색만 제공하므로, 카탈로그가 없으면 "이런 데이터가
있나?"를 확인하려면 검색어를 하나씩 추측해 볼 수밖에 없습니다.

  opendatactl catalog sync            전체 목록 수집 (수십 초)
  opendatactl catalog discover <목표> Codex·Claude·Gemini·Cursor로 검색축 생성 후 탐색
  opendatactl catalog sync --if-stale 오래됐을 때만 수집 — cron/CI 로 주기 갱신할 때
  opendatactl catalog semantic-build  Ollama 의미 벡터 인덱스 생성(선택)
  opendatactl catalog search 폭염     하이브리드 검색(인덱스 없으면 키워드 검색)
  opendatactl catalog search 폭염 --rest-only   호출 가능한(REST) 것만
  opendatactl catalog orgs 폭염       그 주제를 개방한 기관 순위
  opendatactl catalog info            언제 수집했는지 / 몇 건인지`,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(catalogSyncCmd(), catalogSemanticBuildCmd(), catalogSearchCmd(), catalogDiscoverCmd(), catalogOrgsCmd(), catalogInfoCmd())
	return c
}

func catalogSyncCmd() *cobra.Command {
	var dtype string
	var perPage int
	var ifStale bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "포털에서 전체 목록을 받아 로컬 카탈로그 갱신",
		Long: `포털의 전체 오픈API 목록을 수집해 로컬 카탈로그를 갱신합니다.

--if-stale 은 카탈로그가 아직 신선하면 아무것도 하지 않고 성공합니다. 갱신 주기를
판단하는 일을 사람이 기억하지 않아도 되도록, cron 이나 CI 가 조건 없이 걸어두는 용도입니다:

  0 4 * * *  opendatactl catalog sync --if-stale`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if ifStale {
				// Only an existing, still-fresh catalogue is a reason to skip. A
				// missing or unreadable one means sync is exactly what's needed.
				if cur, err := catalog.Load(); err == nil && !cur.Stale() {
					fmt.Fprintf(cmd.ErrOrStderr(), "카탈로그가 아직 신선합니다 (%.0f일 전, %d건) — 건너뜁니다\n",
						cur.Age().Hours()/24, len(cur.Entries))
					return nil
				}
			}
			start := time.Now()
			// Report every page, not every thousand: this runs for minutes, and
			// silence for the first stretch is indistinguishable from a hang.
			cat, err := catalog.Sync(cmd.Context(), newPortalClient(), dtype, perPage, func(n int) {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r  수집 중… %d건", n)
			})
			if err != nil {
				return err
			}
			if err := cat.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "\r카탈로그 %d건 저장 (%s, %.0f초)\n",
				len(cat.Entries), dtype, time.Since(start).Seconds())
			return nil
		},
	}
	c.Flags().StringVar(&dtype, "type", "API", "데이터 유형: API | FILE")
	c.Flags().IntVar(&perPage, "per-page", 200, "페이지당 요청 건수")
	c.Flags().BoolVar(&ifStale, "if-stale", false, "카탈로그가 오래됐을 때만 수집 (cron/CI 용)")
	return c
}

func catalogSearchCmd() *cobra.Command {
	return catalogQueryCmd(false)
}

func catalogDiscoverCmd() *cobra.Command {
	return catalogQueryCmd(true)
}

func catalogQueryCmd(discover bool) *cobra.Command {
	var limit int
	var restOnly bool
	var semantic bool
	var concepts []string
	var ranking string
	var previews bool
	var agent string
	var connections bool
	var maxConnections int
	use := "search <검색어…>"
	short := "로컬 카탈로그 하이브리드 검색 (키워드 + 선택적 의미 벡터)"
	defaultAgent := "none"
	if discover {
		use = "discover <목표…>"
		short = "Codex·Claude·Gemini·Cursor로 검색축을 만들고 기회 탐색"
		defaultAgent = agentplan.ProviderAuto
	}
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > catalog.MaxSearchLimit {
				return fmt.Errorf("--limit은 1~%d 사이여야 합니다", catalog.MaxSearchLimit)
			}
			if maxConnections < 1 || maxConnections > catalog.MaxConnectionCandidates {
				return fmt.Errorf("--max-connections는 1~%d 사이여야 합니다", catalog.MaxConnectionCandidates)
			}
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			cat, err := loadCatalog(cmd)
			if err != nil {
				return err
			}
			q := ""
			for i, a := range args {
				if i > 0 {
					q += " "
				}
				q += a
			}
			var planner, bridgePlanner *agentplan.Plan
			var selectionPlanner *agentplan.SelectionPlan
			if len(concepts) == 0 && agent != "none" {
				generated, planErr := generateDiscoveryPlan(cmd.Context(), q, agent)
				if planErr != nil {
					if agent != agentplan.ProviderAuto {
						return planErr
					}
					planner = &agentplan.Plan{Status: agentplan.StatusUnavailable, Detail: planErr.Error()}
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠ AI 검색 계획을 만들지 못해 원문 검색으로 폴백합니다: %v\n", planErr)
				} else {
					planner = &generated
					concepts = generated.Concepts
					fmt.Fprintf(cmd.ErrOrStderr(), "검색 계획 · %s: %s\n", generated.Provider, strings.Join(generated.Concepts, " · "))
				}
			}
			queryPlan := catalog.QueryPlan{
				Intent: q, Concepts: concepts, Limit: limit, RESTOnly: restOnly,
				IncludePreviews: previews, Ranking: ranking,
			}
			if planner != nil && len(planner.Axes) > 0 {
				queryPlan.Axes = planner.Axes
			}
			res := runCatalogQuery(cmd, cat, queryPlan, semantic)

			// Structured discovery is progressive: plan and retrieve an Anchor,
			// inspect results to retrieve Bridges, then select explicit PKs.
			// Search can emit metadata-backed candidates, never verified joins.
			if discover && connections && planner != nil && len(planner.Axes) > 0 {
				anchors := selectAnchorHits(res.Hits, 1)
				if len(anchors) == 0 {
					res.Abstention = &catalog.Abstention{Reason: "초기 검색 결과에서 Anchor를 회수하지 못해 연결 후보를 만들지 않았습니다"}
				}
				if len(anchors) > 0 {
					expanded, expandErr := expandDiscoveryPlan(cmd.Context(), q, planner.Provider, *planner, res.Hits)
					bridgeAxes := mergeBridgeAxes(nil, nonAnchorAxes(planner.Axes), 7)
					if expandErr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 결과 기반 Bridge 재계획에 실패해 1차 역할축으로 계속합니다: %v\n", expandErr)
					} else {
						bridgePlanner = &expanded
						bridgeAxes = mergeBridgeAxes(expanded.Axes, nonAnchorAxes(planner.Axes), 7)
						if len(expanded.Axes) > 0 {
							fmt.Fprintf(cmd.ErrOrStderr(), "Bridge 재계획 · %s: %s\n",
								expanded.Provider, strings.Join(expanded.Concepts, " · "))
						}
					}
					if len(bridgeAxes) > 0 {
						progressiveAxes := append(anchorAxes(planner.Axes), bridgeAxes...)
						anchorPKs := make([]string, 0, len(anchors))
						for _, anchor := range anchors {
							anchorPKs = append(anchorPKs, anchor.PK)
						}
						bridgePlan := catalog.QueryPlan{
							Intent: q, Axes: progressiveAxes, AnchorPKs: anchorPKs,
							Limit: limit, RESTOnly: restOnly, IncludePreviews: previews,
							Ranking: ranking, MaxConnections: maxConnections,
						}
						res = runCatalogQuery(cmd, cat, bridgePlan, semantic)
						selected, selectErr := composeDiscoveryPlan(cmd.Context(), q, planner.Provider, anchors, res.Hits)
						if selectErr != nil {
							res.Abstention = &catalog.Abstention{Reason: "실제 Bridge PK 선택에 실패해 연결 카드를 만들지 않았습니다: " + selectErr.Error()}
							fmt.Fprintf(cmd.ErrOrStderr(), "⚠ Bridge 후보 선택에 실패해 카드 생성을 중단합니다: %v\n", selectErr)
						} else {
							selectionPlanner = &selected
							if len(selected.Selections) == 0 {
								res.Abstention = &catalog.Abstention{Reason: selected.AbstentionReason}
							} else {
								bridgePlan.BridgeSelections = selected.Selections
								res = runCatalogQuery(cmd, cat, bridgePlan, semantic)
							}
						}
					}
				}
			}
			hits, total := res.Hits, res.Total
			if format != output.Table {
				result := map[string]any{
					"mode": res.Mode, "intent": res.Intent, "queries": res.Queries, "semantic": res.Semantic,
					"terms": res.Terms, "relaxed": res.Relaxed,
					"total": total, "shown": len(hits), "hits": hits,
					"anchors": res.Anchors, "connections": res.Connections,
					"warnings": res.Warnings, "abstention": res.Abstention,
				}
				if planner != nil {
					result["planner"] = planner
				}
				if bridgePlanner != nil {
					result["bridgePlanner"] = bridgePlanner
				}
				if selectionPlanner != nil {
					result["selectionPlanner"] = selectionPlanner
				}
				return output.WriteJSON(cmd.OutOrStdout(), result)
			}
			if total == 0 {
				if res.Abstention != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "연결 후보를 만들지 않았습니다: %s\n", res.Abstention.Reason)
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(), "일치하는 데이터가 없습니다.")
				}
				return nil
			}
			if res.Relaxed {
				if len(res.Queries) > 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "일부 검색축은 모든 단어가 일치하지 않아 절반 이상 일치한 후보까지 포함합니다.")
					fmt.Fprintln(cmd.ErrOrStderr())
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"모든 단어를 포함하는 결과가 없어 일부만 일치하는 것까지 보여줍니다 (검색어: %s)\n\n",
						strings.Join(res.Terms, " "))
				}
			}
			headers := []string{"pk", "유형", "활용신청", "수정일", "발견축", "제공기관", "데이터명"}
			rows := make([][]string, 0, len(hits))
			for _, h := range hits {
				svc := h.SvcType
				if svc == "" {
					svc = "?"
				}
				rows = append(rows, []string{h.PK, svc, fmt.Sprintf("%d", h.ApplyCount), h.ModifiedAt, h.MatchedQuery, h.Org, h.Title})
			}
			if err := output.WriteTable(cmd.OutOrStdout(), headers, rows); err != nil {
				return err
			}
			if len(res.Connections) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\n연결 후보 (아직 실제 join을 확인하지 않은 candidate)")
				connectionRows := make([][]string, 0, len(res.Connections))
				for _, connection := range res.Connections {
					connectionRows = append(connectionRows, []string{
						connection.Anchor.PK, connection.BridgeRole, connection.Bridge.PK,
						strings.Join(connection.Edge.ExpectedKeys, " + "), connection.IncrementalValue,
					})
				}
				if err := output.WriteTable(cmd.OutOrStdout(),
					[]string{"Anchor", "Bridge 역할", "Bridge PK", "예상 결합키", "결합 후 새 판단"},
					connectionRows); err != nil {
					return err
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "⚠ candidate는 데이터 존재만 확인한 상태입니다. describe와 공통 slice의 call 표본으로 결합키·match rate·cardinality를 확인하세요.")
			} else if res.Abstention != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "\n연결 후보를 만들지 않았습니다: %s\n", res.Abstention.Reason)
			}
			if total > len(hits) {
				fmt.Fprintf(cmd.ErrOrStderr(), "\n총 %d건 중 %d건 표시 — 검색어를 좁히거나 --limit 을 올리세요.\n", total, len(hits))
			}
			return nil
		},
	}
	c.Flags().IntVar(&limit, "limit", 20, "표시할 최대 건수")
	c.Flags().BoolVar(&restOnly, "rest-only", true, "호출 가능한 REST만 검색 (전체 탐색은 --rest-only=false)")
	c.Flags().BoolVar(&semantic, "semantic", true, "준비된 Ollama 의미 인덱스를 자동 사용 (--semantic=false 로 비활성화)")
	c.Flags().StringArrayVar(&concepts, "concept", nil, "자연어 목표에서 추론한 구체적 검색축 (반복 가능, MCP 의미 분해 재현용)")
	c.Flags().StringVar(&agent, "agent", defaultAgent, "검색 계획기: none | auto | codex | claude | gemini | cursor (CLI의 기존 로그인 사용)")
	c.Flags().StringVar(&ranking, "ranking", catalog.RankBalanced, "검색축 내 순위: balanced | demand | recent")
	c.Flags().BoolVar(&previews, "previews", true, "JSON 결과에 공식 설명의 짧은 미리보기 포함")
	c.Flags().BoolVar(&connections, "connections", discover, "실제 1차 결과를 보고 Bridge 역할을 재계획해 연결 candidate 생성")
	c.Flags().IntVar(&maxConnections, "max-connections", 3, "반환할 연결 candidate 수")
	return c
}

func runCatalogQuery(cmd *cobra.Command, cat *catalog.Catalog, plan catalog.QueryPlan, semantic bool) catalog.Result {
	if !semantic {
		return cat.SearchPlan(plan)
	}
	idx, indexErr := catalog.LoadSemanticIndex(cat)
	switch {
	case indexErr == nil:
		embedder := catalog.NewOllamaEmbedder(catalog.OllamaURLFromEnv(), idx.Model)
		return cat.SearchHybrid(cmd.Context(), plan, idx, embedder)
	case errors.Is(indexErr, catalog.ErrSemanticIndexStale):
		res := cat.SearchPlan(plan)
		res.Semantic = &catalog.SemanticInfo{Status: catalog.SemanticUnavailable, Detail: "카탈로그 갱신 후 semantic-build 필요"}
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠ 의미 인덱스가 현재 카탈로그와 다릅니다 — `opendatactl catalog semantic-build` 로 갱신하세요.")
		return res
	case errors.Is(indexErr, catalog.ErrSemanticIndexNotBuilt):
		return cat.SearchHybrid(cmd.Context(), plan, nil, nil)
	default:
		res := cat.SearchPlan(plan)
		res.Semantic = &catalog.SemanticInfo{Status: catalog.SemanticUnavailable, Detail: "의미 인덱스 로드 실패"}
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 의미 인덱스를 읽지 못해 키워드 검색으로 폴백합니다: %v\n", indexErr)
		return res
	}
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

func catalogSemanticBuildCmd() *cobra.Command {
	var model, ollamaURL string
	var batchSize int
	var pull bool
	c := &cobra.Command{
		Use:   "semantic-build",
		Short: "Ollama로 전체 카탈로그 의미 벡터 인덱스 생성",
		Long: `선택 기능입니다. Ollama 하나만 설치되어 있으면 모델 다운로드부터 전체 카탈로그
임베딩·로컬 캐시까지 한 번에 처리합니다. 별도 벡터 DB는 필요하지 않습니다. 카탈로그를
sync 한 뒤 다시 실행하면 새 스냅샷에 맞춰 인덱스를 교체합니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := loadCatalog(cmd)
			if err != nil {
				return err
			}
			embedder := catalog.NewOllamaEmbedder(ollamaURL, model)
			if pull {
				fmt.Fprintf(cmd.ErrOrStderr(), "Ollama 모델 확인·다운로드: %s\n", embedder.Model())
				if err := embedder.Pull(cmd.Context()); err != nil {
					return err
				}
			}
			start := time.Now()
			idx, err := catalog.BuildSemanticIndex(cmd.Context(), cat, embedder, batchSize, func(done, total int) {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r  의미 인덱싱… %d/%d (%.0f%%)", done, total, float64(done)*100/float64(total))
			})
			if err != nil {
				return err
			}
			if err := idx.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "\r의미 인덱스 %d건 저장 · 모델 %s · %.1f초\n",
				len(idx.PKs), idx.Model, time.Since(start).Seconds())
			return nil
		},
	}
	c.Flags().StringVar(&model, "model", catalog.DefaultEmbeddingModel, "Ollama 임베딩 모델")
	c.Flags().StringVar(&ollamaURL, "ollama-url", catalog.OllamaURLFromEnv(), "Ollama base URL (기본 http://127.0.0.1:11434)")
	c.Flags().IntVar(&batchSize, "batch-size", 32, "한 번에 임베딩할 카탈로그 항목 수")
	c.Flags().BoolVar(&pull, "pull", true, "빌드 전에 Ollama 모델 확인·다운로드")
	return c
}

func catalogOrgsCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "orgs [검색어…]",
		Short: "주제를 개방한 기관 순위 (검색어 생략 시 전체)",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			cat, err := loadCatalog(cmd)
			if err != nil {
				return err
			}
			q := ""
			for i, a := range args {
				if i > 0 {
					q += " "
				}
				q += a
			}
			orgs := cat.Orgs(q, limit)
			if format != output.Table {
				return output.WriteJSON(cmd.OutOrStdout(), orgs)
			}
			headers := []string{"API수", "활용신청 합", "제공기관"}
			rows := make([][]string, 0, len(orgs))
			for _, o := range orgs {
				rows = append(rows, []string{fmt.Sprintf("%d", o.Count), fmt.Sprintf("%d", o.ApplySum), o.Org})
			}
			return output.WriteTable(cmd.OutOrStdout(), headers, rows)
		},
	}
	c.Flags().IntVar(&limit, "limit", 15, "표시할 기관 수")
	return c
}

func catalogInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "카탈로그 수집 시각·건수",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := loadCatalog(cmd)
			if err != nil {
				return err
			}
			format, ferr := resolveFormat()
			if ferr != nil {
				return ferr
			}
			if format != output.Table {
				return output.WriteJSON(cmd.OutOrStdout(), map[string]any{
					"syncedAt": cat.SyncedAt, "type": cat.Type,
					"entries": len(cat.Entries), "stale": cat.Stale(),
					"svcTypes": cat.SvcTypes(),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "수집 %s (%.0f시간 전) · %s 유형 · %d건%s\n",
				cat.SyncedAt.Local().Format("2006-01-02 15:04"), cat.Age().Hours(),
				cat.Type, len(cat.Entries),
				map[bool]string{true: " · ⚠ 오래됨, sync 권장", false: ""}[cat.Stale()])
			// Which share is callable from the portal at all is the number that
			// decides whether a search result is worth applying for.
			byType := cat.SvcTypes()
			kinds := make([]string, 0, len(byType))
			for k := range byType {
				kinds = append(kinds, k)
			}
			sort.Slice(kinds, func(i, j int) bool { return byType[kinds[i]] > byType[kinds[j]] })
			for _, k := range kinds {
				note := ""
				switch k {
				case catalog.SvcREST:
					note = " — 포털에 명세 있음, describe/call 가능"
				case catalog.SvcLINK:
					note = " — 포털에 명세 없음(제공기관 사이트로 연결)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-6s %6d건%s\n", k, byType[k], note)
			}
			return nil
		},
	}
}

// loadCatalog reads the catalogue and nudges when it is old, so a stale snapshot
// never silently passes for a complete one.
func loadCatalog(cmd *cobra.Command) (*catalog.Catalog, error) {
	cat, err := catalog.Load()
	if errors.Is(err, catalog.ErrNotSynced) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if cat.Stale() {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"⚠ 카탈로그가 %.0f일 전 것입니다 — `opendatactl catalog sync` 로 갱신하세요.\n", cat.Age().Hours()/24)
	}
	return cat, nil
}
