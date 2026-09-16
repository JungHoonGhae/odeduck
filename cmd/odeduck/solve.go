package main

import (
	"context"
	"fmt"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/agentplan"
	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/JungHoonGhae/odeduck/internal/output"
	"github.com/JungHoonGhae/odeduck/internal/providerauth"
	"github.com/spf13/cobra"
)

type solveRunner func(context.Context, string, goalwork.Policy, string, func(goalwork.View)) (goalwork.View, error)

func solveCmd() *cobra.Command { c := solveCommand(runGoal); c.Hidden = true; return c }

func solveCommand(run solveRunner) *cobra.Command {
	var provider string
	var strict bool
	var shareEvidence bool
	var reviewReports bool
	var reviewAnalyses bool
	var reviewFullScope bool
	var rounds int
	cmd := &cobra.Command{Use: "solve <목표>", Short: "목표 → 역할 탐색·검사·표본 조회·분석·결합 (experimental)", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true,
		Long: "키워드를 몰라도 목표를 입력하면 설치된 tool-free agent가 데이터 역할을 추론하고 검색·검사·표본 결합을 반복합니다. 실패하면 대안이나 코드 대응표를 탐색합니다. 관측한 한 원천의 조회·집계도 가능하며 결합은 선택 연산입니다. API/CSV·ZIP·XLSX/STD의 sample_executed는 표본 실행 결과입니다. review_required에서도 같은 목표·예산·만료 안에서 추가 근거와 대안을 찾습니다. 판정의 executionRevision은 해당 결과를 만든 실행을 가리킵니다. 선택형 --review-source-reports는 별도 모델이 원천 보고의 출력별 지지와 원래 목표 적합성을 검토합니다. --review-analyses는 관계·계산의 단위·기간·범위 검토를 추가합니다. 모두 기본 꺼짐이며 명시적 agent와 근거 공개가 필요합니다. 공간·인과·사업 가설·현장 검증은 승인하지 않습니다. 미완료 결과는 실패 코드로 반환합니다. 자동 활용신청은 하지 않습니다. JSON만 출력합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagFormat != "json" {
				return fmt.Errorf("%s outputs a structured JSON artifact; use --format json", cmd.Name())
			}
			if rounds < 1 || rounds > 64 {
				return fmt.Errorf("max-rounds must be 1–64")
			}
			policy := goalwork.Policy{RequireSemantic: strict, MaxRounds: rounds}
			if reviewFullScope && !reviewAnalyses {
				return fmt.Errorf("--review-full-scope requires --review-analyses, --share-evidence and an explicit --agent")
			}
			policy.ReviewFullScope = reviewFullScope
			if shareEvidence {
				switch provider {
				case "codex", "claude", "gemini":
					policy.EvidenceRecipient = provider
				default:
					return fmt.Errorf("--share-evidence requires an explicit --agent=codex|claude|gemini; auto fallback is forbidden")
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "선택한 원천 값을", provider, "계획기에 전송합니다. 개인정보·전송 권한을 확인하세요. 세션당 최대 64 KiB이며 전체 표본은 전송하지 않습니다.")
			}
			if reviewReports || reviewAnalyses {
				if policy.EvidenceRecipient == "" {
					return fmt.Errorf("--review-source-reports/--review-analyses require --share-evidence and an explicit --agent")
				}
				policy.ReviewRecipient = provider
				policy.ReviewAnalyses = reviewAnalyses
				fmt.Fprintln(cmd.ErrOrStderr(), "같은 provider의 별도 tool-free 검토 요청에도 선택 근거를 전송합니다. 최대 세 번이며 모델 판단이지 현장/사람 검증이 아닙니다.")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Hour)
			defer cancel()
			fmt.Fprintln(cmd.ErrOrStderr(), "목표 기반 탐색 시작: agent 호출·검색·API/CSV/ZIP/XLSX/STD 읽기를 최대", rounds, "단계 실행합니다.")
			view, err := run(ctx, args[0], policy, provider, func(v goalwork.View) {
				fmt.Fprintf(cmd.ErrOrStderr(), "  %d단계: %s · 후보 %d · 관측 %d · 실행안 %d · gaps %d\n", v.Revision, v.Status, len(v.Nodes), len(v.Observations), len(v.Compositions), len(v.Gaps))
				if n := len(v.Gaps); n > 0 && v.Gaps[n-1].Revision == v.Revision {
					fmt.Fprintln(cmd.ErrOrStderr(), "    "+v.Gaps[n-1].Detail)
				}
			})
			if view.Goal != "" {
				if writeErr := output.WriteJSON(cmd.OutOrStdout(), view); writeErr != nil {
					return writeErr
				}
			}
			if err != nil {
				return err
			}
			if view.Status != "output_ready" || view.Evaluation == nil || view.Evaluation.NeedsSemanticReview || view.Evaluation.Status != "requirements_met" || view.Evaluation.Review == nil || view.Evaluation.Review.ExecutionRevision != view.Evaluation.ExecutionRevision {
				return fmt.Errorf("목표 미완료: %s — JSON의 evaluation과 gaps를 확인하세요; 검토가 필요한 후보는 목표 완료가 아닙니다", view.Status)
			}
			return nil
		}}
	cmd.Flags().StringVar(&provider, "agent", "auto", "계획 agent: auto | codex | claude | gemini (Cursor는 tool-free 격리 미지원)")
	cmd.Flags().BoolVar(&strict, "require-semantic", true, "실제 의미 검색 사용을 요구; false는 명시적인 lexical 저하 허용")
	cmd.Flags().BoolVar(&shareEvidence, "share-evidence", false, "선택 원천 값을 명시한 단일 --agent에 전송 허용; 기본 비공개, 일반 개인정보 탐지는 아님")
	cmd.Flags().BoolVar(&reviewReports, "review-source-reports", false, "별도 모델 검토로 제한된 원천 보고 완료 허용; --share-evidence/명시 --agent 필수, 계산·가설 승인 아님")
	cmd.Flags().BoolVar(&reviewAnalyses, "review-analyses", false, "원천 보고와 typed 관계·계산의 별도 검토 허용; --share-evidence/명시 --agent 필수, 공간·인과·가설 승인 아님")
	cmd.Flags().BoolVar(&reviewFullScope, "review-full-scope", false, "원천 기반 전체 요청 범위의 추가 검토; --review-analyses 필수, 모집단 인증 아님")
	cmd.Flags().IntVar(&rounds, "max-rounds", 32, "최대 진행 단계 (1–64); agent 호출마다 해당 CLI의 비용/한도 적용")
	return cmd
}

func runGoal(ctx context.Context, goal string, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
	return runGoalRequest(ctx, goalwork.Request{Goal: goal}, policy, provider, progress)
}

func runGoalRequest(ctx context.Context, request goalwork.Request, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
	e, err := prepareGoalRequest(request, policy, provider)
	if err != nil {
		return goalwork.View{}, err
	}
	return goalwork.Run(ctx, e, func(ctx context.Context, v goalwork.View) (goalwork.Decision, error) {
		d, used, err := agentplan.PlanGoal(ctx, v, provider)
		if err == nil {
			provider = used
		}
		return d, err
	}, progress)
}

func prepareGoalRequest(request goalwork.Request, policy goalwork.Policy, provider string) (*goalwork.Engine, error) {
	client := newFetchClient()
	caller := apicall.NewDatasetCaller(client, flagBaseURL, providerauth.Source{})
	deps := goalwork.LiveDependencies(client, flagBaseURL, caller, catalog.Searcher{}, policy)
	acquisitionCheck := deps.Preflight
	deps.Preflight = func(ctx context.Context) (goalwork.Environment, error) {
		env, err := acquisitionCheck(ctx)
		check, plannerErr := agentplan.CheckGoalPlanner(provider)
		env.Checks = append(env.Checks, check)
		if err != nil {
			return env, err
		}
		return env, plannerErr
	}
	if policy.ReviewRecipient != "" {
		deps.Review = func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			response, err := agentplan.ReviewGoal(ctx, in, policy.ReviewRecipient)
			return response.Assessment, err
		}
	}
	return goalwork.StartRequest(request, policy, deps)
}
