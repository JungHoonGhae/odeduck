package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Session is the authenticated data.go.kr session, extracted from the browser
// once at login so later commands can talk to the portal over plain HTTP.
//
// The portal's auth cookies are session-scoped — Chrome discards them when it
// exits — but the values stay valid server-side until the session expires. So
// odeduck copies them out after login and closes the browser, instead of keeping
// a window open for the rest of the day. Authenticated HTTP reads capture and
// persist rotated cookies, extending a sliding session while the portal permits
// it. The one flow that still needs a browser (활용신청 submit, which drives the
// portal's own form JS) relaunches Chrome headless and injects these cookies, so
// nothing ever appears on screen after login.
type Session struct {
	Cookies     map[string]string `json:"cookies"`
	RetrievedAt time.Time         `json:"retrievedAt"`
}

var sessionFileMu sync.Mutex

// sessionOperationSlot serializes each read → authenticated request → rotated
// cookie write transaction inside one process. acquireSessionOperation also
// holds an advisory file lock for the same transaction: Codex, Claude, Gemini,
// Cursor, a CLI, and an MCP server can all be separate processes sharing the
// same rotating data.go.kr session.
var sessionOperationSlot = make(chan struct{}, 1)

func acquireSessionOperation(ctx context.Context) (func(), error) {
	select {
	case sessionOperationSlot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	releaseFile, err := acquireSessionFileLock(ctx)
	if err != nil {
		<-sessionOperationSlot
		return nil, err
	}
	return func() {
		releaseFile()
		<-sessionOperationSlot
	}, nil
}

const maxAuthenticatedPageBytes int64 = 8 << 20 // 8 MiB; account pages are HTML lists/forms

func sessionPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "datagokr-session.json"), nil
}

// saveSession writes the session 0600 — these cookies are credentials.
func saveSession(s *Session) error {
	sessionFileMu.Lock()
	defer sessionFileMu.Unlock()
	return saveSessionUnlocked(s)
}

func saveSessionUnlocked(s *Session) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(path, data, 0o600)
}

// loadSession reads the saved session, or returns ErrNotLoggedIn if none exists.
func loadSession() (*Session, error) {
	sessionFileMu.Lock()
	defer sessionFileMu.Unlock()
	return loadSessionUnlocked()
}

func loadSessionUnlocked() (*Session, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if len(s.Cookies) == 0 {
		return nil, ErrNotLoggedIn
	}
	return &s, nil
}

// clearSession removes everything logout must not leave behind: the extracted
// cookies, the cached serviceKey, and the Chrome profiles.
//
// The profiles matter as much as the files. The login profile accumulates the
// cookies of whatever the human logged in WITH — an SSO provider's session, for
// instance — and the headless profile holds the cookies odeduck injected into it.
// Removing only odeduck's own two files would leave those on disk after the user
// asked to be logged out.
func clearSession() error {
	dirs, err := configDirsForCleanup()
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		for _, file := range []string{"datagokr-session.json", keyCacheFile, daemonStateFile} {
			if err := os.Remove(filepath.Join(dir, file)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		for _, profile := range []string{"chrome-profile", "chrome-headless"} {
			if err := os.RemoveAll(filepath.Join(dir, profile)); err != nil {
				return fmt.Errorf("브라우저 프로파일 삭제 실패 (%s): %w", profile, err)
			}
		}
		headlessProfiles, err := filepath.Glob(filepath.Join(dir, "chrome-headless-*"))
		if err != nil {
			return err
		}
		for _, profile := range headlessProfiles {
			if data, readErr := os.ReadFile(filepath.Join(profile, headlessStateFile)); readErr == nil {
				var state daemonState
				if json.Unmarshal(data, &state) == nil {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 12*time.Second)
					closeBrowser(cleanupCtx, &state)
					cleanupCancel()
				}
			}
			if err := os.RemoveAll(profile); err != nil {
				return fmt.Errorf("브라우저 프로파일 삭제 실패 (%s): %w", filepath.Base(profile), err)
			}
		}
	}
	return nil
}

// extractCookies copies data.go.kr's cookies out of the live browser tab.
func extractCookies(tctx context.Context) (*Session, error) {
	var jar []*network.Cookie
	err := chromedp.Run(tctx, chromedp.ActionFunc(func(c context.Context) error {
		var e error
		jar, e = network.GetCookies().WithURLs([]string{BaseURL}).Do(c)
		return e
	}))
	if err != nil {
		return nil, err
	}
	s := &Session{Cookies: map[string]string{}, RetrievedAt: time.Now().UTC()}
	for _, c := range jar {
		s.Cookies[c.Name] = c.Value
	}
	if len(s.Cookies) == 0 {
		return nil, fmt.Errorf("브라우저에서 쿠키를 얻지 못했습니다")
	}
	return s, nil
}

type sessionPage struct {
	HTML     string
	Location string
	Session  *Session
	Changed  bool
}

// getWithSession fetches a portal path over plain HTTP using the session cookies.
// A real cookie jar follows portal redirects and captures any rotated JSESSIONID;
// callers persist the returned session only after confirming the page is still
// authenticated, so an anonymous redirect can never overwrite working cookies.
func getWithSession(ctx context.Context, s *Session, path string) (*sessionPage, error) {
	target, err := url.Parse(BaseURL + path)
	if err != nil {
		return nil, err
	}
	root, err := url.Parse(BaseURL + "/")
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	seed := make([]*http.Cookie, 0, len(s.Cookies))
	for name, value := range s.Cookies {
		seed = append(seed, &http.Cookie{Name: name, Value: value, Path: "/"})
	}
	jar.SetCookies(root, seed)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	req.Header.Set("Accept", "text/html,*/*")
	req.Header.Set("Referer", BaseURL+"/")
	client := fetch.NewHTTPClient(30 * time.Second)
	client.Jar = jar
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAuthenticatedPageBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxAuthenticatedPageBytes {
		return nil, fmt.Errorf("GET %s: 인증 페이지가 %d바이트 제한을 초과했습니다", path, maxAuthenticatedPageBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	active := append(jar.Cookies(root), jar.Cookies(target)...)
	if resp.Request != nil && resp.Request.URL != nil {
		active = append(active, jar.Cookies(resp.Request.URL)...)
	}
	refreshed, changed := sessionWithCookies(s, active, time.Now().UTC())
	loc := target.String()
	if resp.Request != nil && resp.Request.URL != nil {
		loc = resp.Request.URL.String()
	}
	return &sessionPage{HTML: string(body), Location: loc, Session: refreshed, Changed: changed}, nil
}

func sessionWithCookies(current *Session, cookies []*http.Cookie, now time.Time) (*Session, bool) {
	refreshed := &Session{Cookies: make(map[string]string, len(current.Cookies)), RetrievedAt: current.RetrievedAt}
	for name, value := range current.Cookies {
		refreshed.Cookies[name] = value
	}
	changed := false
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		if old, ok := refreshed.Cookies[cookie.Name]; !ok || old != cookie.Value {
			refreshed.Cookies[cookie.Name] = cookie.Value
			changed = true
		}
	}
	if changed {
		refreshed.RetrievedAt = now
	}
	return refreshed, changed
}

func persistSessionRefresh(page *sessionPage) error {
	if page == nil || !page.Changed {
		return nil
	}
	return saveSession(page.Session)
}

// getAuthed fetches a path with the session and retries once if the portal serves
// the login wall. Observed on a live, valid session: data.go.kr intermittently
// answers an authenticated request with the login page (HTTP 200), and treating
// that as "logged out" makes callers demand a needless re-login.
func getAuthed(ctx context.Context, s *Session, path string) (*sessionPage, error) {
	return getAuthedWith(ctx, s, path, getWithSession, 800*time.Millisecond)
}

type sessionPageGetter func(context.Context, *Session, string) (*sessionPage, error)

func getAuthedWith(ctx context.Context, s *Session, path string, get sessionPageGetter, retryDelay time.Duration) (*sessionPage, error) {
	page, err := get(ctx, s, path)
	if err != nil {
		return nil, err
	}
	if !isLoginWall(page.HTML, page.Location) {
		return page, nil
	}
	changedOnFirstAttempt := page.Changed
	if retryDelay > 0 {
		timer := time.NewTimer(retryDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	retrySession := s
	if page.Session != nil {
		retrySession = page.Session
	}
	page, err = get(ctx, retrySession, path)
	if err != nil {
		return nil, err
	}
	if isLoginWall(page.HTML, page.Location) {
		return nil, ErrNotLoggedIn
	}
	// The retry may not rotate again, but its authenticated session can still be
	// the value issued by the first login-wall response. Preserve that cumulative
	// change so the verified cookie is written to disk by the caller.
	page.Changed = page.Changed || changedOnFirstAttempt
	return page, nil
}

// verifiedSession returns the cookie jar that actually authenticated over plain
// HTTP, including any rotation produced by the verification request itself.
func verifiedSession(ctx context.Context, s *Session) (*Session, bool) {
	page, err := getAuthed(ctx, s, AccountListPath)
	if err != nil || !isAuthed(page.HTML, page.Location) {
		return nil, false
	}
	return page.Session, true
}

// refreshSessionFrom re-reads the cookie jar of a browser we just drove and
// stores it if it still authenticates.
//
// The portal rotates JSESSIONID during the 활용신청 flow: the browser ends up
// holding a different session than the one injected into it, which leaves the
// copy on disk dead the moment that browser exits. Without this, a single apply
// silently costs the user their session and forces a re-login.
func refreshSessionFrom(ctx context.Context, tctx context.Context, st *daemonState) error {
	sess, err := extractCookies(tctx)
	if err != nil {
		// The apply tab inherits the caller's deadline. When it expires, attach
		// again with an independent short context before cleanup kills the only
		// browser holding the rotated cookie.
		sess, err = extractCookiesFromBrowser(st)
		if err != nil {
			return fmt.Errorf("브라우저 세션 추출 실패: %w", err)
		}
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	page, err := getAuthed(refreshCtx, sess, AccountListPath)
	if err != nil || !isAuthed(page.HTML, page.Location) {
		if err == nil {
			err = ErrNotLoggedIn
		}
		return fmt.Errorf("갱신 세션 검증 실패: %w", err)
	}
	// Validation itself can rotate JSESSIONID once more. Persist the verified
	// page's jar, not the pre-validation browser snapshot.
	if err := saveSession(page.Session); err != nil {
		return fmt.Errorf("갱신 세션 저장 실패: %w", err)
	}
	return nil
}

func extractCookiesFromBrowser(st *daemonState) (*Session, error) {
	if st == nil || st.WebSocketURL == "" {
		return nil, fmt.Errorf("브라우저 연결 정보가 없습니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, st.WebSocketURL)
	defer cancelAlloc()
	tctx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()
	if err := chromedp.Run(tctx, network.Enable()); err != nil {
		return nil, err
	}
	return extractCookies(tctx)
}

// browserForApply returns a browser to drive the 활용신청 form with. If a session
// browser is already running it is reused (sess == nil, nothing to inject).
// Otherwise a headless Chrome is started and the saved session is returned for
// the caller to inject — that instance belongs to the caller, which closes it.
func browserForApply(ctx context.Context) (*daemonState, *Session, error) {
	if st, err := loadState(); err == nil && browserUsable(st.Port) {
		return st, nil, nil
	}
	page, err := verifiedSavedPage(ctx)
	if err != nil {
		return nil, nil, err
	}
	cmd, port, profile, err := launchHeadless()
	if err != nil {
		return nil, nil, err
	}
	ws, err := discoverWS(ctx, port, 25*time.Second)
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		_ = os.RemoveAll(profile)
		return nil, nil, err
	}
	state := &daemonState{WebSocketURL: ws, PID: cmd.Process.Pid, Port: port, ProfileDir: profile}
	if err := saveHeadlessState(state); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.RemoveAll(profile)
		return nil, nil, fmt.Errorf("headless 브라우저 상태 저장 실패: %w", err)
	}
	// The MCP server can live for days and launch many short-lived apply
	// browsers. Reap each child when cleanup closes it so exited Chrome processes
	// do not accumulate as zombies.
	go func() { _ = cmd.Wait() }()
	return state, page.Session, nil
}

// injectSession sets the saved cookies on a fresh browser so it is authenticated
// without a re-login, then parks it on the portal root to settle the session.
func injectSession(tctx context.Context, s *Session) error {
	if err := setSessionCookies(tctx, s); err != nil {
		return err
	}
	if err := chromedp.Run(tctx, chromedp.Navigate(BaseURL+"/")); err != nil {
		return err
	}
	time.Sleep(1500 * time.Millisecond) // let the portal settle the injected session
	return nil
}

func setSessionCookies(tctx context.Context, s *Session) error {
	actions := []chromedp.Action{network.Enable()}
	for name, value := range s.Cookies {
		actions = append(actions, network.SetCookie(name, value).
			WithDomain("www.data.go.kr").WithPath("/"))
	}
	return chromedp.Run(tctx, actions...)
}
