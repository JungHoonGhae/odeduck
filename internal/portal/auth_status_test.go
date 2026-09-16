package portal

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginReusesVerifiedSession(t *testing.T) {
	isolatedUserConfigDir(t)
	if err := saveSession(&Session{Cookies: map[string]string{"JSESSIONID": "existing"}}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<title>활용신청 현황</title><div class="mypage-dataset-list"></div>`))
	}))
	defer srv.Close()
	before := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = before })
	var progress bytes.Buffer
	if err := Login(context.Background(), &progress, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(progress.String(), "기존 로그인 세션이 유효") || strings.Contains(progress.String(), "브라우저를 띄웁니다") {
		t.Fatalf("valid session was not reused: %q", progress.String())
	}
	if _, err := loadState(); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("unexpected browser state after reuse: %v", err)
	}
}

func TestCheckSessionMissing(t *testing.T) {
	isolatedUserConfigDir(t)
	if err := CheckSession(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("missing session: %v", err)
	}
}

func TestCheckSessionVerifiesAndPersistsRotation(t *testing.T) {
	isolatedUserConfigDir(t)
	if err := saveSession(&Session{Cookies: map[string]string{"JSESSIONID": "before"}}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("JSESSIONID")
		if err != nil || cookie.Value != "before" || r.URL.Path != AccountListPath {
			t.Errorf("unexpected session probe: cookie=%v path=%s", cookie, r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "after", Path: "/"})
		w.Write([]byte(`<title>활용신청 현황</title><div class="mypage-dataset-list"></div>`))
	}))
	defer srv.Close()
	before := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = before })
	if err := CheckSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	session, err := loadSession()
	if err != nil || session.Cookies["JSESSIONID"] != "after" {
		t.Fatalf("rotated session was not saved: %v", err)
	}
}

func TestCheckSessionDistinguishesExpiryFromProbeFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantLogin bool
	}{
		{"expired", http.StatusOK, `<title>통합 로그인</title>`, true},
		{"unavailable", http.StatusServiceUnavailable, "temporarily unavailable", false},
		{"changed page", http.StatusOK, `<title>점검 중</title>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatedUserConfigDir(t)
			if err := saveSession(&Session{Cookies: map[string]string{"JSESSIONID": "existing"}}); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			before := BaseURL
			BaseURL = srv.URL
			t.Cleanup(func() { BaseURL = before })
			err := CheckSession(context.Background())
			if err == nil || errors.Is(err, ErrNotLoggedIn) != tc.wantLogin {
				t.Fatalf("error=%v want login required=%v", err, tc.wantLogin)
			}
		})
	}
}

func TestApplicationsPreservesNonAuthenticationFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unavailable", http.StatusServiceUnavailable, "temporarily unavailable"},
		{"changed page", http.StatusOK, `<title>점검 중</title>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatedUserConfigDir(t)
			if err := saveSession(&Session{Cookies: map[string]string{"JSESSIONID": "existing"}}); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			before := BaseURL
			BaseURL = srv.URL
			t.Cleanup(func() { BaseURL = before })
			apps, err := Applications(context.Background())
			if err == nil || errors.Is(err, ErrNotLoggedIn) || len(apps) != 0 {
				t.Fatalf("probe failure misclassified: apps=%v err=%v", apps, err)
			}
		})
	}
}
