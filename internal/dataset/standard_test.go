package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

const standardHeaderFixture = `{"fileName":"전국시설표준데이터","columList":[{"columCode":"CODE","columNm":"시설코드"},{"columCode":"N","columNm":"인원"},{"columCode":"INSTT_CODE","columNm":"제공기관코드"}],"tableVO":{"publicDataPk":"300","svcTableNm":"tn_fixture_svc","colNmList":["CODE","N"]},"totalCount":20}`

func standardCatalog(t *testing.T, svc string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "300", Title: "title is not delivery evidence", SvcType: svc}}}).Save(); err != nil {
		t.Fatal(err)
	}
}

func TestStandardInspectionAndSampleOwnTrustedContract(t *testing.T) {
	for _, svc := range []string{"STD", ""} {
		t.Run("delivery_"+svc, func(t *testing.T) {
			standardCatalog(t, svc)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
					t.Error("public standard request sent credentials")
				}
				switch r.URL.Path {
				case "/download/columList.json":
					if r.URL.Query().Get("pk") != "300" || r.URL.Query().Get("ext") != "CSV" {
						t.Errorf("header query: %s", r.URL)
					}
					fmt.Fprint(w, standardHeaderFixture)
				case "/download/standard.json":
					calls++
					q := r.URL.Query()
					if q.Get("svcTableNm") != "tn_fixture_svc" || strings.Join(q["colNmList"], ",") != "CODE,N" || q.Get("perPage") != "2" || q.Get("page") != "1" || q.Get("publicDataPk") != "300" || q.Get("totalCount") != "20" {
						t.Errorf("sample query: %s", r.URL)
					}
					fmt.Fprint(w, `[{"CODE":"001","N":9007199254740993,"INSTT_CODE":null},{"CODE":"002","N":0,"INSTT_CODE":"000"}]`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			i := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL)
			result, err := i.Inspect(context.Background(), InspectionRequest{PK: "300"})
			if err != nil || result.Standard == nil || result.Delivery != "STD" || calls != 0 {
				t.Fatalf("%+v %v calls=%d", result, err, calls)
			}
			if len(result.Standard.Columns) != 3 || result.Standard.TotalCount != 20 || result.Standard.SchemaSHA256 == "" {
				t.Fatalf("missing provenance: %+v", result.Standard)
			}
			sample, err := i.SampleStandard(context.Background(), result.Standard, 2)
			if err != nil || len(sample.Rows) != 2 || !sample.Prefix || sample.SHA256 == "" {
				t.Fatalf("%+v %v", sample, err)
			}
			if sample.Rows[0]["CODE"] != "001" || sample.Rows[0]["N"] != json.Number("9007199254740993") || sample.Rows[0]["INSTT_CODE"] != nil {
				t.Fatalf("lost original types: %+v", sample.Rows)
			}
			forged := *result.Standard
			forged.handle = nil
			if _, err := i.SampleStandard(context.Background(), &forged, 2); err == nil {
				t.Fatal("accepted forged handle")
			}
			if _, err := NewUnifiedInspector(fetch.New(), srv.URL).SampleStandard(context.Background(), result.Standard, 2); err == nil {
				t.Fatal("accepted foreign inspector handle")
			}
			if calls != 1 {
				t.Fatal("invalid handle triggered network")
			}
		})
	}
}

func TestStandardContractRejectsUnprovenOrUnsafeSchema(t *testing.T) {
	for name, body := range map[string]string{
		"wrong_pk":            strings.Replace(standardHeaderFixture, `"publicDataPk":"300"`, `"publicDataPk":"301"`, 1),
		"unsafe_table":        strings.Replace(standardHeaderFixture, "tn_fixture_svc", "tbl;drop table", 1),
		"unsafe_column":       strings.ReplaceAll(standardHeaderFixture, `"CODE"`, `"CODE)--"`),
		"unadvertised_column": strings.Replace(standardHeaderFixture, `["CODE","N"]`, `["MISSING","N"]`, 1),
		"duplicate_column":    strings.Replace(standardHeaderFixture, `"columCode":"N"`, `"columCode":"CODE"`, 1),
		"deep_metadata":       strings.TrimSuffix(standardHeaderFixture, "}") + `,"extra":` + strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33) + "}",
		"empty":               `{}`, "html": `<html>login</html>`,
	} {
		t.Run(name, func(t *testing.T) {
			standardCatalog(t, "")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer srv.Close()
			if _, err := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL).Inspect(context.Background(), InspectionRequest{PK: "300"}); err == nil {
				t.Fatal("accepted unproven standard contract")
			}
		})
	}
}

func TestStandardSampleRejectsResponseDriftAndBounds(t *testing.T) {
	for name, body := range map[string]string{"object": `{"error":"denied"}`, "empty": `[]`, "extra_row": `[{"CODE":"1"},{"CODE":"2"},{"CODE":"3"}]`, "unadvertised": `[{"WRONG":"1"}]`, "nested": `[{"CODE":{"x":1}}]`, "trailing": `[{"CODE":"1"}] {}`, "oversized": `[{"CODE":"` + strings.Repeat("a", (2<<20)+1) + `"}]`} {
		t.Run(name, func(t *testing.T) {
			standardCatalog(t, "STD")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/download/columList.json" {
					fmt.Fprint(w, standardHeaderFixture)
				} else {
					fmt.Fprint(w, body)
				}
			}))
			defer srv.Close()
			i := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL)
			result, err := i.Inspect(context.Background(), InspectionRequest{PK: "300"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = i.SampleStandard(context.Background(), result.Standard, 2); err == nil {
				t.Fatal("accepted drift/bounds violation")
			}
		})
	}
}

func TestStandardInspectionAndSampleRejectRedirectedSources(t *testing.T) {
	for _, stage := range []string{"header", "rows"} {
		t.Run(stage, func(t *testing.T) {
			standardCatalog(t, "STD")
			var destinationCalls atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				destinationCalls.Add(1)
				if stage == "header" {
					fmt.Fprint(w, standardHeaderFixture)
				} else {
					fmt.Fprint(w, `[{"CODE":"001"}]`)
				}
			}))
			defer destination.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "rows" && r.URL.Path == "/download/columList.json" {
					fmt.Fprint(w, standardHeaderFixture)
					return
				}
				http.Redirect(w, r, destination.URL, http.StatusFound)
			}))
			defer srv.Close()
			i := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL)
			result, err := i.Inspect(context.Background(), InspectionRequest{PK: "300"})
			if stage == "rows" {
				if err != nil {
					t.Fatal(err)
				}
				sample, sampleErr := i.SampleStandard(context.Background(), result.Standard, 2)
				err = sampleErr
				if len(sample.Rows) != 0 || sample.SHA256 != "" {
					t.Fatal("redirected rows became source evidence")
				}
			}
			if err == nil || destinationCalls.Load() != 0 {
				t.Fatalf("redirected source accepted or visited: err=%v destinationCalls=%d", err, destinationCalls.Load())
			}
		})
	}
}

func TestStandardJSONStringsAreLossless(t *testing.T) {
	for _, tc := range []struct {
		name, encoded, want string
		invalid             bool
	}{
		{"invalid_utf8_ff", "\"\xff\"", "", true},
		{"invalid_utf8_fe", "\"\xfe\"", "", true},
		{"high_surrogate", `"\ud800"`, "", true},
		{"different_high_surrogate", `"\ud801"`, "", true},
		{"low_surrogate", `"\udc00"`, "", true},
		{"interrupted_pair", `"\ud800x\udc00"`, "", true},
		{"high_high", `"\ud800\ud801"`, "", true},
		{"replacement_literal", `"�"`, "�", false},
		{"replacement_escape", `"\ufffd"`, "�", false},
		{"valid_pair", `"\ud83d\ude00"`, "😀", false},
		{"literal_backslash", `"\\ud800"`, `\ud800`, false},
	} {
		for _, stage := range []string{"header", "rows"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				standardCatalog(t, "STD")
				header, rows := standardHeaderFixture, `[{"CODE":`+tc.encoded+`}]`
				if stage == "header" {
					header = strings.Replace(header, `"전국시설표준데이터"`, tc.encoded, 1)
					rows = `[{"CODE":"001"}]`
				}
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/download/columList.json" {
						fmt.Fprint(w, header)
					} else {
						fmt.Fprint(w, rows)
					}
				}))
				defer srv.Close()
				i := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL)
				result, err := i.Inspect(context.Background(), InspectionRequest{PK: "300"})
				if stage == "header" && tc.invalid {
					if err == nil {
						t.Fatal("lossy header decoding accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				sample, err := i.SampleStandard(context.Background(), result.Standard, 2)
				if tc.invalid {
					if err == nil || len(sample.Rows) != 0 {
						t.Fatal("malformed identifier became a retained string")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				got := result.Standard.Name
				if stage == "rows" {
					got, _ = sample.Rows[0]["CODE"].(string)
				}
				if got != tc.want {
					t.Fatalf("valid Unicode changed: %q != %q", got, tc.want)
				}
			})
		}
	}
}

func TestStandardJSONRejectsAmbiguousObjectMembers(t *testing.T) {
	for _, tc := range []struct {
		name, header, rows string
	}{
		{"header", strings.Replace(standardHeaderFixture, `"publicDataPk":"300"`, `"publicDataPk":"301","publicDataPk":"300"`, 1), `[{"CODE":"001"}]`},
		{"row", standardHeaderFixture, `[{"CODE":"001","CODE":"002"}]`},
		{"escaped_member", standardHeaderFixture, `[{"CODE":"001","\u0043ODE":"002"}]`},
		{"identical_values", standardHeaderFixture, `[{"CODE":"001","CODE":"001"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standardCatalog(t, "STD")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/download/columList.json" {
					fmt.Fprint(w, tc.header)
				} else {
					fmt.Fprint(w, tc.rows)
				}
			}))
			defer srv.Close()
			i := NewUnifiedInspector(fetch.New(fetch.WithDelay(0)), srv.URL)
			result, err := i.Inspect(context.Background(), InspectionRequest{PK: "300"})
			if tc.name != "header" {
				if err != nil {
					t.Fatal(err)
				}
				sample, sampleErr := i.SampleStandard(context.Background(), result.Standard, 2)
				err = sampleErr
				if len(sample.Rows) != 0 {
					t.Fatal("ambiguous source values were retained")
				}
			}
			if err == nil {
				t.Fatal("ambiguous JSON object accepted")
			}
		})
	}
}
