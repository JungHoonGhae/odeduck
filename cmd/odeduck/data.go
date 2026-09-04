package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/output"
	"github.com/JungHoonGhae/odeduck/internal/portal"
	"github.com/JungHoonGhae/odeduck/internal/providerauth"
	"github.com/spf13/cobra"
)

type commandCredentialSource struct {
	explicitDataGoKey string
	defaultSource     providerauth.Source
}

func (s *commandCredentialSource) DataGoKR(ctx context.Context) (string, error) {
	if s.explicitDataGoKey != "" {
		return s.explicitDataGoKey, nil
	}
	return s.defaultSource.DataGoKR(ctx)
}

func (s *commandCredentialSource) InvalidateDataGoKR() {
	if s.explicitDataGoKey == "" {
		s.defaultSource.InvalidateDataGoKR()
	}
}

func (s *commandCredentialSource) External(ctx context.Context, provider, scope string) (string, string, error) {
	return s.defaultSource.External(ctx, provider, scope)
}

func searchCmd() *cobra.Command {
	var dtype, org string
	var perPage, page int
	c := &cobra.Command{
		Use:   "search <keyword>",
		Short: "data.go.kr 데이터셋 검색 (파일 + OpenAPI)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			ds, err := newPortalClient().SearchDatasets(cmd.Context(), portal.SearchOptions{
				Keyword: strings.Join(args, " "),
				Type:    strings.ToUpper(dtype),
				Org:     org,
				PerPage: perPage,
				Page:    page,
			})
			if err != nil {
				return err
			}
			if format == output.Table {
				headers := []string{"pk", "OpenAPI", "제목", "포맷"}
				rows := make([][]string, 0, len(ds))
				for _, d := range ds {
					api := ""
					if d.HasOpenAPI {
						api = "✓"
					}
					rows = append(rows, []string{d.PublicDataPk, api, d.Title, strings.Join(d.Formats, ",")})
				}
				return output.WriteTable(cmd.OutOrStdout(), headers, rows)
			}
			return output.WriteJSON(cmd.OutOrStdout(), ds)
		},
	}
	c.Flags().StringVar(&dtype, "type", "", "데이터 유형: file | api (기본: 전체)")
	c.Flags().StringVar(&org, "org", "", "제공기관 필터")
	c.Flags().IntVar(&perPage, "per-page", 0, "페이지당 결과 수 (기본 10)")
	c.Flags().IntVar(&page, "page", 0, "페이지 번호 (1부터)")
	return c
}

func describeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe <publicDataPk>",
		Short: "OpenAPI 상세 — 상세기능·엔드포인트·요청변수 surface",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := flagBaseURL
			if base == "" {
				base = portal.BaseURL
			}
			spec, err := apicall.DescribeCatalogued(cmd.Context(), newFetchClient(), base, args[0])
			if err != nil {
				return err
			}
			return output.WriteJSON(cmd.OutOrStdout(), spec)
		},
	}
}

func inspectCmd() *cobra.Command {
	var observe bool
	var assetName string
	var delivery string
	c := &cobra.Command{
		Use:   "inspect <publicDataPk>",
		Short: "데이터 상세 — REST/LINK 계약 또는 FILE 실제 스키마 검사",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := flagBaseURL
			if base == "" {
				base = portal.BaseURL
			}
			result, err := dataset.NewUnifiedInspector(newFetchClient(), base).Inspect(cmd.Context(), dataset.InspectionRequest{
				PK: args[0], Delivery: dataset.DeliverySelection(delivery), Observe: observe, Asset: assetName,
			})
			if err != nil {
				return err
			}
			return output.WriteJSON(cmd.OutOrStdout(), result)
		},
	}
	c.Flags().BoolVar(&observe, "observe", false, "FILE 최신 자산을 bounded 다운로드해 실제 CSV/DBF 컬럼과 SHA-256 검사")
	c.Flags().StringVar(&assetName, "asset", "", "검사할 FILE 자산의 정확한 이름 (기본: 목록 첫 번째 최신 자산)")
	c.Flags().StringVar(&delivery, "delivery", "auto", "검사할 제공형: auto | api | file (auto는 복수 제공형을 모두 반환)")
	return c
}

func callCmd() *cobra.Command {
	var key string
	var params []string
	var profileFields []string
	var pk, op string
	var wait time.Duration
	c := &cobra.Command{
		Use:   "call [endpoint]",
		Short: "인증 API 호출 — REST/LINK typed dispatch → XML→JSON",
		Long: `승인된 OpenAPI를 호출합니다. --param k=v 로 요청변수를 전달합니다. data.go.kr 인증키는 로그인 세션에서
자동으로 조회합니다. 구현된 LINK provider는 provider-key로 저장한 scope별 키를 자동 주입합니다.
--key는 data.go.kr REST에만 적용되며 LINK key를 명령행으로 받지 않습니다. 응답은 XML이면 JSON으로 변환해 출력합니다.

--pk 를 주면 포털에서 REST 명세 또는 LINK provider contract를 조회하고 자동 dispatch합니다. 경로는 추측할 수 없는
형태(HeatWaveCasualtiesRegion/getHeatWaveCasualtiesRegionList)이고 틀리면 404·500 이 오므로,
직접 타이핑하기보다 이 방식이 안전합니다. 상세기능이 여럿이면 --op 로 지정하세요.
--pk 를 쓰면 명세의 필수 요청변수가 빠졌는지도 호출 전에 확인합니다.

신청 직후에는 게이트웨이 반영에 보통 7~10분 걸려 403 이 옵니다. --wait 10m 을 주면 그때까지
1분 간격으로 재시도합니다(승인 자체는 즉시 끝나므로 다시 신청할 필요 없습니다).

연결 후보를 검증할 때 --profile-field 를 반복하면 응답 안의 해당 field를 재귀적으로 찾아 raw 값 표본,
count, distinct, null, duplicate 수를 함께 반환합니다. 두 API를 같은 지역·기간으로 호출한 뒤 profile을
비교하세요. 식별자의 leading zero는 보존하며, 실제 join 성공은 양쪽 값 교집합과 cardinality를 별도로
확인해야 합니다. 같은 leaf 이름이 여러 경로에 있으면 값을 합치지 않고 ambiguous=true와 실제 paths를
반환하므로 dotted path로 다시 지정하세요.

예) odeduck call --pk 15077974 --param numOfRows=10
    odeduck call --pk 15077974 --wait 15m --param numOfRows=10   # 방금 신청한 API
    odeduck call --pk 15116894 --op certificationList --param conditionKey=productName --param conditionValue=완구
    odeduck call https://apis.data.go.kr/9760000/.../getX --param numOfRows=10`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := apicall.ProfileBody(nil, profileFields); err != nil {
				return err
			}
			if len(args) == 0 && pk == "" {
				return fmt.Errorf("엔드포인트 URL 또는 --pk 가 필요합니다")
			}
			if len(args) > 0 && pk != "" {
				return fmt.Errorf("엔드포인트 URL 과 --pk 는 함께 쓸 수 없습니다 — 어느 것을 호출할지 하나만 정하세요")
			}
			pm := map[string]string{}
			for _, p := range params {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					return fmt.Errorf("--param 은 k=v 형식이어야 합니다: %q", p)
				}
				pm[kv[0]] = kv[1]
			}
			if len(args) == 0 {
				base := flagBaseURL
				if base == "" {
					base = portal.BaseURL
				}
				caller := apicall.NewDatasetCaller(newFetchClient(), base, &commandCredentialSource{explicitDataGoKey: key})
				res, callErr := caller.Call(cmd.Context(), apicall.DatasetCallRequest{
					PK: pk, Operation: op, Params: pm, Wait: wait,
					OnWait: func(elapsed, remaining time.Duration) {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"\r게이트웨이 반영 대기… %s 경과 (최대 %s 더 기다립니다)", elapsed, remaining)
					},
				})
				if res != nil {
					if len(profileFields) > 0 {
						res.Profile, _ = apicall.ProfileBody(res.Body, profileFields)
					}
					output.WriteJSON(cmd.OutOrStdout(), res)
				}
				return callErr
			}
			endpoint := args[0]
			secureEndpoint, secureErr := apicall.SecureEndpoint(endpoint)
			if secureErr != nil {
				return secureErr
			}
			endpoint = secureEndpoint
			if key == "" {
				k, keyErr := portal.APIKey(cmd.Context())
				if keyErr != nil {
					return fmt.Errorf("인증키를 얻지 못했습니다 (--key 로 직접 지정하거나 `odeduck login` 후 재시도): %w", keyErr)
				}
				key = k
			}
			doCall := func(k string) (*apicall.CallResult, error) {
				if wait <= 0 {
					return apicall.Call(cmd.Context(), newFetchClient(), endpoint, pm, k)
				}
				return apicall.CallWaiting(cmd.Context(), newFetchClient(), endpoint, pm, k, wait,
					func(elapsed, remaining time.Duration) {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"\r게이트웨이 반영 대기… %s 경과 (최대 %s 더 기다립니다)", elapsed, remaining)
					})
			}
			res, err := doCall(key)
			// The cached key may be stale (reissued in the portal). Drop it and
			// retry once with a freshly read key before reporting failure.
			if errors.Is(err, apicall.ErrKeyRejected) && !cmd.Flags().Changed("key") {
				portal.InvalidateCachedKey()
				if fresh, kerr := portal.APIKey(cmd.Context()); kerr == nil && fresh != key {
					fmt.Fprintln(cmd.ErrOrStderr(), "인증키가 거부됐습니다 — 캐시를 버리고 다시 조회해 재시도합니다.")
					res, err = doCall(fresh)
				}
			}
			if res != nil {
				if len(profileFields) > 0 {
					res.Profile, _ = apicall.ProfileBody(res.Body, profileFields)
				}
				output.WriteJSON(cmd.OutOrStdout(), res)
			}
			if err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().StringVar(&key, "key", "", "data.go.kr 계정 인증키만 지정 (LINK provider key는 provider-key로 저장)")
	c.Flags().StringArrayVar(&params, "param", nil, "요청변수 k=v (반복 가능)")
	c.Flags().StringArrayVar(&profileFields, "profile-field", nil, "연결 검증용 응답 field 프로파일 (반복 가능, 최대 8개)")
	c.Flags().StringVar(&pk, "pk", "", "publicDataPk — REST/LINK contract를 조회해 자동 dispatch (URL 대신)")
	c.Flags().StringVar(&op, "op", "", "describe에 나온 operation 이름. 하나뿐이면 생략 가능")
	c.Flags().DurationVar(&wait, "wait", 0, "게이트웨이 반영(403)을 이 시간까지 기다리며 재시도 (예: 10m, 최대 1h)")
	return c
}
