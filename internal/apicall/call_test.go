package apicall

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JungHoonGhae/oddsock/internal/fetch"
)

func TestCallXMLToJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// serviceKey must arrive verbatim (not double-encoded).
		if got := r.URL.Query().Get("serviceKey"); got != "abc+def==" {
			t.Errorf("serviceKey = %q, want verbatim abc+def==", got)
		}
		if got := r.URL.Query().Get("numOfRows"); got != "10" {
			t.Errorf("numOfRows = %q", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<response><header><resultCode>00</resultCode></header>` +
			`<body><items><item><name>서울</name><count>5</count></item></items></body></response>`))
	}))
	defer srv.Close()

	res, err := callTrusted(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL+"/svc/op", map[string]string{"numOfRows": "10"}, "abc+def==")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	m, ok := res.Body.(map[string]any)
	if !ok {
		t.Fatalf("body not a map: %T", res.Body)
	}
	if _, ok := m["header"]; !ok {
		t.Errorf("converted XML missing header: %v", m)
	}
}

func TestCallDoesNotLeakKeyOnTransportError(t *testing.T) {
	key := "SECRET+KEY=="
	escaped := strings.ReplaceAll(key, "+", "%2B")

	_, err := callTrusted(context.Background(), fetch.New(fetch.WithDelay(0)), "http://127.0.0.1:1/x", nil, key)
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("error leaks raw key: %v", err)
	}
	if strings.Contains(err.Error(), escaped) {
		t.Fatalf("error leaks escaped key: %v", err)
	}
}

func TestCallServiceKeyHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<OpenAPI_ServiceResponse><cmmMsgHeader>` +
			`<returnReasonCode>30</returnReasonCode>` +
			`<returnAuthMsg>SERVICE_KEY_IS_NOT_REGISTERED_ERROR</returnAuthMsg>` +
			`</cmmMsgHeader></OpenAPI_ServiceResponse>`))
	}))
	defer srv.Close()

	res, err := callTrusted(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, nil, "wrongkey")
	if res == nil {
		t.Fatal("CallResult must still be returned (surface the body)")
	}
	if err == nil || !strings.Contains(err.Error(), "Encoding") {
		t.Fatalf("expected Encoding/Decoding key hint in error, got %v", err)
	}
}

func TestSecureEndpointAllowsOnlyOfficialHTTPSGateway(t *testing.T) {
	got, err := SecureEndpoint("http://apis.data.go.kr/123/service/op?numOfRows=10")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://apis.data.go.kr/123/service/op?numOfRows=10" {
		t.Fatalf("secure endpoint = %q", got)
	}
	odcloud, err := SecureEndpoint("http://api.odcloud.kr/api/15044249/v1/uddi:test?page=1")
	if err != nil {
		t.Fatalf("official ODCloud gateway rejected: %v", err)
	}
	if odcloud != "https://api.odcloud.kr/api/15044249/v1/uddi:test?page=1" {
		t.Fatalf("secure ODCloud endpoint = %q", odcloud)
	}

	for _, endpoint := range []string{
		"https://evil.example/steal",
		"http://apis.data.go.kr.evil.example/steal",
		"https://api.odcloud.kr.evil.example/steal",
		"https://user@apis.data.go.kr/steal",
		"https://apis.data.go.kr:8443/steal",
		"https://apis.data.go.kr/steal?serviceKey=already-there",
	} {
		if _, err := SecureEndpoint(endpoint); err == nil {
			t.Errorf("SecureEndpoint(%q) unexpectedly allowed", endpoint)
		}
	}
}

func TestCallRejectsExternalEndpointBeforeSendingKey(t *testing.T) {
	_, err := Call(context.Background(), fetch.New(fetch.WithDelay(0)), "https://evil.example/steal", nil, "SECRET")
	if err == nil || !strings.Contains(err.Error(), "공식 게이트웨이") {
		t.Fatalf("Call external endpoint error = %v", err)
	}
}

func TestCallSurfacesNonSuccessHTTPStatusWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"publisher unavailable"}`))
	}))
	defer srv.Close()

	res, err := callTrusted(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL+"/op", nil, "key")
	if !errors.Is(err, ErrHTTPStatus) {
		t.Fatalf("error = %v, want ErrHTTPStatus", err)
	}
	if res == nil || res.Status != http.StatusInternalServerError {
		t.Fatalf("result = %+v, want surfaced 500 body", res)
	}
}

func TestDecodeBodyPreservesLargeJSONInteger(t *testing.T) {
	body := decodeBody("application/json", []byte(`{"items":[{"parcelId":9007199254740993}]}`))
	profile, err := ProfileBody(body, []string{"parcelId"})
	if err != nil {
		t.Fatal(err)
	}
	got := profile.Fields[0]
	if len(got.Values) != 1 || got.Values[0] != "9007199254740993" {
		t.Fatalf("large identifier lost precision: %+v", got)
	}
}
