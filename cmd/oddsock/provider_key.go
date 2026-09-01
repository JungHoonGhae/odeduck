package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JungHoonGhae/oddsock/internal/output"
	"github.com/JungHoonGhae/oddsock/internal/providerauth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func providerKeyCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "provider-key",
		Short: "LINK 제공기관 인증키를 안전하게 저장·확인·삭제",
		Long: "SafetyKorea, FoodSafetyKorea, VWorld가 발급한 키를 provider scope별로 저장합니다. " +
			"비밀값은 명령행 인자로 받지 않으며 MCP에도 노출하지 않습니다.",
	}
	command.AddCommand(providerKeySetCmd(), providerKeyStatusCmd(), providerKeyDeleteCmd())
	return command
}

func providerKeySetCmd() *cobra.Command {
	var domain string
	command := &cobra.Command{
		Use:   "set <" + strings.Join(providerauth.ProviderIDs(), "|") + ">",
		Short: "stdin 또는 숨김 터미널 입력으로 provider key 저장",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := readSecret(cmd)
			if err != nil {
				return err
			}
			if err := providerauth.Set(args[0], providerauth.Credential{Key: key, Domain: strings.TrimSpace(domain)}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%s provider credential을 scope 제한·사용자 전용 파일에 저장했습니다.\n", args[0])
			return nil
		},
	}
	command.Flags().StringVar(&domain, "domain", "", "VWorld 인증키 발급 시 등록한 URL (키와 함께 자동 주입)")
	return command
}

func readSecret(cmd *cobra.Command) (string, error) {
	in := cmd.InOrStdin()
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), "Provider key: ")
		secret, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", err
		}
		return string(secret), nil
	}
	line, err := bufio.NewReader(io.LimitReader(in, 4098)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return "", fmt.Errorf("stdin에서 provider key를 읽지 못했습니다")
	}
	return line, nil
}

func providerKeyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "provider별 key 설정 여부 확인 (비밀값은 출력하지 않음)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			statuses, err := providerauth.Status()
			if err != nil {
				return err
			}
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			if format == output.Table {
				rows := make([][]string, 0, len(statuses))
				for _, status := range statuses {
					configured := ""
					if status.Configured {
						configured = "✓"
					}
					rows = append(rows, []string{status.Provider, configured, status.Scope, status.Domain})
				}
				return output.WriteTable(cmd.OutOrStdout(), []string{"provider", "configured", "scope", "domain"}, rows)
			}
			return output.WriteJSON(cmd.OutOrStdout(), statuses)
		},
	}
}

func providerKeyDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <" + strings.Join(providerauth.ProviderIDs(), "|") + ">",
		Short: "저장된 provider key 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := providerauth.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%s provider credential을 삭제했습니다.\n", args[0])
			return nil
		},
	}
}
