package main

import (
	"errors"
	"fmt"

	"github.com/JungHoonGhae/opendatactl/internal/doctor"
	"github.com/JungHoonGhae/opendatactl/internal/output"
	"github.com/JungHoonGhae/opendatactl/internal/portal"
	"github.com/spf13/cobra"
)

func doctorCmd() *cobra.Command {
	var applyPK string
	var skipApply bool
	var adaptersOnly bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "스크래핑 상태 점검 — data.go.kr 마크업 변경(drift) 감지",
		Long: `opendatactl 의 fragile scraping 이 아직 동작하는지 라이브로 점검합니다.
data.go.kr 이 HTML 을 바꾸면 파서가 조용히 빈 결과를 내므로, doctor 가 각 seam
(검색·REST describe·LINK 인계·활용신청 현황)을 실제로 호출해 데이터가 나오는지 확인합니다.
drift 가 하나라도 있으면 종료코드 1 을 반환합니다(CI 용).

apply 점검은 활용신청 폼을 실제로 열어 필요한 입력요소가 남아 있는지 확인하되 **제출하지 않습니다.**
이미 신청한 데이터셋은 폼이 열리지 않으므로, 신청하지 않은 pk 를 --apply-pk 로 지정하세요.
브라우저를 띄우지 않으려면 --skip-apply 를 쓰세요.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			base := flagBaseURL
			if base == "" {
				base = portal.BaseURL
			}

			fc := newFetchClient()
			checks := []doctor.Check{}
			if adaptersOnly {
				checks = append(checks, doctor.AdapterCheck(cmd.Context(), fc, base))
			} else {
				checks = doctor.Run(cmd.Context(), fc, base)
				checks = append(checks, sessionCheck(cmd))
				checks = append(checks, doctor.APIKeyCheck(cmd.Context()))
				if !skipApply {
					checks = append(checks, doctor.ApplyCheck(cmd.Context(), applyPK))
				}
			}

			if err := renderChecks(cmd, format, checks); err != nil {
				return err
			}
			for _, c := range checks {
				if c.Status == doctor.StatusDrift {
					return fmt.Errorf("drift 감지 — 파서 점검 필요")
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&applyPK, "apply-pk", "", "apply 폼 점검에 쓸 publicDataPk (아직 신청하지 않은 것)")
	c.Flags().BoolVar(&skipApply, "skip-apply", false, "apply 폼 점검 생략 (브라우저를 띄우지 않음)")
	c.Flags().BoolVar(&adaptersOnly, "adapters-only", false, "LINK provider 어댑터 canary와 계약 freshness만 점검")
	return c
}

// sessionCheck probes the login-gated 활용신청 현황 seam. Without a session it is
// reported skipped rather than drift — the parser can't be exercised.
func sessionCheck(cmd *cobra.Command) doctor.Check {
	apps, err := portal.Applications(cmd.Context())
	switch {
	case errors.Is(err, portal.ErrNotLoggedIn):
		return doctor.Check{Name: "applications", Status: doctor.StatusSkipped, Detail: "세션 없음 — `opendatactl login` 후 재점검"}
	case err != nil:
		return doctor.Check{Name: "applications", Status: doctor.StatusDrift, Detail: "요청 실패: " + err.Error()}
	case len(apps) == 0:
		return doctor.Check{Name: "applications", Status: doctor.StatusSkipped, Detail: "활용신청 0건 — 빈 계정인지 마크업 변경인지 판별할 수 없음"}
	default:
		return doctor.Check{Name: "applications", Status: doctor.StatusOK, Detail: fmt.Sprintf("활용신청 %d건 파싱", len(apps))}
	}
}

func renderChecks(cmd *cobra.Command, format output.Format, checks []doctor.Check) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), checks)
	case output.JSONL:
		items := make([]any, len(checks))
		for i := range checks {
			items[i] = checks[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"점검", "상태", "상세"}
		rows := make([][]string, 0, len(checks))
		for _, c := range checks {
			rows = append(rows, []string{c.Name, string(c.Status), c.Detail})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}
