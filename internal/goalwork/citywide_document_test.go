package goalwork_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Offline delivery tests substitute only HTTP. These deliberately synthetic
// sections test the registered reader, not the actual definition or G4 meaning.
func citywideDocumentFixture(t *testing.T, policy goalwork.Policy) goalwork.Dependencies {
	t.Helper()
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: documentHTTP(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("supporting document request exposed credentials")
		}
		var body string
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = `{}`
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = `<ul><li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li></ul>`
		case "https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124":
			body = `<html><body><div class="answers"><div class="tbx">FIXTURE reply header</div><div class="an_txt">FIXTURE definition; not live methodology evidence</div></div><input id="boardIdx" name="boardIdx" value="22124"></body></html>`
		default:
			return nil, fmt.Errorf("unexpected document fixture HTTP: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=UTF-8"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	return goalwork.LiveDependencies(client, "https://www.data.go.kr", nil, catalog.Searcher{}, policy)
}
