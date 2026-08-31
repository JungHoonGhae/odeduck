package main

import (
	"github.com/JungHoonGhae/opendatactl/internal/mcpserver"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "MCP 서버 실행 — 검색→상세→AI 활용신청→호출",
		Long: `opendatactl 을 Model Context Protocol 서버로 노출합니다(stdio).
핵심 tool 은 catalog_search → describe_api → (미승인 시 apply) → call_api 흐름이며,
AI가 데이터 발견뿐 아니라 활용신청·승인 확인·실제 호출까지 이어갑니다.
search_datasets / list_applications 는 최신성·계정 확인을 위한 보조 tool 입니다. 인증키는 모델에
노출하지 않고 call_api 내부에서만 주입합니다.
opendatactl://guide 리소스에 사용 순서가 있습니다. 호출·계정 기능은 로그인 세션 전제입니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcpserver.Serve(cmd.Context(), mcpserver.Deps{
				Fetch:   newFetchClient(),
				BaseURL: flagBaseURL,
			})
		},
	}
}
