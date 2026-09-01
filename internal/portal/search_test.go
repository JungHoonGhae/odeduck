package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/JungHoonGhae/opendatactl/internal/fetch"
)

func TestSearchDatasets(t *testing.T) {
	body, err := os.ReadFile("testdata/search-list.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tcs/dss/selectDataSetList.do" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(body)
	}))
	defer srv.Close()

	c := New(fetch.New(fetch.WithDelay(0)), WithBaseURL(srv.URL))
	ds, err := c.SearchDatasets(context.Background(), SearchOptions{Keyword: "선거"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(ds) == 0 {
		t.Fatal("expected at least one dataset from fixture")
	}
	for _, d := range ds {
		if d.PublicDataPk == "" || d.Title == "" {
			t.Errorf("dataset missing pk/title: %+v", d)
		}
	}
}

// The result list carries per-dataset metadata beside the <dl> — publisher, last
// update, view count and how many people applied for it. Without those a caller
// has to open every dataset page to judge anything.
func TestSearchDatasetsMetadata(t *testing.T) {
	body, err := os.ReadFile("testdata/search-list-meta.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(body)
	}))
	defer srv.Close()

	ds, err := New(fetch.New(fetch.WithDelay(0)), WithBaseURL(srv.URL)).
		SearchDatasets(context.Background(), SearchOptions{Type: "API"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(ds) == 0 {
		t.Fatal("no datasets parsed")
	}
	var withOrg, withMod, withView int
	for _, d := range ds {
		if d.Org != "" {
			withOrg++
		}
		if d.ModifiedAt != "" {
			withMod++
		}
		if d.ViewCount > 0 {
			withView++
		}
	}
	if withOrg == 0 {
		t.Error("제공기관 not parsed for any dataset")
	}
	if withMod == 0 {
		t.Error("수정일 not parsed for any dataset")
	}
	if withView == 0 {
		t.Error("조회수 not parsed for any dataset")
	}
}

func TestSearchDatasetsDetectsFileWithAutomaticOpenAPI(t *testing.T) {
	body := `<article class="apply-result-item">
		<div class="apply-result-link">
			<span class="krds-badge" data-ext="CSV">CSV</span>
			<span class="krds-badge bg-light-primary">JSON + XML</span>
			<a href="/data/15104801/fileData.do">장기요양기관 평가 결과</a>
		</div>
	</article>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	datasets, err := New(fetch.New(fetch.WithDelay(0)), WithBaseURL(srv.URL)).
		SearchDatasets(context.Background(), SearchOptions{Type: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 || !datasets[0].HasOpenAPI {
		t.Fatalf("automatic OpenAPI not detected: %+v", datasets)
	}
	if got := strings.Join(datasets[0].Formats, ","); got != "CSV,JSON,XML" {
		t.Fatalf("formats = %q", got)
	}
}
