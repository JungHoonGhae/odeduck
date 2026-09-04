package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/oddsock/internal/fetch"
	"github.com/JungHoonGhae/oddsock/internal/portal"
)

type officialHTTPFunc func(context.Context, string, http.Header) (*fetch.Response, error)

func (f officialHTTPFunc) GetWithHeadersNoRedirect(ctx context.Context, rawURL string, headers http.Header) (*fetch.Response, error) {
	return f(ctx, rawURL, headers)
}

func officialHTTPHandler(t *testing.T, handler http.Handler) officialHTTPFunc {
	t.Helper()
	return func(ctx context.Context, rawURL string, headers http.Header) (*fetch.Response, error) {
		request := httptest.NewRequest(http.MethodGet, rawURL, nil).WithContext(ctx)
		request.Header = headers.Clone()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		result := recorder.Result()
		defer result.Body.Close()
		return &fetch.Response{
			Status:      result.StatusCode,
			ContentType: result.Header.Get("Content-Type"),
			Body:        append([]byte(nil), recorder.Body.Bytes()...),
		}, nil
	}
}

func sample() *Catalog {
	return &Catalog{
		SyncedAt: time.Now(),
		Type:     "API",
		Entries: []Entry{
			{PK: "1", Title: "행정안전부_무더위쉼터", Org: "행정안전부", ApplyCount: 2796,
				Desc: "폭염 대비 무더위쉼터 위치 정보"},
			{PK: "2", Title: "기상청_단기예보 조회서비스", Org: "기상청", ApplyCount: 63350,
				Desc: "동네예보 기온 강수 정보"},
			{PK: "3", Title: "행정안전부_폭염 인명피해", Org: "행정안전부", ApplyCount: 508,
				Desc: "온열질환자 지역별 현황"},
		},
	}
}

func isolateConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func TestCatalogSaveAtomicallyReplacesSnapshot(t *testing.T) {
	isolateConfigHome(t)
	first := &Catalog{SyncedAt: time.Now(), Type: "API", Entries: []Entry{{PK: "1", Title: "first"}}}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	second := &Catalog{SyncedAt: time.Now(), Type: "API", Entries: []Entry{{PK: "2", Title: "second"}}}
	if err := second.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].PK != "2" {
		t.Fatalf("loaded snapshot = %+v", loaded.Entries)
	}
	dir, err := portal.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, "catalog-*.tmp"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary files after save = %v, err=%v", temps, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "catalog.json")); err != nil {
		t.Fatalf("catalog stat: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("catalog mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestSyncAllKeepsAPIAndFileDatasetsInOneSnapshot(t *testing.T) {
	page := func(pk, href, title, format string) string {
		autoAPI := ""
		if strings.Contains(href, "fileData.do") {
			autoAPI = `<span class="krds-badge bg-light-primary">JSON + XML</span>`
		}
		return `<article class="apply-result-item">
			<div class="apply-result-link"><a href="` + href + `">` + title + `</a><span class="krds-badge" data-ext="` + format + `"></span>` + autoAPI + `</div>
			<div class="apply-result-summary">공식 설명</div>
			<div class="apply-result-category"><span class="krds-badge">사회복지</span><span class="krds-badge">공공기관</span></div>
			<div class="in-result-item"><ul><li><strong>제공기관</strong>테스트기관</li><li><strong>수정일</strong>2026-09-01</li><li><strong>조회수</strong>10</li><li><strong>활용신청</strong>3</li></ul></div>
		</article>`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("currentPage") != "1" {
			_, _ = w.Write([]byte(`<html></html>`))
			return
		}
		switch r.URL.Query().Get("dType") + "/" + r.URL.Query().Get("svcType") {
		case "API/":
			_, _ = w.Write([]byte(page("api", "/data/100/openapi.do", "장기요양기관 상세", "XML")))
		case "API/REST":
			_, _ = w.Write([]byte(page("api", "/data/100/openapi.do", "장기요양기관 상세", "XML")))
		case "API/LINK":
			_, _ = w.Write([]byte(`<html></html>`))
		case "FILE/":
			_, _ = w.Write([]byte(
				page("shared", "/data/100/fileData.do", "장기요양기관 상세 파일", "CSV") +
					page("file", "/data/200/fileData.do", "장기요양기관 평가 결과", "CSV"),
			))
		default:
			t.Fatalf("unexpected sync query: %s", r.URL.RawQuery)
		}
	}))
	defer srv.Close()

	client := portal.New(fetch.New(fetch.WithDelay(0)), portal.WithBaseURL(srv.URL))
	got, err := Sync(context.Background(), client, "ALL", 200, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "ALL" || len(got.Entries) != 2 {
		t.Fatalf("snapshot = type %q entries %+v", got.Type, got.Entries)
	}
	byPK := map[string]Entry{}
	for _, entry := range got.Entries {
		byPK[entry.PK] = entry
	}
	if byPK["100"].SvcType != SvcREST || strings.Join(byPK["100"].DataTypes, ",") != "API,FILE" ||
		strings.Join(byPK["100"].Formats, ",") != "XML,CSV,JSON" {
		t.Fatalf("API entry = %+v", byPK["100"])
	}
	if byPK["200"].SvcType != SvcFILE || strings.Join(byPK["200"].DataTypes, ",") != "FILE,API" || strings.Join(byPK["200"].Formats, ",") != "CSV,JSON,XML" {
		t.Fatalf("FILE entry = %+v", byPK["200"])
	}
	hits := got.Search("평가", 10, false).Hits
	if len(hits) != 1 || hits[0].NextAction != "inspect_dataset" || !strings.HasSuffix(hits[0].DetailURL, "/data/200/fileData.do") {
		t.Fatalf("FILE search handoff = %+v", hits)
	}
	dualHits := got.Search("상세", 10, false).Hits
	if len(dualHits) != 1 || !strings.HasSuffix(dualHits[0].DetailURL, "/data/100/fileData.do") {
		t.Fatalf("API+FILE search handoff = %+v", dualHits)
	}
}

func TestOfficialSourceBuildsCatalogFromDocumentedDatasetOperation(t *testing.T) {
	requests := map[string]int{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.odcloud.kr" {
			t.Fatalf("official catalogue origin = %s", r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Infuser secret-key" {
			t.Fatalf("Authorization = %q, want Infuser credential header", got)
		}
		if r.URL.Query().Get("perPage") != "2" {
			t.Fatalf("official catalogue request = %s", r.URL)
		}
		requests[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path + ":" + r.URL.Query().Get("page") {
		case "/api/15077093/v1/dataset:1":
			_, _ = w.Write([]byte(`{"currentCount":2,"matchCount":3,"totalCount":3,"data":[
				{"id":"100","title":"기상 관측 API","org_nm":"기상청","new_category_nm":"환경기상","download_cnt":42,"view_cnt":90,"updated_at":"2026-08-31","ext":"JSON+XML","page_url":"https://www.data.go.kr/data/100/openapi.do","swagger_json_url":"https://infuser.odcloud.kr/oas/100","desc":"관측 설명"},
				{"id":"200","title":"외부 제공 API","org_nm":"서울시","new_category_nm":"공공행정","download_cnt":7,"view_cnt":30,"updated_at":"2026-08-30","ext":"JSON","page_url":"https://www.data.go.kr/data/200/openapi.do","swagger_json_url":"-","desc":"LINK 설명"}
			]}`))
		case "/api/15077093/v1/dataset:2":
			_, _ = w.Write([]byte(`{"currentCount":1,"matchCount":3,"totalCount":3,"data":[
				{"id":"300","title":"상권 매출 파일","org_nm":"서울신용보증재단","new_category_nm":"산업경제","download_cnt":3,"view_cnt":120,"updated_at":"2026-08-29","ext":"CSV","page_url":"https://www.data.go.kr/data/300/fileData.do","swagger_json_url":"-","desc":"파일 설명"}
			]}`))
		case "/api/15077093/v1/open-data-list:1":
			_, _ = w.Write([]byte(`{"currentCount":2,"matchCount":2,"totalCount":2,"data":[
				{"list_id":"100","list_title":"기상 관측 API","api_type":"REST","request_cnt":50,"data_format":"JSON","operation_seq":"1","operation_nm":"관측 조회","operation_url":"https://apis.data.go.kr/weather/getObservation","request_param_nm_en":"\"base_date\",\"nx\"","is_confirmed_for_dev_nm":"자동승인","is_confirmed_for_prod_nm":"심의승인","is_deleted":"N","is_list_deleted":"N"},
				{"list_id":"200","list_title":"외부 제공 API","api_type":"LINK","link_url":"https://data.seoul.go.kr/example","request_cnt":9,"is_deleted":"N","is_list_deleted":"N"}
			]}`))
		case "/api/15077093/v1/file-data-list:1":
			_, _ = w.Write([]byte(`{"currentCount":2,"matchCount":2,"totalCount":2,"data":[
				{"list_id":"100","list_title":"기상 관측 API","title":"기상 관측 파일 2026","org_nm":"기상청","ext":"CSV","download_cnt":60,"updated_at":"2026-08-31","is_deleted":"N","is_list_deleted":"N"},
				{"list_id":"300","list_title":"상권 매출 파일","title":"상권 매출 2026","org_nm":"서울신용보증재단","ext":"CSV","download_cnt":4,"updated_at":"2026-08-30","is_deleted":"N","is_list_deleted":"N"}
			]}`))
		default:
			t.Fatalf("unexpected official request %s", r.URL)
		}
	})

	source := newOfficialSource(officialHTTPHandler(t, handler), "secret-key")
	var progress []int
	got, err := source.Sync(context.Background(), "ALL", 2, func(n int) { progress = append(progress, n) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceOfficial || got.Type != "ALL" || len(got.Entries) != 3 ||
		requests["/api/15077093/v1/dataset"] != 2 || requests["/api/15077093/v1/open-data-list"] != 1 ||
		requests["/api/15077093/v1/file-data-list"] != 1 {
		t.Fatalf("catalog = %+v, requests=%v", got, requests)
	}
	byPK := map[string]Entry{}
	for _, entry := range got.Entries {
		byPK[entry.PK] = entry
	}
	if byPK["100"].SvcType != SvcREST || strings.Join(byPK["100"].DataTypes, ",") != "API,FILE" || strings.Join(byPK["100"].Formats, ",") != "JSON,XML,CSV" || byPK["100"].ApplyCount != 60 {
		t.Fatalf("REST entry = %+v", byPK["100"])
	}
	api := byPK["100"].OfficialAPI
	if api == nil || api.APIType != SvcREST || api.DevApproval != "자동승인" || api.ProdApproval != "심의승인" ||
		len(api.Operations) != 1 || api.Operations[0].Name != "관측 조회" ||
		strings.Join(api.Operations[0].RequestNames, ",") != "base_date,nx" {
		t.Fatalf("official operation contract = %+v", api)
	}
	if byPK["200"].SvcType != SvcLINK || byPK["200"].ApplyCount != 9 {
		t.Fatalf("LINK entry = %+v", byPK["200"])
	}
	if byPK["300"].SvcType != SvcFILE || strings.Join(byPK["300"].DataTypes, ",") != "FILE" || byPK["300"].ViewCount != 120 {
		t.Fatalf("FILE entry = %+v", byPK["300"])
	}
	if strings.Join(intStrings(progress), ",") != "2,3,3,3" {
		t.Fatalf("progress = %v", progress)
	}
}

func TestOfficialEntryPreservesUnknownDeliveryButRejectsDeletedRows(t *testing.T) {
	unknown, ok := entryFromOfficial(officialRow{
		ID: "400", Title: "신규 제공 형태", PageURL: "https://publisher.example/dataset/400",
	})
	if !ok || unknown.PK != "400" || unknown.SvcType != "" || len(unknown.DataTypes) != 0 {
		t.Fatalf("unknown official row = %+v, ok=%v", unknown, ok)
	}
	if _, ok := entryFromOfficial(officialRow{
		ID: "500", Title: "폐기 데이터", PageURL: "https://www.data.go.kr/data/500/fileData.do", IsDeleted: "Y",
	}); ok {
		t.Fatal("official row marked deleted must not enter the release catalogue")
	}
}

func TestOfficialSourceRejectsKeysThatDoNotActuallyPaginate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"page":1,"perPage":10,"currentCount":1,"totalCount":2,"data":[
			{"id":"100","title":"첫 행","page_url":"https://www.data.go.kr/data/100/fileData.do","is_deleted":"N"}
		]}`))
	})
	_, err := newOfficialSource(officialHTTPHandler(t, handler), "test-only-key").Sync(context.Background(), "ALL", 1000, nil)
	if err == nil || !strings.Contains(err.Error(), "pagination") {
		t.Fatalf("non-paginating credential error = %v", err)
	}
}

func TestOfficialSourceTreatsRedirectAsFailure(t *testing.T) {
	var requested string
	transport := officialHTTPFunc(func(_ context.Context, rawURL string, _ http.Header) (*fetch.Response, error) {
		requested = rawURL
		return &fetch.Response{Status: http.StatusFound}, nil
	})
	_, err := newOfficialSource(transport, "test-only-key").Sync(context.Background(), "ALL", 10, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("redirect error = %v", err)
	}
	if !strings.HasPrefix(requested, DefaultOfficialBaseURL+officialDatasetPath+"?") {
		t.Fatalf("credentialed origin = %q", requested)
	}
}

func TestInstallSnapshotUpgradesLessCompleteLocalCatalogAndPreservesNewerOfficial(t *testing.T) {
	isolateConfigHome(t)
	bundle := func(catalog *Catalog) []byte {
		var compressed bytes.Buffer
		zw := gzip.NewWriter(&compressed)
		if err := json.NewEncoder(zw).Encode(catalog); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return compressed.Bytes()
	}
	prebuilt := &Catalog{
		SyncedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Type: "ALL", Source: SourceOfficial,
		Entries: []Entry{{PK: "100", Title: "공식 snapshot", SvcType: SvcREST, DataTypes: []string{"API"}}},
	}
	validated, err := ValidateSnapshot(bytes.NewReader(bundle(prebuilt)))
	if err != nil {
		t.Fatal(err)
	}
	if validated.Installed || validated.Reason != "validated" || validated.Entries != 1 {
		t.Fatalf("validation result = %+v", validated)
	}
	if _, err := Load(); err == nil {
		t.Fatal("validation-only path must not install local state")
	}
	result, err := InstallSnapshot(bytes.NewReader(bundle(prebuilt)))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Installed || result.Entries != 1 || result.Source != SourceOfficial {
		t.Fatalf("install result = %+v", result)
	}

	newer := &Catalog{
		SyncedAt: prebuilt.SyncedAt.Add(time.Hour), Type: "ALL", Source: SourceWeb,
		Entries: []Entry{{PK: "200", Title: "사용자 최신 snapshot", SvcType: SvcFILE, DataTypes: []string{"FILE"}}},
	}
	if err := newer.Save(); err != nil {
		t.Fatal(err)
	}
	result, err = InstallSnapshot(bytes.NewReader(bundle(prebuilt)))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Installed {
		t.Fatalf("less complete web snapshot must not block official prebuilt: %+v", result)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].PK != "100" {
		t.Fatalf("official prebuilt was not installed: %+v", loaded)
	}

	newerOfficial := &Catalog{
		SyncedAt: prebuilt.SyncedAt.Add(2 * time.Hour), Type: "ALL", Source: SourceOfficial,
		Entries: []Entry{{PK: "300", Title: "더 최신 공식 snapshot", SvcType: SvcREST, DataTypes: []string{"API"}}},
	}
	if err := newerOfficial.Save(); err != nil {
		t.Fatal(err)
	}
	result, err = InstallSnapshot(bytes.NewReader(bundle(prebuilt)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Installed || result.Reason != "local_newer_or_complete" {
		t.Fatalf("newer official preservation result = %+v", result)
	}
}

func TestInstallSnapshotRejectsNonOfficialAndDuplicateNodes(t *testing.T) {
	isolateConfigHome(t)
	encode := func(catalog *Catalog) []byte {
		var compressed bytes.Buffer
		zw := gzip.NewWriter(&compressed)
		_ = json.NewEncoder(zw).Encode(catalog)
		_ = zw.Close()
		return compressed.Bytes()
	}
	for name, candidate := range map[string]*Catalog{
		"web source": {
			SyncedAt: time.Now(), Type: "ALL", Source: SourceWeb,
			Entries: []Entry{{PK: "100", Title: "web"}},
		},
		"duplicate PK": {
			SyncedAt: time.Now(), Type: "ALL", Source: SourceOfficial,
			Entries: []Entry{{PK: "100", Title: "first"}, {PK: "100", Title: "second"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := InstallSnapshot(bytes.NewReader(encode(candidate))); err == nil {
				t.Fatal("invalid prebuilt snapshot was accepted")
			}
		})
	}
}

func intStrings(values []int) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = fmt.Sprint(value)
	}
	return out
}

// Every term must match, and matches rank by demand — the agent should see the
// heavily used dataset first rather than whatever happened to be stored first.
func TestSearchRanksByDemand(t *testing.T) {
	c := sample()
	r := c.Search("기온", 10, false)
	hits, total := r.Hits, r.Total
	if total != 1 || len(hits) != 1 || hits[0].PK != "2" {
		t.Fatalf("기온 → %d hits (total %d), first %+v", len(hits), total, hits)
	}

	r = c.Search("행정안전부", 10, false)
	hits, total = r.Hits, r.Total
	if total != 2 {
		t.Fatalf("행정안전부 → total %d, want 2", total)
	}
	if hits[0].ApplyCount < hits[1].ApplyCount {
		t.Errorf("not ranked by applyCount: %d then %d", hits[0].ApplyCount, hits[1].ApplyCount)
	}
}

func TestSearchRanksFilesByViewsWhenApplicationsDoNotExist(t *testing.T) {
	c := &Catalog{Entries: []Entry{
		{PK: "quiet", Title: "장기요양기관 평가", SvcType: SvcFILE, ViewCount: 10},
		{PK: "used", Title: "장기요양기관 평가", SvcType: SvcFILE, ViewCount: 900},
	}}
	hits := c.Search("장기요양기관 평가", 10, false).Hits
	if len(hits) != 2 || hits[0].PK != "used" || hits[0].ViewCount != 900 {
		t.Fatalf("FILE demand ranking = %+v", hits)
	}
}

// Precision first: when entries match every term, only those are returned and
// the result is not marked relaxed.
func TestSearchPrefersEveryTermMatching(t *testing.T) {
	r := sample().Search("행정안전부 온열질환자", 10, false)
	if r.Relaxed {
		t.Error("both terms matched, should not relax")
	}
	if r.Total != 1 {
		t.Errorf("both-terms search → total %d, want 1", r.Total)
	}
}

// Recall as a fallback: a query no entry fully satisfies must still answer, and
// must say it loosened the query rather than pretending the result is exact.
func TestSearchRelaxesWhenNothingMatchesEveryTerm(t *testing.T) {
	r := sample().Search("행정안전부 존재하지않는말", 10, false)
	if !r.Relaxed {
		t.Fatal("no entry has both terms — expected a relaxed result, not silence")
	}
	if r.Total == 0 {
		t.Fatal("relaxed search returned nothing")
	}
	if r.Hits[0].Matched != 1 {
		t.Errorf("relaxed hit should report how many terms it matched, got %d", r.Hits[0].Matched)
	}
}

// A place named in the user's question is a high-signal title match. Generic
// prose in another region's description must not outrank the local dataset just
// because it happens to repeat more workflow words.
func TestSearchRelaxedKeepsGeographicTitleMatchAheadOfDescriptionNoise(t *testing.T) {
	c := &Catalog{Entries: []Entry{
		{
			PK: "jeju", Title: "제주 실종 접수 해제 현황", SvcType: SvcFILE,
			Desc: "성인 현황",
		},
		{
			PK: "jeonbuk", Title: "전북 실종 해제 현황", SvcType: SvcFILE,
			Desc: "성인 신고 접수 처리 미해제 현황",
		},
	}}

	r := c.Search("제주 성인 실종 신고 접수 처리 해제 미해제", 2, false)
	if !r.Relaxed || len(r.Hits) != 2 {
		t.Fatalf("relaxed result = %+v", r)
	}
	if r.Hits[0].PK != "jeju" {
		t.Fatalf("ranked hits = %+v, want geographic title match first", r.Hits)
	}
}

func TestSearchPlanRecentKeepsGeographicTitleMatchAheadOfDescriptionNoise(t *testing.T) {
	c := &Catalog{Entries: []Entry{
		{
			PK: "jeju", Title: "제주 실종 접수 해제 현황", SvcType: SvcFILE,
			Desc: "성인 현황", ModifiedAt: "2025-01-01",
		},
		{
			PK: "jeonbuk", Title: "전북 실종 해제 현황", SvcType: SvcFILE,
			Desc: "성인 신고 접수 처리 미해제 현황", ModifiedAt: "2026-12-31",
		},
	}}

	r := c.SearchPlan(QueryPlan{
		Intent: "제주 성인 실종 신고 접수 처리 해제 미해제",
		Limit:  2, Ranking: RankRecent,
	})
	if !r.Relaxed || len(r.Hits) != 2 {
		t.Fatalf("relaxed recent result = %+v", r)
	}
	if r.Hits[0].PK != "jeju" {
		t.Fatalf("recent-ranked hits = %+v, want geographic relevance before recency", r.Hits)
	}
}

// A single unmatchable word is a genuine miss, not something to widen.
func TestSearchSingleTermMissStaysEmpty(t *testing.T) {
	if r := sample().Search("존재하지않는말", 10, false); r.Total != 0 || r.Relaxed {
		t.Errorf("single unmatchable term → total %d relaxed %v, want 0/false", r.Total, r.Relaxed)
	}
}

// The query is written by a person or an agent, not filled into a search form:
// particles and filler words must not decide whether it finds anything.
func TestSearchHandlesNaturalLanguage(t *testing.T) {
	plain := sample().Search("폭염 인명피해", 10, false)
	natural := sample().Search("폭염으로 인한 인명피해 데이터 알려줘", 10, false)
	if natural.Total != plain.Total {
		t.Errorf("natural phrasing → %d hits, keyword phrasing → %d; should agree", natural.Total, plain.Total)
	}
	if natural.Relaxed {
		t.Errorf("natural phrasing should not need relaxing, terms=%v", natural.Terms)
	}
}

// Trimming must not eat a word: 고가 is not 고 + the particle 가.
func TestQueryTermsKeepsShortWordsIntact(t *testing.T) {
	for q, want := range map[string]string{
		"폭염에":   "폭염",
		"광진구에서": "광진구",
		"고가":    "고가",
		"실거래가":  "실거래",
	} {
		if got := queryTerms(q); len(got) != 1 || got[0] != want {
			t.Errorf("queryTerms(%q) = %v, want [%s]", q, got, want)
		}
	}
}

func TestQueryTermsSplitsAgentPunctuation(t *testing.T) {
	got := queryTerms("공매·압류재산/매각-현황")
	want := []string{"공매", "압류재산", "매각", "현황"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("queryTerms punctuation = %v, want %v", got, want)
	}
}

func TestQueryTermsDropsFillersAfterParticleStripping(t *testing.T) {
	got := queryTerms("폭염 데이터를 자료를 정보를 찾아줘")
	if len(got) != 1 || got[0] != "폭염" {
		t.Fatalf("queryTerms inflected fillers = %v, want [폭염]", got)
	}
}

func TestSearchPlanRecentInspectsBeyondExternalResultCap(t *testing.T) {
	c := &Catalog{}
	for i := 0; i < 150; i++ {
		c.Entries = append(c.Entries, Entry{
			PK: fmt.Sprintf("%d", i), Title: "상권 매출", SvcType: SvcREST,
			ApplyCount: 1000 - i, ModifiedAt: fmt.Sprintf("2025-01-%02d", i%28+1),
		})
	}
	c.Entries[149].ModifiedAt = "2026-08-31"
	res := c.SearchPlan(QueryPlan{Concepts: []string{"상권"}, Limit: 1, RESTOnly: true, Ranking: RankRecent})
	if len(res.Hits) != 1 || res.Hits[0].PK != "149" {
		t.Fatalf("recent result = %+v, want low-demand newest entry beyond top 100", res.Hits)
	}
}

func TestSearchPlanBalancedPreservesDemandRecentInterleave(t *testing.T) {
	c := &Catalog{Entries: []Entry{
		{PK: "popular-old", Title: "상권 현황", ApplyCount: 1000, ModifiedAt: "2020-01-01"},
		{PK: "popular-new", Title: "상권 현황", ApplyCount: 900, ModifiedAt: "2025-01-01"},
		{PK: "quiet-newest", Title: "상권 현황", ApplyCount: 1, ModifiedAt: "2026-09-01"},
		{PK: "quiet-new", Title: "상권 현황", ApplyCount: 0, ModifiedAt: "2026-08-01"},
	}}
	res := c.SearchPlan(QueryPlan{Concepts: []string{"상권"}, Limit: 4, Ranking: RankBalanced})
	want := []string{"popular-old", "quiet-newest", "popular-new", "quiet-new"}
	if len(res.Hits) != len(want) {
		t.Fatalf("hits = %+v", res.Hits)
	}
	for i, pk := range want {
		if res.Hits[i].PK != pk {
			t.Fatalf("rank %d = %s, want %s: %+v", i, res.Hits[i].PK, pk, res.Hits)
		}
	}
}

// The description is what makes matching work, and is exactly what must not be
// handed back — ten descriptions is thousands of characters of an agent's context.
func TestSearchDoesNotReturnDescriptions(t *testing.T) {
	hits := sample().Search("폭염", 10, false).Hits
	if len(hits) == 0 {
		t.Fatal("expected description-matched hits")
	}
	for _, h := range hits {
		if strings.Contains(h.Title, "온열질환자 지역별") {
			t.Error("description leaked into the title field")
		}
	}
	// Hit has no Desc field at all; assert the type stays that way.
	var h any = hits[0]
	if _, bad := h.(interface{ GetDesc() string }); bad {
		t.Error("Hit must not expose a description")
	}
}

// A capped result set still reports how many matched, so a caller knows to narrow.
func TestSearchCapsButReportsTotal(t *testing.T) {
	r := sample().Search("", 2, false)
	hits, total := r.Hits, r.Total
	if len(hits) != 2 || total != 3 {
		t.Errorf("limit 2 over 3 entries → %d hits, total %d", len(hits), total)
	}
}

func TestStale(t *testing.T) {
	c := sample()
	if c.Stale() {
		t.Error("just-synced catalogue must not be stale")
	}
	c.SyncedAt = time.Now().Add(-StaleAfter - time.Hour)
	if !c.Stale() {
		t.Error("old catalogue must be stale")
	}
}

func TestCatalogCoverageDistinguishesFreshButNarrowSnapshots(t *testing.T) {
	api := &Catalog{Type: "API"}
	if !api.CoversType("API") || api.CoversType("ALL") || api.CoversType("FILE") {
		t.Fatalf("API coverage is wrong")
	}
	all := &Catalog{Type: "ALL"}
	for _, requested := range []string{"ALL", "API", "FILE"} {
		if !all.CoversType(requested) {
			t.Errorf("ALL should cover %s", requested)
		}
	}
}

func TestPreserveOnAutoSyncRejectsCoverageDowngrade(t *testing.T) {
	entry := func(pk string, deliveries ...string) Entry {
		return Entry{PK: pk, Title: pk, DataTypes: deliveries}
	}
	current := &Catalog{Type: "ALL", Source: SourceCombined, Entries: []Entry{
		entry("1", "API", "FILE"), entry("2", "API", "FILE"), entry("3", "FILE"),
	}}
	fileOnly := &Catalog{Type: "ALL", Source: SourceOfficialFile, Entries: []Entry{
		entry("1", "FILE"), entry("2", "FILE"), entry("3", "FILE"),
	}}
	if !PreserveOnAutoSync(current, fileOnly) {
		t.Fatal("auto sync should preserve a composite snapshot over CSV-only fallback")
	}
	refreshed := &Catalog{Type: "ALL", Source: SourceCombined, Entries: append([]Entry(nil), current.Entries...)}
	if PreserveOnAutoSync(current, refreshed) {
		t.Fatal("same-quality composite refresh should be accepted")
	}
	catastrophic := &Catalog{Type: "ALL", Source: SourceCombined, Entries: []Entry{entry("1", "FILE")}}
	if !PreserveOnAutoSync(current, catastrophic) {
		t.Fatal("automatic refresh should stop a material same-source coverage collapse")
	}
}

// A word in the dataset's name is a stronger signal than the same word buried in
// its description — otherwise a relaxed search recommends whatever popular
// dataset happens to mention the word.
func TestSearchPrefersNameMatchesOverDescriptionMatches(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "1", Title: "인기 있는 교통 통계", ApplyCount: 9000, Desc: "지역 상권 변화도 참고할 수 있습니다"},
		{PK: "2", Title: "소상공인시장진흥공단_상권정보", ApplyCount: 10},
	}}
	r := c.Search("상권", 10, false)
	if r.Hits[0].PK != "2" {
		t.Errorf("name match should outrank a description mention; got pk=%s first", r.Hits[0].PK)
	}
}

// A LINK dataset has no spec on the portal, so an agent heading for describe →
// call must be able to exclude them: applying for one spends a real application
// on something it cannot call. Unlabelled entries are excluded too — restOnly
// promises "the portal says REST", and an unverified guess is not that.
func TestSearchRESTOnlyExcludesLinkAndUnknown(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "rest", Title: "폭염 정보", SvcType: SvcREST, ApplyCount: 1},
		{PK: "link", Title: "폭염 연계", SvcType: SvcLINK, ApplyCount: 900},
		{PK: "unknown", Title: "폭염 기타", ApplyCount: 500},
	}}
	if all := c.Search("폭염", 10, false); all.Total != 3 {
		t.Errorf("unfiltered search → %d, want 3", all.Total)
	}
	r := c.Search("폭염", 10, true)
	if r.Total != 1 || r.Hits[0].PK != "rest" {
		t.Fatalf("restOnly → total %d first %v, want 1 rest", r.Total, r.Hits)
	}
	if r.Hits[0].SvcType != SvcREST {
		t.Errorf("hit should carry its service type, got %q", r.Hits[0].SvcType)
	}
}

// Semantic interpretation belongs to the MCP host model, not a growing list of
// hard-coded synonyms in the catalogue. SearchPlan executes those inferred data
// axes independently and interleaves them, so one prolific publisher cannot
// bury every other way of satisfying a broad user goal.
func TestSearchPlanDiversifiesSemanticQueries(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "onbid-list", Title: "온비드 공매 부동산 물건", SvcType: SvcREST, ApplyCount: 1200},
		{PK: "onbid-detail", Title: "온비드 공매 부동산 상세", SvcType: SvcREST, ApplyCount: 1100},
		{PK: "bid", Title: "나라장터 입찰 공고", SvcType: SvcREST, ApplyCount: 9000},
		{PK: "auction", Title: "도매시장 실시간 경매 가격", SvcType: SvcREST, ApplyCount: 800},
		{PK: "noise", Title: "인기 관광 정보", SvcType: SvcREST, ApplyCount: 50000},
	}}

	r := c.SearchPlan(QueryPlan{
		Intent: "돈이 될 만한 데이터를 찾아줘",
		Concepts: []string{
			"온비드 공매", "나라장터 입찰", "도매시장 경매",
		},
		Limit: 3, RESTOnly: true, IncludePreviews: true,
	})
	if r.Mode != SearchModePlanned {
		t.Fatalf("mode = %q, want %q", r.Mode, SearchModePlanned)
	}
	if len(r.Hits) != 3 {
		t.Fatalf("hits = %+v, want one candidate from each semantic query", r.Hits)
	}
	want := []string{"onbid-list", "bid", "auction"}
	for i, pk := range want {
		if r.Hits[i].PK != pk {
			t.Errorf("hit[%d] = %s, want %s; hits=%+v", i, r.Hits[i].PK, pk, r.Hits)
		}
		if r.Hits[i].MatchedQuery == "" {
			t.Errorf("hit[%d] does not explain which semantic query found it", i)
		}
	}
	if r.Total != 4 { // both Onbid rows plus one row from each other axis
		t.Errorf("total = %d, want 4 distinct candidates", r.Total)
	}
}

func TestSearchPlanFallsBackToOrdinarySearch(t *testing.T) {
	c := sample()
	plain := c.Search("폭염", 10, false)
	planned := c.SearchPlan(QueryPlan{Intent: "폭염", Limit: 10})
	if planned.Mode != SearchModeLexical || len(planned.Hits) != len(plain.Hits) || planned.Hits[0].PK != plain.Hits[0].PK {
		t.Fatalf("unplanned search changed behavior: plain=%+v planned=%+v", plain, planned)
	}
}

func TestSearchPlanBuildsConnectionsOnlyFromExplicitBridgeSelections(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "온비드 공매 물건", Org: "한국자산관리공사", SvcType: SvcREST},
		{PK: "demand", Title: "상권 점포 개폐업 이력", Org: "부산광역시", SvcType: SvcREST},
		{PK: "rent", Title: "오피스텔 전월세 실거래", Org: "국토교통부", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Intent: "공매 부동산의 실제 수요를 판단",
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "상권 수요", Query: "상권 점포 개폐업", Contribution: "쇠퇴 상권인지 가격 기회인지 구분", Edge: EdgeHypothesis{
				Kinds: []string{"spatial", "temporal"}, ExpectedKeys: []string{"법정동코드", "기준연월"},
			}},
			{Role: "임대 수익", Query: "오피스텔 전월세", Contribution: "예상 임대 현금흐름 계산", Edge: EdgeHypothesis{
				Kinds: []string{"spatial", "temporal"}, ExpectedKeys: []string{"법정동코드", "계약연월"},
			}},
		},
		AnchorPKs: []string{"anchor"},
		BridgeSelections: []BridgeSelection{
			{PK: "demand", Role: "상권 수요", IncrementalValue: "쇠퇴 상권인지 가격 기회인지 구분",
				WhyCandidate: "점포 개폐업 이력이라는 제목이 상권의 실제 수요 변화를 뒷받침함",
				Edge:         EdgeHypothesis{Kinds: []string{"spatial", "temporal"}, ExpectedKeys: []string{"법정동코드", "기준연월"}}},
			{PK: "rent", Role: "임대 수익", IncrementalValue: "예상 임대 현금흐름 계산",
				WhyCandidate: "전월세 실거래라는 제목이 임대 현금흐름의 관측값을 제공함",
				Edge:         EdgeHypothesis{Kinds: []string{"spatial", "temporal"}, ExpectedKeys: []string{"법정동코드", "계약연월"}}},
		},
		Limit: 10, RESTOnly: true, MaxConnections: 2,
	})
	if len(res.Anchors) != 1 || res.Anchors[0].PK != "anchor" {
		t.Fatalf("anchors = %+v", res.Anchors)
	}
	if len(res.Connections) != 2 {
		t.Fatalf("connections = %+v, want one per role", res.Connections)
	}
	for _, connection := range res.Connections {
		if connection.Status != ConnectionStatusCandidate {
			t.Errorf("status = %q, search must never claim verification", connection.Status)
		}
		if connection.Anchor.PK != "anchor" || connection.Bridge.PK == "anchor" {
			t.Errorf("bad pair = %+v", connection)
		}
		if len(connection.EvidenceRequired) < 3 || connection.ClaimBoundary == "" {
			t.Errorf("candidate lacks verification boundary: %+v", connection)
		}
	}
}

func TestSearchPlanExposesRoleDiverseOptionsWithoutInflatingFinalCards(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "요양시설 매물", SvcType: SvcREST},
		{PK: "demand-a", Title: "시군구 장기요양 인정자", SvcType: SvcREST, ApplyCount: 10},
		{PK: "demand-b", Title: "시군구 고령인구 전망", SvcType: SvcFILE, ApplyCount: 9},
		{PK: "property-a", Title: "건축물대장 용도 면적", SvcType: SvcREST, ApplyCount: 8},
		{PK: "property-b", Title: "토지이용계획 용도지역", SvcType: SvcFILE, ApplyCount: 7},
	}}
	spatial := EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"시군구코드"}}
	res := c.SearchPlan(QueryPlan{
		Intent: "요양시설 인수 기회",
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "요양시설 매물"},
			{Role: "지역 수요", Query: "시군구 장기요양 고령인구", Contribution: "공급 대비 잠재 수요 비교", Edge: spatial},
			{Role: "부동산 제약", Query: "건축물대장 토지이용계획", Contribution: "시설 운영과 증축 제약 확인", Edge: spatial},
		},
		AnchorPKs: []string{"anchor"}, Limit: 1,
	})
	if len(res.Connections) != 0 {
		t.Fatalf("unselected options became connection cards: %+v", res.Connections)
	}
	if len(res.ConnectionOptions) != 2 {
		t.Fatalf("option groups = %+v", res.ConnectionOptions)
	}
	seen := map[string]bool{}
	for _, group := range res.ConnectionOptions {
		if len(group.Nodes) != 2 {
			t.Fatalf("role %q nodes = %+v, want two alternatives despite result limit", group.Role, group.Nodes)
		}
		for _, node := range group.Nodes {
			seen[node.PK] = true
		}
	}
	for _, pk := range []string{"demand-a", "demand-b", "property-a", "property-b"} {
		if !seen[pk] {
			t.Errorf("option pool omitted %s: %+v", pk, res.ConnectionOptions)
		}
	}
	if got := res.ConnectionOptions[0].Nodes[1]; got.SvcType != SvcFILE {
		t.Fatalf("file discovery metadata was not preserved: %+v", got)
	}
}

func TestConnectionEvidenceIncludesFileAlternativeWhenRESTIsPrimary(t *testing.T) {
	evidence := evidenceFor(
		EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}},
		Hit{PK: "dual", SvcType: SvcREST, DataTypes: []string{"API", "FILE"}},
	)
	if len(evidence) == 0 || !strings.Contains(evidence[0], "FILE 노드") {
		t.Fatalf("API+FILE evidence = %v, want FILE inspection guidance", evidence)
	}
}

func TestConnectionOptionsBoundRolesNodesAndDuplicatePKs(t *testing.T) {
	entries := []Entry{{PK: "anchor", Title: "기준 데이터", SvcType: SvcREST}}
	axes := []DiscoveryAxis{{Role: "anchor", Query: "기준 데이터"}}
	for role := 1; role <= 7; role++ {
		axes = append(axes, DiscoveryAxis{
			Role: fmt.Sprintf("역할%d", role), Query: fmt.Sprintf("후보%d", role), Contribution: "새 판단",
			Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"시군구코드"}},
		})
		for candidate := 1; candidate <= 4; candidate++ {
			pk := fmt.Sprintf("r%d-%d", role, candidate)
			if candidate == 1 && role > 1 {
				pk = "shared"
			}
			entries = append(entries, Entry{PK: pk, Title: fmt.Sprintf("후보%d 자료 %d", role, candidate), SvcType: SvcREST})
		}
	}
	res := (&Catalog{Entries: entries}).SearchPlan(QueryPlan{Axes: axes, AnchorPKs: []string{"anchor"}, Limit: 100})
	if len(res.ConnectionOptions) != 7 {
		t.Fatalf("roles = %d, want 7: %+v", len(res.ConnectionOptions), res.ConnectionOptions)
	}
	seen := map[string]bool{}
	for _, group := range res.ConnectionOptions {
		if len(group.Nodes) > MaxOptionsPerRole {
			t.Fatalf("role %q nodes = %d", group.Role, len(group.Nodes))
		}
		for _, node := range group.Nodes {
			if node.PK == "anchor" || seen[node.PK] {
				t.Fatalf("duplicate or anchor option: %+v", node)
			}
			seen[node.PK] = true
		}
	}
	if len(seen) > 7*MaxOptionsPerRole {
		t.Fatalf("options = %d, want at most %d", len(seen), 7*MaxOptionsPerRole)
	}
}

func TestSearchPlanRejectsSelectionWithoutCandidateEvidence(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "bridge", Title: "상권 점포 개폐업", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "상권 수요", Query: "상권 점포", Contribution: "쇠퇴 상권을 구분",
				Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{{
			PK: "bridge", Role: "상권 수요", IncrementalValue: "쇠퇴 상권을 구분",
			Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}},
		}},
		Limit: 10, RESTOnly: true,
	})
	if len(res.Connections) != 0 || res.Abstention == nil {
		t.Fatalf("selection without whyCandidate must abstain: %+v", res)
	}
}

func TestSearchPlanRejectsSelectionOutsideCurrentHits(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "visible", Title: "상권 점포 개폐업", SvcType: SvcREST},
		{PK: "unseen", Title: "하천 수질 측정", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "상권 수요", Query: "상권 점포", Contribution: "쇠퇴 상권을 구분",
				Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{{
			PK: "unseen", Role: "상권 수요", IncrementalValue: "쇠퇴 상권을 구분", WhyCandidate: "검색 밖 후보",
			Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}},
		}},
		Limit: 10, RESTOnly: true,
	})
	if len(res.Connections) != 0 || res.Abstention == nil {
		t.Fatalf("selection outside current hits must abstain: %+v", res)
	}
}

func TestSearchPlanRejectsAnchorNotRetrievedUnderAnchorRole(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "arbitrary", Title: "임의 데이터", SvcType: SvcREST},
		{PK: "bridge", Title: "상권 점포 개폐업", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{{
			Role: "상권 수요", Query: "상권 점포", Contribution: "쇠퇴 상권을 구분",
			Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}},
		}},
		AnchorPKs: []string{"arbitrary"}, BridgeSelections: []BridgeSelection{{
			PK: "bridge", WhyCandidate: "점포 개폐업 제목 근거",
		}},
		Limit: 10, RESTOnly: true,
	})
	if len(res.Anchors) != 0 || len(res.Connections) != 0 || res.Abstention == nil {
		t.Fatalf("arbitrary catalog PK must not be relabelled as anchor: %+v", res)
	}
}

func TestSearchPlanDerivesCandidateContractFromRetrievedHit(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "bridge", Title: "상권 점포 개폐업", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "상권 수요", Query: "상권 점포", Contribution: "쇠퇴 상권을 구분",
				Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{{
			PK: "bridge", Role: "환경 위험", IncrementalValue: "모델이 지어낸 가치", WhyCandidate: "점포 개폐업 제목 근거",
			Edge: EdgeHypothesis{Kinds: []string{"entity"}, ExpectedKeys: []string{"inventedId"}},
		}},
		Limit: 10, RESTOnly: true,
	})
	if len(res.Connections) != 1 {
		t.Fatalf("connections = %+v", res.Connections)
	}
	got := res.Connections[0]
	if got.BridgeRole != "상권 수요" || got.IncrementalValue != "쇠퇴 상권을 구분" ||
		len(got.Edge.ExpectedKeys) != 1 || got.Edge.ExpectedKeys[0] != "법정동코드" {
		t.Fatalf("candidate contract was relabelled by selection: %+v", got)
	}
}

func TestSearchPlanEmitsAtMostOneConnectionPerRetrievedRole(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "bridge-a", Title: "상권 점포 개폐업 A", SvcType: SvcREST},
		{PK: "bridge-b", Title: "상권 점포 개폐업 B", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "상권 수요", Query: "상권 점포 개폐업", Contribution: "쇠퇴 상권을 구분",
				Edge: EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		},
		AnchorPKs: []string{"anchor"}, BridgeSelections: []BridgeSelection{
			{PK: "bridge-a", WhyCandidate: "A 제목 근거"},
			{PK: "bridge-b", WhyCandidate: "B 제목 근거"},
		},
		Limit: 10, RESTOnly: true,
	})
	if len(res.Connections) != 1 || res.Connections[0].Bridge.PK != "bridge-a" {
		t.Fatalf("duplicate roles must be suppressed deterministically: %+v", res.Connections)
	}
}

func TestSearchPlanAbstainsWhenBridgeAxisHasNoJoinContract(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "noise", Title: "관광 인기 순위", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "새로움", Query: "관광 인기", Contribution: "흥미로운 조합"},
		},
		AnchorPKs:        []string{"anchor"},
		BridgeSelections: []BridgeSelection{{PK: "noise", Role: "새로움", IncrementalValue: "흥미로운 조합"}},
		Limit:            10, RESTOnly: true,
	})
	if len(res.Connections) != 0 || res.Abstention == nil {
		t.Fatalf("invalid bridge should abstain, got connections=%+v abstention=%+v", res.Connections, res.Abstention)
	}
}

func TestSearchPlanRejectsProxyAxisWithoutTransform(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "bridge", Title: "상권 정보", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "수요", Query: "상권", Contribution: "수요 판정",
				Edge: EdgeHypothesis{Kinds: []string{"proxy"}, ExpectedKeys: []string{"주소"}}},
		},
		AnchorPKs: []string{"anchor"},
		BridgeSelections: []BridgeSelection{{
			PK: "bridge", Role: "수요", IncrementalValue: "수요 판정",
			Edge: EdgeHypothesis{Kinds: []string{"proxy"}, ExpectedKeys: []string{"주소"}},
		}},
		Limit: 10, RESTOnly: true,
	})
	if len(res.Connections) != 0 || res.Abstention == nil {
		t.Fatalf("proxy without transform must abstain: %+v", res)
	}
}

func TestSearchPlanDoesNotAutoSelectTopRankedBridge(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "anchor", Title: "공매 물건", SvcType: SvcREST},
		{PK: "wrong", Title: "임대료 기준 정보", SvcType: SvcREST},
	}}
	res := c.SearchPlan(QueryPlan{
		Axes: []DiscoveryAxis{
			{Role: "anchor", Query: "공매 물건"},
			{Role: "권리부담", Query: "임대료", Contribution: "위험조정 가격",
				Edge: EdgeHypothesis{Kinds: []string{"entity"}, ExpectedKeys: []string{"건물관리번호"}}},
		},
		AnchorPKs: []string{"anchor"}, Limit: 10, RESTOnly: true,
	})
	if len(res.Hits) != 2 || len(res.Connections) != 0 || res.Abstention != nil {
		t.Fatalf("retrieval without explicit selection must not create a card: %+v", res)
	}
}

func TestSearchPlanRelaxationRejectsSingleGenericWord(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "relevant", Title: "국유재산 매각 공고", SvcType: SvcREST, ApplyCount: 10},
		{PK: "noise", Title: "인기 대기오염 현황", SvcType: SvcREST, ApplyCount: 9000},
	}}
	r := c.SearchPlan(QueryPlan{Intent: "저가 자산", Concepts: []string{"국유재산 매각 현황"}, Limit: 10, RESTOnly: true})
	if len(r.Hits) != 1 || r.Hits[0].PK != "relevant" {
		t.Fatalf("planned relaxed hits = %+v, want only the two-term match", r.Hits)
	}
}
