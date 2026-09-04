package apicall

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

type fakeCredentialSource struct {
	provider, scope string
	key, domain     string
	dataGoKey       string
	dataGoErr       error
	externalErr     error
	dataGoReads     int
	externalReads   int
	invalidations   int
}

func (f *fakeCredentialSource) DataGoKR(context.Context) (string, error) {
	f.dataGoReads++
	return f.dataGoKey, f.dataGoErr
}
func (f *fakeCredentialSource) InvalidateDataGoKR() { f.invalidations++ }
func (f *fakeCredentialSource) External(_ context.Context, provider, scope string) (string, string, error) {
	f.externalReads++
	f.provider, f.scope = provider, scope
	return f.key, f.domain, f.externalErr
}

func TestDatasetCallerDispatchesKnownLinkThroughTypedProvider(t *testing.T) {
	const pk = "15116894"
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<html><body><ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul></body></html>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer portalServer.Close()

	providerTransport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/openapi/api/cert/certificationDetail.json" || req.Header.Get("AuthKey") != "SAFETY-SECRET-123" {
			t.Fatalf("provider request = %s headers=%v", req.URL, req.Header)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"resultCode":"2000","resultData":{"certNum":"SU123"}}`)), Request: req}, nil
	})
	credentials := &fakeCredentialSource{key: "SAFETY-SECRET-123"}
	caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials,
		newExternalCaller(&http.Client{Transport: providerTransport}, nil))

	result, err := caller.Call(context.Background(), DatasetCallRequest{
		PK: pk, Operation: "certificationDetail", Params: map[string]string{"certNum": "SU123"},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Status != 200 || credentials.provider != "safetykorea" || credentials.scope != "https://www.safetykorea.kr/openapi/api/" {
		t.Fatalf("result=%#v credential=%s/%s", result, credentials.provider, credentials.scope)
	}
}

func TestDatasetCallerKeepsUnsupportedAndInsecureLinksFailClosed(t *testing.T) {
	for _, target := range []string{
		"https://provider.example/uninspected",
		"https://data.seoul.go.kr/dataList/OA-15799/A/1/datasetView.do",
	} {
		t.Run(target, func(t *testing.T) {
			portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "selectApiLinkUrl") {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"linkUrl":"` + target + `","status":true}`))
					return
				}
				_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
			}))
			defer portalServer.Close()
			credentials := &fakeCredentialSource{key: "MUST-NOT-BE-READ"}
			caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials,
				newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatal("blocked LINK reached provider transport")
					return nil, nil
				})}, nil))
			_, err := caller.Call(context.Background(), DatasetCallRequest{PK: "1", Operation: "anything"})
			if err == nil || credentials.externalReads != 0 {
				t.Fatalf("error=%v credential reads=%d", err, credentials.externalReads)
			}
		})
	}
}

func TestDatasetCallerRESTPreflightFailsBeforeCredentialOrNetwork(t *testing.T) {
	body, err := os.ReadFile("testdata/openapi-swagger.html")
	if err != nil {
		t.Fatal(err)
	}
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write(body)
	}))
	defer portalServer.Close()

	credentials := &fakeCredentialSource{dataGoKey: "MUST-NOT-BE-READ"}
	caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials, NewExternalCaller())
	_, err = caller.Call(context.Background(), DatasetCallRequest{
		PK: "15127057", Params: map[string]string{"pageNo": "1"},
	})
	if err == nil || !strings.Contains(err.Error(), "numOfRows") {
		t.Fatalf("missing-param error = %v", err)
	}
	if credentials.dataGoReads != 0 || credentials.externalReads != 0 {
		t.Fatalf("preflight read credentials: data.go=%d external=%d", credentials.dataGoReads, credentials.externalReads)
	}
}

func TestDatasetCallerBlocksBulkOnlyContractBeforeCredentialRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15000020"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Source: catalog.SourceOfficial,
		Entries: []catalog.Entry{{PK: pk, Title: "불완전 계약", SvcType: catalog.SvcREST,
			OfficialAPI: &catalog.OfficialAPIContract{APIType: catalog.SvcREST,
				Operations: []catalog.OfficialAPIOperation{{Name: "list", URL: "https://apis.data.go.kr/test/list", RequestNames: []string{"pageNo"}}}}}},
	}).Save(); err != nil {
		t.Fatal(err)
	}
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fallback unavailable", http.StatusBadGateway)
	}))
	defer portalServer.Close()
	credentials := &fakeCredentialSource{dataGoKey: "MUST-NOT-BE-READ"}
	caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials, NewExternalCaller())
	_, err := caller.Call(context.Background(), DatasetCallRequest{PK: pk, Params: map[string]string{"pageNo": "1"}})
	if err == nil || !strings.Contains(err.Error(), "상세 호출 계약") {
		t.Fatalf("error = %v", err)
	}
	if credentials.dataGoReads != 0 {
		t.Fatalf("credential reads = %d", credentials.dataGoReads)
	}
}

func TestDatasetCallerDoesNotTreatEmptyHTMLAsDetailedInvocationEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15000021"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Source: catalog.SourceOfficial,
		Entries: []catalog.Entry{{PK: pk, Title: "빈 상세 계약", SvcType: catalog.SvcREST,
			OfficialAPI: &catalog.OfficialAPIContract{APIType: catalog.SvcREST,
				Operations: []catalog.OfficialAPIOperation{{Name: "list", URL: "https://apis.data.go.kr/test/list", RequestNames: []string{"pageNo"}}}}}},
	}).Save(); err != nil {
		t.Fatal(err)
	}
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>상세 화면</h1></body></html>`))
	}))
	defer portalServer.Close()

	credentials := &fakeCredentialSource{dataGoKey: "MUST-NOT-BE-READ"}
	caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials, NewExternalCaller())
	caller.rest = func(context.Context, *fetch.Client, string, map[string]string, string, time.Duration, func(time.Duration, time.Duration)) (*CallResult, error) {
		return nil, errors.New("REST invocation must remain blocked")
	}
	_, err := caller.Call(context.Background(), DatasetCallRequest{PK: pk, Params: map[string]string{"pageNo": "1"}})
	if err == nil || !strings.Contains(err.Error(), "상세 호출 계약") {
		t.Fatalf("error = %v, want missing detailed invocation contract", err)
	}
	if credentials.dataGoReads != 0 {
		t.Fatalf("credential reads = %d, want 0", credentials.dataGoReads)
	}
}

func TestDatasetCallerScopesSwaggerEvidenceToItsOperation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15000022"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Source: catalog.SourceOfficial,
		Entries: []catalog.Entry{{PK: pk, Title: "혼합 Swagger 계약", SvcType: catalog.SvcREST,
			OfficialAPI: &catalog.OfficialAPIContract{APIType: catalog.SvcREST,
				Operations: []catalog.OfficialAPIOperation{{Name: "Bulk B", URL: "https://apis.data.go.kr/test/bulkB", RequestNames: []string{"pageNo"}}}}}},
	}).Save(); err != nil {
		t.Fatal(err)
	}
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
			<ul><li><strong class="key">API 유형</strong><div class="value">REST</div></li></ul>
			<script>var swaggerJson = ` + "`" + `{"swagger":"2.0","host":"apis.data.go.kr","basePath":"/test","schemes":["https"],"paths":{"/swaggerA":{"get":{"summary":"Swagger A"}}}}` + "`" + `;</script>
		</body></html>`))
	}))
	defer portalServer.Close()

	credentials := &fakeCredentialSource{dataGoKey: "TEST-KEY"}
	caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials, NewExternalCaller())
	var invoked []string
	caller.rest = func(_ context.Context, _ *fetch.Client, endpoint string, _ map[string]string, _ string, _ time.Duration, _ func(time.Duration, time.Duration)) (*CallResult, error) {
		invoked = append(invoked, endpoint)
		return &CallResult{Status: http.StatusOK, Body: map[string]any{"ok": true}}, nil
	}
	if _, err := caller.Call(context.Background(), DatasetCallRequest{PK: pk, Operation: "swaggerA"}); err != nil {
		t.Fatalf("parameterless Swagger operation: %v", err)
	}
	if credentials.dataGoReads != 1 || len(invoked) != 1 || !strings.HasSuffix(invoked[0], "/swaggerA") {
		t.Fatalf("Swagger dispatch reads=%d invoked=%v", credentials.dataGoReads, invoked)
	}

	_, err := caller.Call(context.Background(), DatasetCallRequest{PK: pk, Operation: "bulkB", Params: map[string]string{"pageNo": "1"}})
	if err == nil || !strings.Contains(err.Error(), "상세 호출 계약") {
		t.Fatalf("unmatched bulk operation error = %v", err)
	}
	if credentials.dataGoReads != 1 || len(invoked) != 1 {
		t.Fatalf("bulk operation crossed credential boundary: reads=%d invoked=%v", credentials.dataGoReads, invoked)
	}
}

type rotatingCredentialSource struct {
	keys          []string
	errs          []error
	reads         int
	invalidations int
}

func (s *rotatingCredentialSource) DataGoKR(context.Context) (string, error) {
	index := s.reads
	s.reads++
	if index < len(s.errs) && s.errs[index] != nil {
		return "", s.errs[index]
	}
	if len(s.keys) == 0 {
		return "", nil
	}
	if index >= len(s.keys) {
		index = len(s.keys) - 1
	}
	return s.keys[index], nil
}
func (s *rotatingCredentialSource) InvalidateDataGoKR() { s.invalidations++ }
func (*rotatingCredentialSource) External(context.Context, string, string) (string, string, error) {
	return "", "", errors.New("unexpected external credential read")
}

func TestDatasetCallerRESTRetriesOnceOnlyWhenRejectedKeyActuallyRefreshes(t *testing.T) {
	body, err := os.ReadFile("testdata/openapi-swagger.html")
	if err != nil {
		t.Fatal(err)
	}
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write(body)
	}))
	defer portalServer.Close()

	tests := []struct {
		name      string
		keys      []string
		errs      []error
		wantCalls []string
		wantErr   bool
	}{
		{name: "new key succeeds", keys: []string{"OLD-KEY", "NEW-KEY"}, wantCalls: []string{"OLD-KEY", "NEW-KEY"}},
		{name: "same key is not retried", keys: []string{"OLD-KEY", "OLD-KEY"}, wantCalls: []string{"OLD-KEY"}, wantErr: true},
		{name: "refresh failure is not retried", keys: []string{"OLD-KEY"}, errs: []error{nil, errors.New("session expired")}, wantCalls: []string{"OLD-KEY"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := &rotatingCredentialSource{keys: tt.keys, errs: tt.errs}
			caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials, NewExternalCaller())
			var calledKeys []string
			caller.rest = func(_ context.Context, _ *fetch.Client, endpoint string, params map[string]string, key string, wait time.Duration, onWait func(time.Duration, time.Duration)) (*CallResult, error) {
				if !strings.HasSuffix(endpoint, "/getClslVioltDetailInfo_2") || params["pageNo"] != "1" || params["numOfRows"] != "10" {
					t.Fatalf("REST dispatch endpoint=%q params=%#v", endpoint, params)
				}
				if wait != 3*time.Second || onWait == nil {
					t.Fatalf("wait dispatch = %s callback nil=%v", wait, onWait == nil)
				}
				calledKeys = append(calledKeys, key)
				if key == "OLD-KEY" {
					return &CallResult{Status: http.StatusOK, Body: map[string]any{"error": "bad key"}}, ErrKeyRejected
				}
				return &CallResult{Status: http.StatusOK, Body: map[string]any{"ok": true}}, nil
			}
			result, callErr := caller.Call(context.Background(), DatasetCallRequest{
				PK: "15127057", Params: map[string]string{"pageNo": "1", "numOfRows": "10"}, Wait: 3 * time.Second,
				OnWait: func(time.Duration, time.Duration) {},
			})
			if (callErr != nil) != tt.wantErr {
				t.Fatalf("Call error = %v, wantErr=%v result=%#v", callErr, tt.wantErr, result)
			}
			if strings.Join(calledKeys, ",") != strings.Join(tt.wantCalls, ",") {
				t.Fatalf("called keys = %v, want %v", calledKeys, tt.wantCalls)
			}
			if credentials.invalidations != 1 || credentials.reads != 2 {
				t.Fatalf("credential lifecycle reads=%d invalidations=%d", credentials.reads, credentials.invalidations)
			}
		})
	}
}

func TestDatasetCallerSurfacesCredentialFailuresWithoutProviderRequest(t *testing.T) {
	const pk = "15116894"
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer portalServer.Close()

	credentials := &fakeCredentialSource{externalErr: errors.New("not configured")}
	caller := newDatasetCaller(fetch.New(fetch.WithDelay(0)), portalServer.URL, credentials,
		newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("credential failure reached provider transport")
			return nil, nil
		})}, nil))
	_, err := caller.Call(context.Background(), DatasetCallRequest{
		PK: pk, Operation: "certificationDetail", Params: map[string]string{"certNum": "SU123"},
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") || !strings.Contains(err.Error(), "신청:") {
		t.Fatalf("credential error = %v", err)
	}
	if credentials.externalReads != 1 || credentials.scope != "https://www.safetykorea.kr/openapi/api/" {
		t.Fatalf("credential lookup = %d %s/%s", credentials.externalReads, credentials.provider, credentials.scope)
	}
}

func TestDatasetCallerRejectsInvalidInitializationAndRequest(t *testing.T) {
	credentials := &fakeCredentialSource{}
	for name, tc := range map[string]struct {
		caller  *DatasetCaller
		request DatasetCallRequest
	}{
		"nil caller":        {nil, DatasetCallRequest{PK: "1"}},
		"missing transport": {&DatasetCaller{credentials: credentials}, DatasetCallRequest{PK: "1"}},
		"missing pk":        {newDatasetCaller(fetch.New(fetch.WithDelay(0)), "https://data.go.kr", credentials, NewExternalCaller()), DatasetCallRequest{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.caller.Call(context.Background(), tc.request); err == nil {
				t.Fatal("invalid call succeeded")
			}
		})
	}
}

func TestResolveExternalOperationUsesAdvertisedContract(t *testing.T) {
	one := &ExternalContract{Operations: []ExternalOperation{{Name: "list"}}}
	if got, err := resolveExternalOperation(one, ""); err != nil || got != "list" {
		t.Fatalf("single operation = %q, %v", got, err)
	}
	if got, err := resolveExternalOperation(one, "list"); err != nil || got != "list" {
		t.Fatalf("explicit operation = %q, %v", got, err)
	}
	if _, err := resolveExternalOperation(one, "invented"); err == nil {
		t.Fatal("unknown operation succeeded")
	}
	many := &ExternalContract{Operations: []ExternalOperation{{Name: "one"}, {Name: "two"}}}
	if _, err := resolveExternalOperation(many, ""); err == nil || !strings.Contains(err.Error(), "one, two") {
		t.Fatalf("ambiguous operation error = %v", err)
	}
}
