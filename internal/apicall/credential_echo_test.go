package apicall_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

type echoTransport func(*http.Request) (*http.Response, error)

func (f echoTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCallWithholdsCredentialEchoEvenOnSuccessfulResponses(t *testing.T) {
	for _, tc := range []struct{ name, body, key, contentType string }{
		{"json", `{"items":[{"value":"fixture-secret+token=="}]}`, "fixture-secret+token==", "application/json"},
		{"json escape", `{"items":[{"value":"fixture\u002dsecret+token=="}]}`, "fixture-secret+token==", "application/json"},
		{"encoded key decoded echo", `{"value":"fixture-secret+token=="}`, "fixture-secret%2Btoken%3D%3D", "application/json"},
		{"url echo", `{"url":"https://apis.data.go.kr/op?serviceKey=fixture-secret%2btoken%3d%3d"}`, "fixture-secret+token==", "application/json"},
		{"xml escape", `<response><value>fixture&#45;secret+token==</value></response>`, "fixture-secret+token==", "application/xml"},
		{"field name", `{"fixture-secret+token==":"echo"}`, "fixture-secret+token==", "application/json"},
		{"text", `failure: fixture-secret+token==`, "fixture-secret+token==", "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range []int{200, 403, 500} {
				client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: echoTransport(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {tc.contentType}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
				})}))
				result, err := apicall.Call(context.Background(), client, "https://apis.data.go.kr/test/op", nil, tc.key)
				if result != nil || err == nil {
					t.Fatalf("credential-bearing response must be withheld: status=%d returned=%t error=%v", status, result != nil, err)
				}
				if strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), "token") {
					t.Fatal("diagnostic leaked credential")
				}
			}
		})
	}
}
