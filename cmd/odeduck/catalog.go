package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/discovery"
	"github.com/JungHoonGhae/odeduck/internal/output"
	"github.com/JungHoonGhae/odeduck/internal/portal"
	"github.com/spf13/cobra"
)

var (
	newOfficialSource = func(key string) catalog.SyncSource {
		return catalog.NewOfficialSource(newFetchClient(), key)
	}
)

func catalogPortalBaseURL() string {
	if flagBaseURL != "" {
		return flagBaseURL
	}
	return portal.BaseURL
}

func newCombinedCatalogSource() catalog.SyncSource {
	return catalog.NewCombinedSource(
		catalog.NewOfficialFileSource(newFetchClient(), catalogPortalBaseURL()),
		catalog.NewWebSource(newPortalClient()),
	)
}

func catalogCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "catalog",
		Short: "공개데이터 카탈로그 — API와 파일에서 무엇이 존재하는지 즉시 검색",
		Long: `data.go.kr 의 오픈API와 파일데이터 목록을 로컬에 한 번 받아두고, 이후 검색을 네트워크 없이
즉시 처리합니다. 포털은 키워드 검색만 제공하므로, 카탈로그가 없으면 "이런 데이터가
있나?"를 확인하려면 검색어를 하나씩 추측해 볼 수밖에 없습니다.

  odeduck catalog sync            전체 목록 갱신 (웹 보강 시 수십 분)
  odeduck catalog sync --source official-file  빠른 월간 CSV 목록 (일부 복수 제공형 제외)
  odeduck catalog discover <목표> Codex·Claude·Gemini·Cursor로 검색축 생성 후 탐색
  odeduck catalog sync --if-stale 오래됐을 때만 수집 — cron/CI 로 주기 갱신할 때
  odeduck catalog semantic-build  Ollama 의미 벡터 인덱스 생성(선택)
  odeduck catalog search 폭염     하이브리드 검색(인덱스 없으면 키워드 검색)
  odeduck catalog search 폭염 --rest-only   포털 명세가 있는 REST만
  odeduck catalog orgs 폭염       그 주제를 개방한 기관 순위
  odeduck catalog info            언제 수집했는지 / 몇 건인지`,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(catalogSyncCmd(), catalogInstallSnapshotCmd(), catalogSemanticBuildCmd(), catalogSearchCmd(), catalogDiscoverCmd(), catalogOrgsCmd(), catalogInfoCmd(), catalogValidateReleaseCmd())
	return c
}

func catalogInstallSnapshotCmd() *cobra.Command {
	var checkOnly bool
	command := &cobra.Command{
		Use:   "install-snapshot <catalog.json.gz>",
		Short: "검증된 릴리즈 사전 구축 카탈로그를 검사하거나 원자적으로 설치",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			var result catalog.InstallResult
			var installErr error
			if checkOnly {
				result, installErr = catalog.ValidateSnapshot(file)
			} else {
				result, installErr = catalog.InstallSnapshot(file)
			}
			closeErr := file.Close()
			if installErr != nil {
				return installErr
			}
			if closeErr != nil {
				return closeErr
			}
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			if format != output.Table {
				return output.WriteJSON(cmd.OutOrStdout(), result)
			}
			if checkOnly {
				fmt.Fprintf(cmd.OutOrStdout(), "사전 구축 카탈로그 검증 완료 (%d건, %s, %s)\n", result.Entries, result.Source, result.SyncedAt)
				return nil
			}
			if result.Installed {
				fmt.Fprintf(cmd.OutOrStdout(), "사전 구축 카탈로그 %d건 설치 (%s, %s)\n", result.Entries, result.Source, result.SyncedAt)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "기존 로컬 카탈로그가 더 최신하고 coverage도 동등 이상이라 유지했습니다 (%s)\n", result.SyncedAt)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&checkOnly, "check-only", false, "로컬 상태를 바꾸지 않고 압축·구조·출처만 검증")
	return command
}

func catalogSyncCmd() *cobra.Command {
	var dtype string
	var sourceMode string
	var perPage int
	var ifStale bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "포털에서 전체 목록을 받아 로컬 카탈로그 갱신",
		Long: `공식 목록조회 API를 우선 사용해 전체 오픈API와 파일데이터 카탈로그를 갱신합니다.
기본 auto는 계정 인증키로 공식 API를 시도하고, 해당 API가 기관 전용이라 사용할 수 없으면
공개 월간 목록 CSV에 웹 제공형을 보강합니다. 웹 보강이 실패하면 CSV-only로 fallback합니다.
빠른 CSV 목록만 필요하면 --source official-file을 사용하세요. 전체 웹 보강은 수십 분 걸릴 수 있습니다.
릴리즈 snapshot은 --source official-file+web으로 로그인·secret 없이 재현할 수 있습니다.

--if-stale 은 카탈로그가 아직 신선하면 아무것도 하지 않고 성공합니다. 갱신 주기를
판단하는 일을 사람이 기억하지 않아도 되도록, cron 이나 CI 가 조건 없이 걸어두는 용도입니다:

  0 4 * * *  odeduck catalog sync --if-stale`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sourceMode = strings.ToLower(strings.TrimSpace(sourceMode))
			if sourceMode != "auto" && sourceMode != catalog.SourceOfficial && sourceMode != catalog.SourceCombined && sourceMode != catalog.SourceOfficialFile && sourceMode != catalog.SourceWeb {
				return fmt.Errorf("--source는 auto, official, official-file+web, official-file, web 중 하나여야 합니다")
			}
			if ifStale {
				// Only an existing, still-fresh catalogue is a reason to skip. A
				// missing or unreadable one means sync is exactly what's needed.
				if cur, err := catalog.Load(); err == nil && !cur.Stale() && cur.CoversType(dtype) {
					fmt.Fprintf(cmd.ErrOrStderr(), "카탈로그가 아직 신선합니다 (%.0f일 전, %d건) — 건너뜁니다\n",
						cur.Age().Hours()/24, len(cur.Entries))
					return nil
				}
			}
			start := time.Now()
			progress := func(n int) {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r  수집 중… %d건", n)
			}
			var cat *catalog.Catalog
			var err error
			var source catalog.SyncSource
			if sourceMode == catalog.SourceOfficialFile {
				source = catalog.NewOfficialFileSource(newFetchClient(), catalogPortalBaseURL())
				cat, err = source.Sync(cmd.Context(), dtype, perPage, progress)
			}
			if sourceMode == catalog.SourceCombined {
				source = newCombinedCatalogSource()
				cat, err = source.Sync(cmd.Context(), dtype, perPage, progress)
			}
			if sourceMode == "auto" || sourceMode == catalog.SourceOfficial {
				key := strings.TrimSpace(os.Getenv("ODEDUCK_CATALOG_KEY"))
				var keyErr error
				if key == "" {
					key, keyErr = portal.APIKey(cmd.Context())
				}
				if keyErr == nil {
					source = newOfficialSource(key)
					cat, err = source.Sync(cmd.Context(), dtype, perPage, progress)
				} else {
					err = keyErr
				}
				if sourceMode == catalog.SourceOfficial && err != nil {
					return err
				}
				if sourceMode == "auto" && err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n⚠ 공식 카탈로그 API를 사용할 수 없어 공개 월간 목록 CSV로 전환합니다: %v\n", err)
					cat, err = nil, nil
				}
			}
			if sourceMode == "auto" && cat == nil && err == nil {
				source = newCombinedCatalogSource()
				cat, err = source.Sync(cmd.Context(), dtype, perPage, progress)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n⚠ 공개 월간 목록과 웹 보강을 결합할 수 없어 CSV-only fallback을 시도합니다: %v\n", err)
					cat, err = nil, nil
					source = catalog.NewOfficialFileSource(newFetchClient(), catalogPortalBaseURL())
					cat, err = source.Sync(cmd.Context(), dtype, perPage, progress)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n⚠ 공개 월간 목록 CSV를 사용할 수 없어 웹 수집으로 전환합니다: %v\n", err)
						cat, err = nil, nil
					}
				}
			}
			if cat == nil && err == nil {
				source = catalog.NewWebSource(newPortalClient())
				cat, err = source.Sync(cmd.Context(), dtype, perPage, progress)
			}
			if err != nil {
				return err
			}
			if sourceMode == "auto" {
				if current, loadErr := catalog.Load(); loadErr == nil && catalog.PreserveOnAutoSync(current, cat) {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n⚠ 새 snapshot(source=%s, %d건)이 기존 snapshot(source=%s, %d건)의 coverage를 낮춰 기존 파일을 유지합니다\n",
						cat.Source, len(cat.Entries), current.Source, len(current.Entries))
					return nil
				}
			}
			if err := cat.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "\r카탈로그 %d건 저장 (%s, source=%s, %.0f초)\n",
				len(cat.Entries), dtype, cat.Source, time.Since(start).Seconds())
			return nil
		},
	}
	c.Flags().StringVar(&dtype, "type", "ALL", "데이터 유형: ALL | API | FILE")
	c.Flags().StringVar(&sourceMode, "source", "auto", "수집 원천: auto | official | official-file+web | official-file | web")
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
	return catalogQueryCommand(discover, nil)
}

func catalogQueryCommand(discover bool, planner discovery.Planner) *cobra.Command {
	var limit int
	var restOnly bool
	var semantic bool
	var requireSemantic bool
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
			if requireSemantic && !semantic {
				return fmt.Errorf("--require-semantic과 --semantic=false는 함께 사용할 수 없습니다")
			}
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			cat, err := loadCatalog(cmd)
			if err != nil {
				return err
			}
			runner := discovery.Runner{
				Planner: planner,
				Search: func(ctx context.Context, plan catalog.QueryPlan) (catalog.Result, error) {
					result, err := (catalog.Searcher{}).Search(ctx, cat, plan, catalog.SearchOptions{Semantic: semantic, RequireSemantic: requireSemantic})
					for _, warning := range result.Warnings {
						fmt.Fprintln(cmd.ErrOrStderr(), "⚠ "+warning)
					}
					return result, err
				},
				Progress: func(message string) { fmt.Fprintln(cmd.ErrOrStderr(), message) },
			}
			out, err := runner.Run(cmd.Context(), discovery.Request{
				Plan: catalog.QueryPlan{
					Intent: strings.Join(args, " "), Concepts: concepts, Limit: limit, RESTOnly: restOnly,
					IncludePreviews: previews, Ranking: ranking, MaxConnections: maxConnections,
				},
				Provider: agent, Connections: discover && connections,
			})
			if err != nil {
				return err
			}
			res := out.Result
			planner, bridgePlanner, selectionPlanner := out.Planner, out.BridgePlanner, out.SelectionPlanner
			hits, total := res.Hits, res.Total
			if format != output.Table {
				result := map[string]any{
					"mode": res.Mode, "intent": res.Intent, "queries": res.Queries, "semantic": res.Semantic,
					"terms": res.Terms, "relaxed": res.Relaxed,
					"total": total, "shown": len(hits), "hits": hits,
					"anchors": res.Anchors, "connectionOptions": res.ConnectionOptions, "connections": res.Connections,
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
			headers := []string{"pk", "유형", "활용신청", "조회", "수정일", "발견축", "제공기관", "데이터명"}
			rows := make([][]string, 0, len(hits))
			for _, h := range hits {
				svc := h.SvcType
				if svc == "" {
					svc = "?"
				}
				rows = append(rows, []string{h.PK, svc, fmt.Sprintf("%d", h.ApplyCount), fmt.Sprintf("%d", h.ViewCount), h.ModifiedAt, h.MatchedQuery, h.Org, h.Title})
			}
			if err := output.WriteTable(cmd.OutOrStdout(), headers, rows); err != nil {
				return err
			}
			if len(res.ConnectionOptions) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\n조합 선택지 (역할별 검색 후보, 아직 연결 주장이 아님)")
				var optionRows [][]string
				for _, group := range res.ConnectionOptions {
					for _, node := range group.Nodes {
						optionRows = append(optionRows, []string{group.Role, node.PK, node.SvcType, node.Org, node.Title})
					}
				}
				if err := output.WriteTable(cmd.OutOrStdout(), []string{"역할", "PK", "유형", "제공기관", "데이터명"}, optionRows); err != nil {
					return err
				}
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
	c.Flags().BoolVar(&restOnly, "rest-only", false, "포털 명세가 있는 REST만 검색 (기본은 LINK까지 발견)")
	c.Flags().BoolVar(&semantic, "semantic", true, "준비된 Ollama 의미 인덱스를 자동 사용 (--semantic=false 로 비활성화)")
	c.Flags().BoolVar(&requireSemantic, "require-semantic", false, "의미 검색이 실제 사용되지 않으면 폴백하지 않고 실패 (고신뢰 조사·평가용)")
	c.Flags().StringArrayVar(&concepts, "concept", nil, "자연어 목표에서 추론한 구체적 검색축 (반복 가능, MCP 의미 분해 재현용)")
	c.Flags().StringVar(&agent, "agent", defaultAgent, "검색 계획기: none | auto | codex | claude | gemini | cursor (CLI의 기존 로그인 사용)")
	c.Flags().StringVar(&ranking, "ranking", catalog.RankBalanced, "검색축 내 순위: balanced | demand | recent")
	c.Flags().BoolVar(&previews, "previews", true, "JSON 결과에 공식 설명의 짧은 미리보기 포함")
	c.Flags().BoolVar(&connections, "connections", discover, "실제 1차 결과를 보고 Bridge 역할을 재계획해 연결 candidate 생성")
	c.Flags().IntVar(&maxConnections, "max-connections", 3, "반환할 연결 candidate 수")
	return c
}

func catalogSemanticBuildCmd() *cobra.Command {
	var model, ollamaURL string
	var batchSize int
	var pull bool
	c := &cobra.Command{
		Use:   "semantic-build",
		Short: "Ollama로 전체 카탈로그 의미 벡터 인덱스 생성",
		Long: `선택 기능입니다. Ollama 하나만 설치되어 있으면 모델 다운로드부터 카탈로그
임베딩·로컬 캐시까지 한 번에 처리합니다. 별도 벡터 DB는 필요하지 않습니다. 카탈로그를
sync 한 뒤 다시 실행하면 변경되거나 새로 생긴 문서만 임베딩하고 기존 벡터는 재사용합니다.`,
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
			idx, stats, err := catalog.RefreshSemanticIndex(cmd.Context(), cat, embedder, batchSize, func(done, total int) {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r  의미 인덱싱… %d/%d (%.0f%%)", done, total, float64(done)*100/float64(total))
			})
			if err != nil {
				return err
			}
			if err := idx.Save(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "\r의미 인덱스 %d건 저장 · 재사용 %d · 새 임베딩 %d · 모델 %s · %.1f초\n",
				len(idx.PKs), stats.Reused, stats.Embedded, idx.Model, time.Since(start).Seconds())
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
			headers := []string{"데이터수", "활용신청 합", "제공기관"}
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
					"source":  cat.Source,
					"entries": len(cat.Entries), "stale": cat.Stale(),
					"svcTypes": cat.SvcTypes(),
				})
			}
			source := cat.Source
			if source == "" {
				source = "legacy"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "수집 %s (%.0f시간 전) · %s 원천 · %s 유형 · %d건%s\n",
				cat.SyncedAt.Local().Format("2006-01-02 15:04"), cat.Age().Hours(),
				source, cat.Type, len(cat.Entries),
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
				case catalog.SvcFILE:
					note = " — 로그인 없이 파일 상세·다운로드 확인 가능, call 대상 아님"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-6s %6d건%s\n", k, byType[k], note)
			}
			return nil
		},
	}
}

func catalogValidateReleaseCmd() *cobra.Command {
	var snapshot string
	command := &cobra.Command{
		Use:    "validate-release",
		Short:  "사람이 검수한 대표 검색 질의로 snapshot 회귀 검사",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var cat *catalog.Catalog
			var err error
			if snapshot == "" {
				cat, err = loadCatalog(cmd)
			} else {
				file, openErr := os.Open(snapshot)
				if openErr != nil {
					return openErr
				}
				cat, err = catalog.ReadSnapshot(file)
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
				if err == nil && cat.Stale() {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 배포 카탈로그가 %.0f일 전 것입니다 — Catalog refresh 작업으로 갱신하세요.\n", cat.Age().Hours()/24)
				}
			}
			if err != nil {
				return err
			}
			golden, err := catalog.DefaultReleaseGoldenSet()
			if err != nil {
				return err
			}
			report := catalog.ValidateReleaseQuality(cat, golden)
			if err := output.WriteJSON(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			if !report.Passed {
				return fmt.Errorf("release 검색 품질 회귀 검사에 실패했습니다")
			}
			return nil
		},
	}
	command.Flags().StringVar(&snapshot, "snapshot", "", "로컬 상태를 바꾸지 않고 지정한 catalog.json.gz 검사")
	return command
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
			"⚠ 카탈로그가 %.0f일 전 것입니다 — `odeduck catalog sync` 로 갱신하세요.\n", cat.Age().Hours()/24)
	}
	return cat, nil
}
