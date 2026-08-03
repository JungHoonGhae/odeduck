// Package doctor is a liveness check for gongctl's fragile scraping. data.go.kr
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

	"github.com/JungHoonGhae/gongctl/internal/apicall"
	"github.com/JungHoonGhae/gongctl/internal/catalog"
	"github.com/JungHoonGhae/gongctl/internal/fetch"
	"github.com/JungHoonGhae/gongctl/internal/portal"
)

// CanaryPK is a stable, long-lived OpenAPI dataset (중앙선거관리위원회
// PofelcddInfoInqireService) used to probe the describe scraper. If data.go.kr
// ever retires it, the describe check will report drift — update this pk then.
const CanaryPK = "15000908"

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
	return []Check{
		searchCheck(ctx, fc, baseURL),
		describeCheck(ctx, fc, baseURL),
		catalogCheck(),
	}
}

// catalogCheck reports the local catalogue's freshness. A stale snapshot is the
// quiet failure mode of local discovery: searches keep succeeding while silently
// missing everything published since the last sync, so doctor — the command that
// exists to make quiet problems loud — is where it belongs.
func catalogCheck() Check {
	cat, err := catalog.Load()
	if errors.Is(err, catalog.ErrNotSynced) {
		return Check{"catalog", StatusSkipped, "카탈로그 없음 — `gongctl catalog sync` 로 만들면 검색이 즉시 처리됩니다"}
	}
	if err != nil {
		return Check{"catalog", StatusDrift, "카탈로그를 읽지 못했습니다: " + err.Error()}
	}
	days := cat.Age().Hours() / 24
	if cat.Stale() {
		return Check{"catalog", StatusDrift, fmt.Sprintf(
			"%.0f일 전 스냅샷 (%d건) — 그 이후 신설된 API 가 검색에서 누락됩니다. `gongctl catalog sync`",
			days, len(cat.Entries))}
	}
	return Check{"catalog", StatusOK, fmt.Sprintf("%.1f일 전 수집 (%d건)", days, len(cat.Entries))}
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
	case len(spec.Operations) == 0:
		return Check{"describe", StatusDrift, "상세기능 0건 파싱 — openapi.do 마크업이 바뀌었을 수 있음 (pk=" + CanaryPK + ")"}
	default:
		return Check{"describe", StatusOK, fmt.Sprintf("%d개 상세기능 파싱 (pk=%s)", len(spec.Operations), CanaryPK)}
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
			return Check{"apply", StatusSkipped, "세션 없음 — `gongctl login` 후 재점검"}
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
