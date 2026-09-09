package dataset

import (
	"context"
	"net/http"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func TestSampleCSVPreservesTuplesAndDeclaresPrefix(t *testing.T) {
	i := NewInspector(fixtureTransport{gets: map[string]*fetch.Response{"https://data.test/a.csv": {Status: 200, Body: []byte("code,year\n001,2025\n002,2024\n")}}}, "")
	s, err := i.SampleCSV(context.Background(), Asset{Name: "a.csv", Request: Request{Method: http.MethodGet, URL: "https://data.test/a.csv"}}, 1)
	if err != nil || len(s.Rows) != 1 || s.Rows[0]["code"] != "001" || !s.Prefix || s.SHA256 == "" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestSampleCSVPreservesEmptyStringsWithoutInventingNull(t *testing.T) {
	i := NewInspector(fixtureTransport{gets: map[string]*fetch.Response{"https://data.test/a.csv": {Status: 200, Body: []byte("id,empty,quoted,space,literal\n001,,\"\", ,null\n")}}}, "")
	s, err := i.SampleCSV(context.Background(), Asset{Name: "a.csv", Request: Request{Method: http.MethodGet, URL: "https://data.test/a.csv"}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"id": "001", "empty": "", "quoted": "", "space": " ", "literal": "null"} {
		if got, exists := s.Rows[0][field]; !exists || got != want {
			t.Fatalf("CSV source value changed: %s=%#v, want %q", field, got, want)
		}
	}
}

func TestSampleCSVRejectsAmbiguousHeadersAndMalformedRows(t *testing.T) {
	for _, body := range []string{"code,code\n001,002\n", ",year\n001,2025\n", "code,year\n001\n"} {
		i := NewInspector(fixtureTransport{gets: map[string]*fetch.Response{"https://data.test/a.csv": {Status: 200, Body: []byte(body)}}}, "")
		if _, err := i.SampleCSV(context.Background(), Asset{Name: "a.csv", Request: Request{Method: http.MethodGet, URL: "https://data.test/a.csv"}}, 10); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestSampleCSVSelectsExactScopeBeforeRowLimit(t *testing.T) {
	body := "province,city,code\n서울,중구,001\n충남,공주,002\n충남,공주,003\n충남,아산,004\n"
	i := NewInspector(fixtureTransport{gets: map[string]*fetch.Response{"https://data.test/a.csv": {Status: 200, Body: []byte(body)}}}, "")
	asset := Asset{Name: "a.csv", Request: Request{Method: http.MethodGet, URL: "https://data.test/a.csv"}}
	s, err := i.SampleCSVSelected(context.Background(), asset, 1, map[string]string{"province": "충남", "city": "공주"})
	if err != nil || len(s.Rows) != 1 || s.Rows[0]["code"] != "002" || !s.Prefix || s.Selection == nil || s.Selection.ScannedRows != 3 || s.Selection.MatchedRows != 2 || s.Selection.Exhausted {
		t.Fatalf("selection must precede prefix limit: %+v %v", s, err)
	}
	s, err = i.SampleCSVSelected(context.Background(), asset, 10, map[string]string{"province": "충남", "city": "공주"})
	if err != nil || len(s.Rows) != 2 || s.Prefix || !s.Selection.Exhausted || s.Selection.ScannedRows != 4 || s.Selection.MatchedRows != 2 {
		t.Fatalf("%+v %v", s, err)
	}
	for _, where := range []map[string]string{{"unknown": "x"}, {"code": "2"}, {"province": "충"}, {"city": ""}} {
		if _, err := i.SampleCSVSelected(context.Background(), asset, 10, where); err == nil {
			t.Fatalf("unknown/prefix/coerced/empty filter accepted: %v", where)
		}
	}
}

func TestCSVSelectionDoesNotSkipMalformedExcludedRows(t *testing.T) {
	i := NewInspector(fixtureTransport{gets: map[string]*fetch.Response{"https://data.test/a.csv": {Status: 200, Body: []byte("scope,code\nexcluded,001,extra\nselected,002\n")}}}, "")
	if _, err := i.SampleCSVSelected(context.Background(), Asset{Name: "a.csv", Request: Request{Method: http.MethodGet, URL: "https://data.test/a.csv"}}, 1, map[string]string{"scope": "selected"}); err == nil {
		t.Fatal("malformed excluded record hidden")
	}
}
