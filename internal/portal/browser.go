package portal

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// BaseURL is the portal root. It is a var, not a const, so tests can point the
// CDP probe at a local fixture server (see probe_live_test.go).
var BaseURL = "https://www.data.go.kr"

// LoginTimeout bounds the interactive login wait.
const LoginTimeout = 5 * time.Minute

// Login ensures a live, authenticated browser session exists. It launches (or
// reuses) opendatactl's detached Chrome, opens the login page, and waits until the
// session can actually load an authenticated page. The browser is left running
// so later commands re-attach to it. progress receives status lines (may be nil).
func Login(ctx context.Context, progress io.Writer, keepBrowser bool) error {
	release, err := acquireSessionOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	logln := func(format string, a ...any) {
		if progress != nil {
			fmt.Fprintf(progress, format+"\n", a...)
		}
	}

	st, _ := loadState()
	if st == nil || !browserUsable(st.Port) {
		// A recorded browser that is alive but unusable (window closed, zero page
		// targets) still holds the debug port, so launching a fresh one on the same
		// port would collide with it. End it first — it is ours, we recorded its pid.
		if st != nil && st.PID > 0 && wsAlive(st.Port) {
			logln("이전 브라우저가 창 없이 남아 있어 정리하고 새로 띄웁니다.")
			killTree(st.PID)
			for i := 0; i < 20 && wsAlive(st.Port); i++ {
				time.Sleep(150 * time.Millisecond)
			}
		}
		logln("브라우저를 띄웁니다… 열리는 창에서 data.go.kr 에 로그인하세요 (네이버/카카오/아이디).")
		cmd, err := launchBrowser(BaseURL + "/sso/login.do")
		if err != nil {
			return err
		}
		ws, err := discoverWS(ctx, debugPort, 20*time.Second)
		if err != nil {
			return err
		}
		// Record the PID: closing the window is not enough to end the process
		// (the debug port keeps Chrome alive with zero tabs), so closeBrowser
		// needs something to signal.
		st = &daemonState{WebSocketURL: ws, Port: debugPort, PID: cmd.Process.Pid}
		if err := saveState(st); err != nil {
			return err
		}
	} else {
		logln("기존 브라우저에 연결합니다. 로그인이 안 돼 있으면 열린 창에서 로그인하세요.")
	}

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, st.WebSocketURL)
	defer cancelAlloc()

	// One blank tab, never navigated — it exists only as a handle for reading the
	// profile's cookie jar. Navigating a probe tab to an authenticated page while
	// the user is still logging in bounces it to the login wall, which shows up as
	// a SECOND login screen and steals focus from the tab they are typing in.
	// Reading cookies needs no navigation, and the "are we logged in yet?" question
	// is then answered over plain HTTP, where a timeout is safe to apply.
	tctx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()
	if err := chromedp.Run(tctx, network.Enable()); err != nil {
		return err
	}

	logln("로그인 완료를 기다리는 중… (최대 %d분)", int(LoginTimeout.Minutes()))
	deadline := time.Now().Add(LoginTimeout)
	tick := 0
	for time.Now().Before(deadline) {
		sess, cerr := extractCookies(tctx)
		var verified *Session
		works := false
		if cerr == nil {
			verified, works = verifiedSession(ctx, sess)
		}
		if cerr == nil && works {
			logln("✅ 로그인 확인.")
			if err := saveState(st); err != nil {
				return err
			}
			if err := saveSession(verified); err != nil {
				return err
			}
			// Copy the session out of the browser so later commands don't need a
			// window open. Verified before we close anything — if the cookies
			// don't authenticate on their own, keep the browser as the fallback.
			if keepBrowser {
				if err := setSessionCookies(tctx, verified); err != nil {
					return fmt.Errorf("회전된 세션을 브라우저에 반영하지 못했습니다: %w", err)
				}
				logln("   (--keep-browser: 브라우저를 열어 둡니다.)")
				return nil
			}
			// The HTTP verification can rotate JSESSIONID away from the value still
			// in Chrome. Save the verified jar directly instead of extracting and
			// invalidating the old browser cookie a second time.
			cancelTab()
			closeBrowser(ctx, st)
			logln("   세션을 저장하고 브라우저를 닫았습니다 — 이후 명령은 창 없이 동작합니다.")
			return nil
		}
		tick++
		if tick%5 == 0 {
			logln("[대기] 아직 로그인 전입니다… 열린 창에서 로그인해주세요.")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("로그인 시간 초과 (%d분) — 열린 창에서 로그인 후 다시 시도하세요", int(LoginTimeout.Minutes()))
}

// closeBrowser shuts the session browser down. Best-effort: the point is to get
// the window off the user's screen once its cookies have been copied out.
//
// It sends the CDP Browser.close command rather than cancelling the chromedp
// context: with a RemoteAllocator opendatactl only ATTACHED to this browser, so
// cancelling merely detaches and leaves the window on screen (the bug the old
// Logout shipped).
func closeBrowser(ctx context.Context, st *daemonState) {
	if !wsAlive(st.Port) {
		return
	}
	// Ask Chrome to close itself first so it flushes its profile. This needs a
	// tab to attach to; with zero tabs it fails harmlessly and the kill below
	// does the work.
	func() {
		allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, st.WebSocketURL)
		defer cancel()
		tctx, tcancel := chromedp.NewContext(allocCtx)
		defer tcancel()
		chromedp.Run(tctx, chromedp.ActionFunc(func(c context.Context) error {
			return browser.Close().Do(c)
		}))
	}()
	for i := 0; i < 20 && wsAlive(st.Port); i++ {
		time.Sleep(150 * time.Millisecond)
	}
	// Chrome lingers with the debug port open even after its last window closes,
	// so signal the process tree when it is still answering.
	if wsAlive(st.Port) && st.PID > 0 {
		killTree(st.PID)
		for i := 0; i < 20 && wsAlive(st.Port); i++ {
			time.Sleep(150 * time.Millisecond)
		}
	}
}

// Applications reads the 활용신청 현황 list. It prefers the saved session over
// plain HTTP (no browser needed) and falls back to driving a live browser over
// CDP when only that is available.
func Applications(ctx context.Context) ([]Application, error) {
	release, err := acquireSessionOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if sess, serr := loadSession(); serr == nil {
		if page, herr := getAuthed(ctx, sess, AccountListPath); herr == nil && isAuthed(page.HTML, page.Location) {
			if perr := persistSessionRefresh(page); perr != nil {
				return nil, fmt.Errorf("세션 자동 갱신 저장 실패: %w", perr)
			}
			sess = page.Session
			// The portal paginates at 10; without this the 11th application
			// onward vanishes silently, and an agent asking "did I already apply
			// for this?" gets a wrong answer.
			return collectApplicationPages(page.HTML, func(pageIndex int) (string, error) {
				more, merr := getAuthed(ctx, sess, fmt.Sprintf("%s?pageIndex=%d", AccountListPath, pageIndex))
				if merr != nil {
					return "", merr
				}
				if !isAuthed(more.HTML, more.Location) {
					return "", ErrNotLoggedIn
				}
				if perr := persistSessionRefresh(more); perr != nil {
					return "", fmt.Errorf("세션 자동 갱신 저장 실패: %w", perr)
				}
				sess = more.Session
				return more.HTML, nil
			})
		}
		// Session expired or insufficient — fall through to the browser, if any.
	}

	st, err := loadState()
	if err != nil {
		return nil, err
	}
	if !browserUsable(st.Port) {
		return nil, ErrNotLoggedIn
	}
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, st.WebSocketURL)
	defer cancelAlloc()
	tctx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()
	// Hard bound on the whole probe. Safe here (unlike inside probeIn) because
	// this tab is single-use: the timeout closing it is exactly what we want.
	tctx, tcancel := context.WithTimeout(tctx, 40*time.Second)
	defer tcancel()

	html, loc, err := probeIn(tctx, AccountListPath)
	if err != nil {
		return nil, err
	}
	if !isAuthed(html, loc) {
		return nil, ErrNotLoggedIn
	}
	return collectApplicationPages(html, func(pageIndex int) (string, error) {
		moreHTML, moreLoc, err := probeIn(tctx, fmt.Sprintf("%s?pageIndex=%d", AccountListPath, pageIndex))
		if err != nil {
			return "", err
		}
		if !isAuthed(moreHTML, moreLoc) {
			return "", ErrNotLoggedIn
		}
		return moreHTML, nil
	})
}

// Logout closes any live browser and clears the saved state and session cookies.
func Logout(ctx context.Context) error {
	release, err := acquireAllSessionOperations(ctx)
	if err != nil {
		return err
	}
	defer release()
	dirs, err := configDirsForCleanup()
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if st, loadErr := loadStateFrom(filepath.Join(dir, daemonStateFile)); loadErr == nil {
			closeBrowser(ctx, st)
		}
	}
	if err := clearSession(); err != nil {
		return err
	}
	return nil
}

// probeIn navigates the given (reused) tab to path, lets the SSO trampoline
// settle, and returns the final HTML + URL.
//
// It uses tctx DIRECTLY and never derives a cancellable child from it: with
// chromedp, cancelling (or expiring) a context derived from a tab context closes
// the tab, so a per-probe timeout would kill the very tab the caller wants to
// reuse. Login polls this in a loop — a child cancel here left every probe after
// the first failing with "context canceled", so a successful login was never
// detected. Callers that need a hard bound put the timeout on the tab context at
// creation time instead (see Applications, Apply).
func probeIn(tctx context.Context, path string) (html, loc string, err error) {
	if err = chromedp.Run(tctx, chromedp.Navigate(BaseURL+path)); err != nil {
		return "", "", err
	}
	// Settle: the portal may bounce through /sso/profile.do (an auto-submitting
	// form). Wait until the URL is no longer that trampoline, then read.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if e := chromedp.Run(tctx, chromedp.Location(&loc)); e != nil {
			return "", "", e
		}
		if !strings.Contains(loc, "/sso/profile.do") {
			if e := chromedp.Run(tctx, chromedp.OuterHTML("html", &html, chromedp.ByQuery)); e == nil {
				return html, loc, nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return html, loc, nil
}

// isAuthed reports whether a probed page is the authenticated 활용신청 현황 list
// rather than the login wall.
func isAuthed(html, loc string) bool {
	if isLoginWall(html, loc) {
		return false
	}
	return strings.Contains(html, "mypage-dataset-list") || strings.Contains(html, "활용신청 현황")
}

// isLoginWall reports whether the portal served the login page instead of the
// requested one. data.go.kr answers an expired session with HTTP 200 and this
// page rather than a redirect or 401, so every authenticated read has to check
// for it — otherwise the caller mistakes "logged out" for "page changed".
func isLoginWall(html, loc string) bool {
	if strings.Contains(loc, "common-login") || strings.Contains(loc, "auth.data.go.kr") {
		return true
	}
	// The marker must come from the page's own <title>, not from anywhere in the
	// HTML: the authenticated pages now carry "통합 로그인" inside a script comment,
	// and matching the whole document made every authenticated read look like the
	// login wall — login itself polled for 5 minutes and gave up on a live session.
	return strings.Contains(pageTitle(html), "로그인")
}

// pageTitle returns the document title, or "" if there is none.
func pageTitle(html string) string {
	start := strings.Index(html, "<title>")
	if start < 0 {
		return ""
	}
	rest := html[start+len("<title>"):]
	end := strings.Index(rest, "</title>")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// probeViaBrowser fetches a portal path through the live browser. Used as the
// fallback when the saved session cookies are not enough (see Applications,
// APIKey), so both share one CDP path instead of duplicating tab setup.
func probeViaBrowser(ctx context.Context, st *daemonState, path string) (string, error) {
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, st.WebSocketURL)
	defer cancelAlloc()
	tctx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()
	tctx, tcancel := context.WithTimeout(tctx, 40*time.Second)
	defer tcancel()

	html, loc, err := probeIn(tctx, path)
	if err != nil {
		return "", err
	}
	if strings.Contains(loc, "common-login") || strings.Contains(loc, "auth.data.go.kr") {
		return "", ErrNotLoggedIn
	}
	return html, nil
}
