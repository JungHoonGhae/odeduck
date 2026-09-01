package portal

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionFileLockSerializesIndependentHandles(t *testing.T) {
	isolatedUserConfigDir(t)

	release, err := acquireSessionFileLock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()

	waitCtx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if _, err := acquireSessionFileLock(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second file lock error = %v, want deadline exceeded", err)
	}

	release()
	released = true
	releaseAgain, err := acquireSessionFileLock(context.Background())
	if err != nil {
		t.Fatalf("file lock after release: %v", err)
	}
	releaseAgain()
}

func TestAllSessionLocksWaitForLegacyProcess(t *testing.T) {
	configHome := isolatedUserConfigDir(t)
	current := filepath.Join(configHome, configDirName)
	legacy := filepath.Join(configHome, compatibilityConfigDirNames[0])
	for _, dir := range []string{current, legacy} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	releaseLegacy, err := acquireSessionFileLockIn(context.Background(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if _, err := acquireAllSessionOperations(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("all-directory lock error = %v, want deadline exceeded", err)
	}
	releaseLegacy()

	releaseAll, err := acquireAllSessionOperations(context.Background())
	if err != nil {
		t.Fatalf("all-directory lock after legacy release: %v", err)
	}
	releaseAll()
}

func TestSessionWithCookiesCapturesRotationWithoutMutatingOriginal(t *testing.T) {
	originalTime := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	refreshTime := originalTime.Add(time.Hour)
	original := &Session{
		Cookies:     map[string]string{"JSESSIONID": "old", "KEEP": "same"},
		RetrievedAt: originalTime,
	}

	refreshed, changed := sessionWithCookies(original, []*http.Cookie{
		{Name: "JSESSIONID", Value: "rotated"},
		{Name: "KEEP", Value: "same"},
		{Name: "NEW", Value: "issued"},
	}, refreshTime)

	if !changed {
		t.Fatal("rotated response cookies must mark the session changed")
	}
	if got := refreshed.Cookies["JSESSIONID"]; got != "rotated" {
		t.Fatalf("JSESSIONID = %q, want rotated", got)
	}
	if got := refreshed.Cookies["NEW"]; got != "issued" {
		t.Fatalf("NEW = %q, want issued", got)
	}
	if !refreshed.RetrievedAt.Equal(refreshTime) {
		t.Fatalf("RetrievedAt = %v, want %v", refreshed.RetrievedAt, refreshTime)
	}
	if got := original.Cookies["JSESSIONID"]; got != "old" {
		t.Fatalf("original session mutated: JSESSIONID = %q", got)
	}
}

func TestClearSessionRemovesCrashLeftHeadlessProfiles(t *testing.T) {
	isolatedUserConfigDir(t)
	dir, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(dir, "chrome-headless-crash-leftover")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "Cookies"), []byte("credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := clearSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("crash-left headless profile still exists: %v", err)
	}
}

func TestClearSessionRemovesCredentialsFromCurrentAndLegacyDirectories(t *testing.T) {
	configHome := isolatedUserConfigDir(t)
	for _, name := range append([]string{configDirName}, compatibilityConfigDirNames...) {
		dir := filepath.Join(configHome, name)
		for _, profile := range []string{"chrome-profile", "chrome-headless", "chrome-headless-crash"} {
			if err := os.MkdirAll(filepath.Join(dir, profile), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		for _, file := range []string{"datagokr-session.json", keyCacheFile, daemonStateFile} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("credential"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte("public"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := clearSession(); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{configDirName}, compatibilityConfigDirNames...) {
		dir := filepath.Join(configHome, name)
		for _, path := range []string{
			filepath.Join(dir, "datagokr-session.json"),
			filepath.Join(dir, keyCacheFile),
			filepath.Join(dir, daemonStateFile),
			filepath.Join(dir, "chrome-profile"),
			filepath.Join(dir, "chrome-headless"),
			filepath.Join(dir, "chrome-headless-crash"),
		} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("sensitive path still exists after logout cleanup: %s (%v)", path, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "catalog.json")); err != nil {
			t.Fatalf("public catalog should remain in %s: %v", dir, err)
		}
	}
}

func TestSessionWithCookiesDoesNotRewriteUnchangedSession(t *testing.T) {
	originalTime := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	original := &Session{Cookies: map[string]string{"JSESSIONID": "same"}, RetrievedAt: originalTime}

	refreshed, changed := sessionWithCookies(original,
		[]*http.Cookie{{Name: "JSESSIONID", Value: "same"}}, originalTime.Add(time.Hour))

	if changed {
		t.Fatal("identical cookies must not trigger a session-file rewrite")
	}
	if !refreshed.RetrievedAt.Equal(originalTime) {
		t.Fatalf("RetrievedAt changed without a cookie rotation: %v", refreshed.RetrievedAt)
	}
}

func TestGetAuthedRetriesWithCookieRotatedByLoginWall(t *testing.T) {
	original := &Session{Cookies: map[string]string{"JSESSIONID": "old"}}
	rotated := &Session{Cookies: map[string]string{"JSESSIONID": "rotated"}}
	calls := 0

	page, err := getAuthedWith(context.Background(), original, "/account", func(_ context.Context, got *Session, _ string) (*sessionPage, error) {
		calls++
		switch calls {
		case 1:
			if got.Cookies["JSESSIONID"] != "old" {
				t.Fatalf("first request session = %q, want old", got.Cookies["JSESSIONID"])
			}
			return &sessionPage{
				HTML:     `<html><title>로그인</title><form action="/login"></form></html>`,
				Location: BaseURL + "/login",
				Session:  rotated,
				Changed:  true,
			}, nil
		case 2:
			if got.Cookies["JSESSIONID"] != "rotated" {
				t.Fatalf("retry session = %q, want rotated", got.Cookies["JSESSIONID"])
			}
			return &sessionPage{
				HTML:     `<html><title>마이페이지</title></html>`,
				Location: BaseURL + "/account",
				Session:  rotated,
				Changed:  false,
			}, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return nil, nil
		}
	}, 0)
	if err != nil {
		t.Fatalf("getAuthedWith: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if page.Session.Cookies["JSESSIONID"] != "rotated" {
		t.Fatalf("returned session = %q, want rotated", page.Session.Cookies["JSESSIONID"])
	}
	if !page.Changed {
		t.Fatal("first-attempt rotation must remain marked for persistence after a successful retry")
	}
}
