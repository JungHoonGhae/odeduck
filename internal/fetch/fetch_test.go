package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientCapsTLSForDataGoKRHostsOnly(t *testing.T) {
	c := New(WithDelay(0))
	router, ok := c.http.Transport.(*hostTransport)
	if !ok {
		t.Fatalf("transport = %T, want *hostTransport", c.http.Transport)
	}

	portal, ok := router.dataGoKR.(*http.Transport)
	if !ok {
		t.Fatalf("data.go.kr transport = %T, want *http.Transport", router.dataGoKR)
	}
	if portal.TLSClientConfig == nil {
		t.Fatal("data.go.kr transport has no TLS policy")
	}
	if got := portal.TLSClientConfig.MinVersion; got != tls.VersionTLS12 {
		t.Errorf("data.go.kr TLS min = %x, want TLS 1.2", got)
	}
	if got := portal.TLSClientConfig.MaxVersion; got != tls.VersionTLS12 {
		t.Errorf("data.go.kr TLS max = %x, want TLS 1.2", got)
	}

	for _, host := range []string{"data.go.kr", "www.data.go.kr", "auth.data.go.kr", "apis.data.go.kr"} {
		if !isDataGoKRHost(host) {
			t.Errorf("%q should use the data.go.kr TLS transport", host)
		}
	}
	for _, host := range []string{"example.com", "notdata.go.kr", "data.go.kr.example.com"} {
		if isDataGoKRHost(host) {
			t.Errorf("%q must keep the default TLS transport", host)
		}
	}
}

// Get surfaces status/content-type/body and does NOT treat non-200 as an error
// (data.go.kr returns 200 — and sometimes non-200 — with a meaningful body the
// caller must see).
func TestGetSurfacesNon200Body(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<err>nope</err>"))
	}))
	defer srv.Close()

	res, err := New(WithDelay(0)).Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get errored on non-200: %v", err)
	}
	if res.Status != 500 {
		t.Errorf("status = %d, want 500", res.Status)
	}
	if res.ContentType != "application/xml" {
		t.Errorf("contentType = %q", res.ContentType)
	}
	if string(res.Body) != "<err>nope</err>" {
		t.Errorf("body = %q", res.Body)
	}
}

// Get stamps the configured User-Agent.
func TestGetSetsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	New(WithDelay(0), WithUserAgent("test-agent/1.0")).Get(context.Background(), srv.URL)
	if gotUA != "test-agent/1.0" {
		t.Errorf("User-Agent = %q, want test-agent/1.0", gotUA)
	}
}

func TestGetWithHeadersNoRedirectDoesNotForwardAuthorization(t *testing.T) {
	var redirected bool
	destination := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		redirected = true
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("redirected Authorization = %q, want empty", got)
		}
	}))
	defer destination.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, destination.URL, http.StatusFound)
	}))
	defer origin.Close()

	response, err := New(WithDelay(0)).GetWithHeadersNoRedirect(context.Background(), origin.URL, http.Header{
		"Authorization": {"Infuser secret-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusFound {
		t.Fatalf("status = %d, want redirect response", response.Status)
	}
	if redirected {
		t.Fatal("credentialed request followed redirect")
	}
}

// Two Gets on one client are spaced by at least the throttle delay.
func TestThrottleSpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	c := New(WithDelay(200 * time.Millisecond))
	start := time.Now()
	c.Get(context.Background(), srv.URL)
	c.Get(context.Background(), srv.URL)
	elapsed := time.Since(start)
	if elapsed < 180*time.Millisecond {
		t.Errorf("two throttled Gets took %v, want >= ~200ms spacing", elapsed)
	}
}

// GetDoc returns a parseable document on 200 and errors on non-200 (parser
// pages are only useful when served 200 with HTML).
func TestGetDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1 class="t">hi</h1></body></html>`))
	}))
	defer srv.Close()

	c := New(WithDelay(0))
	doc, err := c.GetDoc(context.Background(), srv.URL+"/ok")
	if err != nil {
		t.Fatalf("GetDoc 200: %v", err)
	}
	if got := doc.Find("h1.t").Text(); got != "hi" {
		t.Errorf("parsed text = %q, want hi", got)
	}
	if _, err := c.GetDoc(context.Background(), srv.URL+"/bad"); err == nil {
		t.Error("GetDoc on 404 should error")
	}
}

func TestPostFormDocSendsFormAndParsesHTML(t *testing.T) {
	var gotMethod, gotContentType, gotValue string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotValue = r.Form.Get("oprtinSeqNo")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<div class="operation">두 번째 기능</div>`))
	}))
	defer srv.Close()

	doc, err := New(WithDelay(0)).PostFormDoc(context.Background(), srv.URL, url.Values{
		"oprtinSeqNo": {"29457"},
	})
	if err != nil {
		t.Fatalf("PostFormDoc: %v", err)
	}
	if gotMethod != http.MethodPost || !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("request = %s %q", gotMethod, gotContentType)
	}
	if gotValue != "29457" {
		t.Errorf("oprtinSeqNo = %q", gotValue)
	}
	if got := doc.Find(".operation").Text(); got != "두 번째 기능" {
		t.Errorf("parsed text = %q", got)
	}
}

func TestPostFormReturnsBoundedRawResponse(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form.Get("seq")
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK fixture"))
	}))
	defer srv.Close()

	res, err := New(WithDelay(0)).PostForm(context.Background(), srv.URL, url.Values{"seq": {"51"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "51" || res.Status != http.StatusOK || res.ContentType != "application/zip" || string(res.Body) != "PK fixture" {
		t.Fatalf("post form got=%q response=%+v", got, res)
	}
}

func TestGetRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("123456789"))
	}))
	defer srv.Close()

	_, err := New(WithDelay(0), WithMaxResponseBytes(8)).Get(context.Background(), srv.URL)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("oversized response error = %v, want ErrResponseTooLarge", err)
	}
}

func TestOpenGETUsesSeparateStreamingTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("streamed"))
	}))
	defer srv.Close()

	client := New(WithDelay(0), WithStreamTimeout(5*time.Second))
	client.http.Timeout = time.Nanosecond
	response, err := client.OpenGET(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("OpenGET inherited interactive timeout: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "streamed" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestOpenPostFormStreamsBodyAndForm(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		got = r.Form.Get("seq")
		_, _ = w.Write([]byte("streamed-post"))
	}))
	defer server.Close()

	response, err := New(WithDelay(0)).OpenPostForm(context.Background(), server.URL, url.Values{"seq": {"51"}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "streamed-post" || got != "51" {
		t.Fatalf("form=%q body=%q err=%v", got, body, err)
	}
}
