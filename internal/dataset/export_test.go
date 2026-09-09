package dataset_test

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

const monthlyCSVURL = "https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3"

// Reduced official form shape with invented records, not a goal oracle.
//
//go:embed testdata/mois_monthly_export.html
var exportHTML string

//go:embed testdata/mois_monthly_export.csv
var exportCSV string

func exportParams() map[string]string {
	return map[string]string{"month": "2026-07", "registration": "all", "provinceCode": "2800000000", "ageFrom": "6", "ageTo": "7"}
}

func TestMonthlyExportEnforcesRegistrationStartMonth(t *testing.T) {
	// Official form goSearch, preserved with its byte hash in the applicability
	// research: all from 2008; Y/N from October 2010; O from January 2015.
	for _, tc := range []struct {
		registration, before, first string
	}{
		{"all", "2007-12", "2008-01"},
		{"resident", "2010-09", "2010-10"},
		{"unknown", "2010-09", "2010-10"},
		{"overseas", "2014-12", "2015-01"},
	} {
		t.Run(tc.registration, func(t *testing.T) {
			params := exportParams()
			params["registration"], params["month"] = tc.registration, tc.before
			if err := dataset.ValidateExportSelection("mois-monthly-age-csv", params); err == nil {
				t.Errorf("unsupported registration/month passed: %s/%s", tc.registration, tc.before)
			}
			params["month"] = tc.first
			if err := dataset.ValidateExportSelection("mois-monthly-age-csv", params); err != nil {
				t.Errorf("first supported month rejected: %s/%s: %v", tc.registration, tc.first, err)
			}
		})
	}
}

func TestMonthlyExportBoundsRetentionButScansAllRows(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(exportCSV), "\n")
	for _, tc := range []struct {
		name, row    string
		count, limit int
		tooLarge     bool
	}{
		{"one retained row", lines[3], 1001, 1, false},
		{"maximum retained rows", lines[3], 1001, 1000, false},
		{"retained byte limit", strings.Replace(lines[3], "Child", strings.Repeat("x", 60<<10), 1), 40, 1000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := lines[0] + "\n" + lines[1] + "\n" + strings.Repeat(tc.row+"\n", tc.count)
			i := documentInspector(t, monthlyDocumentURL, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() == monthlyDocumentURL {
					return documentResponse(exportHTML), nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/csv"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
			if err != nil {
				t.Fatal(err)
			}
			s, err := i.SampleExport(context.Background(), c, "mois-monthly-age-csv", exportParams(), tc.limit)
			if tc.tooLarge {
				if err == nil || !strings.Contains(err.Error(), "2 MiB") || len(s.Rows) != 0 || s.SHA256 != "" {
					t.Fatalf("retained byte limit yielded a partial observation: %+v %v", s, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Rows) != tc.limit || !s.Prefix || s.Selection.ScannedRows != 1002 || s.Selection.MatchedRows != 1001 || s.Selection.ReturnedRows != tc.limit || !s.Selection.Exhausted || s.CSV.DataRecords[0] != 2 || s.CSV.DataRecords[tc.limit-1] != tc.limit+1 || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
				t.Fatalf("retained prefix confused with complete selection: rows=%d %+v", len(s.Rows), s.Selection)
			}
		})
	}
}

func TestMonthlyExportRejectsUninspectedPublicReferences(t *testing.T) {
	i := documentInspector(t, monthlyDocumentURL, nil)
	c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(c)
	var reconstructed dataset.Contract
	if err := json.Unmarshal(wire, &reconstructed); err != nil {
		t.Fatal(err)
	}
	for _, untrusted := range []*dataset.Contract{nil, {}, &reconstructed} {
		if _, err := i.SampleExport(context.Background(), untrusted, "mois-monthly-age-csv", exportParams(), 1000); err == nil || !strings.Contains(err.Error(), "not issued by inspection") {
			t.Fatalf("public contract authorized export: %v", err)
		}
	}
}

func TestMonthlyExportLiveContract(t *testing.T) {
	if os.Getenv("ODEDUCK_MONTHLY_EXPORT_LIVE") != "1" {
		t.Skip("opt-in official July 2026 monthly export revision check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	i := dataset.NewInspector(fetch.New(), "")
	c, err := i.Inspect(ctx, dataset.Ref{PK: "3033304", Delivery: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	params := exportParams()
	params["ageTo"] = "17"
	s, err := i.SampleExport(ctx, c, "mois-monthly-age-csv", params, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed independent source-research revision, not a dynamically generated oracle.
	if s.SHA256 != "911940f3c38ffb7ed487a820607560616dedd5d226dcec1c0b9d7ffb65c4e47f" || s.Bytes != 1050109 || len(s.Rows) != 177 || len(s.CSV.Export.Columns) != 43 || s.Selection.ScannedRows != 3919 || s.Selection.MatchedRows != 177 || s.Prefix || !s.Selection.Exhausted || s.CSV.Encoding != "euc-kr" {
		t.Fatalf("fixed source revision/extent changed: hash=%s bytes=%d rows=%d selection=%+v", s.SHA256, s.Bytes, len(s.Rows), s.Selection)
	}
	t.Logf("csv=%s bytes=%d encoding=%s scanned=%d retained=%d columns=%d page=%s pageBytes=%d", s.SHA256, s.Bytes, s.CSV.Encoding, s.Selection.ScannedRows, len(s.Rows), len(s.CSV.Export.Columns), s.CSV.Export.PageSHA256, s.CSV.Export.PageBytes)
}

func TestMonthlyExportRejectsDriftAndDamagedExcludedTail(t *testing.T) {
	for _, scenario := range []string{"month", "age", "repeated field", "button", "html as csv", "csv month", "csv sex", "tail", "excluded code", "page redirect", "csv redirect", "oversized page", "oversized csv"} {
		t.Run(scenario, func(t *testing.T) {
			page, csv := exportHTML, exportCSV
			switch scenario {
			case "month":
				page = strings.ReplaceAll(page, `value="07"`, `value="08"`)
			case "age":
				page = strings.ReplaceAll(page, `name="sltArgTypeB" type="hidden" value="7"`, `name="sltArgTypeB" type="hidden" value="8"`)
			case "repeated field":
				page = strings.Replace(page, `<input name="sltArgTypeA"`, `<input name="sltArgTypeB" type="hidden" value="7"><input name="sltArgTypeA"`, 1)
			case "button":
				page = strings.ReplaceAll(page, "downloadCsvAge.do", "different.do")
			case "csv month":
				csv = strings.ReplaceAll(csv, "2026년07월", "2026년08월")
			case "csv sex":
				csv = strings.ReplaceAll(csv, "_남_", "_other_")
			case "tail":
				csv += `"unterminated`
			case "excluded code":
				csv = strings.Replace(csv, "Other (1100000000)", "Other invalid", 1)
			}
			i := documentInspector(t, monthlyDocumentURL, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() == monthlyDocumentURL {
					res := documentResponse(page)
					if scenario == "page redirect" {
						res.StatusCode = 307
						res.Header.Set("Location", "https://unregistered.example/")
					}
					if scenario == "oversized page" {
						res.ContentLength = 1<<20 + 1
					}
					return res, nil
				}
				res := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/csv"}}, Body: io.NopCloser(strings.NewReader(csv))}
				if scenario == "html as csv" {
					res.Header.Set("Content-Type", "text/html")
				}
				if scenario == "csv redirect" {
					res.StatusCode = 302
					res.Header.Set("Location", "https://unregistered.example/")
				}
				if scenario == "oversized csv" {
					res.ContentLength = 64<<20 + 1
				}
				return res, nil
			})
			c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
			if err != nil {
				t.Fatal(err)
			}
			// Retain only one matching row; the rest and excluded rows must still be checked.
			s, err := i.SampleExport(context.Background(), c, "mois-monthly-age-csv", exportParams(), 1)
			if err == nil || len(s.Rows) != 0 || s.SHA256 != "" {
				t.Fatalf("drift or partial scan became a successful observation: %+v %v", s, err)
			}
		})
	}
}

func TestMonthlyExportPreservesPublisherRowsAndActualSelection(t *testing.T) {
	i := documentInspector(t, monthlyDocumentURL, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != monthlyDocumentURL {
			t.Fatal("export crossed its public POST contract")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.PostForm.Get("sltArgTypeA") != "6" || r.PostForm.Get("sltArgTypeB") != "7" || r.PostForm.Get("searchMonthStart") != "07" || r.PostForm.Get("sltOrgLvl1") != "2800000000" {
			t.Fatalf("typed choices changed: %v", r.PostForm)
		}
		switch r.URL.String() {
		case monthlyDocumentURL:
			return documentResponse(exportHTML), nil
		case monthlyCSVURL:
			if r.PostForm.Get("state") != "3" || r.PostForm.Get("category") != "month" || r.PostForm.Get("sum") != "sum" || r.PostForm.Get("gender") != "gender" {
				t.Fatalf("CSV form drift: %v", r.PostForm)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/octet-stream;charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(exportCSV))}, nil
		default:
			return nil, fmt.Errorf("unexpected request")
		}
	})
	c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Exports) != 1 || c.Exports[0].ID != "mois-monthly-age-csv" || len(c.Assets) != 0 {
		t.Fatalf("export not distinguished from static assets: %+v", c)
	}
	s, err := i.SampleExport(context.Background(), c, "mois-monthly-age-csv", exportParams(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 2 || s.Rows[0]["행정구역"] != "Province (2800000000)" || s.Rows[1]["2026년07월_계_연령구간인구수"] != "10" || len(s.Rows[0]) != 13 {
		t.Fatalf("publisher rows changed or aggregates removed: %+v", s.Rows)
	}
	if s.Prefix || s.Selection == nil || s.Selection.ScannedRows != 3 || s.Selection.MatchedRows != 2 || s.Selection.ReturnedRows != 2 || !s.Selection.Exhausted || !reflect.DeepEqual(s.CSV.DataRecords, []int{2, 3}) || !reflect.DeepEqual(s.CSV.StartLines, []int{3, 4}) {
		t.Fatalf("lost full scan or original positions: %+v %+v", s.Selection, s.CSV)
	}
	p := s.CSV.Export
	if p == nil || p.Reference.ID != c.Exports[0].ID || p.Choices.Month != "2026-07" || p.Choices.AgeTo != 7 || p.PageSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(exportHTML))) || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(exportCSV))) || len(p.Columns) != 13 || p.Request.URL != monthlyCSVURL {
		t.Fatalf("missing inspected choices / original-byte provenance: %+v", p)
	}
}
