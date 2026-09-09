package fetch_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func TestPublicFormStreamsExactRequestWithoutSharedCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/export" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("unexpected request: %s %s %s", r.Method, r.URL, r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("public request inherited credentials")
		}
		if r.Header.Get("Referer") != "http://"+r.Host+"/selection" {
			t.Errorf("missing exact source-page referer: %q", r.Header.Get("Referer"))
		}
		if err := r.ParseForm(); err != nil || r.PostForm.Encode() != "age=6&label=a%2Bb&registration=" {
			t.Errorf("form changed: %v %v", r.PostForm, err)
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "public-response", Path: "/"})
		w.Header().Set("Content-Type", "text/csv")
		_, _ = io.WriteString(w, "field\nvalue\n")
	}))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(server.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "private-original"}})
	client := fetch.New(fetch.WithDelay(0), fetch.WithMaxResponseBytes(1), fetch.WithHTTPClient(&http.Client{Jar: jar}))
	res, err := client.OpenPublicPostFormNoRedirect(context.Background(), server.URL+"/export", server.URL+"/selection", url.Values{"registration": {""}, "age": {"6"}, "label": {"a+b"}})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil || res.Status != 200 || res.ContentType != "text/csv" || string(body) != "field\nvalue\n" {
		t.Fatalf("stream response: %+v %q %v", res, body, err)
	}
	if cookies := jar.Cookies(u); len(cookies) != 1 || cookies[0].Value != "private-original" {
		t.Fatal("public response mutated the shared cookie jar")
	}
}

func TestPublicFormRejectsCredentialURLsAndUnboundReferer(t *testing.T) {
	var visits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visits.Add(1)
	}))
	defer server.Close()
	withUser, _ := url.Parse(server.URL)
	withUser.User = url.UserPassword("fixture", "not-a-real-secret")
	for _, pair := range [][2]string{
		{withUser.String(), server.URL},
		{server.URL, withUser.String()},
		{server.URL, "https://other.example/selection"},
		{server.URL, ""},
		{server.URL + "#fragment", server.URL},
	} {
		if _, err := fetch.New(fetch.WithDelay(0)).OpenPublicPostFormNoRedirect(context.Background(), pair[0], pair[1], nil); err == nil {
			t.Fatal("accepted credentials, fragment or unbound referer")
		}
	}
	if visits.Load() != 0 {
		t.Fatal("invalid public request reached HTTP")
	}
}

func TestPublicFormReturnsRedirectWithoutResubmitting(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var destinations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/destination" {
					destinations.Add(1)
					return
				}
				http.Redirect(w, r, "/destination", status)
			}))
			defer server.Close()
			res, err := fetch.New(fetch.WithDelay(0)).OpenPublicPostFormNoRedirect(context.Background(), server.URL, server.URL+"/selection", url.Values{"month": {"2026-07"}})
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.Status != status || destinations.Load() != 0 {
				t.Fatal("redirect followed or response hidden")
			}
		})
	}
}
