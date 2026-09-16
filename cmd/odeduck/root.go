package main

import (
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/output"
	"github.com/JungHoonGhae/odeduck/internal/portal"
	"github.com/JungHoonGhae/odeduck/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagFormat  string
	flagDelay   time.Duration
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   version.CommandName,
	Short: "목표에 필요한 공공데이터를 발견하고, 실제 결과와 근거까지",
	Long: `odeduck — 목표에서 공공데이터 산출물까지 이어가는 오데덕입니다.

이루려는 일은 odeduck goal로 시작합니다. 특정 데이터 검색·신청·호출도 개별 명령으로 제공합니다.

사람은 브라우저에서 한 번만 로그인(odeduck login)하면, 이후 검색·활용신청·호출을
CLI 또는 MCP(에이전트)로 처리합니다. 포털 UI를 다시 건드릴 필요가 없습니다.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVarP(&flagFormat, "format", "f", "json", "출력 형식: json | jsonl | table")
	pf.DurationVar(&flagDelay, "delay", fetch.DefaultDelay, "요청 간 최소 간격 (rate limit)")
	pf.StringVar(&flagBaseURL, "base-url", "", "포털 base URL 재정의 (테스트용)")

	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(loginCmd(), logoutCmd(), statusCmd())
	rootCmd.AddCommand(searchCmd(), inspectCmd(), describeCmd(), callCmd(), keyCmd())
	rootCmd.AddCommand(providerKeyCmd())
	rootCmd.AddCommand(catalogCmd())
	rootCmd.AddCommand(goalCmd(), solveCmd())
	rootCmd.AddCommand(applyCmd(), applicationsCmd())
	rootCmd.AddCommand(doctorCmd())
	rootCmd.AddCommand(mcpCmd())
}

// resolveFormat parses the global --format flag into an output.Format.
func resolveFormat() (output.Format, error) {
	return output.Parse(flagFormat)
}

// newFetchClient builds the shared HTTP transport from the global flags.
func newFetchClient() *fetch.Client {
	return fetch.New(fetch.WithDelay(flagDelay))
}

// newPortalClient builds a portal search client over the shared transport.
func newPortalClient() *portal.Client {
	opts := []portal.Option{}
	if flagBaseURL != "" {
		opts = append(opts, portal.WithBaseURL(flagBaseURL))
	}
	return portal.New(newFetchClient(), opts...)
}
