package apicall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

type externalRoundTripFunc func(*http.Request) (*http.Response, error)

func (f externalRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestExternalCallerInvokesTypedSafetyKoreaOperation(t *testing.T) {
	contract := &ExternalContract{
		AdapterID: "safetykorea",
		Auth: &ExternalAuthContract{
			Placement:       "header",
			Name:            "AuthKey",
			CredentialScope: "https://www.safetykorea.kr/openapi/api/",
		},
	}
	transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "www.safetykorea.kr" ||
			req.URL.Path != "/openapi/api/cert/certificationList.json" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		}
		if got := req.Header.Get("AuthKey"); got != "SAFETY-SECRET-123" {
			t.Fatalf("AuthKey = %q", got)
		}
		if strings.Contains(req.URL.String(), "SAFETY-SECRET-123") {
			t.Fatal("header credential leaked into URL")
		}
		if req.URL.Query().Get("conditionKey") != "productName" || req.URL.Query().Get("conditionValue") != "완구" {
			t.Fatalf("query = %s", req.URL.RawQuery)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"resultCode":"2000","resultMsg":"Success","resultData":[]}`)),
			Request:    req,
		}, nil
	})
	caller := newExternalCaller(&http.Client{Transport: transport}, nil)
	result, err := caller.Call(context.Background(), contract, "certificationList", map[string]string{
		"conditionKey": "productName", "conditionValue": "완구",
	}, ExternalCredential{Key: "SAFETY-SECRET-123"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("result = %#v", result)
	}
	body, ok := result.Body.(map[string]any)
	if !ok || body["resultCode"] != "2000" {
		t.Fatalf("body = %#v", result.Body)
	}
}

func TestExternalCallerRedactsSecretsAndRejectsProviderFailures(t *testing.T) {
	contract := &ExternalContract{AdapterID: "safetykorea", Auth: &ExternalAuthContract{
		Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/",
	}}
	const secret = "SECRET+VALUE/123"
	params := map[string]string{"certNum": "SU123"}

	t.Run("transport error", func(t *testing.T) {
		transport := externalRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("dial https://provider.example/%s?key=%s", url.PathEscape(secret), url.QueryEscape(secret))
		})
		_, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(
			context.Background(), contract, "certificationDetail", params, ExternalCredential{Key: secret})
		if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), url.PathEscape(secret)) || strings.Contains(err.Error(), url.QueryEscape(secret)) {
			t.Fatalf("transport error leaked secret: %v", err)
		}
	})

	t.Run("provider body", func(t *testing.T) {
		transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"resultCode":"4000","resultMsg":"bad ` + secret + `"}`)), Request: req}, nil
		})
		result, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(
			context.Background(), contract, "certificationDetail", params, ExternalCredential{Key: secret})
		if err == nil || result != nil || strings.Contains(err.Error(), secret) {
			t.Fatal("credential-bearing provider failure must be withheld, not returned with replacement values")
		}
	})

	t.Run("redirect", func(t *testing.T) {
		transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://other.example/"}}, Body: io.NopCloser(strings.NewReader("redirect")), Request: req}, nil
		})
		_, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(
			context.Background(), contract, "certificationDetail", params, ExternalCredential{Key: secret})
		if !errors.Is(err, ErrExternalRedirect) {
			t.Fatalf("error = %v, want ErrExternalRedirect", err)
		}
	})
}

func TestExternalCallerRejectsOversizedStructuredAndBinaryResponses(t *testing.T) {
	contract := &ExternalContract{AdapterID: "safetykorea", Auth: &ExternalAuthContract{
		Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/",
	}}
	for _, contentType := range []string{"application/json", "image/png"} {
		t.Run(contentType, func(t *testing.T) {
			transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{contentType}},
					Body:       io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, int(fetch.DefaultMaxResponseBytes)+1))),
					Request:    req,
				}, nil
			})
			_, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(
				context.Background(), contract, "certificationDetail", map[string]string{"certNum": "SU123"},
				ExternalCredential{Key: "SAFETY-SECRET-123"})
			if !errors.Is(err, fetch.ErrResponseTooLarge) {
				t.Fatalf("error = %v, want ErrResponseTooLarge", err)
			}
		})
	}
}

func TestExternalCallerSupportsAllDocumentedSafetyKoreaOperations(t *testing.T) {
	contract := &ExternalContract{
		AdapterID: "safetykorea",
		Auth:      &ExternalAuthContract{Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/"},
	}
	tests := []struct {
		operation string
		path      string
		params    map[string]string
	}{
		{"certificationList", "/openapi/api/cert/certificationList.json", map[string]string{"conditionKey": "certNum", "conditionValue": "SU123"}},
		{"certificationDetail", "/openapi/api/cert/certificationDetail.json", map[string]string{"certNum": "SU123"}},
		{"recallList", "/openapi/api/recall/recallList.json", map[string]string{"conditionKey": "publishDate", "conditionValue": "20260101"}},
		{"recallDetail", "/openapi/api/recall/recallDetail.json", map[string]string{"recallUid": "3802"}},
		{"fRecallList", "/openapi/api/recall/fRecallList.json", map[string]string{"conditionKey": "recallBrandName", "conditionValue": "brand"}},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", req.URL.Path, tt.path)
				}
				for name, want := range tt.params {
					if got := req.URL.Query().Get(name); got != want {
						t.Errorf("%s = %q, want %q", name, got, want)
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"resultCode":"2000"}`)), Request: req}, nil
			})
			caller := newExternalCaller(&http.Client{Transport: transport}, nil)
			if _, err := caller.Call(context.Background(), contract, tt.operation, tt.params, ExternalCredential{Key: "SAFETY-SECRET-123"}); err != nil {
				t.Fatalf("Call: %v", err)
			}
		})
	}

	caller := newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid input reached transport")
		return nil, nil
	})}, nil)
	for _, tc := range []struct {
		op     string
		params map[string]string
	}{
		{"certificationList", map[string]string{"conditionKey": "invented", "conditionValue": "x"}},
		{"recallDetail", map[string]string{"recallUid": ""}},
		{"unknown", map[string]string{}},
	} {
		if _, err := caller.Call(context.Background(), contract, tc.op, tc.params, ExternalCredential{Key: "SAFETY-SECRET-123"}); err == nil {
			t.Errorf("invalid %s params %#v succeeded", tc.op, tc.params)
		}
	}
}

func TestExternalCallerInspectsAndInvokesFoodSafetyKoreaService(t *testing.T) {
	contract := &ExternalContract{
		AdapterID:         "foodsafetykorea",
		ProviderServiceID: "I-0040",
		DocumentationURL:  "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040",
		Auth:              &ExternalAuthContract{Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/"},
	}
	inspections := 0
	apiRequests := 0
	inspector := func(_ context.Context, got *ExternalContract) (map[string]Param, error) {
		inspections++
		if got.ProviderServiceID != "I-0040" {
			t.Fatalf("inspected contract = %#v", got)
		}
		return map[string]Param{
			"dataType": {Name: "dataType", Required: "필수"},
			"startIdx": {Name: "startIdx", Required: "필수"},
			"endIdx":   {Name: "endIdx", Required: "필수"},
			"PRMS_DT":  {Name: "PRMS_DT", Required: "선택"},
		}, nil
	}
	transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		apiRequests++
		wantPath := "/api/FOOD-SECRET-123/I-0040/json/1/10/PRMS_DT=20260101"
		if req.URL.Scheme != "https" || req.URL.Host != "openapi.foodsafetykorea.go.kr" || req.URL.EscapedPath() != wantPath {
			t.Fatalf("request URL = %s, want path %s", req.URL, wantPath)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"I-0040":{"total_count":"1","RESULT":{"CODE":"INFO-000","MSG":"정상처리되었습니다."}}}`)), Request: req}, nil
	})
	caller := newExternalCaller(&http.Client{Transport: transport}, inspector)
	for i := 0; i < 2; i++ {
		result, err := caller.Call(context.Background(), contract, "list", map[string]string{
			"dataType": "json", "startIdx": "1", "endIdx": "10", "PRMS_DT": "20260101",
		}, ExternalCredential{Key: "FOOD-SECRET-123"})
		if err != nil {
			t.Fatalf("Call %d: %v", i+1, err)
		}
		if result.Status != 200 {
			t.Fatalf("result = %#v", result)
		}
	}
	if inspections != 1 || apiRequests != 2 {
		t.Fatalf("inspections=%d API requests=%d, want one schema inspection and two calls", inspections, apiRequests)
	}

	for _, params := range []map[string]string{
		{"dataType": "json", "startIdx": "1", "endIdx": "1001"},
		{"dataType": "yaml", "startIdx": "1", "endIdx": "10"},
		{"dataType": "json", "startIdx": "1", "endIdx": "10", "INVENTED": "x"},
	} {
		if _, err := caller.Call(context.Background(), contract, "list", params, ExternalCredential{Key: "FOOD-SECRET-123"}); err == nil {
			t.Errorf("invalid params %#v succeeded", params)
		}
	}
}

func TestFoodSafetySchemaCacheExpiresAndSeparatesAdapterRevisions(t *testing.T) {
	inspections := 0
	caller := newExternalCaller(&http.Client{}, func(_ context.Context, _ *ExternalContract) (map[string]Param, error) {
		inspections++
		return map[string]Param{"dataType": {Name: "dataType", Required: "필수"}}, nil
	})
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	caller.now = func() time.Time { return now }
	contract := &ExternalContract{
		AdapterRevision: 2, ProviderServiceID: "I-0040",
		DocumentationURL: "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040",
	}

	if _, err := caller.foodSafetySchema(context.Background(), contract); err != nil {
		t.Fatal(err)
	}
	if _, err := caller.foodSafetySchema(context.Background(), contract); err != nil {
		t.Fatal(err)
	}
	revised := *contract
	revised.AdapterRevision++
	if _, err := caller.foodSafetySchema(context.Background(), &revised); err != nil {
		t.Fatal(err)
	}
	now = now.Add(foodSchemaCacheTTL)
	if _, err := caller.foodSafetySchema(context.Background(), contract); err != nil {
		t.Fatal(err)
	}
	if inspections != 3 {
		t.Fatalf("inspections=%d, want initial + revision + expiry", inspections)
	}
}

func TestExternalCallerParsesFoodSafetyKoreaOfficialParameterTable(t *testing.T) {
	contract := &ExternalContract{
		AdapterID:         "foodsafetykorea",
		ProviderServiceID: "I-0040",
		DocumentationURL:  "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040",
		Auth:              &ExternalAuthContract{Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/"},
	}
	requests := 0
	transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		switch req.URL.Host {
		case "www.foodsafetykorea.go.kr":
			body := `<html><body>
				<input id="svc_no" value="I-0040">
				<table><caption>응답결과</caption><tbody><tr><td>1</td><td>INVENTED</td><td>필수</td><td>무시</td></tr></tbody></table>
				<table><caption> 요청인자 </caption><tbody>
				<tr><td>1</td><td>keyId</td><td>필수</td><td>인증키</td></tr>
				<tr><td>2</td><td>serviceId</td><td>필수</td><td>서비스</td></tr>
				<tr><td>3</td><td>dataType</td><td>필수</td><td>json 또는 xml</td></tr>
				<tr><td>4</td><td>startIdx</td><td>필수</td><td>시작</td></tr>
				<tr><td>5</td><td>endIdx</td><td>필수</td><td>끝</td></tr>
				<tr><td>6</td><td>PRMS_DT</td><td>선택</td><td>허가일자</td></tr>
				</tbody></table></body></html>`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		case "openapi.foodsafetykorea.go.kr":
			if got := req.URL.EscapedPath(); got != "/api/FOOD-SECRET-123/I-0040/json/1/10/PRMS_DT=20260101" {
				t.Fatalf("API path = %q", got)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"I-0040":{"RESULT":{"CODE":"INFO-000"}}}`)), Request: req}, nil
		default:
			t.Fatalf("unexpected host %q", req.URL.Host)
			return nil, nil
		}
	})
	caller := newExternalCaller(&http.Client{Transport: transport}, nil)
	if _, err := caller.Call(context.Background(), contract, "list", map[string]string{
		"dataType": "json", "startIdx": "1", "endIdx": "10", "PRMS_DT": "20260101",
	}, ExternalCredential{Key: "FOOD-SECRET-123"}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want documentation inspection + API call", requests)
	}
}

func TestExternalCallerInvokesTypedVWorldFamilies(t *testing.T) {
	baseContract := ExternalContract{
		AdapterID: "vworld",
		Auth:      &ExternalAuthContract{Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/"},
	}
	tests := []struct {
		family, operation, path string
		serviceID               string
		params                  map[string]string
		wantInjected            map[string]string
	}{
		{"data", "GetFeature", "/req/data", "adsigg", map[string]string{"geomFilter": "POINT(127 37)", "format": "json", "size": "10"}, map[string]string{"service": "data", "request": "GetFeature", "data": "LT_C_ADSIGG_INFO"}},
		{"address", "GetCoord", "/req/address", "", map[string]string{"type": "road", "address": "판교로 242", "format": "json"}, map[string]string{"service": "address", "request": "GetCoord"}},
		{"search", "Search", "/req/search", "", map[string]string{"type": "district", "query": "삼평동", "category": "L4", "format": "json"}, map[string]string{"service": "search", "request": "search"}},
		{"ogc", "wms.GetMap", "/req/wms", "", map[string]string{"layers": "lt_c_adsigg", "bbox": "37,127,38,128", "width": "256", "height": "256"}, map[string]string{"service": "WMS", "request": "GetMap"}},
	}
	for _, tt := range tests {
		t.Run(tt.family+"/"+tt.operation, func(t *testing.T) {
			contract := baseContract
			contract.ProviderFamily = tt.family
			contract.ProviderServiceID = tt.serviceID
			transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Scheme != "https" || req.URL.Host != "api.vworld.kr" || req.URL.Path != tt.path {
					t.Fatalf("URL = %s", req.URL)
				}
				if req.URL.Query().Get("key") != "VWORLD-SECRET-123" || req.URL.Query().Get("domain") != "https://example.com/map" {
					t.Fatalf("credential query missing: %s", req.URL.RawQuery)
				}
				for name, value := range tt.wantInjected {
					if req.URL.Query().Get(name) != value {
						t.Errorf("%s = %q, want %q", name, req.URL.Query().Get(name), value)
					}
				}
				contentType, body := "application/json", `{"response":{"status":"OK","result":{}}}`
				if tt.operation == "wms.GetMap" {
					contentType, body = "image/png", "PNG"
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			result, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(context.Background(), &contract, tt.operation, tt.params,
				ExternalCredential{Key: "VWORLD-SECRET-123", Domain: "https://example.com/map"})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if tt.operation == "wms.GetMap" && (result.BodyEncoding != "base64" || result.Body != "UE5H") {
				t.Fatalf("binary result = %#v", result)
			}
		})
	}

	contract := baseContract
	contract.ProviderFamily = "search"
	caller := newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid params reached transport")
		return nil, nil
	})}, nil)
	for _, params := range []map[string]string{
		{"type": "district", "query": "삼평동"},
		{"type": "place", "query": "카페", "key": "attacker"},
		{"type": "invented", "query": "x"},
	} {
		if _, err := caller.Call(context.Background(), &contract, "Search", params, ExternalCredential{Key: "VWORLD-SECRET-123"}); err == nil {
			t.Errorf("invalid params %#v succeeded", params)
		}
	}
}

func TestVWorldNumericBoundaries(t *testing.T) {
	contract := &ExternalContract{
		AdapterID:         "vworld",
		Auth:              &ExternalAuthContract{Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/"},
		ProviderServiceID: "adsigg",
	}
	tests := []struct {
		name, family, operation, param, value string
		base                                  map[string]string
		wantErr                               bool
	}{
		{"size zero", "data", "GetFeature", "size", "0", map[string]string{"geomFilter": "POINT(127 37)"}, true},
		{"size negative", "data", "GetFeature", "size", "-1", map[string]string{"geomFilter": "POINT(127 37)"}, true},
		{"size max", "data", "GetFeature", "size", "1000", map[string]string{"geomFilter": "POINT(127 37)"}, false},
		{"size over", "data", "GetFeature", "size", "1001", map[string]string{"geomFilter": "POINT(127 37)"}, true},
		{"page zero", "data", "GetFeature", "page", "0", map[string]string{"geomFilter": "POINT(127 37)"}, true},
		{"page negative", "data", "GetFeature", "page", "-1", map[string]string{"geomFilter": "POINT(127 37)"}, true},
		{"width max", "ogc", "wms.GetMap", "width", "2048", map[string]string{"layers": "x", "bbox": "1,2,3,4", "height": "1"}, false},
		{"width over", "ogc", "wms.GetMap", "width", "2049", map[string]string{"layers": "x", "bbox": "1,2,3,4", "height": "1"}, true},
		{"height zero", "ogc", "wms.GetMap", "height", "0", map[string]string{"layers": "x", "bbox": "1,2,3,4", "width": "1"}, true},
		{"height max", "ogc", "wms.GetMap", "height", "2048", map[string]string{"layers": "x", "bbox": "1,2,3,4", "width": "1"}, false},
		{"feature count over", "ogc", "wms.GetFeatureInfo", "feature_count", "1001", map[string]string{"layers": "x", "bbox": "1,2,3,4", "width": "1", "height": "1", "query_layers": "x", "i": "0", "j": "0"}, true},
		{"index zero", "ogc", "wms.GetFeatureInfo", "i", "0", map[string]string{"layers": "x", "bbox": "1,2,3,4", "width": "1", "height": "1", "query_layers": "x", "j": "0"}, false},
		{"index negative", "ogc", "wms.GetFeatureInfo", "i", "-1", map[string]string{"layers": "x", "bbox": "1,2,3,4", "width": "1", "height": "1", "query_layers": "x", "j": "0"}, true},
		{"start index zero", "ogc", "wfs.GetFeature", "startindex", "0", map[string]string{"typename": "x"}, false},
		{"start index negative", "ogc", "wfs.GetFeature", "startindex", "-1", map[string]string{"typename": "x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract.ProviderFamily = tt.family
			params := make(map[string]string, len(tt.base)+1)
			for name, value := range tt.base {
				params[name] = value
			}
			params[tt.param] = tt.value
			_, err := buildVWorldRequest(context.Background(), contract, tt.operation, params, ExternalCredential{Key: "VWORLD-SECRET-123"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestExternalCallerFailsClosedBeforeTransportOnUntrustedContractsAndParams(t *testing.T) {
	never := newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid contract or params reached provider transport")
		return nil, nil
	})}, func(context.Context, *ExternalContract) (map[string]Param, error) {
		return map[string]Param{
			"dataType": {Name: "dataType"}, "startIdx": {Name: "startIdx"}, "endIdx": {Name: "endIdx"},
		}, nil
	})
	safety := &ExternalContract{AdapterID: "safetykorea", Auth: &ExternalAuthContract{
		Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/",
	}}
	vworld := &ExternalContract{AdapterID: "vworld", ProviderFamily: "data", ProviderServiceID: "adsigg", Auth: &ExternalAuthContract{
		Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/",
	}}
	food := &ExternalContract{AdapterID: "foodsafetykorea", ProviderServiceID: "I-0040", Auth: &ExternalAuthContract{
		Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/",
	}}
	tests := []struct {
		name       string
		contract   *ExternalContract
		operation  string
		params     map[string]string
		credential ExternalCredential
	}{
		{"nil contract", nil, "x", nil, ExternalCredential{Key: "LONG-ENOUGH-KEY"}},
		{"empty credential", safety, "certificationDetail", map[string]string{"certNum": "SU123"}, ExternalCredential{}},
		{"unknown adapter", &ExternalContract{AdapterID: "unknown", Auth: &ExternalAuthContract{}}, "x", nil, ExternalCredential{Key: "LONG-ENOUGH-KEY"}},
		{"safety extra param", safety, "certificationDetail", map[string]string{"certNum": "SU123", "other": "x"}, ExternalCredential{Key: "LONG-ENOUGH-KEY"}},
		{"food invalid operation", food, "detail", map[string]string{"dataType": "json", "startIdx": "1", "endIdx": "10"}, ExternalCredential{Key: "FOOD-SECRET-123"}},
		{"food injected key", food, "list", map[string]string{"dataType": "json", "startIdx": "1", "endIdx": "10", "keyId": "attacker"}, ExternalCredential{Key: "FOOD-SECRET-123"}},
		{"vworld feature without filter", vworld, "GetFeature", map[string]string{"data": "LT_C_ADSIGG_INFO"}, ExternalCredential{Key: "VWORLD-SECRET-123"}},
		{"vworld caller-selected dataset", vworld, "GetFeature", map[string]string{"data": "OTHER", "geomFilter": "POINT(127 37)"}, ExternalCredential{Key: "VWORLD-SECRET-123"}},
		{"vworld invalid registered domain", vworld, "GetFeature", map[string]string{"geomFilter": "POINT(127 37)"}, ExternalCredential{Key: "VWORLD-SECRET-123", Domain: "javascript:alert(1)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := never.Call(context.Background(), tt.contract, tt.operation, tt.params, tt.credential); err == nil {
				t.Fatal("invalid external call succeeded")
			}
		})
	}

	if _, err := (*ExternalCaller)(nil).Call(context.Background(), safety, "certificationDetail", map[string]string{"certNum": "SU123"}, ExternalCredential{Key: "LONG-ENOUGH-KEY"}); err == nil {
		t.Fatal("nil caller succeeded")
	}
}

func TestExternalCallerSurfacesEachProviderErrorContractAndHTTPFailure(t *testing.T) {
	foodSchema := func(context.Context, *ExternalContract) (map[string]Param, error) {
		return map[string]Param{
			"dataType": {Name: "dataType"}, "startIdx": {Name: "startIdx"}, "endIdx": {Name: "endIdx"},
		}, nil
	}
	tests := []struct {
		name        string
		contract    *ExternalContract
		operation   string
		params      map[string]string
		credential  ExternalCredential
		status      int
		body        string
		contentType string
		want        error
		inspector   foodSafetyInspector
	}{
		{
			name: "FoodSafetyKorea result code",
			contract: &ExternalContract{AdapterID: "foodsafetykorea", ProviderServiceID: "I-0040", Auth: &ExternalAuthContract{
				Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/",
			}},
			operation: "list", params: map[string]string{"dataType": "json", "startIdx": "1", "endIdx": "10"},
			credential: ExternalCredential{Key: "FOOD-SECRET-123"}, status: 200,
			body: `{"I-0040":{"RESULT":{"CODE":"ERROR-503","MSG":"서비스 점검"}}}`, want: ErrExternalProvider, inspector: foodSchema,
		},
		{
			name: "FoodSafetyKorea XML result code",
			contract: &ExternalContract{AdapterID: "foodsafetykorea", ProviderServiceID: "I-0040", Auth: &ExternalAuthContract{
				Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/",
			}},
			operation: "list", params: map[string]string{"dataType": "xml", "startIdx": "1", "endIdx": "10"},
			credential: ExternalCredential{Key: "FOOD-SECRET-123"}, status: 200, contentType: "application/xml",
			body: `<I-0040><RESULT><CODE>ERROR-503</CODE><MSG>서비스 점검</MSG></RESULT></I-0040>`, want: ErrExternalProvider, inspector: foodSchema,
		},
		{
			name: "VWorld response status",
			contract: &ExternalContract{AdapterID: "vworld", ProviderFamily: "address", Auth: &ExternalAuthContract{
				Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/",
			}},
			operation: "GetCoord", params: map[string]string{"type": "road", "address": "판교로 242"},
			credential: ExternalCredential{Key: "VWORLD-SECRET-123"}, status: 200,
			body: `{"response":{"status":"ERROR","error":{"code":"INVALID_KEY"}}}`, want: ErrExternalProvider,
		},
		{
			name: "VWorld XML response status",
			contract: &ExternalContract{AdapterID: "vworld", ProviderFamily: "address", Auth: &ExternalAuthContract{
				Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/",
			}},
			operation: "GetCoord", params: map[string]string{"type": "road", "address": "판교로 242"},
			credential: ExternalCredential{Key: "VWORLD-SECRET-123"}, status: 200, contentType: "application/xml",
			body: `<response><status>ERROR</status><error><code>INVALID_KEY</code></error></response>`, want: ErrExternalProvider,
		},
		{
			name: "provider HTTP status",
			contract: &ExternalContract{AdapterID: "safetykorea", Auth: &ExternalAuthContract{
				Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/",
			}},
			operation: "certificationDetail", params: map[string]string{"certNum": "SU123"},
			credential: ExternalCredential{Key: "SAFETY-SECRET-123"}, status: 503,
			body: `{"message":"unavailable"}`, want: ErrHTTPStatus,
		},
		{
			name: "SafetyKorea unrecognized success envelope",
			contract: &ExternalContract{AdapterID: "safetykorea", Auth: &ExternalAuthContract{
				Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/",
			}},
			operation: "certificationDetail", params: map[string]string{"certNum": "SU123"},
			credential: ExternalCredential{Key: "SAFETY-SECRET-123"}, status: 200,
			body: `{"message":"unknown"}`, want: ErrExternalProvider,
		},
		{
			name: "FoodSafetyKorea unrecognized success envelope",
			contract: &ExternalContract{AdapterID: "foodsafetykorea", ProviderServiceID: "I-0040", Auth: &ExternalAuthContract{
				Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/",
			}},
			operation: "list", params: map[string]string{"dataType": "json", "startIdx": "1", "endIdx": "10"},
			credential: ExternalCredential{Key: "FOOD-SECRET-123"}, status: 200,
			body: `{"unexpected":{}}`, want: ErrExternalProvider, inspector: foodSchema,
		},
		{
			name: "VWorld unrecognized success envelope",
			contract: &ExternalContract{AdapterID: "vworld", ProviderFamily: "address", Auth: &ExternalAuthContract{
				Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/",
			}},
			operation: "GetCoord", params: map[string]string{"type": "road", "address": "판교로 242"},
			credential: ExternalCredential{Key: "VWORLD-SECRET-123"}, status: 200,
			body: `{"unexpected":{}}`, want: ErrExternalProvider,
		},
		{
			name: "VWorld OGC exception document",
			contract: &ExternalContract{AdapterID: "vworld", ProviderFamily: "ogc", Auth: &ExternalAuthContract{
				Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/",
			}},
			operation: "wfs.GetFeature", params: map[string]string{"typename": "lt_c_adsigg"},
			credential: ExternalCredential{Key: "VWORLD-SECRET-123"}, status: 200, contentType: "application/xml",
			body: `<ows:ExceptionReport xmlns:ows="http://www.opengis.net/ows"><ows:Exception><ows:ExceptionText>invalid key</ows:ExceptionText></ows:Exception></ows:ExceptionReport>`, want: ErrExternalProvider,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				contentType := tt.contentType
				if contentType == "" {
					contentType = "application/json"
				}
				return &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(tt.body)), Request: req}, nil
			})
			result, err := newExternalCaller(&http.Client{Transport: transport}, tt.inspector).Call(
				context.Background(), tt.contract, tt.operation, tt.params, tt.credential)
			if !errors.Is(err, tt.want) || result == nil || result.Status != tt.status {
				t.Fatalf("result=%#v error=%v, want %v", result, err, tt.want)
			}
		})
	}
}

func TestFoodSafetyKoreaInspectorDetectsDocumentationDrift(t *testing.T) {
	base := &ExternalContract{
		AdapterID: "foodsafetykorea", ProviderServiceID: "I-0040",
		DocumentationURL: "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040",
		Auth:             &ExternalAuthContract{Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/"},
	}
	tests := []struct {
		name          string
		documentation string
		status        int
		body          string
	}{
		{"untrusted documentation URL", "https://evil.example/api/openApiInfo.do?svc_no=I-0040", 200, ""},
		{"HTTP failure", base.DocumentationURL, 503, "unavailable"},
		{"service mismatch", base.DocumentationURL, 200, `<input id="svc_no" value="OTHER">`},
		{"missing request table", base.DocumentationURL, 200, `<input id="svc_no" value="I-0040"><table><caption>응답결과</caption></table>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := *base
			contract.DocumentationURL = tt.documentation
			requests := 0
			transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: tt.status, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(tt.body)), Request: req}, nil
			})
			_, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(context.Background(), &contract, "list",
				map[string]string{"dataType": "json", "startIdx": "1", "endIdx": "10"}, ExternalCredential{Key: "FOOD-SECRET-123"})
			if err == nil {
				t.Fatal("documentation drift was accepted")
			}
			if tt.name == "untrusted documentation URL" && requests != 0 {
				t.Fatalf("untrusted documentation triggered %d requests", requests)
			}
		})
	}
}

func TestExternalCallerSupportsDocumentedVWorldOGCOperations(t *testing.T) {
	contract := &ExternalContract{AdapterID: "vworld", ProviderFamily: "ogc", Auth: &ExternalAuthContract{
		Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/",
	}}
	tests := []struct {
		operation, path, service, request, responseRoot string
		params                                          map[string]string
	}{
		{"wms.GetCapabilities", "/req/wms", "WMS", "GetCapabilities", "WMS_Capabilities", map[string]string{}},
		{"wms.GetFeatureInfo", "/req/wms", "WMS", "GetFeatureInfo", "FeatureInfoResponse", map[string]string{
			"layers": "lt_c_adsigg", "bbox": "37,127,38,128", "width": "256", "height": "256", "query_layers": "lt_c_adsigg", "i": "20", "j": "30", "info_format": "application/json",
		}},
		{"wfs.GetFeature", "/req/wfs", "WFS", "GetFeature", "FeatureCollection", map[string]string{"typename": "lt_c_adsigg", "output": "application/json", "count": "10"}},
		{"wfs.GetCapabilities", "/req/wfs", "WFS", "GetCapabilities", "WFS_Capabilities", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			transport := externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != tt.path || req.URL.Query().Get("service") != tt.service || req.URL.Query().Get("request") != tt.request {
					t.Fatalf("request = %s", req.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/xml"}}, Body: io.NopCloser(strings.NewReader("<" + tt.responseRoot + "/>")), Request: req}, nil
			})
			if _, err := newExternalCaller(&http.Client{Transport: transport}, nil).Call(context.Background(), contract, tt.operation, tt.params, ExternalCredential{Key: "VWORLD-SECRET-123"}); err != nil {
				t.Fatalf("Call: %v", err)
			}
		})
	}
}
