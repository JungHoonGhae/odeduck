package goalwork_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// This opt-in diagnostic replays hash-pinned original bytes through the actual
// inspectors, scanners and Engine. Portal listing/resolver metadata is a fixture,
// not a new historical lookup, live acquisition or autonomous discovery result.
func TestSourceComparisonReplaysOriginalG4Bytes(t *testing.T) {
	replayOriginalG4Comparison(t)
}

type originalG4ComparisonReplay struct {
	Original, Export goalwork.Acquired
	ExportRequest    goalwork.SampleRequest
	Comparison       goalwork.SourceComparison
}

func replayOriginalG4Comparison(t *testing.T) originalG4ComparisonReplay {
	t.Helper()
	paths := []string{os.Getenv("ODEDUCK_COMPARISON_ORIGINAL_CSV"), os.Getenv("ODEDUCK_COMPARISON_EXPORT_CSV"), os.Getenv("ODEDUCK_COMPARISON_EXPORT_HTML")}
	if paths[0] == "" && paths[1] == "" && paths[2] == "" {
		t.Skip("set all three ODEDUCK_COMPARISON_* source paths for preserved-byte replay")
	}
	hashes := []string{
		"6920fdafd269554d259e8498301f004c9799f499c38832de9352a0e7def916c5",
		"911940f3c38ffb7ed487a820607560616dedd5d226dcec1c0b9d7ffb65c4e47f",
		"1a1423e466dbc572ebee91b7cfeabf5ee7ac39085dc44c2b91ce3b689bfef982",
	}
	var bodies [][]byte
	for i, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != hashes[i] {
			t.Fatalf("source %d revision changed; preserve the prior oracle", i)
		}
		bodies = append(bodies, b)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{
		{PK: "15097972", Title: "comparison replay original", SvcType: "FILE"},
		{PK: "3033304", Title: "comparison replay export", SvcType: "FILE"},
	}}).Save(); err != nil {
		t.Fatal(err)
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: documentHTTP(func(r *http.Request) (*http.Response, error) {
		var body []byte
		contentType := "text/html; charset=UTF-8"
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/15097972/fileData.json":
			body = []byte(`{"alternateName":"July original replay","encodingFormat":"CSV"}`)
		case "https://www.data.go.kr/data/15097972/fileData.do":
			body = []byte(`<button onclick="fileDetailObj.fn_fileDataDown('15097972','uddi:replay','','1','3')">download</button>`)
		case "https://www.data.go.kr/tcs/dss/selectFileDataDownload.do":
			body = []byte(`{"status":true,"atchFileId":"FILE_REPLAY","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"July original replay","orginlFileNm":"original.csv","atchFileExtsn":"csv"}}`)
		case "https://www.data.go.kr/cmm/cmm/fileDownload.do?atchFileId=FILE_REPLAY&dataNm=July+original+replay&fileDetailSn=1":
			body, contentType = bodies[0], "application/octet-stream"
		case "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = []byte(`{}`)
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = []byte(`<li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li>`)
		case "https://jumin.mois.go.kr/ageStatMonth.do":
			body = bodies[2]
		case "https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3":
			body, contentType = bodies[1], "application/octet-stream; charset=utf-8"
		default:
			return nil, fmt.Errorf("replay has no HTTP fixture for %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}))
	policy := goalwork.Policy{EvidenceRecipient: "claude"}
	deps := goalwork.LiveDependencies(client, "", nil, catalog.Searcher{}, policy)
	var replay originalG4ComparisonReplay
	acquire := deps.Sample
	deps.Sample = func(ctx context.Context, request goalwork.SampleRequest, inspection goalwork.Inspection) (goalwork.Acquired, error) {
		a, err := acquire(ctx, request, inspection)
		if err == nil {
			if request.PK == "15097972" {
				replay.Original = a
			} else {
				replay.Export, replay.ExportRequest = a, request
			}
		}
		return a, err
	}
	e, err := goalwork.Start("Diagnostic of the original G4 source applicability, not goal completion", policy, deps)
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "source comparison diagnostic", Region: "Incheon", Period: "July 2026", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "source records"}}, Outputs: []goalwork.OutputRequirement{{ID: "code", Role: "r", Type: "string", Description: "original code"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "comparison replay", Role: "r"})
	for _, pk := range []string{"15097972", "3033304"} {
		advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: pk})
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "15097972", Delivery: "file", Asset: "original.csv", ScanCSV: true, Where: map[string]string{"시도명": "인천광역시"}}})
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "3033304", Delivery: "file", Operation: "mois-monthly-age-csv", Params: map[string]string{"month": "2026-07", "registration": "all", "provinceCode": "2800000000", "ageFrom": "6", "ageTo": "17"}}})
	left, right := e.View().Observations[0], e.View().Observations[1]
	if left.RowCount != 162 || right.RowCount != 177 || left.ContentSHA256 != hashes[0] || right.ContentSHA256 != hashes[1] || left.Selection.ScannedRows != 3619 || right.Selection.ScannedRows != 3919 || !left.Selection.Exhausted || !right.Selection.Exhausted || left.Selection.MatchedRows != left.RowCount || right.Selection.MatchedRows != right.RowCount || left.Selection.ReturnedRows != left.RowCount || right.Selection.ReturnedRows != right.RowCount || right.CSV.Export.PageSHA256 != hashes[2] {
		t.Fatalf("original acquisition/selection changed: rows=%d/%d selections=%+v/%+v csv=%s/%s page=%s", left.RowCount, right.RowCount, left.Selection, right.Selection, left.ContentSHA256, right.ContentSHA256, right.CSV.Export.PageSHA256)
	}
	comparison := goalwork.SourceComparison{
		Left:  goalwork.ComparisonSide{Observation: left.ID, RowsSHA256: left.RowsSHA256, Keys: []goalwork.ComparisonKey{{Field: "행정기관코드", Rule: "exact"}}},
		Right: goalwork.ComparisonSide{Observation: right.ID, RowsSHA256: right.RowsSHA256, Keys: []goalwork.ComparisonKey{{Field: "행정구역", Rule: "trailing_parenthesized_digits_v1", Digits: 10}}},
	}
	// Independent mapping fixed in verify-range.mjs before this operator existed.
	// Production code and runtime guidance contain none of these source answers.
	for sexIndex, sex := range []string{"계", "남", "여"} {
		for term := -2; term <= 17; term++ {
			if term >= 0 && term < 6 {
				continue
			}
			var fields []string
			label := "총인구수"
			if term == -2 {
				fields = []string{[]string{"계", "남자", "여자"}[sexIndex]}
			} else {
				from, through := term, term
				label = fmt.Sprintf("%d세", term)
				if term == -1 {
					from, through, label = 6, 17, "연령구간인구수"
				}
				for age := from; age <= through; age++ {
					if sexIndex != 2 {
						fields = append(fields, fmt.Sprintf("%d세남자", age))
					}
					if sexIndex != 1 {
						fields = append(fields, fmt.Sprintf("%d세여자", age))
					}
				}
			}
			operand := goalwork.Measure{Format: "decimal_v1", Unit: "persons"}
			if len(fields) == 1 {
				operand.Field = fields[0]
			} else {
				operand.Op, operand.Fields = "sum_fields", fields
			}
			comparison.Checks = append(comparison.Checks, goalwork.NumericComparison{ID: fmt.Sprintf("sex%d_term%d", sexIndex, term+2), Left: operand, Right: goalwork.Measure{Field: "2026년07월_" + sex + "_" + label, Format: "grouped_decimal_v1", Unit: "persons"}})
		}
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: left.PK, Delivery: "file", Compare: &comparison}})
	want := map[string]int{"leftRows": 162, "rightRows": 177, "matchedPairs": 162, "leftOnly": 0, "rightOnly": 15, "leftUnresolved": 0, "rightUnresolved": 0, "checks": 42, "comparisons": 6804, "equal": 6804, "different": 0, "missing": 0, "invalid": 0}
	if got := readComparisonSummary(t, e); !equalJSON(got, want) {
		t.Fatalf("product comparison differs from independent oracle: %v", got)
	}
	o := e.View().Observations[2]
	if o.RowCount != 70 || o.Comparison.Left.ContentSHA256 != hashes[0] || o.Comparison.Right.ContentSHA256 != hashes[1] {
		t.Fatal("report lost complete counts, diagnostics or pinned parents")
	}
	// Independent source row numbers include the header; CSV provenance does not.
	wantExtraLines := []int{1288, 1289, 1290, 1291, 1310, 1317, 1339, 1355, 1376, 1399, 1412, 1413, 1430, 1439, 1454}
	var extraLines []int
	for _, origin := range o.Comparison.Records[55:] {
		if len(origin.Left) != 0 || len(origin.Right) != 1 {
			t.Fatal("one-sided original record was merged or discarded")
		}
		extraLines = append(extraLines, right.CSV.StartLines[origin.Right[0]-1])
	}
	slices.Sort(extraLines)
	if !slices.Equal(extraLines, wantExtraLines) {
		t.Fatalf("changed extra-record addresses: %v", extraLines)
	}
	for _, branch := range [][2]int{{1357, 1453}, {1359, 1456}, {1362, 1459}, {1366, 1463}} {
		found := false
		for _, pair := range o.Comparison.Pairs {
			if left.CSV.StartLines[pair[0]-1] == branch[0] && right.CSV.StartLines[pair[1]-1] == branch[1] {
				found = true
			}
		}
		if !found {
			t.Fatalf("lost township-branch original pair: %v", branch)
		}
	}
	wire, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(wire), "서도면볼음출장소") || len(e.View().Evidence) != 1 || e.View().Artifact != nil || e.View().Status == "output_ready" {
		t.Fatal("comparison leaked source rows or became goal completion")
	}
	t.Log("preserved-byte product replay: 162/177 original rows, 162 unique pairs, 42 checks, 6804 equal positions, 15 right-only originals, four township branches retained; no autonomous/model/G4 completion claim")
	replay.Comparison = comparison
	return replay
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
