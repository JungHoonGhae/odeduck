package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

const (
	officialDatasetPath    = "/15077093/v1/dataset"
	officialOpenDataPath   = "/15077093/v1/open-data-list"
	officialFileDataPath   = "/15077093/v1/file-data-list"
	DefaultOfficialBaseURL = "https://api.odcloud.kr/api"
)

// ErrOfficialUnauthorized means the account-wide data.go.kr key has not been
// approved for the catalogue-listing API (or is no longer valid). Auto mode may
// safely fall back to the public web source; forced official mode surfaces it.
var ErrOfficialUnauthorized = errors.New("공식 카탈로그 API 인증이 승인되지 않았습니다")

// SyncSource is the catalogue collection seam. Official and web adapters both
// hide pagination, type classification, merging and ordering behind one operation.
type SyncSource interface {
	Sync(context.Context, string, int, func(int)) (*Catalog, error)
}

// OfficialSource reads the documented 공공 데이터 포털 목록 조회 API. The
// serviceKey is sent only in the Authorization header, never in a URL.
type OfficialSource struct {
	http officialHTTP
	key  string
}

// officialHTTP deliberately exposes only the non-redirecting credentialed GET
// used by this source. Tests inject a deterministic implementation without
// making the production origin configurable.
type officialHTTP interface {
	GetWithHeadersNoRedirect(context.Context, string, http.Header) (*fetch.Response, error)
}

// NewOfficialSource pins catalogue credentials to the documented ODCloud
// origin. Unlike portal HTML sources, it intentionally accepts no base URL:
// test-only portal overrides must never redirect an account-wide API key.
func NewOfficialSource(transport *fetch.Client, key string) *OfficialSource {
	return newOfficialSource(transport, key)
}

func newOfficialSource(transport officialHTTP, key string) *OfficialSource {
	return &OfficialSource{http: transport, key: strings.TrimSpace(key)}
}

type officialEnvelope[T any] struct {
	Page         int `json:"page"`
	PerPage      int `json:"perPage"`
	CurrentCount int `json:"currentCount"`
	MatchCount   int `json:"matchCount"`
	TotalCount   int `json:"totalCount"`
	Data         []T `json:"data"`
}

type officialRow struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Org            string `json:"org_nm"`
	Category       string `json:"new_category_nm"`
	DownloadCount  int    `json:"download_cnt"`
	ViewCount      int    `json:"view_cnt"`
	UpdatedAt      string `json:"updated_at"`
	Format         string `json:"ext"`
	PageURL        string `json:"page_url"`
	SwaggerJSONURL string `json:"swagger_json_url"`
	Description    string `json:"desc"`
	Keywords       string `json:"keywords"`
	IsDeleted      string `json:"is_deleted"`
}

type officialOpenDataRow struct {
	ListID            string `json:"list_id"`
	ListTitle         string `json:"list_title"`
	Organization      string `json:"org_nm"`
	Category          string `json:"new_category_nm"`
	Description       string `json:"desc"`
	Keywords          string `json:"keywords"`
	UpdatedAt         string `json:"updated_at"`
	APIType           string `json:"api_type"`
	GuideURL          string `json:"guide_url"`
	EndpointURL       string `json:"end_point_url"`
	LinkURL           string `json:"link_url"`
	DataFormat        string `json:"data_format"`
	RequestCount      int    `json:"request_cnt"`
	DevApproval       string `json:"is_confirmed_for_dev_nm"`
	ProdApproval      string `json:"is_confirmed_for_prod_nm"`
	OperationSequence string `json:"operation_seq"`
	OperationName     string `json:"operation_nm"`
	OperationURL      string `json:"operation_url"`
	RequestParamNames string `json:"request_param_nm_en"`
	MetaURL           string `json:"meta_url"`
	IsListDeleted     string `json:"is_list_deleted"`
	IsDeleted         string `json:"is_deleted"`
}

type officialFileDataRow struct {
	ListID        string `json:"list_id"`
	ListTitle     string `json:"list_title"`
	Organization  string `json:"org_nm"`
	Category      string `json:"new_category_nm"`
	Description   string `json:"desc"`
	Keywords      string `json:"keywords"`
	UpdatedAt     string `json:"updated_at"`
	Format        string `json:"ext"`
	DownloadCount int    `json:"download_cnt"`
	IsListDeleted string `json:"is_list_deleted"`
	IsDeleted     string `json:"is_deleted"`
}

func (s *OfficialSource) Sync(ctx context.Context, rawScope string, perPage int, progress func(int)) (*Catalog, error) {
	_, scope, err := catalogTypes(rawScope)
	if err != nil {
		return nil, err
	}
	if s.http == nil {
		return nil, fmt.Errorf("공식 카탈로그 source가 설정되지 않았습니다")
	}
	if s.key == "" {
		return nil, ErrOfficialUnauthorized
	}
	if perPage <= 0 {
		perPage = 200
	}
	if perPage > 1000 {
		perPage = 1000
	}

	seen := map[string]Entry{}
	if err := readOfficialPages(ctx, s, officialDatasetPath, perPage, func(rows []officialRow) {
		for _, row := range rows {
			entry, ok := entryFromOfficial(row)
			if !ok || !scopeIncludes(scope, entry) {
				continue
			}
			seen[entry.PK] = mergeEntry(seen[entry.PK], entry)
		}
		if progress != nil {
			progress(len(seen))
		}
	}); err != nil {
		return nil, err
	}
	if scope == "ALL" || scope == "API" {
		if err := readOfficialPages(ctx, s, officialOpenDataPath, perPage, func(rows []officialOpenDataRow) {
			for _, row := range rows {
				entry, ok := entryFromOfficialOpenData(row)
				if ok {
					entry.OfficialAPI.EvidenceURL = DefaultOfficialBaseURL + officialOpenDataPath
					seen[entry.PK] = mergeEntry(seen[entry.PK], entry)
				}
			}
			if progress != nil {
				progress(len(seen))
			}
		}); err != nil {
			return nil, err
		}
	}
	if scope == "ALL" || scope == "FILE" {
		if err := readOfficialPages(ctx, s, officialFileDataPath, perPage, func(rows []officialFileDataRow) {
			for _, row := range rows {
				entry, ok := entryFromOfficialFileData(row)
				if ok {
					seen[entry.PK] = mergeEntry(seen[entry.PK], entry)
				}
			}
			if progress != nil {
				progress(len(seen))
			}
		}); err != nil {
			return nil, err
		}
	}

	catalog := &Catalog{SyncedAt: time.Now().UTC(), Type: scope, Source: SourceOfficial}
	for _, entry := range seen {
		catalog.Entries = append(catalog.Entries, entry)
	}
	sortEntries(catalog.Entries)
	return catalog, nil
}

func readOfficialPages[T any](ctx context.Context, source *OfficialSource, endpoint string, perPage int, visit func([]T)) error {
	received := 0
	for page := 1; ; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "perPage": {strconv.Itoa(perPage)}}
		requestURL := DefaultOfficialBaseURL + endpoint + "?" + query.Encode()
		res, err := source.http.GetWithHeadersNoRedirect(ctx, requestURL, http.Header{
			"Authorization": {"Infuser " + source.key},
			"Accept":        {"application/json"},
		})
		if err != nil {
			return err
		}
		if res.Status == http.StatusUnauthorized || res.Status == http.StatusForbidden {
			return ErrOfficialUnauthorized
		}
		if res.Status != http.StatusOK {
			return fmt.Errorf("공식 카탈로그 API %s가 HTTP %d를 반환했습니다", endpoint, res.Status)
		}
		var batch officialEnvelope[T]
		if err := json.Unmarshal(res.Body, &batch); err != nil {
			return fmt.Errorf("공식 카탈로그 %s JSON 해석 실패: %w", endpoint, err)
		}
		if batch.TotalCount < 0 || batch.CurrentCount != len(batch.Data) {
			return fmt.Errorf("공식 카탈로그 %s pagination 계약이 일치하지 않습니다", endpoint)
		}
		if batch.Page != 0 && batch.Page != page {
			return fmt.Errorf("공식 카탈로그 %s pagination이 요청 page=%d 대신 page=%d를 반환했습니다 — 전체 조회 승인이 있는 키가 필요합니다", endpoint, page, batch.Page)
		}
		visit(batch.Data)
		received += batch.CurrentCount
		if batch.CurrentCount == 0 || received >= batch.TotalCount {
			return nil
		}
	}
}

func entryFromOfficial(row officialRow) (Entry, bool) {
	pk := strings.TrimSpace(row.ID)
	if pk == "" || strings.TrimSpace(row.Title) == "" || officialDeleted(row.IsDeleted) {
		return Entry{}, false
	}
	target, err := url.Parse(strings.TrimSpace(row.PageURL))
	entry := Entry{
		PK: pk, Title: strings.TrimSpace(row.Title), Org: strings.TrimSpace(row.Org),
		Category: strings.TrimSpace(row.Category), ApplyCount: row.DownloadCount,
		ViewCount: row.ViewCount, ModifiedAt: strings.TrimSpace(row.UpdatedAt),
		Formats: splitOfficialFormats(row.Format),
		Desc:    strings.TrimSpace(strings.TrimSpace(row.Description) + " " + strings.TrimSpace(row.Keywords)),
	}
	trustedPage := err == nil && (strings.EqualFold(strings.TrimSuffix(target.Hostname(), "."), "www.data.go.kr") ||
		strings.EqualFold(strings.TrimSuffix(target.Hostname(), "."), "data.go.kr"))
	switch {
	case trustedPage && strings.HasSuffix(target.Path, "/openapi.do"):
		entry.DataTypes = []string{"API"}
		if swagger := strings.TrimSpace(row.SwaggerJSONURL); swagger != "" && swagger != "-" {
			entry.SvcType = SvcREST
		} else {
			entry.SvcType = SvcLINK
		}
	case trustedPage && strings.HasSuffix(target.Path, "/fileData.do"):
		entry.DataTypes = []string{"FILE"}
		entry.SvcType = SvcFILE
	default:
		entry.DataTypes = nil
	}
	return entry, true
}

func entryFromOfficialOpenData(row officialOpenDataRow) (Entry, bool) {
	pk := strings.TrimSpace(row.ListID)
	if pk == "" || strings.TrimSpace(row.ListTitle) == "" || officialDeleted(row.IsDeleted) || officialDeleted(row.IsListDeleted) {
		return Entry{}, false
	}
	apiType := strings.ToUpper(strings.TrimSpace(row.APIType))
	serviceType := ""
	switch {
	case strings.Contains(apiType, SvcLINK):
		serviceType = SvcLINK
	case strings.Contains(apiType, SvcREST):
		serviceType = SvcREST
	}
	operation := OfficialAPIOperation{
		Sequence: strings.TrimSpace(row.OperationSequence), Name: strings.TrimSpace(row.OperationName),
		URL: strings.TrimSpace(row.OperationURL), RequestNames: splitOfficialNames(row.RequestParamNames),
	}
	contract := &OfficialAPIContract{
		APIType: apiType, GuideURL: strings.TrimSpace(row.GuideURL), EndpointURL: strings.TrimSpace(row.EndpointURL),
		LinkURL: strings.TrimSpace(row.LinkURL), MetaURL: strings.TrimSpace(row.MetaURL),
		DevApproval: strings.TrimSpace(row.DevApproval), ProdApproval: strings.TrimSpace(row.ProdApproval),
		EvidenceKind: "official_api", EvidenceURL: DefaultOfficialBaseURL + officialOpenDataPath,
	}
	if officialOperationKey(operation) != "\x00\x00" {
		contract.Operations = []OfficialAPIOperation{operation}
	}
	return Entry{
		PK: pk, Title: strings.TrimSpace(row.ListTitle), Org: strings.TrimSpace(row.Organization),
		Category: strings.TrimSpace(row.Category), ApplyCount: row.RequestCount,
		ModifiedAt: strings.TrimSpace(row.UpdatedAt), SvcType: serviceType, DataTypes: []string{"API"},
		Formats:     splitOfficialFormats(row.DataFormat),
		Desc:        strings.TrimSpace(strings.TrimSpace(row.Description) + " " + strings.TrimSpace(row.Keywords)),
		OfficialAPI: contract,
	}, true
}

func entryFromOfficialFileData(row officialFileDataRow) (Entry, bool) {
	pk := strings.TrimSpace(row.ListID)
	if pk == "" || strings.TrimSpace(row.ListTitle) == "" || officialDeleted(row.IsDeleted) || officialDeleted(row.IsListDeleted) {
		return Entry{}, false
	}
	return Entry{
		PK: pk, Title: strings.TrimSpace(row.ListTitle), Org: strings.TrimSpace(row.Organization),
		Category: strings.TrimSpace(row.Category), ApplyCount: row.DownloadCount,
		ModifiedAt: strings.TrimSpace(row.UpdatedAt), SvcType: SvcFILE, DataTypes: []string{"FILE"},
		Formats: splitOfficialFormats(row.Format),
		Desc:    strings.TrimSpace(strings.TrimSpace(row.Description) + " " + strings.TrimSpace(row.Keywords)),
	}, true
}

func officialDeleted(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "y", "yes", "true", "1":
		return true
	default:
		return false
	}
}

func scopeIncludes(scope string, entry Entry) bool {
	if scope == "ALL" {
		return true
	}
	for _, dataType := range entry.DataTypes {
		if dataType == scope {
			return true
		}
	}
	return false
}

func splitOfficialFormats(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case '+', ',', '/', '|', ';':
			return true
		default:
			return false
		}
	})
	return appendUniqueFold(nil, parts...)
}

func splitOfficialNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for index := range parts {
		parts[index] = strings.Trim(strings.TrimSpace(parts[index]), "\"")
	}
	return appendUniqueExact(nil, parts...)
}
