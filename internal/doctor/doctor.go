// Package doctor is a liveness check for odeduck's fragile scraping. data.go.kr
// can redesign its HTML at any time, and the parsers degrade to *empty* results
// rather than crashing — so drift is otherwise silent. doctor drives each
// scraping seam against the live portal and reports whether it still yields data,
// turning silent drift into a loud, checkable signal (arch review candidate B).
package doctor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/portal"
	"github.com/JungHoonGhae/odeduck/internal/providerauth"
)

// CanaryPK is a stable, long-lived OpenAPI dataset (중앙선거관리위원회
// PofelcddInfoInqireService) used to probe the describe scraper. If data.go.kr
// ever retires it, the describe check will report drift — update this pk then.
const CanaryPK = "15000908"

// CombinedSwaggerCanaryPK is an API+FILE node whose legacy openapi.do route is
// absent and whose callable operation comes from the exact ODCloud Swagger URL
// referenced by the combined fileData page. It protects that fallback as a
// separate seam from the legacy describe canary above.
const CombinedSwaggerCanaryPK = "15127058"

// FileCanaryPK exercises data.go.kr's standard metadata, labelled FILE detail
// page and download-contract resolver without downloading the asset.
const FileCanaryPK = "15117154"

// SeoulFileCanaryPK exercises the external Seoul handoff, documented provider
// catalogue and versioned FILE asset parser without downloading the asset.
const SeoulFileCanaryPK = "15076355"

// LinkCanaryPK is retained for compatibility with tests and downstream probes.
// The live check now gets its full provider/variant inventory from apicall.
const LinkCanaryPK = "15116894"

const (
	linkContractMaxAge = 180 * 24 * time.Hour
	linkClockSkew      = 24 * time.Hour
)

// Status is the outcome of one check.
type Status string

const (
	StatusOK      Status = "ok"      // seam still yields data
	StatusDrift   Status = "drift"   // seam parsed to nothing — markup likely changed
	StatusSkipped Status = "skipped" // precondition missing (e.g. not logged in)
)

// Check is one diagnostic result.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Run drives the read-only scraping seams (dataset search, OpenAPI describe)
// against baseURL and reports whether each still yields data. It needs no login;
// the session-scoped seams (활용신청 현황) are checked separately by the caller.
func Run(ctx context.Context, fc *fetch.Client, baseURL string) []Check {
	return runAt(ctx, fc, baseURL, time.Now())
}

func runAt(ctx context.Context, fc *fetch.Client, baseURL string, now time.Time) []Check {
	return []Check{
		searchCheck(ctx, fc, baseURL),
		describeCheck(ctx, fc, baseURL),
		combinedSwaggerCheck(ctx, fc, baseURL),
		fileAssetCheck(ctx, fc, baseURL, FileCanaryPK, "file-asset", "datagokr-file"),
		fileAssetCheck(ctx, fc, baseURL, SeoulFileCanaryPK, "seoul-file", "seoul-file"),
		linkCheckAt(ctx, fc, baseURL, now),
		catalogCheck(),
		semanticCheck(),
	}
}

func combinedSwaggerCheck(ctx context.Context, fc *fetch.Client, baseURL string) Check {
	spec, err := apicall.Describe(ctx, fc, baseURL, CombinedSwaggerCanaryPK)
	if err != nil {
		return Check{"odcloud-swagger", StatusDrift, "요청 실패: " + err.Error()}
	}
	if spec.DataName == "" || !strings.Contains(strings.ToUpper(spec.APIType), "REST") {
		return Check{"odcloud-swagger", StatusDrift, fmt.Sprintf(
			"통합 API+FILE 페이지의 이름 또는 REST 유형을 복원하지 못함 (pk=%s)", CombinedSwaggerCanaryPK)}
	}
	for _, operation := range spec.Operations {
		if operation.ContractKind == apicall.ContractKindOfficialSwagger && operation.Endpoint != "" {
			return Check{"odcloud-swagger", StatusOK, fmt.Sprintf(
				"공식 Swagger에서 %d개 operation 복원 (pk=%s)", len(spec.Operations), CombinedSwaggerCanaryPK)}
		}
	}
	return Check{"odcloud-swagger", StatusDrift, fmt.Sprintf(
		"통합 페이지가 참조한 공식 Swagger operation을 복원하지 못함 (pk=%s)", CombinedSwaggerCanaryPK)}
}

func fileAssetCheck(ctx context.Context, transport dataset.Transport, baseURL, pk, name, wantAdapter string) Check {
	contract, err := dataset.NewInspector(transport, baseURL).Inspect(ctx, dataset.Ref{PK: pk, Delivery: dataset.DeliveryFile})
	if err != nil {
		return Check{name, StatusDrift, "요청 실패: " + err.Error()}
	}
	if contract.Name == "" || contract.AdapterID != wantAdapter || contract.Capability != dataset.CapabilityRetrievable || len(contract.Assets) == 0 {
		return Check{name, StatusDrift, fmt.Sprintf(
			"FILE 계약의 이름·adapter·다운로드 자산을 복원하지 못함 (pk=%s, adapter=%s, assets=%d)",
			pk, contract.AdapterID, len(contract.Assets))}
	}
	wantEvidence := map[string]bool{
		dataset.EvidenceStandardMetadata:      false,
		dataset.EvidenceFirstPartyWebContract: false,
	}
	if wantAdapter == "seoul-file" {
		wantEvidence[dataset.EvidenceOfficialAPI] = false
		if len(contract.Alternatives) == 0 {
			return Check{name, StatusDrift, fmt.Sprintf("서울 공식 catalogue가 제공 형태를 반환하지 않음 (pk=%s)", pk)}
		}
	}
	for _, evidence := range contract.Evidence {
		if _, ok := wantEvidence[evidence.Kind]; ok {
			wantEvidence[evidence.Kind] = true
		}
	}
	for kind, present := range wantEvidence {
		if !present {
			return Check{name, StatusDrift, fmt.Sprintf("%s evidence가 누락됨 (pk=%s)", kind, pk)}
		}
	}
	return Check{name, StatusOK, fmt.Sprintf("%s@r%d · 다운로드 자산 %d개 확인, 내려받지 않음 (pk=%s)",
		contract.AdapterID, contract.AdapterRevision, len(contract.Assets), pk)}
}

// catalogCheck reports the local catalogue's freshness. A stale snapshot is the
// quiet failure mode of local discovery: searches keep succeeding while silently
// missing everything published since the last sync, so doctor — the command that
// exists to make quiet problems loud — is where it belongs.
func catalogCheck() Check {
	cat, err := catalog.Load()
	if errors.Is(err, catalog.ErrNotSynced) {
		return Check{"catalog", StatusSkipped, "카탈로그 없음 — `odeduck catalog sync` 로 만들면 검색이 즉시 처리됩니다"}
	}
	if err != nil {
		return Check{"catalog", StatusDrift, "카탈로그를 읽지 못했습니다: " + err.Error()}
	}
	days := cat.Age().Hours() / 24
	if cat.Stale() {
		return Check{"catalog", StatusDrift, fmt.Sprintf(
			"%.0f일 전 스냅샷 (%d건) — 그 이후 신설된 API 가 검색에서 누락됩니다. `odeduck catalog sync`",
			days, len(cat.Entries))}
	}
	return Check{"catalog", StatusOK, fmt.Sprintf("%.1f일 전 수집 (%d건)", days, len(cat.Entries))}
}

// semanticCheck is optional-health, not an installation requirement. A missing
// index is skipped; a built but unreadable one is drift because the user opted
// into semantic search and would otherwise get a silent quality regression.
func semanticCheck() Check {
	cat, err := catalog.Load()
	if err != nil {
		return Check{"semantic", StatusSkipped, "카탈로그 없음 — 의미 인덱스 점검 생략"}
	}
	idx, err := catalog.LoadSemanticIndex(cat)
	switch {
	case err == nil:
		dim := 0
		if len(idx.Vectors) > 0 {
			dim = len(idx.Vectors[0])
		}
		return Check{"semantic", StatusOK, fmt.Sprintf("%d건 × %d차원 · %s", len(idx.PKs), dim, idx.Model)}
	case errors.Is(err, catalog.ErrSemanticIndexNotBuilt):
		return Check{"semantic", StatusSkipped, "선택 기능 미설치 — `odeduck catalog semantic-build` 로 활성화"}
	case errors.Is(err, catalog.ErrSemanticIndexStale):
		return Check{"semantic", StatusSkipped, "카탈로그 갱신 후 의미 인덱스 갱신 필요 — 호환 벡터는 재사용됨"}
	default:
		return Check{"semantic", StatusDrift, "의미 인덱스를 읽지 못했습니다: " + err.Error()}
	}
}

func searchCheck(ctx context.Context, fc *fetch.Client, baseURL string) Check {
	pc := portal.New(fc, portal.WithBaseURL(baseURL))
	ds, err := pc.SearchDatasets(ctx, portal.SearchOptions{})
	switch {
	case err != nil:
		return Check{"search", StatusDrift, "요청 실패: " + err.Error()}
	case len(ds) == 0:
		return Check{"search", StatusDrift, "데이터셋 0건 파싱 — selectDataSetList.do 마크업이 바뀌었을 수 있음"}
	default:
		return Check{"search", StatusOK, fmt.Sprintf("%d개 데이터셋 파싱", len(ds))}
	}
}

func describeCheck(ctx context.Context, fc *fetch.Client, baseURL string) Check {
	spec, err := apicall.Describe(ctx, fc, baseURL, CanaryPK)
	switch {
	case err != nil:
		return Check{"describe", StatusDrift, "요청 실패: " + err.Error()}
	case spec.DataName == "":
		return Check{"describe", StatusDrift, "OpenAPI 이름을 파싱하지 못함 — openapi.do 제목 마크업이 바뀌었을 수 있음 (pk=" + CanaryPK + ")"}
	case spec.APIType == "":
		return Check{"describe", StatusDrift, "API 유형을 파싱하지 못함 — openapi.do 요약 마크업이 바뀌었을 수 있음 (pk=" + CanaryPK + ")"}
	case spec.Approval == nil || spec.Approval.Dev == "":
		return Check{"describe", StatusDrift, "개발단계 심의유형을 파싱하지 못함 — 안전하게 활용신청 여부를 판단할 수 없음 (pk=" + CanaryPK + ")"}
	case len(spec.Operations) == 0:
		return Check{"describe", StatusDrift, "상세기능 0건 파싱 — openapi.do 마크업이 바뀌었을 수 있음 (pk=" + CanaryPK + ")"}
	}
	for _, op := range spec.Operations {
		if op.Endpoint == "" || len(op.Params) == 0 {
			return Check{"describe", StatusDrift, fmt.Sprintf(
				"상세기능 %q 의 엔드포인트 또는 요청변수가 비어 있음 — 명세 조각 마크업이 바뀌었을 수 있음 (pk=%s)",
				op.Name, CanaryPK)}
		}
	}
	return Check{"describe", StatusOK, fmt.Sprintf(
		"%d개 상세기능·요청변수와 심의유형 파싱 (pk=%s)", len(spec.Operations), CanaryPK)}
}

func linkCheck(ctx context.Context, fc *fetch.Client, baseURL string) Check {
	return linkCheckAt(ctx, fc, baseURL, time.Now())
}

// AdapterCheck runs only the provider registry canaries. It is exported for a
// lightweight scheduled CI job that should not need a browser, portal session,
// local catalogue, or data.go.kr API key.
func AdapterCheck(ctx context.Context, fc *fetch.Client, baseURL string) Check {
	return linkCheck(ctx, fc, baseURL)
}

func linkCheckAt(ctx context.Context, fc *fetch.Client, baseURL string, now time.Time) Check {
	adapters := apicall.ExternalProviderAdapters()
	canaryCount := 0
	providerStatus := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		for _, canary := range adapter.Canaries {
			canaryCount++
			spec, err := apicall.Describe(ctx, fc, baseURL, canary.PK)
			switch {
			case err != nil:
				return Check{"link", StatusDrift, fmt.Sprintf("%s/%s 요청 실패 (pk=%s): %v", adapter.ID, canary.Variant, canary.PK, err)}
			case !strings.Contains(spec.APIType, "LINK"):
				return Check{"link", StatusDrift, fmt.Sprintf("%s/%s LINK 유형을 파싱하지 못함 (pk=%s)", adapter.ID, canary.Variant, canary.PK)}
			case spec.LinkURL != canary.URL:
				return Check{"link", StatusDrift, fmt.Sprintf(
					"%s/%s portal URL 변경 — 공식 계약과 fixture를 재검증하세요 (pk=%s, expected=%s, got=%s)",
					adapter.ID, canary.Variant, canary.PK, canary.URL, spec.LinkURL)}
			case spec.Handoff == nil || spec.Handoff.URL != spec.LinkURL ||
				spec.Handoff.Trust != apicall.HandoffPublisherUntrusted ||
				spec.Handoff.FetchPolicy != apicall.HandoffSafeFetcherRequired ||
				spec.Handoff.State != apicall.HandoffContractKnown:
				return Check{"link", StatusDrift, fmt.Sprintf(
					"%s/%s LINK가 더 이상 확인된 계약으로 해석되지 않음 — URL matcher를 검토하세요 (pk=%s, url=%s)",
					adapter.ID, canary.Variant, canary.PK, spec.LinkURL)}
			}

			contract := spec.Handoff.Contract
			if contract == nil || contract.AdapterID != adapter.ID || contract.AdapterRevision != adapter.Revision ||
				contract.Provider != adapter.Provider ||
				contract.DocumentationURL == "" || contract.ApplicationURL == "" ||
				!validInvocationContract(contract) || contract.Auth == nil ||
				contract.Auth.Name == "" || contract.Auth.CredentialScope == "" {
				return Check{"link", StatusDrift, fmt.Sprintf(
					"%s/%s 문서·신청·인증 metadata가 바뀌었거나 누락됨 (pk=%s)", adapter.ID, canary.Variant, canary.PK)}
			}
			verifiedAt, err := time.Parse("2006-01-02", contract.VerifiedAt)
			age := now.Sub(verifiedAt)
			if err != nil || age < -linkClockSkew || age > linkContractMaxAge {
				return Check{"link", StatusDrift, fmt.Sprintf(
					"%s 외부 계약 검증이 180일 이상 경과했거나 날짜를 해석할 수 없음 — 공식 문서를 다시 확인하세요 (verifiedAt=%s)",
					adapter.ID, contract.VerifiedAt)}
			}
		}
		providerStatus = append(providerStatus, fmt.Sprintf("%s@r%d", adapter.ID, adapter.Revision))
	}
	return Check{"link", StatusOK, fmt.Sprintf("provider 어댑터 %d개·canary %d개 정상 (%s)",
		len(adapters), canaryCount, strings.Join(providerStatus, ", "))}
}

func validInvocationContract(contract *apicall.ExternalContract) bool {
	if contract == nil || contract.Auth == nil {
		return false
	}
	switch contract.InvocationState {
	case apicall.InvocationImplemented:
		return len(contract.Operations) > 0 && strings.HasPrefix(contract.Auth.CredentialScope, "https://") &&
			providerauth.Supports(contract.AdapterID, contract.Auth.CredentialScope)
	case apicall.InvocationNotImplemented:
		return len(contract.Operations) == 0
	case apicall.InvocationBlockedInsecureTransport:
		return len(contract.Operations) == 0 && strings.HasPrefix(contract.Auth.CredentialScope, "http://")
	default:
		return false
	}
}

// ApplyCanaryPKs are datasets to try opening the 활용신청 form for. There is more
// than one on purpose.
//
// The portal serves the form only until the account applies, so a single fixed pk
// stops testing anything the moment someone applies for it — and the check cannot
// tell that apart from the form having moved, which is the one thing it exists to
// catch. Reading the ambiguous case as "already applied" reports skipped and exits
// 0 through a real breakage; reading it as drift cries wolf every time.
//
// Trying several removes the ambiguity without matching anything: one account
// having already applied for all of them is unlikely, so all of them failing is
// evidence about the form rather than about this account. The healthy case still
// costs one attempt, because the first success returns.
//
// Chosen for being REST, auto-approved at 개발단계, and obscure enough that an
// account is unlikely to hold them: 공정거래위원회 기업집단, 영천시 태양광 허가,
// 국립무형유산원 기증기탁.
var ApplyCanaryPKs = []string{"15091886", "15157823", "15094326"}

// ApplyCheck drives the 활용신청 form without submitting and reports whether apply
// could still fill it. This is the only automated coverage of 활용신청 — the one
// capability nothing else replaces, and until now the only seam doctor did not
// touch, so a portal redesign would have left every other check green while the
// thing that matters was broken.
//
// It lives in the CLI's check set rather than Run because it needs a session and
// starts a browser, which the read-only checks deliberately avoid.
func ApplyCheck(ctx context.Context, pk string) Check {
	candidates := ApplyCanaryPKs
	explicit := pk != ""
	if explicit {
		candidates = []string{pk}
	}

	var refused []string
	for _, c := range candidates {
		probe, err := portal.ProbeApplyForm(ctx, c)
		switch {
		case errors.Is(err, portal.ErrNotLoggedIn):
			return Check{"apply", StatusSkipped, "세션 없음 — `odeduck login` 후 재점검"}
		case errors.Is(err, portal.ErrFormUnreachable):
			// Ambiguous on its own: either this account already applied, or the
			// form moved. Try the next candidate rather than guess.
			refused = append(refused, c)
			continue
		case err != nil:
			return Check{"apply", StatusDrift, "폼 점검 실패: " + err.Error()}
		case !probe.OK():
			return Check{"apply", StatusDrift, fmt.Sprintf(
				"신청 폼에서 다음 요소를 찾지 못했습니다: %s — 이 상태로는 활용신청이 동작하지 않습니다",
				strings.Join(probe.Missing(), ", "))}
		default:
			return Check{"apply", StatusOK, fmt.Sprintf(
				"신청 폼 정상 — 상세기능 %d개 확인, 제출하지 않음 (pk=%s)", probe.Operations, c)}
		}
	}

	if explicit {
		// The caller picked the pk, so "you picked one you already applied for" is
		// the likely reading and the fix is theirs.
		return Check{"apply", StatusSkipped, fmt.Sprintf(
			"지정한 pk=%s 의 신청 폼이 열리지 않았습니다 — 이미 신청한 데이터셋일 수 있습니다. "+
				"신청하지 않은 pk 를 `--apply-pk` 로 지정하세요", pk)}
	}
	return Check{"apply", StatusDrift, fmt.Sprintf(
		"카나리 %d개(%s) 전부 신청 폼이 열리지 않았습니다 — 한 계정이 전부 이미 신청했을 가능성은 낮으므로 "+
			"폼 경로·진입 플로우가 바뀐 것을 의심하세요. 이 계정이 정말 전부 신청했다면 "+
			"`--apply-pk` 로 신청하지 않은 pk 를 지정해 재점검하세요",
		len(refused), strings.Join(refused, ", "))}
}

// APIKeyCheck verifies the live key page without surfacing the credential. It
// deliberately bypasses the cached key: a cache hit would make doctor report
// green even after the portal removed or renamed the source field.
func APIKeyCheck(ctx context.Context) Check {
	err := portal.ProbeAPIKey(ctx)
	switch {
	case err == nil:
		return Check{"api-key", StatusOK, "활성 인증키 필드 확인 (값은 출력하지 않음)"}
	case errors.Is(err, portal.ErrNotLoggedIn):
		return Check{"api-key", StatusSkipped, "세션 없음 — `odeduck login` 후 재점검"}
	case errors.Is(err, portal.ErrAPIKeyNotIssued):
		return Check{"api-key", StatusSkipped, "활성 인증키 필드는 있으나 아직 발급된 키 없음"}
	default:
		return Check{"api-key", StatusDrift, "인증키 페이지 점검 실패: " + err.Error()}
	}
}
