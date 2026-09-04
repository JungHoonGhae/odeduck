package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// APIKeyListPath is the 인증키 발급현황 page, which carries the account's active
// serviceKey.
const APIKeyListPath = "/iim/api/selectApiKeyList.do"

var (
	// ErrAPIKeyFieldMissing means the page no longer carries the field the key
	// parser relies on. This is markup drift, not an account without a key.
	ErrAPIKeyFieldMissing = errors.New("활성 인증키 필드가 없습니다")
	// ErrAPIKeyNotIssued means the expected field exists but has no value yet.
	ErrAPIKeyNotIssued = errors.New("아직 발급된 인증키가 없습니다")
)

// APIKey returns the account's serviceKey (일반 인증키). data.go.kr issues one key
// per account on the first approved application and reuses it for every API, so
// this is all a caller needs to start calling.
//
// This is what closes the loop for an agent: it can search, apply, see the
// approval, and then fetch the key itself instead of asking a human to copy it
// out of the portal.
func APIKey(ctx context.Context) (string, error) {
	release, err := acquireSessionOperation(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	// Why the session path failed matters — a stale cookie and a changed page
	// need different fixes — so it is reported rather than swallowed when the
	// browser fallback is also unavailable.
	var sessErr error
	if sess, err := loadSession(); err == nil {
		page, herr := getAuthed(ctx, sess, APIKeyListPath)
		if herr != nil {
			sessErr = herr
		} else if key, kerr := apiKeyFromAccountPage(page.HTML); kerr != nil {
			sessErr = kerr
		} else {
			if err := persistSessionRefresh(page); err != nil {
				return "", fmt.Errorf("세션 자동 갱신 저장 실패: %w", err)
			}
			cacheKey(key)
			return key, nil
		}
	} else {
		sessErr = err
	}

	// The key outlives the browser session by years (see 만료예정일 in
	// list_applications), so a cached one keeps call working after the session
	// expires — only apply/applications actually need a live session.
	if key := cachedKey(); key != "" {
		return key, nil
	}

	st, err := loadState()
	if err != nil || !browserUsable(st.Port) {
		return "", sessErr
	}
	html, err := probeViaBrowser(ctx, st, APIKeyListPath)
	if err != nil {
		return "", err
	}
	key, err := apiKeyFromAccountPage(html)
	if err == nil {
		cacheKey(key)
	}
	return key, err
}

// ProbeAPIKey checks the live authenticated key page without returning or
// caching the credential. doctor uses this path so a previously cached key
// cannot hide markup drift on the page that originally issued it.
func ProbeAPIKey(ctx context.Context) error {
	release, err := acquireSessionOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	sess, err := loadSession()
	if err != nil {
		return err
	}
	page, err := getAuthed(ctx, sess, APIKeyListPath)
	if err != nil {
		return err
	}
	if _, err = apiKeyFromAccountPage(page.HTML); err != nil {
		return err
	}
	return persistSessionRefresh(page)
}

// keyCacheFile stores the account serviceKey so calls survive session expiry.
const keyCacheFile = "datagokr-apikey"

func cacheKey(key string) {
	dir, err := configDir()
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, keyCacheFile), []byte(key), 0o600) // credential
}

// InvalidateCachedKey drops the cached serviceKey so the next APIKey call reads it
// from the portal again. Call this when the gateway rejects the key: the user may
// have reissued it, which silently invalidates the copy on disk.
func InvalidateCachedKey() {
	if dir, err := configDir(); err == nil {
		os.Remove(filepath.Join(dir, keyCacheFile))
	}
}

func cachedKey() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, keyCacheFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// parseAPIKey pulls the active key out of the 인증키 발급현황 page.
//
// The page carries it twice: a hidden input holding the current key, and a table
// listing every key ever issued (신규발급 plus any 재발급). Only the hidden input
// identifies which one is *active* — a reissued key invalidates the original, and
// the table does not say which row won — so that is the single source read here.
// The value is the plain (Decoding) form; apicall.Call escapes it as needed.
func parseAPIKey(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}
	key, _ := doc.Find("#pblisrCrtfcKeyPlain").First().Attr("value")
	if doc.Find("#pblisrCrtfcKeyPlain").Length() == 0 {
		return "", fmt.Errorf("%w — 페이지 구조 변경 가능성, `odeduck doctor` 로 확인", ErrAPIKeyFieldMissing)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("%w — 첫 활용신청이 승인된 뒤 발급됩니다", ErrAPIKeyNotIssued)
	}
	return key, nil
}

// apiKeyFromAccountPage distinguishes an authenticated key page whose active
// field changed from an expired session that landed on the generic portal home.
// Both are HTTP 200 responses without #pblisrCrtfcKeyPlain, but only the former
// is parser drift. Keeping that distinction prevents doctor from reporting a
// portal redesign when the actual action is simply `odeduck login`.
func apiKeyFromAccountPage(html string) (string, error) {
	key, err := parseAPIKey(html)
	if !errors.Is(err, ErrAPIKeyFieldMissing) {
		return key, err
	}

	doc, parseErr := goquery.NewDocumentFromReader(strings.NewReader(html))
	if parseErr != nil {
		return "", parseErr
	}
	identified := false
	doc.Find("h1, h2, h3, h4, .page-title").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if strings.Contains(strings.Join(strings.Fields(s.Text()), " "), "인증키 발급현황") {
			identified = true
			return false
		}
		return true
	})
	if !identified {
		return "", ErrNotLoggedIn
	}
	return "", err
}
