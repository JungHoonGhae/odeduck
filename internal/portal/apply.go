package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// 목적분류(prcusePrpos) 코드. 자동승인 개발계정 신청의 활용목적 카테고리.
const (
	PurposeWeb                = "PROS01" // 웹 사이트 개발
	PurposeApp                = "PROS02" // 앱개발
	PurposeEtc                = "PROS03" // 기타
	PurposeRef                = "PROS04" // 참고자료
	PurposeResearch           = "PROS05" // 연구(논문 등)
	multiCloudApplicationPath = "multiCloudApiRequestForm.do"
)

// A headless apply browser uses one fixed profile and CDP port. Serializing the
// flow prevents concurrent MCP calls in this process from corrupting the profile
// or attaching to each other's authenticated tab.
var applyBrowserSlot = make(chan struct{}, 1)

// NormalizePurposeCategory accepts the human-facing names used by CLI/MCP and
// the portal codes used internally. Refusing unknown values is safer than
// silently filing a commercial app as academic research.
func NormalizePurposeCategory(category string) (string, error) {
	category = strings.TrimSpace(category)
	switch strings.ToLower(category) {
	case "web", strings.ToLower(PurposeWeb):
		return PurposeWeb, nil
	case "app", strings.ToLower(PurposeApp):
		return PurposeApp, nil
	case "etc", strings.ToLower(PurposeEtc):
		return PurposeEtc, nil
	case "ref", strings.ToLower(PurposeRef):
		return PurposeRef, nil
	case "research", strings.ToLower(PurposeResearch):
		return PurposeResearch, nil
	default:
		return "", fmt.Errorf("활용목적 분류가 필요합니다: web | app | research | ref | etc")
	}
}

// ApplySummary is shown to the user for confirmation before submitting.
type ApplySummary struct {
	PublicDataPk string `json:"publicDataPk"`
	DataName     string `json:"dataName"`
	Operations   int    `json:"operations"` // 신청할 상세기능 수
	Category     string `json:"category"`   // 목적분류 코드
	Purpose      string `json:"purpose"`    // 활용목적 내용
}

// ApplyResult reports the outcome.
type ApplyResult struct {
	Submitted bool   `json:"submitted"`
	Canceled  bool   `json:"canceled,omitempty"`
	Message   string `json:"message"`
}

// Apply fills and submits a data.go.kr OpenAPI 활용신청 (auto-approved dev
// account) for one publicDataPk, using the live browser session. It is
// deliberately single-pk and purpose-required: applications are account-scoped,
// account-mutating actions, so opendatactl never bulk-applies speculatively. confirm
// is called with the filled summary; submission happens only if it returns true.
//
// The form's own fn_save() performs validation and POSTs — opendatactl drives the
// portal's real logic rather than re-implementing the request, so it stays
// robust to field changes. Any validation alert() is captured as the failure.
func Apply(ctx context.Context, pk, purpose, category string, confirm func(ApplySummary) bool) (*ApplyResult, error) {
	if err := ValidatePublicDataPK(pk); err != nil {
		return nil, err
	}
	if strings.TrimSpace(purpose) == "" {
		return nil, fmt.Errorf("활용목적이 필요합니다 (--purpose)")
	}
	normalizedCategory, err := NormalizePurposeCategory(category)
	if err != nil {
		return nil, err
	}
	category = normalizedCategory
	var res *ApplyResult
	err = onApplyForm(ctx, pk, func(tctx context.Context, dialog func() string) error {
		var aerr error
		res, aerr = fillAndSubmit(tctx, pk, purpose, category, confirm, dialog)
		return aerr
	})
	if err != nil {
		if res != nil {
			return res, fmt.Errorf("활용신청 결과(%s) 이후 세션 정리 실패: %w", res.Message, err)
		}
		return nil, err
	}
	return res, nil
}

// onApplyForm opens the 활용신청 form for pk in a browser carrying the saved session
// and hands the tab to body. Everything it does before that — injecting the session,
// completing the SSO trampoline, setting the currentMyMenuId precondition, settling
// on the form — is portal-shaped and is exactly what breaks when data.go.kr changes.
//
// ProbeApplyForm goes through this same function rather than its own copy. A harness
// that reaches the form by a different route proves nothing about whether apply can
// still reach it, which is the failure this project has to detect: apply is the one
// capability nothing else replaces, and its only previous check was a human running
// it against a real account.
//
// dialog, passed to body, returns the last JavaScript dialog message the page raised
// (validation alerts and the success notice both arrive that way).
func onApplyForm(ctx context.Context, pk string, body func(tctx context.Context, dialog func() string) error) (retErr error) {
	releaseSession, err := acquireSessionOperation(ctx)
	if err != nil {
		return err
	}
	defer releaseSession()
	select {
	case applyBrowserSlot <- struct{}{}:
		defer func() { <-applyBrowserSlot }()
	case <-ctx.Done():
		return ctx.Err()
	}

	// Submission has to run in a browser: opendatactl drives the portal's own
	// fn_save() so the page builds and validates the payload (see
	// docs/adr/0001). Reuse a live browser if there is one; otherwise start a
	// headless one and inject the saved session, so no window appears.
	st, sess, err := browserForApply(ctx)
	if err != nil {
		return err
	}

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, st.WebSocketURL)
	defer cancelAlloc()
	tctx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()
	tctx, tcancel := context.WithTimeout(tctx, 90*time.Second)
	defer tcancel()

	if sess != nil {
		// Ownership starts when browserForApply launches the process. Register
		// cleanup before cookie injection, because injection can partially succeed
		// and then fail while leaving an authenticated browser behind.
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cleanupCancel()
			closeBrowser(cleanupCtx, st)
			if st.ProfileDir != "" {
				_ = os.RemoveAll(st.ProfileDir)
			}
		}() // headless instance is ours; don't leave it running
		if err := injectSession(tctx, sess); err != nil {
			return fmt.Errorf("세션 주입 실패: %w", err)
		}
	}
	// Registered after cleanup so it runs first. Every later exit path — including
	// an unreachable form or canceled body — captures rotations before the owned
	// browser is terminated. The named return preserves the original failure too.
	defer func() {
		retErr = errors.Join(retErr, refreshSessionFrom(ctx, tctx, st))
	}()

	// Capture any JS dialog (validation alert / success notice) and accept it.
	var dialogMu sync.Mutex
	var lastDialog string
	chromedp.ListenTarget(tctx, func(ev any) {
		if d, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			dialogMu.Lock()
			lastDialog = d.Message
			dialogMu.Unlock()
			go chromedp.Run(tctx, page.HandleJavaScriptDialog(true))
		}
	})
	dialog := func() string {
		dialogMu.Lock()
		defer dialogMu.Unlock()
		return lastDialog
	}

	// 워밍업: 새 탭의 www 세션을 먼저 인증 상태로 만든다(SSO 트램펄린 완료).
	// 이걸 안 하면 폼 진입의 첫 네비게이션이 트램펄린을 타며 리다이렉트를 잃는다.
	// tctx 직접 사용(자식 타임아웃 컨텍스트 취소가 탭을 닫는 것을 회피).
	chromedp.Run(tctx, chromedp.Navigate(BaseURL+AccountListPath))
	warmDeadline := time.Now().Add(15 * time.Second)
	var warmLoc string
	for time.Now().Before(warmDeadline) {
		chromedp.Run(tctx, chromedp.Location(&warmLoc))
		if warmLoc != "" && !strings.Contains(warmLoc, "/sso/profile.do") {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if strings.Contains(warmLoc, "common-login") || strings.Contains(warmLoc, "auth.data.go.kr") {
		return ErrNotLoggedIn
	}

	// 신청 폼 진입: currentMyMenuId 쿠키가 전제조건(없으면 index.do 로 튕김).
	// Match the portal's current fn_goOpenAPIRequestForm contract: individual
	// accounts pass an empty isBusinessApply value. The legacy hard-coded "N"
	// now redirects a valid request back to index.do on the KRDS portal.
	formURL := fmt.Sprintf("%s/tcs/dss/redirectDevAcountRequestForm.do?publicDataPk=%s&isBusinessApply=", BaseURL, pk)
	var loc string
	if err := chromedp.Run(tctx,
		network.SetCookie("currentMyMenuId", "M020105").WithDomain("www.data.go.kr").WithPath("/"),
		chromedp.Navigate(formURL),
	); err != nil {
		return err
	}
	// settle: legacy REST form, ODCloud multi form, or index.do.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		chromedp.Run(tctx, chromedp.Location(&loc))
		if isApplyFormLocation(loc) || strings.Contains(loc, "index.do") {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if strings.Contains(loc, "common-login") || strings.Contains(loc, "auth.data.go.kr") {
		return ErrNotLoggedIn
	}
	if strings.Contains(loc, "index.do") || !isApplyFormLocation(loc) {
		return fmt.Errorf("%w (pk=%s) — 이미 신청했거나 신청 불가한 데이터일 수 있습니다", ErrFormUnreachable, pk)
	}
	return body(tctx, dialog)
}

func isApplyFormLocation(location string) bool {
	return strings.Contains(location, "selectDevAcountRequestForm.do") ||
		strings.Contains(location, multiCloudApplicationPath)
}

// ErrFormUnreachable means the 활용신청 form did not load for this pk. It is a
// distinct sentinel because the two reasons demand opposite reactions: an
// already-applied-for dataset is normal and expected, while markup or flow changes
// mean apply is broken. A checker that cannot tell them apart either cries wolf or
// stays quiet through a real breakage.
var ErrFormUnreachable = errors.New("신청 폼에 접근하지 못했습니다")

func fillAndSubmit(tctx context.Context, pk, purpose, category string,
	confirm func(ApplySummary) bool, dialog func() string) (*ApplyResult, error) {
	// 폼 채우기 + 요약 추출. fill=true 이므로 실제로 값이 채워진다.
	fillJS := applyFormJS(purpose, category, true)

	var raw string
	if err := chromedp.Run(tctx, chromedp.Evaluate(fillJS, &raw)); err != nil {
		return nil, fmt.Errorf("폼 채우기 실패: %w", err)
	}
	var filled FormProbe
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &filled); err != nil {
		return nil, fmt.Errorf("폼 채우기 결과를 읽지 못했습니다: %w", err)
	}
	// Refuse to submit a form we could not fill. fn_save() on a page whose fields
	// moved would post whatever the page happens to hold, against a real account.
	if missing := filled.Missing(); len(missing) > 0 {
		return nil, fmt.Errorf("신청 폼 구조가 예상과 다릅니다 — 없는 요소: %s (포털 마크업 변경 가능성, `opendatactl doctor` 로 확인)",
			strings.Join(missing, ", "))
	}

	summary := ApplySummary{
		PublicDataPk: pk,
		DataName:     filled.DataName,
		Operations:   filled.Operations,
		Category:     category,
		Purpose:      purpose,
	}
	if confirm != nil && !confirm(summary) {
		return &ApplyResult{Submitted: false, Canceled: true, Message: "사용자가 취소함 (제출 안 함)"}, nil
	}

	// 제출: 폼의 fn_save() 가 검증 → confirm("신청하시겠습니까?") → AJAX POST.
	// confirm/완료 알림은 위 dialog 리스너가 모두 수락한다.
	if err := chromedp.Run(tctx, chromedp.Evaluate(`(function(){ try{ fn_save(); return 'ok'; }catch(e){ return ''+e; } })()`, nil)); err != nil {
		return nil, fmt.Errorf("제출 호출 실패: %w", err)
	}
	time.Sleep(4 * time.Second) // confirm 수락 + POST + 처리 대기

	// 성공 판정 = 목록(ground truth)에 반영됐는지. 폼은 AJAX 제출이라 위치로는
	// 판별이 안 되므로 활용신청 현황을 다시 읽어 데이터명이 나타났는지 확인한다.
	dlg := dialog()

	if listHTML, _, lerr := probeListLenient(tctx); lerr == nil {
		if apps, perr := parseApplications(listHTML); perr == nil {
			for _, a := range apps {
				if sameApplicationTitle(a.Title, filled.DataName) {
					return &ApplyResult{Submitted: true, Message: "신청 완료 (자동승인): " + a.Status}, nil
				}
			}
		}
	}
	// The account list is eventually consistent: production showed the portal's
	// success alert immediately but the reused tab still rendered the old list.
	// The alert comes from this form's fn_save response, so its exact message is a
	// valid success signal when list propagation lags. Keep the match narrow to
	// avoid treating validation or unrelated "completed" dialogs as submission.
	if isApplySuccessDialog(dlg) {
		return &ApplyResult{Submitted: true, Message: "신청 완료 (포털 성공 응답; 신청내역 반영 대기 중)"}, nil
	}
	// 목록에 없으면 거부(검증 실패 등). dialog 메시지를 사유로.
	msg := "제출이 반영되지 않았습니다"
	if dlg != "" && !strings.Contains(dlg, "신청하시겠습니까") {
		msg += ": " + strings.ReplaceAll(dlg, "\n", " ")
	}
	return &ApplyResult{Submitted: false, Message: msg}, nil
}

func sameApplicationTitle(applicationTitle, dataName string) bool {
	applicationTitle = strings.Join(strings.Fields(applicationTitle), " ")
	dataName = strings.Join(strings.Fields(dataName), " ")
	return dataName != "" && applicationTitle == dataName
}

func isApplySuccessDialog(message string) bool {
	message = strings.Join(strings.Fields(message), " ")
	return message == "활용신청이 완료되었습니다." ||
		strings.HasPrefix(message, "신청이 완료되었습니다. 신청된 API는 1~2시간 후 호출 가능합니다.")
}

// probeListLenient navigates the reused tab to the 활용신청 현황 list and returns
// its HTML, settling past the SSO trampoline.
func probeListLenient(tctx context.Context) (html, loc string, err error) {
	if e := chromedp.Run(tctx, chromedp.Navigate(BaseURL+AccountListPath)); e != nil {
		return "", "", e
	}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		chromedp.Run(tctx, chromedp.Location(&loc))
		if loc != "" && !strings.Contains(loc, "/sso/profile.do") {
			if chromedp.Run(tctx, chromedp.OuterHTML("html", &html, chromedp.ByQuery)) == nil && len(html) > 1000 {
				return html, loc, nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return html, loc, nil
}
