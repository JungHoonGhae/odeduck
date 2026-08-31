package portal

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// fakeCDP serves the two endpoints the usability check reads, with a chosen
// /json/list payload, and returns its port.
func fakeCDP(t *testing.T, list string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json/version":
			w.Write([]byte(`{"Browser":"Chrome/150"}`))
		case "/json/list":
			w.Write([]byte(list))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// The bug this guards: on macOS a Chrome whose last window was closed keeps its
// debug port listening and answers /json/version exactly like a healthy browser,
// while every attempt to open a tab fails with -32000. login attached to one of
// these and died there, with nothing in the error pointing at the real state.
func TestBrowserUsableRejectsWindowlessBrowser(t *testing.T) {
	port := fakeCDP(t, `[]`)
	if !wsAlive(port) {
		t.Fatal("the port answers, so liveness must still report true — that distinction is the point")
	}
	if browserUsable(port) {
		t.Error("a browser with zero targets cannot open a tab and must not be reused")
	}
}

// Only a page target can host our work; a leftover service worker cannot.
func TestBrowserUsableRequiresAPageTarget(t *testing.T) {
	if browserUsable(fakeCDP(t, `[{"type":"service_worker","url":"x"}]`)) {
		t.Error("service worker only → not usable")
	}
	if !browserUsable(fakeCDP(t, `[{"type":"page","url":"https://www.data.go.kr/"}]`)) {
		t.Error("a page target → usable")
	}
}

func TestBrowserUsableFalseWhenNothingListening(t *testing.T) {
	port := fakeCDP(t, `[{"type":"page"}]`)
	srvGone := port + 20000 // nothing is listening here
	if browserUsable(srvGone) {
		t.Error("no browser at all → not usable")
	}
}
