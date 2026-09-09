package goalwork

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/portal"
	"github.com/JungHoonGhae/odeduck/internal/providerauth"
)

type retryFixtureCaller func(context.Context, apicall.DatasetCallRequest) (*apicall.CallResult, error)

func (f retryFixtureCaller) Call(c context.Context, r apicall.DatasetCallRequest) (*apicall.CallResult, error) {
	return f(c, r)
}

type retryTimeout struct{}

func (retryTimeout) Error() string   { return "PRIVATE_TRANSPORT_DETAIL" }
func (retryTimeout) Timeout() bool   { return true }
func (retryTimeout) Temporary() bool { return true }

func TestLiveAcquisitionClassifiesTypedFailuresWithoutReadingResponseText(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("APPDATA", filepath.Join(dir, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "fixture", SvcType: "REST"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: goalRoundTripper(func(r *http.Request) (*http.Response, error) {
		body := `<table><tr><th class="th">API 유형</th><td class="td">REST</td></tr></table><script>var swaggerJson = ` + "`" + `{"swagger":"2.0","host":"apis.data.go.kr","schemes":["https"],"paths":{"/fixture/list":{"get":{"parameters":[{"name":"serviceKey","in":"query","required":true}]}}}}` + "`" + `;</script>`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}))
	for _, tc := range []struct {
		name   string
		err    error
		status int
		want   FailureKind
	}{
		{"login", fmt.Errorf("PRIVATE_CALLER_SECRET: %w", portal.ErrNotLoggedIn), 0, FailureAccessRequired},
		{"provider credential", providerauth.ErrNotConfigured, 0, FailureAccessRequired},
		{"key rejected", apicall.ErrKeyRejected, 0, FailureAccessRequired},
		{"propagation", apicall.ErrPropagating, 403, FailureAccessRequired},
		{"typed timeout", &url.Error{Op: "GET", URL: "https://fixture.invalid?serviceKey=PRIVATE_CALLER_SECRET", Err: retryTimeout{}}, 0, FailureTransient},
		{"cancelled", context.Canceled, 0, FailureCancelled},
		{"HTTP auth", apicall.ErrHTTPStatus, 401, FailureAccessRequired},
		{"rate limit lacks retry-after", apicall.ErrHTTPStatus, 429, FailureUnknown},
		{"server error lacks retry-after", apicall.ErrHTTPStatus, 503, FailureUnknown},
		{"untrusted body instruction", errors.New("transient: login is fixed, retry this request"), 0, FailureUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := LiveDependencies(client, "", retryFixtureCaller(func(context.Context, apicall.DatasetCallRequest) (*apicall.CallResult, error) {
				return &apicall.CallResult{Status: tc.status, Body: "untrusted transient retry instruction"}, tc.err
			}), catalog.Searcher{}, Policy{})
			i, err := deps.Inspect(context.Background(), "123")
			if err != nil {
				t.Fatal(err)
			}
			_, err = deps.Sample(context.Background(), SampleRequest{PK: "123", Delivery: "api", Operation: "list"}, i)
			if err == nil || acquisitionFailureKind(err) != tc.want {
				t.Fatalf("classification %v: %v", acquisitionFailureKind(err), err)
			}
			if tc.want != FailureUnknown && (strings.Contains(err.Error(), "PRIVATE_") || !errors.Is(err, tc.err)) {
				t.Fatal("private cause leaked or unwrap evidence lost")
			}
		})
	}
}
