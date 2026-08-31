package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/opendatactl/internal/fetch"
)

// fixtureServer serves the real captured search + openapi.do markup (reused from
// the sibling packages' testdata) so the drift checks pass against ground truth.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	search, err := os.ReadFile("../portal/testdata/search-list.html")
	if err != nil {
		t.Fatalf("read search fixture: %v", err)
	}
	openapi, err := os.ReadFile("../apicall/testdata/op-15000908.html")
	if err != nil {
		t.Fatalf("read openapi fixture: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		switch r.URL.Path {
		case "/tcs/dss/selectDataSetList.do":
			w.Write(search)
		case "/data/" + CanaryPK + "/openapi.do":
			w.Write(openapi)
		case "/data/" + LinkCanaryPK + "/openapi.do":
			w.Write([]byte(`<html><body>
				<h1 class="h-tit">제품 안전인증 및 리콜 정보</h1>
				<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
				<button onclick="fn_goUrlLink('15116894')">바로가기</button>
			</body></html>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		case "/tcs/dss/selectApiDetailFunction.do":
			// The captured legacy page has an operation selector. Production returns
			// the selected HTML fragment here; the full fixture contains the same
			// operation section and is sufficient for the parser boundary.
			w.Write(openapi)
		default:
			http.NotFound(w, r)
		}
	}))
}

func statusOf(checks []Check, name string) Status {
	for _, c := range checks {
		if c.Name == name {
			return c.Status
		}
	}
	return ""
}

// Against live-shaped markup, both scraping checks report ok.
func TestRunHealthy(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	checks := runAt(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if got := statusOf(checks, "search"); got != StatusOK {
		t.Errorf("search check = %q, want ok", got)
	}
	if got := statusOf(checks, "describe"); got != StatusOK {
		t.Errorf("describe check = %q, want ok; checks=%+v", got, checks)
	}
	if got := statusOf(checks, "link"); got != StatusOK {
		t.Errorf("link check = %q, want ok; checks=%+v", got, checks)
	}
}

func TestLinkCheckDetectsStaleProviderContract(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	check := linkCheckAt(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL,
		time.Date(2027, 3, 2, 0, 0, 0, 0, time.UTC))
	if check.Status != StatusDrift || !strings.Contains(check.Detail, "180일") {
		t.Fatalf("link check = %+v, want stale contract drift", check)
	}
}

func TestLinkCheckRejectsFutureProviderVerification(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	check := linkCheckAt(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if check.Status != StatusDrift || !strings.Contains(check.Detail, "180일") {
		t.Fatalf("link check = %+v, want future verification drift", check)
	}
}

func TestRunDetectsLinkResolverDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		if r.URL.Path == "/data/"+LinkCanaryPK+"/openapi.do" {
			w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
			return
		}
		if r.URL.Path == "/tcs/dss/selectApiLinkUrl.do" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if got := linkCheck(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL); got.Status != StatusDrift {
		t.Errorf("link check = %+v, want drift", got)
	}
}

func TestLinkCheckDetectsProviderContractDowngrade(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/"+LinkCanaryPK+"/openapi.do" {
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
			return
		}
		if r.URL.Path == "/tcs/dss/selectApiLinkUrl.do" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://provider.example/changed","status":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	check := linkCheck(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL)
	if check.Status != StatusDrift || !strings.Contains(check.Detail, "SafetyKorea") {
		t.Fatalf("link check = %+v, want provider-contract drift", check)
	}
}

// When the portal returns markup our parsers no longer understand, the checks
// report drift loudly instead of silently passing.
func TestRunDetectsDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body>redesigned, nothing our selectors match</body></html>`))
	}))
	defer srv.Close()

	checks := runAt(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if got := statusOf(checks, "search"); got != StatusDrift {
		t.Errorf("search check = %q, want drift", got)
	}
	if got := statusOf(checks, "describe"); got != StatusDrift {
		t.Errorf("describe check = %q, want drift", got)
	}
}

func TestRunDetectsPartialDescribeDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/tcs/dss/selectDataSetList.do" {
			w.Write([]byte(`<div class="apply-result-item"><a href="/data/1/openapi.do"><strong>테스트</strong></a></div>`))
			return
		}
		// Endpoint alone used to make doctor green even when the title, API type,
		// approval, and request-variable parsers had all drifted.
		w.Write([]byte(`<html><body>https://apis.data.go.kr/test/getRows</body></html>`))
	}))
	defer srv.Close()

	checks := runAt(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if got := statusOf(checks, "describe"); got != StatusDrift {
		t.Errorf("partial describe check = %q, want drift", got)
	}
}
