package catalog

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

const (
	officialFileDatasetPK   = "15062804"
	maxOfficialFileBytes    = int64(512 << 20)
	officialFileEvidenceURL = "https://www.data.go.kr/data/15062804/fileData.do"
)

var officialFileDownload = regexp.MustCompile(`fileDetailObj\.fn_fileDataDown\(\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*\)`)

// OfficialFileSource reads the portal's public monthly 목록개방현황 CSV. It is
// the no-credential machine-readable fallback for the restricted bulk API: one
// streamed download replaces roughly a thousand HTML search pages.
type OfficialFileSource struct {
	http       *fetch.Client
	base       string
	maxBytes   int64
	retryDelay time.Duration
}

type combinedSource struct {
	primary    SyncSource
	enrichment SyncSource
	retryDelay []time.Duration
}

// NewCombinedSource joins the fast official monthly snapshot with current web
// delivery discovery. The official rows provide classifications/provenance;
// the web adapter restores API+FILE co-representations omitted by the CSV.
func NewCombinedSource(primary, enrichment SyncSource) SyncSource {
	return &combinedSource{
		primary: primary, enrichment: enrichment,
		retryDelay: []time.Duration{250 * time.Millisecond, time.Second},
	}
}

func (s *combinedSource) Sync(ctx context.Context, scope string, perPage int, progress func(int)) (*Catalog, error) {
	if s == nil || s.primary == nil || s.enrichment == nil {
		return nil, fmt.Errorf("combined 카탈로그 source가 설정되지 않았습니다")
	}
	base, err := s.syncWithRetry(ctx, "공식 월간 snapshot", s.primary, scope, perPage, progress)
	if err != nil {
		return nil, err
	}
	extra, err := s.syncWithRetry(ctx, "웹 제공형 보강", s.enrichment, scope, perPage, progress)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]Entry, len(base.Entries)+len(extra.Entries))
	for _, entry := range base.Entries {
		seen[entry.PK] = mergeEntry(seen[entry.PK], entry)
	}
	for _, entry := range extra.Entries {
		seen[entry.PK] = mergeEntry(seen[entry.PK], entry)
	}
	entries := make([]Entry, 0, len(seen))
	for _, entry := range seen {
		entries = append(entries, entry)
	}
	sortEntries(entries)
	return &Catalog{SyncedAt: time.Now().UTC(), Type: base.Type, Source: SourceCombined, Entries: entries}, nil
}

func (s *combinedSource) syncWithRetry(
	ctx context.Context,
	label string,
	source SyncSource,
	scope string,
	perPage int,
	progress func(int),
) (*Catalog, error) {
	for attempt := 0; ; attempt++ {
		result, err := source.Sync(ctx, scope, perPage, progress)
		if err == nil {
			return result, nil
		}
		if !transientSyncError(err) || attempt >= len(s.retryDelay) {
			return nil, err
		}
		timer := time.NewTimer(s.retryDelay[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("%s 재시도 중단: %w", label, ctx.Err())
		case <-timer.C:
		}
	}
}

func transientSyncError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}

func NewOfficialFileSource(transport *fetch.Client, baseURL string) *OfficialFileSource {
	return &OfficialFileSource{
		http: transport, base: strings.TrimRight(baseURL, "/"),
		maxBytes: maxOfficialFileBytes, retryDelay: time.Second,
	}
}

func (s *OfficialFileSource) Sync(ctx context.Context, rawScope string, _ int, progress func(int)) (*Catalog, error) {
	_, scope, err := catalogTypes(rawScope)
	if err != nil {
		return nil, err
	}
	if s.http == nil || s.base == "" {
		return nil, fmt.Errorf("공식 파일 카탈로그 source가 설정되지 않았습니다")
	}
	downloadURL, err := s.resolveDownloadURL(ctx)
	if err != nil {
		return nil, err
	}
	response, err := s.http.OpenGET(ctx, downloadURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.Status != http.StatusOK {
		return nil, fmt.Errorf("공식 목록 snapshot 다운로드가 HTTP %d를 반환했습니다", response.Status)
	}
	maxBytes := s.maxBytes
	if maxBytes <= 0 {
		maxBytes = maxOfficialFileBytes
	}
	if response.ContentLength > maxBytes {
		return nil, fmt.Errorf("공식 목록 snapshot이 허용 크기 %d bytes를 초과했습니다", maxBytes)
	}

	limited := &io.LimitedReader{R: response.Body, N: maxBytes + 1}
	reader := csv.NewReader(limited)
	reader.ReuseRecord = true
	head, err := reader.Read()
	if err != nil {
		if limited.N <= 0 {
			return nil, fmt.Errorf("공식 목록 snapshot이 허용 크기 %d bytes를 초과했습니다", maxBytes)
		}
		return nil, fmt.Errorf("공식 목록 snapshot header 해석 실패: %w", err)
	}
	columns := make(map[string]int, len(head))
	for index, name := range head {
		columns[strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))] = index
	}
	required := []string{"목록키", "목록유형", "목록명", "제공기관", "수정일", "설명", "목록 URL"}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("공식 목록 snapshot에 필수 column %q이 없습니다", name)
		}
	}

	entries := make([]Entry, 0, 100000)
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if limited.N <= 0 {
				return nil, fmt.Errorf("공식 목록 snapshot이 허용 크기 %d bytes를 초과했습니다", maxBytes)
			}
			return nil, fmt.Errorf("공식 목록 snapshot CSV 해석 실패: %w", readErr)
		}
		entry, ok := entryFromOfficialFileSnapshot(record, columns)
		if !ok || !scopeIncludes(scope, entry) {
			continue
		}
		entries = append(entries, entry)
		if progress != nil && len(entries)%1000 == 0 {
			progress(len(entries))
		}
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("공식 목록 snapshot이 허용 크기 %d bytes를 초과했습니다", maxBytes)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("공식 목록 snapshot에 %s 데이터가 없습니다", scope)
	}
	sortEntries(entries)
	return &Catalog{SyncedAt: time.Now().UTC(), Type: scope, Source: SourceOfficialFile, Entries: entries}, nil
}

type officialFileResolution struct {
	Status       bool   `json:"status"`
	AtchFileID   string `json:"atchFileId"`
	FileDetailSn string `json:"fileDetailSn"`
	File         struct {
		DataName string `json:"dataNm"`
	} `json:"fileDataRegistVO"`
}

func (s *OfficialFileSource) resolveDownloadURL(ctx context.Context) (string, error) {
	detailURL := s.base + "/data/" + officialFileDatasetPK + "/fileData.do"
	var page *fetch.Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		page, err = s.http.Get(ctx, detailURL)
		if err == nil {
			break
		}
		if attempt == 2 {
			return "", err
		}
		timer := time.NewTimer(s.retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if page.Status != http.StatusOK {
		return "", fmt.Errorf("공식 목록 snapshot 상세가 HTTP %d를 반환했습니다", page.Status)
	}
	match := officialFileDownload.FindStringSubmatch(string(page.Body))
	if len(match) != 6 || match[1] != officialFileDatasetPK {
		return "", fmt.Errorf("공식 목록 snapshot 다운로드 계약을 찾지 못했습니다")
	}
	form := url.Values{
		"publicDataPk": {match[1]}, "publicDataDetailPk": {match[2]},
		"atchFileId": {match[3]}, "fileDetailSn": {match[4]}, "publicDataTyCode": {"PR0051"},
	}
	resolvedResponse, err := s.http.PostForm(ctx, s.base+"/tcs/dss/selectFileDataDownload.do", form)
	if err != nil {
		return "", err
	}
	if resolvedResponse.Status != http.StatusOK {
		return "", fmt.Errorf("공식 목록 snapshot asset 조회가 HTTP %d를 반환했습니다", resolvedResponse.Status)
	}
	var resolved officialFileResolution
	if err := json.Unmarshal(resolvedResponse.Body, &resolved); err != nil {
		return "", fmt.Errorf("공식 목록 snapshot asset 응답 해석 실패: %w", err)
	}
	if !resolved.Status || resolved.AtchFileID == "" || resolved.FileDetailSn == "" {
		return "", fmt.Errorf("공식 목록 snapshot asset 식별자가 비어 있습니다")
	}
	return s.base + "/cmm/cmm/fileDownload.do?" + url.Values{
		"atchFileId": {resolved.AtchFileID}, "fileDetailSn": {resolved.FileDetailSn}, "dataNm": {resolved.File.DataName},
	}.Encode(), nil
}

func entryFromOfficialFileSnapshot(record []string, columns map[string]int) (Entry, bool) {
	value := func(name string) string {
		index, ok := columns[name]
		if !ok || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	pk, title := value("목록키"), value("목록명")
	if pk == "" || title == "" {
		return Entry{}, false
	}
	typeName := strings.ToUpper(value("목록유형"))
	entry := Entry{
		PK: pk, Title: title, Org: value("제공기관"), Category: value("분류체계"),
		ApplyCount: parseOfficialFileCount(value("다운로드_활용신청건수")),
		ViewCount:  parseOfficialFileCount(value("조회수")), ModifiedAt: value("수정일"),
		Formats: splitOfficialFormats(value("확장자(데이터포맷)")),
		Desc:    strings.TrimSpace(value("설명") + " " + value("키워드")),
	}
	switch typeName {
	case "STD":
		entry.SvcType, entry.DataTypes = SvcSTD, []string{"STD"}
	case "FILE":
		entry.SvcType, entry.DataTypes = SvcFILE, []string{"FILE"}
	case "API":
		entry.DataTypes = []string{"API"}
		apiType := strings.ToUpper(value("API 유형"))
		switch apiType {
		case SvcREST:
			entry.SvcType = SvcREST
		case SvcLINK:
			entry.SvcType = SvcLINK
		}
		dev, prod := splitOfficialFileApproval(value("심의 유형"))
		entry.OfficialAPI = &OfficialAPIContract{
			APIType: apiType, DevApproval: dev, ProdApproval: prod, MetaURL: value("목록 URL"),
			EvidenceKind: "official_catalog_file", EvidenceURL: officialFileEvidenceURL,
		}
	}
	return entry, true
}

func parseOfficialFileCount(raw string) int {
	value, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(raw), ",", ""))
	return value
}

func splitOfficialFileApproval(raw string) (string, string) {
	raw = strings.Join(strings.Fields(raw), " ")
	parts := strings.Split(raw, "/")
	clean := func(value, label string) string {
		value = strings.TrimSpace(value)
		value = strings.TrimSpace(strings.TrimPrefix(value, label))
		value = strings.TrimSpace(strings.TrimPrefix(value, ":"))
		return value
	}
	if len(parts) == 2 {
		return clean(parts[0], "개발단계"), clean(parts[1], "운영단계")
	}
	return "", ""
}
