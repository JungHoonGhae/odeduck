package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func TestOfficialFileSourceStreamsPublicMonthlySnapshot(t *testing.T) {
	header := "\ufeff목록키,목록유형,목록명,분류체계,제공기관,확장자(데이터포맷),다운로드_활용신청건수,수정일,설명,키워드,API 유형,심의 유형,조회수,목록 URL\n"
	rows := strings.Join([]string{
		`100,API,상권 API,산업고용,기관A,"JSON, XML","1,234",2026-08-31,설명,키워드,REST,개발단계 : 자동승인 / 운영단계 : 심의승인,99,https://www.data.go.kr/data/100/openapi.do`,
		`200,FILE,상권 파일,산업고용,기관B,CSV,12,2026-08-30,파일 설명,파일 키워드,,,88,https://www.data.go.kr/data/200/fileData.do`,
		`300,STD,제공표준,공공행정,기관C,CSV,3,2026-08-29,표준 설명,표준 키워드,,,77,https://www.data.go.kr/data/300/standard.do`,
	}, "\n") + "\n"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + officialFileDatasetPK + "/fileData.do":
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15062804','uddi:snapshot','','1','3')">download</button>`))
		case "/tcs/dss/selectFileDataDownload.do":
			if err := r.ParseForm(); err != nil || r.Form.Get("publicDataPk") != officialFileDatasetPK {
				t.Fatalf("resolution form = %v, err=%v", r.Form, err)
			}
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE_SNAPSHOT","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"catalog.csv"}}`))
		case "/cmm/cmm/fileDownload.do":
			if r.URL.Query().Get("atchFileId") != "FILE_SNAPSHOT" {
				t.Fatalf("download query = %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte(header + rows))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer server.Close()

	got, err := NewOfficialFileSource(fetch.New(fetch.WithDelay(0)), server.URL).Sync(context.Background(), "ALL", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceOfficialFile || len(got.Entries) != 3 {
		t.Fatalf("catalog = %+v", got)
	}
	api, ok := got.Find("100")
	if !ok || api.SvcType != SvcREST || api.ApplyCount != 1234 || api.OfficialAPI == nil ||
		api.OfficialAPI.DevApproval != "자동승인" || api.OfficialAPI.ProdApproval != "심의승인" ||
		api.OfficialAPI.EvidenceKind != "official_catalog_file" || api.OfficialAPI.EvidenceURL != officialFileEvidenceURL ||
		strings.Join(api.Formats, ",") != "JSON,XML" {
		t.Fatalf("API entry = %+v", api)
	}
	file, ok := got.Find("200")
	if !ok || file.SvcType != SvcFILE || strings.Join(file.DataTypes, ",") != "FILE" {
		t.Fatalf("FILE entry = %+v", file)
	}
	standard, ok := got.Find("300")
	if !ok || standard.SvcType != "STD" || strings.Join(standard.DataTypes, ",") != "STD" {
		t.Fatalf("standard entry = %+v", standard)
	}
	if hit := hitFromEntry(&standard); hit.DetailURL != "https://www.data.go.kr/data/300/standard.do" {
		t.Fatalf("standard detail = %+v", hit)
	}
}

func TestOfficialFileSourceRetriesTransientDetailTransportFailure(t *testing.T) {
	var detailAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + officialFileDatasetPK + "/fileData.do":
			detailAttempts++
			if detailAttempts < 3 {
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Fatal(err)
				}
				_ = connection.Close()
				return
			}
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15062804','uddi:snapshot','','1','3')">download</button>`))
		case "/tcs/dss/selectFileDataDownload.do":
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"catalog.csv"}}`))
		case "/cmm/cmm/fileDownload.do":
			_, _ = w.Write([]byte("목록키,목록유형,목록명,제공기관,수정일,설명,목록 URL\n100,FILE,자료,기관,2026-09-04,설명,https://example.com\n"))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer server.Close()

	source := NewOfficialFileSource(fetch.New(fetch.WithDelay(0)), server.URL)
	source.retryDelay = 0
	if _, err := source.Sync(context.Background(), "ALL", 0, nil); err != nil {
		t.Fatal(err)
	}
	if detailAttempts != 3 {
		t.Fatalf("detail attempts = %d, want 3", detailAttempts)
	}
}

func TestOfficialFileSourceRejectsInvalidDownloads(t *testing.T) {
	const requiredHeader = "목록키,목록유형,목록명,제공기관,수정일,설명,목록 URL\n"
	tests := map[string]struct {
		download func(http.ResponseWriter)
		limit    int64
		want     string
	}{
		"non-200": {
			download: func(w http.ResponseWriter) { http.Error(w, "unavailable", http.StatusBadGateway) },
			want:     "HTTP 502",
		},
		"declared-too-large": {
			download: func(w http.ResponseWriter) { w.Header().Set("Content-Length", "257") },
			limit:    256,
			want:     "허용 크기 256",
		},
		"stream-overflow": {
			download: func(w http.ResponseWriter) {
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				_, _ = w.Write([]byte(requiredHeader + strings.Repeat("x", 300)))
			},
			limit: 256,
			want:  "허용 크기 256",
		},
		"malformed-csv": {
			download: func(w http.ResponseWriter) { _, _ = w.Write([]byte(requiredHeader + `"unterminated`)) },
			want:     "CSV 해석 실패",
		},
		"missing-column": {
			download: func(w http.ResponseWriter) {
				_, _ = w.Write([]byte("목록키,목록유형,목록명\n100,API,누락\n"))
			},
			want: "필수 column",
		},
		"empty": {
			download: func(w http.ResponseWriter) { _, _ = w.Write([]byte(requiredHeader)) },
			want:     "데이터가 없습니다",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/data/" + officialFileDatasetPK + "/fileData.do":
					_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15062804','uddi:snapshot','','1','3')">download</button>`))
				case "/tcs/dss/selectFileDataDownload.do":
					_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"catalog.csv"}}`))
				case "/cmm/cmm/fileDownload.do":
					test.download(w)
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
			}))
			defer server.Close()

			source := NewOfficialFileSource(fetch.New(fetch.WithDelay(0)), server.URL)
			if test.limit > 0 {
				source.maxBytes = test.limit
			}
			_, err := source.Sync(context.Background(), "ALL", 0, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestOfficialFileSourceRejectsBrokenResolutionContracts(t *testing.T) {
	tests := map[string]struct {
		detailStatus  int
		detail        string
		resolveStatus int
		resolve       string
		want          string
	}{
		"detail-non-200":            {detailStatus: http.StatusBadGateway, want: "상세가 HTTP 502"},
		"missing-download-contract": {detailStatus: http.StatusOK, detail: "<html></html>", want: "다운로드 계약"},
		"resolution-non-200":        {detailStatus: http.StatusOK, detail: `<button onclick="fileDetailObj.fn_fileDataDown('15062804','u','','1','3')">d</button>`, resolveStatus: http.StatusBadGateway, want: "asset 조회가 HTTP 502"},
		"malformed-resolution":      {detailStatus: http.StatusOK, detail: `<button onclick="fileDetailObj.fn_fileDataDown('15062804','u','','1','3')">d</button>`, resolveStatus: http.StatusOK, resolve: `{`, want: "응답 해석 실패"},
		"empty-identifiers":         {detailStatus: http.StatusOK, detail: `<button onclick="fileDetailObj.fn_fileDataDown('15062804','u','','1','3')">d</button>`, resolveStatus: http.StatusOK, resolve: `{"status":true}`, want: "식별자가 비어"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/data/" + officialFileDatasetPK + "/fileData.do":
					status := test.detailStatus
					if status == 0 {
						status = http.StatusOK
					}
					w.WriteHeader(status)
					_, _ = w.Write([]byte(test.detail))
				case "/tcs/dss/selectFileDataDownload.do":
					status := test.resolveStatus
					if status == 0 {
						status = http.StatusOK
					}
					w.WriteHeader(status)
					_, _ = w.Write([]byte(test.resolve))
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
			}))
			defer server.Close()

			_, err := NewOfficialFileSource(fetch.New(fetch.WithDelay(0)), server.URL).Sync(context.Background(), "ALL", 0, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

type fixedSyncSource struct{ catalog *Catalog }

func (source fixedSyncSource) Sync(context.Context, string, int, func(int)) (*Catalog, error) {
	return source.catalog, nil
}

type timeoutSyncError struct{}

func (timeoutSyncError) Error() string   { return "temporary timeout" }
func (timeoutSyncError) Timeout() bool   { return true }
func (timeoutSyncError) Temporary() bool { return true }

type flakySyncSource struct {
	catalog   *Catalog
	failures  int
	attempts  int
	permanent error
}

func (source *flakySyncSource) Sync(context.Context, string, int, func(int)) (*Catalog, error) {
	source.attempts++
	if source.permanent != nil {
		return nil, source.permanent
	}
	if source.attempts <= source.failures {
		return nil, fmt.Errorf("download failed: %w", timeoutSyncError{})
	}
	return source.catalog, nil
}

func TestCombinedSourceRetriesOnlyTransientNetworkFailures(t *testing.T) {
	primary := &flakySyncSource{catalog: &Catalog{Type: "ALL"}, failures: 1}
	enrichment := &flakySyncSource{catalog: &Catalog{Type: "ALL"}}
	source := NewCombinedSource(primary, enrichment).(*combinedSource)
	source.retryDelay = []time.Duration{time.Millisecond}

	if _, err := source.Sync(context.Background(), "ALL", 1000, nil); err != nil {
		t.Fatal(err)
	}
	if primary.attempts != 2 || enrichment.attempts != 1 {
		t.Fatalf("attempts primary=%d enrichment=%d", primary.attempts, enrichment.attempts)
	}

	permanent := &flakySyncSource{permanent: fmt.Errorf("schema drift")}
	if _, err := NewCombinedSource(permanent, enrichment).Sync(context.Background(), "ALL", 1000, nil); err == nil {
		t.Fatal("permanent parser error must fail without retry")
	}
	if permanent.attempts != 1 {
		t.Fatalf("permanent error attempts = %d, want 1", permanent.attempts)
	}
}

func TestCombinedSourcePreservesOfficialFactsAndWebCoRepresentation(t *testing.T) {
	official := &Catalog{Type: "ALL", Entries: []Entry{{
		PK: "100", Title: "공식 제목", SvcType: SvcFILE, DataTypes: []string{"FILE"}, Formats: []string{"CSV"},
		OfficialAPI: &OfficialAPIContract{APIType: SvcREST, DevApproval: "자동승인"},
	}}}
	web := &Catalog{Type: "ALL", Entries: []Entry{
		{PK: "100", Title: "웹 제목", SvcType: SvcREST, DataTypes: []string{"API"}, Formats: []string{"JSON"}},
		{PK: "200", Title: "웹 신규", SvcType: SvcLINK, DataTypes: []string{"API"}},
	}}
	got, err := NewCombinedSource(fixedSyncSource{official}, fixedSyncSource{web}).Sync(context.Background(), "ALL", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceCombined || got.SyncedAt.Before(time.Now().Add(-time.Minute)) || len(got.Entries) != 2 {
		t.Fatalf("combined = %+v", got)
	}
	shared, ok := got.Find("100")
	if !ok || shared.SvcType != SvcREST || strings.Join(shared.DataTypes, ",") != "FILE,API" ||
		strings.Join(shared.Formats, ",") != "CSV,JSON" || shared.OfficialAPI == nil {
		t.Fatalf("shared = %+v", shared)
	}
}
