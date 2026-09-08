package main

import (
	"context"
	"fmt"

	"github.com/JungHoonGhae/odeduck/internal/mcpserver"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	return mcpCommand(mcpserver.Serve)
}

func mcpCommand(serve func(context.Context, mcpserver.Deps) error) *cobra.Command {
	var shareEvidence bool
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP 서버 실행 — 검색→상세→AI 활용신청→호출",
		Long: `odeduck 을 Model Context Protocol 서버로 노출합니다(stdio).
핵심 tool 은 catalog_search → inspect_dataset → (미승인 시 apply) → call_api 흐름이며,
AI가 데이터 발견뿐 아니라 활용신청·승인 확인·실제 호출까지 이어갑니다.
search_datasets / list_applications 는 최신성·계정 확인을 위한 보조 tool 입니다. 인증키는 모델에
노출하지 않고 call_api 내부에서만 주입합니다.
odeduck://guide 리소스에 사용 순서가 있습니다. 호출·계정 기능은 로그인 세션 전제입니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if shareEvidence {
				fmt.Fprintln(cmd.ErrOrStderr(), "목표의 선택 원천 값을 연결된 MCP host에 전송하도록 허용합니다. 개인정보·전송 권한을 확인하세요. 기존 Artifact/call_api 원문 출력과는 별도 설정입니다.")
			}
			return serve(cmd.Context(), mcpserver.Deps{
				Fetch:             newFetchClient(),
				BaseURL:           flagBaseURL,
				ShareGoalEvidence: shareEvidence,
			})
		},
	}
	cmd.Flags().BoolVar(&shareEvidence, "share-goal-evidence", false, "목표 실행의 선택 원천 값을 MCP host에 공개 허용; 모델 tool argument로 변경 불가")
	return cmd
}
