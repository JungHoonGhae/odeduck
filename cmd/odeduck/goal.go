package main

import (
	"context"
	"fmt"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/spf13/cobra"
)

type goalRunner func(context.Context, goalwork.Request, goalwork.Policy, string, func(goalwork.View)) (goalwork.View, error)

func goalCmd() *cobra.Command { return goalCommand(runGoalRequest) }

func goalCommand(run goalRunner) *cobra.Command {
	var userContext string
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, provider string, progress func(goalwork.View)) (goalwork.View, error) {
		return run(ctx, goalwork.Request{Goal: goal, Context: userContext}, policy, provider, progress)
	})
	cmd.Flags().StringVar(&userContext, "context", "", "목표 해석에 필요한 이전 대화·제외 조건 (선택, 최대 8000 bytes; 원천 근거·권한은 아님)")
	cmd.Use = "goal <목표>"
	cmd.Short = "키워드 없이 목표에서 필요한 데이터·검증된 산출물까지"
	cmd.Long = `이루려는 목표를 입력하면 필요한 자료를 발견하고, 실제 원천을 읽어 분석·연결한 결과와 근거를 만듭니다.
검사·실행 결과가 부족하면 대안을 탐색합니다. 목표 충족 여부는 같은 실행기가 검사합니다.

--review-with codex|claude|gemini는 계획과 별도 결과 검토에 사용할 수신자를 고정합니다.
선택한 원천 값과 요청 범위를 해당 제공자의 계획·검토 요청에 전송하도록 허용합니다.
기존 근거 크기·호출 횟수 제한, 인증키 보호, 공간·인과·현장 판단의 검토 제한은 유지합니다.
자료·접근권한이 부족하면 결과와 함께 미완료 이유를 반환합니다. JSON으로 출력합니다.

예: odeduck goal "사람들이 잘 모르는 흥미로운 사실을 발견해 근거와 함께 설명해주세요" --review-with codex`
	var reviewer string
	cmd.Flags().StringVar(&reviewer, "review-with", "", "계획·별도 검토 수신자 및 선택 근거 전송 허용: codex | claude | gemini")
	for _, name := range []string{"agent", "share-evidence", "review-source-reports", "review-analyses", "review-full-scope"} {
		_ = cmd.Flags().MarkHidden(name)
	}
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if reviewer == "" {
			return fmt.Errorf("목표 완료 검토 설정이 필요합니다. --review-with codex|claude|gemini로 계획·검토 수신자와 선택 원천 값 전송을 허용하세요. 모델 호출 전 중단했습니다")
		}
		if reviewer != "codex" && reviewer != "claude" && reviewer != "gemini" {
			return fmt.Errorf("--review-with must be codex, claude or gemini; no automatic recipient fallback")
		}
		for _, name := range []string{"agent", "share-evidence", "review-source-reports", "review-analyses", "review-full-scope"} {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("goal uses --review-with; do not combine it with legacy --%s", name)
			}
		}
		for _, setting := range [][2]string{{"agent", reviewer}, {"share-evidence", "true"}, {"review-analyses", "true"}, {"review-full-scope", "true"}} {
			if err := cmd.Flags().Set(setting[0], setting[1]); err != nil {
				return err
			}
		}
		return nil
	}
	return cmd
}
