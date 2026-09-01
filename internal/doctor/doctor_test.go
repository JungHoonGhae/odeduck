package doctor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/oddsock/internal/apicall"
	"github.com/JungHoonGhae/oddsock/internal/fetch"
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
		case "/data/" + CombinedSwaggerCanaryPK + "/openapi.do":
			http.Error(w, "legacy route removed", http.StatusInternalServerError)
		case "/data/" + CombinedSwaggerCanaryPK + "/fileData.do":
			w.Write([]byte(`<h1 class="h-tit">서울교통공사_월별 승하차인원</h1>
				<script>const options = {url: 'https://infuser.odcloud.kr/oas/docs?namespace=15127058/v1'};</script>`))
		case "/oas/docs":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"swagger":"2.0","host":"api.odcloud.kr","basePath":"/api/15127058/v1","schemes":["https"],"paths":{"/getRows":{"get":{"summary":"월별 승하차 조회","parameters":[{"name":"page","in":"query","required":true}]}}}}`))
		case "/catalog/" + FileCanaryPK + "/fileData.json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"전국지식산업센터현황","creator":{"name":"한국산업단지공단"},"encodingFormat":"CSV"}`))
		case "/data/" + FileCanaryPK + "/fileData.do":
			w.Write([]byte(`<script>fileDetailObj.fn_fileDataDown('15117154','10','FILE_CANARY','1','센터.csv')</script>`))
		case "/tcs/dss/selectFileDataDownload.do":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":true,"atchFileId":"FILE_CANARY","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"전국지식산업센터현황","orginlFileNm":"센터.csv","atchFileExtsn":"csv"}}`))
		case "/catalog/" + SeoulFileCanaryPK + "/fileData.json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"서울시 상권 추정매출","creator":{"name":"서울특별시"},"encodingFormat":"CSV"}`))
		case "/data/" + SeoulFileCanaryPK + "/fileData.do":
			w.Write([]byte(`<ul class="info-ul">
				<li><strong class="key">제공형태</strong><div class="value">기관자체에서 다운로드(제공데이터URL기재)</div></li>
				<li><strong class="key">URL</strong><div class="value"><a href="http://data.seoul.go.kr/dataList/OA-15572/S/1/datasetView.do">바로가기</a></div></li>
			</ul>`))
		case "/sample/json/SearchOpenDataServiceList/1/5/OA-15572/":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"SearchOpenDataServiceList":{"list_total_count":2,"RESULT":{"CODE":"INFO-000"},"row":[
				{"INF_ID":"OA-15572","INF_NM":"서울시 상권분석서비스(추정매출-상권)","MNG_ORGAN_NAME":"서울신용보증재단","SRV_TYPE":"FILE","SHORT_URL":"https://data.seoul.go.kr/dataList/OA-15572/F/1/datasetView.do"},
				{"INF_ID":"OA-15572","INF_NM":"서울시 상권분석서비스(추정매출-상권)","MNG_ORGAN_NAME":"서울신용보증재단","SRV_TYPE":"OPENAPI","SHORT_URL":"https://data.seoul.go.kr/dataList/OA-15572/A/1/datasetView.do"}
			]}}`))
		case "/dataList/OA-15572/F/1/datasetView.do":
			w.Write([]byte(`<form name="frmFile"><input name="infSeq" value="3"></form><table><tbody><tr>
				<td><button title="추정매출.zip" onclick="downloadFile('51')">다운로드</button></td><td></td><td></td><td>13.7</td><td>2026.05.18.</td>
			</tr></tbody></table>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			if canary, ok := adapterCanary(r.URL.Query().Get("publicDataPk")); ok {
				fmt.Fprintf(w, `{"linkUrl":%q,"status":true}`, canary.URL)
				return
			}
			http.NotFound(w, r)
		case "/tcs/dss/selectApiDetailFunction.do":
			// The captured legacy page has an operation selector. Production returns
			// the selected HTML fragment here; the full fixture contains the same
			// operation section and is sufficient for the parser boundary.
			w.Write(openapi)
		default:
			if _, ok := adapterCanary(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/data/"), "/openapi.do")); ok &&
				strings.HasSuffix(r.URL.Path, "/openapi.do") {
				w.Write([]byte(`<html><body>
					<h1 class="h-tit">LINK canary</h1>
					<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
					<button onclick="fn_goUrlLink('canary')">바로가기</button>
				</body></html>`))
				return
			}
			http.NotFound(w, r)
		}
	}))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// fixtureFetch keeps every first-party/official-provider request inside the
// fixture server while preserving the original path and query. Production uses
// the real fixed origins; this is only the live-shape test seam.
func fixtureFetch(t *testing.T, server *httptest.Server) *fetch.Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		copyURL := *request.URL
		copyURL.Scheme = target.Scheme
		copyURL.Host = target.Host
		clone.URL = &copyURL
		clone.Host = target.Host
		return http.DefaultTransport.RoundTrip(clone)
	})
	return fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: transport}))
}

func adapterCanary(pk string) (apicall.ExternalProviderAdapterCanary, bool) {
	for _, adapter := range apicall.ExternalProviderAdapters() {
		for _, canary := range adapter.Canaries {
			if canary.PK == pk {
				return canary, true
			}
		}
	}
	return apicall.ExternalProviderAdapterCanary{}, false
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

	checks := runAt(context.Background(), fixtureFetch(t, srv), srv.URL,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if got := statusOf(checks, "search"); got != StatusOK {
		t.Errorf("search check = %q, want ok", got)
	}
	if got := statusOf(checks, "describe"); got != StatusOK {
		t.Errorf("describe check = %q, want ok; checks=%+v", got, checks)
	}
	for _, name := range []string{"odcloud-swagger", "file-asset", "seoul-file"} {
		if got := statusOf(checks, name); got != StatusOK {
			t.Errorf("%s check = %q, want ok; checks=%+v", name, got, checks)
		}
	}
	if got := statusOf(checks, "link"); got != StatusOK {
		t.Errorf("link check = %q, want ok; checks=%+v", got, checks)
	}
}

func TestAdapterCheckRunsTheStandaloneCanaryPath(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()

	check := AdapterCheck(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL)
	if check.Status != StatusOK || !strings.Contains(check.Detail, "canary 11개") {
		t.Fatalf("adapter check = %+v", check)
	}
}

func TestInvocationContractValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		contract *apicall.ExternalContract
		want     bool
	}{
		{"nil", nil, false},
		{"unknown state", &apicall.ExternalContract{InvocationState: "future", Auth: &apicall.ExternalAuthContract{CredentialScope: "https://example.com/"}}, false},
		{"implemented supported provider", &apicall.ExternalContract{AdapterID: "safetykorea", InvocationState: apicall.InvocationImplemented, Operations: []apicall.ExternalOperation{{Name: "x"}}, Auth: &apicall.ExternalAuthContract{CredentialScope: "https://www.safetykorea.kr/openapi/api/"}}, true},
		{"implemented without stored provider", &apicall.ExternalContract{AdapterID: "future", InvocationState: apicall.InvocationImplemented, Operations: []apicall.ExternalOperation{{Name: "x"}}, Auth: &apicall.ExternalAuthContract{CredentialScope: "https://example.com/"}}, false},
		{"implemented without operations", &apicall.ExternalContract{AdapterID: "safetykorea", InvocationState: apicall.InvocationImplemented, Auth: &apicall.ExternalAuthContract{CredentialScope: "https://www.safetykorea.kr/openapi/api/"}}, false},
		{"implemented over HTTP", &apicall.ExternalContract{AdapterID: "safetykorea", InvocationState: apicall.InvocationImplemented, Operations: []apicall.ExternalOperation{{Name: "x"}}, Auth: &apicall.ExternalAuthContract{CredentialScope: "http://example.com/"}}, false},
		{"blocked HTTPS is inconsistent", &apicall.ExternalContract{InvocationState: apicall.InvocationBlockedInsecureTransport, Auth: &apicall.ExternalAuthContract{CredentialScope: "https://example.com/"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validInvocationContract(tt.contract); got != tt.want {
				t.Fatalf("validInvocationContract = %v, want %v", got, tt.want)
			}
		})
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
	if check.Status != StatusDrift || !strings.Contains(check.Detail, "safetykorea") {
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
	for _, name := range []string{"odcloud-swagger", "file-asset", "seoul-file"} {
		if got := statusOf(checks, name); got != StatusDrift {
			t.Errorf("%s check = %q, want drift", name, got)
		}
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
