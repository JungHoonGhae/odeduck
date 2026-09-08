package apicall

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExternalCallerWithholdsCredentialBearingResponses(t *testing.T) {
	for _, tc := range []struct{ name, body, contentType string }{
		{"literal", `{"resultCode":"2000","resultData":[{"value":"fixture-secret+token=="}]}`, "application/json"},
		{"json escaped", `{"resultCode":"2000","resultData":[{"value":"fixture\u002dsecret+token=="}]}`, "application/json"},
		{"xml escaped", `<response><resultCode>2000</resultCode><value>fixture&#45;secret+token==</value></response>`, "application/xml"},
		{"encoded URL", `{"resultCode":"2000","value":"key=fixture-secret%2btoken%3d%3d"}`, "application/json"},
		{"content type", `{"resultCode":"2000"}`, "application/json;extra=fixture-secret+token=="},
		{"binary", "binary-fixture-secret+token==", "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contract := &ExternalContract{AdapterID: "safetykorea", Auth: &ExternalAuthContract{Placement: "header", Name: "AuthKey", CredentialScope: "https://www.safetykorea.kr/openapi/api/"}}
			caller := newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {tc.contentType}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})}, nil)
			result, err := caller.Call(context.Background(), contract, "certificationDetail", map[string]string{"certNum": "SU123"}, ExternalCredential{Key: "fixture-secret+token=="})
			if result != nil || err == nil {
				t.Fatalf("credential-bearing result must be withheld; returned=%t err=%v", result != nil, err)
			}
			if strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), "token") {
				t.Fatal("credential in diagnostic")
			}
		})
	}
}

func TestExternalCredentialBoundaryCoversQueryAndPathPlacement(t *testing.T) {
	for _, tc := range []struct {
		contract  ExternalContract
		operation string
		params    map[string]string
	}{
		{ExternalContract{AdapterID: "vworld", ProviderFamily: "address", Auth: &ExternalAuthContract{Placement: "query", Name: "key", CredentialScope: "https://api.vworld.kr/req/"}}, "GetCoord", map[string]string{"type": "road", "address": "판교로 242"}},
		{ExternalContract{AdapterID: "foodsafetykorea", ProviderServiceID: "I-0040", Auth: &ExternalAuthContract{Placement: "path", Name: "keyId", CredentialScope: "https://openapi.foodsafetykorea.go.kr/api/"}}, "list", map[string]string{"dataType": "json", "startIdx": "1", "endIdx": "10"}},
	} {
		t.Run(tc.contract.AdapterID, func(t *testing.T) {
			for _, status := range []int{200, 500} {
				called := false
				caller := newExternalCaller(&http.Client{Transport: externalRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					called = true
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"value":"fixture\u002dsecret-token"}`)), Request: req}, nil
				})}, func(context.Context, *ExternalContract) (map[string]Param, error) {
					return map[string]Param{"dataType": {Name: "dataType", Required: "필수"}, "startIdx": {Name: "startIdx", Required: "필수"}, "endIdx": {Name: "endIdx", Required: "필수"}}, nil
				})
				result, err := caller.Call(context.Background(), &tc.contract, tc.operation, tc.params, ExternalCredential{Key: "fixture-secret-token"})
				if !called || result != nil || err == nil || strings.Contains(err.Error(), "fixture-secret") {
					t.Fatalf("credential placement boundary failed: called=%t returned=%t err=%v", called, result != nil, err)
				}
			}
		})
	}
}
