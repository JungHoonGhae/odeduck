// Package dataset turns a catalogued Data Node into an evidence-backed delivery
// contract. It keeps portal HTML, provider handoffs, download forms and schema
// drift behind one small inspection interface.
package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/JungHoonGhae/oddsock/internal/fetch"
	"github.com/JungHoonGhae/oddsock/internal/portal"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

const (
	DeliveryFile          = "FILE"
	CapabilityInspectable = "inspectable"
	CapabilityRetrievable = "retrievable"

	EvidenceStandardMetadata      = "standard_metadata"
	EvidenceOfficialAPI           = "official_api"
	EvidenceFirstPartyWebContract = "first_party_web_contract"
	StabilityDocumented           = "documented"
	StabilityFallback             = "fallback"
)

// Ref is the compact identity and delivery fact already established by catalog
// search. Inspect never guesses a delivery kind from a page that happens to
// respond successfully.
type Ref struct {
	PK       string `json:"pk"`
	Delivery string `json:"delivery"`
}

// Contract is the observed delivery contract for one Data Node.
type Contract struct {
	Ref             Ref               `json:"ref"`
	Name            string            `json:"name,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	UpdateCycle     string            `json:"updateCycle,omitempty"`
	ModifiedAt      string            `json:"modifiedAt,omitempty"`
	DeclaredFormat  string            `json:"declaredFormat,omitempty"`
	SourceURL       string            `json:"sourceUrl"`
	AdapterID       string            `json:"adapterId"`
	AdapterRevision int               `json:"adapterRevision"`
	VerifiedAt      string            `json:"verifiedAt"`
	Capability      string            `json:"capability"`
	Evidence        []Evidence        `json:"evidence,omitempty"`
	Alternatives    []Alternative     `json:"alternatives,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Assets          []Asset           `json:"assets,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
}

// Alternative is a provider-advertised representation of the same logical
// dataset. It is discovery evidence, not a promise that oddsock can safely
// invoke that representation with a credential.
type Alternative struct {
	Delivery string `json:"delivery"`
	URL      string `json:"url"`
}

// Evidence identifies which part of a Contract came from a documented,
// standardized interface and which part depends on a first-party web contract.
// This prevents a working fallback from being mistaken for a stable public API.
type Evidence struct {
	Purpose   string `json:"purpose"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Stability string `json:"stability"`
}

// Asset is one immutable-looking provider file advertised by a detail page.
// ModifiedAt and SizeBytes are publisher claims until Retrieve observes bytes.
type Asset struct {
	Name       string  `json:"name"`
	Format     string  `json:"format,omitempty"`
	SizeBytes  int64   `json:"sizeBytes,omitempty"`
	ModifiedAt string  `json:"modifiedAt,omitempty"`
	Request    Request `json:"request"`
}

// Request is a typed, non-secret retrieval instruction. Only adapters create
// it; callers cannot supply arbitrary origins through Inspect.
type Request struct {
	Method string     `json:"method"`
	URL    string     `json:"url"`
	Form   url.Values `json:"form,omitempty"`
}

// Observation is evidence from bytes actually retrieved through an Adapter.
// It is intentionally a bounded schema profile, not the whole dataset body.
type Observation struct {
	SHA256   string         `json:"sha256"`
	Bytes    int64          `json:"bytes"`
	Files    []ObservedFile `json:"files,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

type ObservedFile struct {
	Name       string   `json:"name"`
	Format     string   `json:"format"`
	Columns    []string `json:"columns,omitempty"`
	SampleRows int      `json:"sampleRows,omitempty"`
}

// Transport is the true-external seam used by the production HTTP adapter and
// deterministic tests.
type Transport interface {
	Get(context.Context, string) (*fetch.Response, error)
	PostForm(context.Context, string, url.Values) (*fetch.Response, error)
}

// StreamingTransport is implemented by the production fetch client. Small
// fixture transports may implement only Transport and keep using buffered
// responses, while real FILE observation streams bytes to a temporary file.
type StreamingTransport interface {
	OpenGET(context.Context, string) (*fetch.StreamResponse, error)
	OpenPostForm(context.Context, string, url.Values) (*fetch.StreamResponse, error)
}

// Inspector hides portal/provider-specific inspection behind one operation.
type Inspector struct {
	http Transport
	base string
}

func NewInspector(transport Transport, portalBaseURL string) *Inspector {
	if portalBaseURL == "" {
		portalBaseURL = portal.BaseURL
	}
	return &Inspector{http: transport, base: strings.TrimRight(portalBaseURL, "/")}
}

const (
	maxArchiveEntries           = 128
	maxExpandedBytes            = 64 << 20
	maxObservationDownloadBytes = 512 << 20
	maxSampleRows               = 5
)

var portalDownload = regexp.MustCompile(`fileDetailObj\.fn_fileDataDown\(\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*\)`)
var seoulDatasetPath = regexp.MustCompile(`^/dataList/(OA-[0-9]+)/(S|F|A)/1/datasetView\.do$`)
var seoulDownload = regexp.MustCompile(`downloadFile\(\s*'([0-9]+)'\s*\)`)

// Inspect resolves one FILE Data Node into metadata and bounded typed download
// instructions. Other delivery kinds will be added through this same interface;
// unsupported kinds fail closed today.
func (i *Inspector) Inspect(ctx context.Context, ref Ref) (*Contract, error) {
	if err := portal.ValidatePublicDataPK(ref.PK); err != nil {
		return nil, err
	}
	if strings.ToUpper(strings.TrimSpace(ref.Delivery)) != DeliveryFile {
		return nil, fmt.Errorf("dataset delivery %q is not inspectable yet", ref.Delivery)
	}
	standard, standardEvidence, standardWarning := i.standardMetadata(ctx, ref.PK)
	detailURL := i.base + "/data/" + ref.PK + "/fileData.do"
	res, err := i.http.Get(ctx, detailURL)
	if err != nil {
		return nil, err
	}
	if res.Status != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %d", detailURL, res.Status)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(res.Body))
	if err != nil {
		return nil, err
	}
	htmlMetadata := fileMetadata(doc)
	metadata := standard.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	for key, value := range htmlMetadata {
		if _, exists := metadata[key]; !exists {
			metadata[key] = value
		}
	}
	name, provider, updateCycle, modifiedAt, declaredFormat := standard.Name, standard.Provider, standard.UpdateCycle, standard.ModifiedAt, standard.DeclaredFormat
	if name == "" {
		name = htmlMetadata["파일데이터명"]
	}
	if provider == "" {
		provider = htmlMetadata["제공기관"]
	}
	if updateCycle == "" {
		updateCycle = htmlMetadata["업데이트 주기"]
	}
	if modifiedAt == "" {
		modifiedAt = htmlMetadata["수정일"]
	}
	if declaredFormat == "" {
		declaredFormat = strings.ToUpper(htmlMetadata["확장자"])
	}
	contract := &Contract{
		Ref: ref, Name: name, Provider: provider,
		UpdateCycle: updateCycle, ModifiedAt: modifiedAt,
		DeclaredFormat: declaredFormat, SourceURL: detailURL,
		AdapterID: "datagokr-file", AdapterRevision: 1, VerifiedAt: "2026-09-01",
		Capability: CapabilityInspectable, Metadata: metadata,
	}
	if standardEvidence.URL != "" {
		contract.Evidence = append(contract.Evidence, standardEvidence)
	}
	contract.Evidence = append(contract.Evidence, Evidence{
		Purpose: "asset-resolution", Kind: EvidenceFirstPartyWebContract,
		URL: detailURL, Stability: StabilityFallback,
	})
	if standardWarning != "" {
		contract.Warnings = append(contract.Warnings, standardWarning)
	}

	if source := externalSourceURL(doc, htmlMetadata); source != "" {
		if normalized, ok := normalizeSeoulDatasetURL(source); ok {
			return i.inspectSeoul(ctx, contract, normalized)
		}
		contract.Warnings = append(contract.Warnings, "외부 제공기관 파일은 아직 typed Adapter가 없어 공식 URL만 확인했습니다")
		return contract, nil
	}

	match := portalDownload.FindStringSubmatch(string(res.Body))
	if len(match) != 6 {
		contract.Warnings = append(contract.Warnings, "포털 다운로드 계약을 확인하지 못했습니다")
		return contract, nil
	}
	asset, err := i.resolvePortalAsset(ctx, match)
	if err != nil {
		return nil, err
	}
	contract.Assets = []Asset{asset}
	contract.Capability = CapabilityRetrievable
	return contract, nil
}

type schemaDataset struct {
	Name                string `json:"name"`
	AlternateName       string `json:"alternateName"`
	Description         string `json:"description"`
	License             string `json:"license"`
	DateModified        string `json:"dateModified"`
	DatasetTimeInterval string `json:"datasetTimeInterval"`
	EncodingFormat      string `json:"encodingFormat"`
	Creator             struct {
		Name string `json:"name"`
	} `json:"creator"`
}

type standardContractMetadata struct {
	Name           string
	Provider       string
	UpdateCycle    string
	ModifiedAt     string
	DeclaredFormat string
	Metadata       map[string]string
}

// standardMetadata prefers the portal's official Schema.org representation.
// Its failure is non-fatal because some legacy nodes do not publish it; the
// first-party detail page remains a visibly labelled fallback.
func (i *Inspector) standardMetadata(ctx context.Context, pk string) (standardContractMetadata, Evidence, string) {
	metadataURL := i.base + "/catalog/" + pk + "/fileData.json"
	res, err := i.http.Get(ctx, metadataURL)
	if err != nil {
		return standardContractMetadata{}, Evidence{}, "표준 메타데이터를 읽지 못해 포털 HTML 메타데이터로 대체했습니다"
	}
	if res.Status != http.StatusOK {
		return standardContractMetadata{}, Evidence{}, fmt.Sprintf("표준 메타데이터가 HTTP %d를 반환해 포털 HTML 메타데이터로 대체했습니다", res.Status)
	}
	var schema schemaDataset
	if err := json.Unmarshal(res.Body, &schema); err != nil {
		return standardContractMetadata{}, Evidence{}, "표준 메타데이터 JSON을 해석하지 못해 포털 HTML 메타데이터로 대체했습니다"
	}
	name := strings.TrimSpace(schema.AlternateName)
	if name == "" {
		name = strings.TrimSpace(schema.Name)
	}
	metadata := map[string]string{}
	for key, value := range map[string]string{
		"name": schema.Name, "alternateName": schema.AlternateName,
		"description": schema.Description, "license": schema.License,
	} {
		if strings.TrimSpace(value) != "" {
			metadata[key] = strings.TrimSpace(value)
		}
	}
	return standardContractMetadata{
			Name: name, Provider: strings.TrimSpace(schema.Creator.Name),
			UpdateCycle:    strings.TrimSpace(schema.DatasetTimeInterval),
			ModifiedAt:     strings.TrimSpace(schema.DateModified),
			DeclaredFormat: strings.ToUpper(strings.TrimSpace(schema.EncodingFormat)),
			Metadata:       metadata,
		}, Evidence{
			Purpose: "metadata", Kind: EvidenceStandardMetadata,
			URL: metadataURL, Stability: StabilityDocumented,
		}, ""
}

// Observe retrieves an Adapter-created Asset and returns a bounded profile of
// supported tabular members. Production downloads stream to a temporary file;
// ZIP members are never extracted and expanded size and entry count are capped.
func (i *Inspector) Observe(ctx context.Context, asset Asset) (*Observation, error) {
	response, err := i.openAsset(ctx, asset.Request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.Status != http.StatusOK {
		return nil, fmt.Errorf("%s %s: unexpected status %d", asset.Request.Method, asset.Request.URL, response.Status)
	}
	if response.ContentLength > maxObservationDownloadBytes {
		return nil, fmt.Errorf("asset download가 허용 크기 %d bytes를 초과했습니다", maxObservationDownloadBytes)
	}

	temporary, err := os.CreateTemp("", "oddsock-observe-*")
	if err != nil {
		return nil, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	defer temporary.Close()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxObservationDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if written > maxObservationDownloadBytes {
		return nil, fmt.Errorf("asset download가 허용 크기 %d bytes를 초과했습니다", maxObservationDownloadBytes)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	observation := &Observation{SHA256: fmt.Sprintf("%x", hash.Sum(nil)), Bytes: written}
	if isZIPFile(asset.Name, temporary) {
		files, warnings, err := inspectZIP(temporary, written)
		if err != nil {
			return nil, err
		}
		observation.Files, observation.Warnings = files, warnings
		return observation, nil
	}
	body, err := io.ReadAll(io.LimitReader(temporary, maxExpandedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxExpandedBytes {
		return nil, fmt.Errorf("tabular asset이 검사 허용 한도 %d bytes를 초과했습니다", maxExpandedBytes)
	}
	file, ok, err := inspectTabular(asset.Name, body)
	if err != nil {
		return nil, err
	}
	if ok {
		observation.Files = []ObservedFile{file}
	} else {
		observation.Warnings = append(observation.Warnings, "지원되는 CSV 또는 DBF 스키마를 찾지 못했습니다")
	}
	return observation, nil
}

func (i *Inspector) openAsset(ctx context.Context, request Request) (*fetch.StreamResponse, error) {
	if stream, ok := i.http.(StreamingTransport); ok {
		switch request.Method {
		case http.MethodGet:
			return stream.OpenGET(ctx, request.URL)
		case http.MethodPost:
			return stream.OpenPostForm(ctx, request.URL, request.Form)
		default:
			return nil, fmt.Errorf("unsupported asset request method %q", request.Method)
		}
	}
	var (
		response *fetch.Response
		err      error
	)
	switch request.Method {
	case http.MethodGet:
		response, err = i.http.Get(ctx, request.URL)
	case http.MethodPost:
		response, err = i.http.PostForm(ctx, request.URL, request.Form)
	default:
		return nil, fmt.Errorf("unsupported asset request method %q", request.Method)
	}
	if err != nil {
		return nil, err
	}
	return &fetch.StreamResponse{
		Status: response.Status, ContentType: response.ContentType,
		ContentLength: int64(len(response.Body)), Body: io.NopCloser(bytes.NewReader(response.Body)),
	}, nil
}

func isZIPFile(name string, file *os.File) bool {
	if strings.EqualFold(path.Ext(name), ".zip") {
		return true
	}
	header := make([]byte, 4)
	read, _ := file.ReadAt(header, 0)
	return read == len(header) && bytes.Equal(header, []byte{'P', 'K', 3, 4})
}

func inspectZIP(reader io.ReaderAt, size int64) ([]ObservedFile, []string, error) {
	zr, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, nil, fmt.Errorf("ZIP 해석 실패: %w", err)
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, nil, fmt.Errorf("ZIP 항목이 허용 개수 %d개를 초과했습니다", maxArchiveEntries)
	}
	var files []ObservedFile
	var warnings []string
	var expanded uint64
	var observedBytes int64
	for _, member := range zr.File {
		expanded += member.UncompressedSize64
		if expanded > maxExpandedBytes {
			return nil, nil, fmt.Errorf("ZIP 해제 크기가 허용 한도 %d bytes를 초과했습니다", maxExpandedBytes)
		}
		memberName := decodeArchiveName(member.Name, member.NonUTF8)
		ext := strings.ToLower(path.Ext(memberName))
		if ext != ".csv" && ext != ".dbf" {
			continue
		}
		rc, err := member.Open()
		if err != nil {
			return nil, nil, err
		}
		remaining := int64(maxExpandedBytes) - observedBytes
		contents, readErr := io.ReadAll(io.LimitReader(rc, remaining+1))
		closeErr := rc.Close()
		if readErr != nil {
			return nil, nil, readErr
		}
		if closeErr != nil {
			return nil, nil, closeErr
		}
		if int64(len(contents)) > remaining {
			return nil, nil, fmt.Errorf("ZIP의 실제 해제 크기가 허용 한도 %d bytes를 초과했습니다", maxExpandedBytes)
		}
		observedBytes += int64(len(contents))
		observed, ok, inspectErr := inspectTabular(memberName, contents)
		if inspectErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s 스키마 검사 실패: %v", memberName, inspectErr))
			continue
		}
		if ok {
			files = append(files, observed)
		}
	}
	if len(files) == 0 {
		warnings = append(warnings, "ZIP에서 지원되는 CSV 또는 DBF 파일을 찾지 못했습니다")
	}
	return files, warnings, nil
}

func decodeArchiveName(name string, nonUTF8 bool) string {
	if !nonUTF8 || utf8.ValidString(name) {
		return name
	}
	decoded, _, err := transform.String(korean.EUCKR.NewDecoder(), name)
	if err != nil {
		return name
	}
	return decoded
}

func inspectTabular(name string, body []byte) (ObservedFile, bool, error) {
	switch strings.ToLower(path.Ext(name)) {
	case ".csv":
		columns, rows, err := inspectCSV(body)
		return ObservedFile{Name: name, Format: "CSV", Columns: columns, SampleRows: rows}, true, err
	case ".dbf":
		columns, rows, err := inspectDBF(body)
		return ObservedFile{Name: name, Format: "DBF", Columns: columns, SampleRows: rows}, true, err
	default:
		return ObservedFile{}, false, nil
	}
}

func inspectCSV(body []byte) ([]string, int, error) {
	decoded := body
	if !utf8.Valid(decoded) {
		var err error
		decoded, _, err = transform.Bytes(korean.EUCKR.NewDecoder(), decoded)
		if err != nil {
			return nil, 0, fmt.Errorf("CSV 인코딩 해석 실패: %w", err)
		}
	}
	decoded = bytes.TrimPrefix(decoded, []byte{0xEF, 0xBB, 0xBF})
	firstLine := string(decoded)
	if index := strings.IndexByte(firstLine, '\n'); index >= 0 {
		firstLine = firstLine[:index]
	}
	delimiter := ','
	if strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
		delimiter = '\t'
	} else if strings.Count(firstLine, "|") > strings.Count(firstLine, ",") {
		delimiter = '|'
	}
	reader := csv.NewReader(bytes.NewReader(decoded))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	columns, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("CSV header 해석 실패: %w", err)
	}
	for index := range columns {
		columns[index] = strings.TrimSpace(columns[index])
	}
	rows := 0
	for rows < maxSampleRows {
		_, err = reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, rows, fmt.Errorf("CSV 표본 해석 실패: %w", err)
		}
		rows++
	}
	return columns, rows, nil
}

func inspectDBF(body []byte) ([]string, int, error) {
	if len(body) < 33 {
		return nil, 0, fmt.Errorf("DBF header가 너무 짧습니다")
	}
	records := int(uint32(body[4]) | uint32(body[5])<<8 | uint32(body[6])<<16 | uint32(body[7])<<24)
	headerLength := int(uint16(body[8]) | uint16(body[9])<<8)
	if headerLength < 33 || headerLength > len(body) {
		return nil, 0, fmt.Errorf("DBF header length %d가 유효하지 않습니다", headerLength)
	}
	var columns []string
	for offset := 32; offset+32 <= headerLength; offset += 32 {
		if body[offset] == 0x0D {
			break
		}
		name := string(bytes.TrimRight(body[offset:offset+11], "\x00 "))
		if name != "" {
			columns = append(columns, name)
		}
	}
	if len(columns) == 0 {
		return nil, 0, fmt.Errorf("DBF field를 찾지 못했습니다")
	}
	if records > maxSampleRows {
		records = maxSampleRows
	}
	return columns, records, nil
}

func fileMetadata(doc *goquery.Document) map[string]string {
	metadata := map[string]string{}
	doc.Find("li").Each(func(_ int, item *goquery.Selection) {
		key := strings.TrimSpace(item.Find("strong.key").First().Text())
		if key == "" {
			return
		}
		valueNode := item.Find("div.value").First().Clone()
		valueNode.Find("script,style").Remove()
		value := strings.Join(strings.Fields(valueNode.Text()), " ")
		metadata[key] = value
	})
	return metadata
}

func externalSourceURL(doc *goquery.Document, metadata map[string]string) string {
	var source string
	doc.Find("li").EachWithBreak(func(_ int, item *goquery.Selection) bool {
		if strings.TrimSpace(item.Find("strong.key").First().Text()) != "URL" {
			return true
		}
		source, _ = item.Find("div.value a").First().Attr("href")
		return false
	})
	if source == "" {
		source = metadata["URL"]
	}
	return strings.TrimSpace(source)
}

type portalFileResolution struct {
	Status       bool   `json:"status"`
	AtchFileID   string `json:"atchFileId"`
	FileDetailSn string `json:"fileDetailSn"`
	File         struct {
		DataName     string `json:"dataNm"`
		OriginalName string `json:"orginlFileNm"`
		Extension    string `json:"atchFileExtsn"`
	} `json:"fileDataRegistVO"`
}

func (i *Inspector) resolvePortalAsset(ctx context.Context, match []string) (Asset, error) {
	resolveURL := i.base + "/tcs/dss/selectFileDataDownload.do"
	form := url.Values{
		"publicDataPk":       {match[1]},
		"publicDataDetailPk": {match[2]},
		"atchFileId":         {match[3]},
		"fileDetailSn":       {match[4]},
		"publicDataTyCode":   {"PR0051"},
	}
	res, err := i.http.PostForm(ctx, resolveURL, form)
	if err != nil {
		return Asset{}, err
	}
	if res.Status != http.StatusOK {
		return Asset{}, fmt.Errorf("POST %s: unexpected status %d", resolveURL, res.Status)
	}
	var resolved portalFileResolution
	if err := json.Unmarshal(res.Body, &resolved); err != nil {
		return Asset{}, fmt.Errorf("포털 파일 계약 응답 해석 실패: %w", err)
	}
	if !resolved.Status || resolved.AtchFileID == "" || resolved.FileDetailSn == "" {
		return Asset{}, fmt.Errorf("포털이 파일 다운로드 계약을 반환하지 않았습니다")
	}
	name := resolved.File.OriginalName
	if name == "" {
		name = resolved.File.DataName
	}
	download := i.base + "/cmm/cmm/fileDownload.do?" + url.Values{
		"atchFileId":   {resolved.AtchFileID},
		"fileDetailSn": {resolved.FileDetailSn},
		"dataNm":       {resolved.File.DataName},
	}.Encode()
	return Asset{
		Name: name, Format: strings.ToUpper(resolved.File.Extension),
		Request: Request{Method: http.MethodGet, URL: download},
	}, nil
}

func normalizeSeoulDatasetURL(raw string) (string, bool) {
	target, err := url.Parse(raw)
	if err != nil || target.User != nil || target.RawQuery != "" || target.Fragment != "" || target.Port() != "" {
		return "", false
	}
	if strings.ToLower(strings.TrimSuffix(target.Hostname(), ".")) != "data.seoul.go.kr" || !seoulDatasetPath.MatchString(target.EscapedPath()) {
		return "", false
	}
	target.Scheme = "https"
	target.Host = "data.seoul.go.kr"
	return target.String(), true
}

type seoulCatalogResponse struct {
	Catalog struct {
		Total  int `json:"list_total_count"`
		Result struct {
			Code string `json:"CODE"`
		} `json:"RESULT"`
		Rows []struct {
			ID          string `json:"INF_ID"`
			Name        string `json:"INF_NM"`
			Provider    string `json:"MNG_ORGAN_NAME"`
			UpdateCycle string `json:"CHNG_LOAD_CYCLE"`
			Description string `json:"INF_EXP"`
			ModifiedAt  string `json:"LAST_UPD_DTTM"`
			Delivery    string `json:"SRV_TYPE"`
			URL         string `json:"SHORT_URL"`
		} `json:"row"`
	} `json:"SearchOpenDataServiceList"`
}

type seoulDiscovery struct {
	Alternatives []Alternative
	FileURL      string
	Name         string
	Provider     string
	UpdateCycle  string
	ModifiedAt   string
	Description  string
	Evidence     Evidence
}

// seoulRepresentations asks Seoul's documented catalogue OpenAPI which
// representations exist. The public sample key is intentionally used: this
// lookup carries no user credential even though Seoul only serves the endpoint
// over HTTP. Actual credentialed API invocation remains blocked elsewhere.
func (i *Inspector) seoulRepresentations(ctx context.Context, datasetID string) (seoulDiscovery, error) {
	catalogURL := "http://openapi.seoul.go.kr:8088/sample/json/SearchOpenDataServiceList/1/5/" + datasetID + "/"
	res, err := i.http.Get(ctx, catalogURL)
	if err != nil {
		return seoulDiscovery{}, err
	}
	if res.Status != http.StatusOK {
		return seoulDiscovery{}, fmt.Errorf("GET %s: unexpected status %d", catalogURL, res.Status)
	}
	var response seoulCatalogResponse
	if err := json.Unmarshal(res.Body, &response); err != nil {
		return seoulDiscovery{}, fmt.Errorf("서울 서비스 목록 JSON 해석 실패: %w", err)
	}
	if response.Catalog.Result.Code != "INFO-000" {
		return seoulDiscovery{}, fmt.Errorf("서울 서비스 목록 API code %q", response.Catalog.Result.Code)
	}
	discovery := seoulDiscovery{Evidence: Evidence{
		Purpose: "provider-representations", Kind: EvidenceOfficialAPI,
		URL: catalogURL, Stability: StabilityDocumented,
	}}
	for _, row := range response.Catalog.Rows {
		if row.ID != datasetID {
			continue
		}
		delivery := strings.ToUpper(strings.TrimSpace(row.Delivery))
		normalized, ok := normalizeSeoulRepresentationURL(row.URL, datasetID, delivery)
		if !ok {
			continue
		}
		discovery.Alternatives = append(discovery.Alternatives, Alternative{Delivery: delivery, URL: normalized})
		if delivery == "FILE" {
			discovery.FileURL = normalized
		}
		if discovery.Name == "" {
			discovery.Name = strings.TrimSpace(row.Name)
			discovery.Provider = strings.TrimSpace(row.Provider)
			discovery.UpdateCycle = strings.TrimSpace(row.UpdateCycle)
			discovery.ModifiedAt = strings.TrimSpace(row.ModifiedAt)
			discovery.Description = strings.TrimSpace(row.Description)
		}
	}
	if len(discovery.Alternatives) == 0 {
		return seoulDiscovery{}, fmt.Errorf("서울 서비스 목록 API가 %s의 제공 형태를 반환하지 않았습니다", datasetID)
	}
	return discovery, nil
}

func normalizeSeoulRepresentationURL(raw, datasetID, delivery string) (string, bool) {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target.Scheme != "https" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || target.Port() != "" ||
		strings.ToLower(strings.TrimSuffix(target.Hostname(), ".")) != "data.seoul.go.kr" {
		return "", false
	}
	wantCode := map[string]string{"SHEET": "S", "FILE": "F", "OPENAPI": "A"}[delivery]
	if wantCode == "" || path.Clean(target.Path) != "/dataList/"+datasetID+"/"+wantCode+"/1/datasetView.do" {
		return "", false
	}
	target.Host = "data.seoul.go.kr"
	return target.String(), true
}

func (i *Inspector) inspectSeoul(ctx context.Context, contract *Contract, sourceURL string) (*Contract, error) {
	pathMatch := seoulDatasetPath.FindStringSubmatch(path.Clean(mustParseURLPath(sourceURL)))
	if len(pathMatch) != 3 {
		return nil, fmt.Errorf("서울 열린데이터광장 dataset id를 확인하지 못했습니다")
	}
	infID := pathMatch[1]
	discovery, catalogErr := i.seoulRepresentations(ctx, infID)
	if catalogErr != nil {
		contract.Warnings = append(contract.Warnings, "서울 공식 서비스 목록 API를 읽지 못해 공공데이터포털의 제공 URL로 대체했습니다: "+catalogErr.Error())
	} else {
		contract.Alternatives = discovery.Alternatives
		contract.Evidence = append(contract.Evidence, discovery.Evidence)
		if discovery.FileURL != "" {
			sourceURL = discovery.FileURL
		}
		if discovery.Name != "" {
			contract.Name = discovery.Name
		}
		if discovery.Provider != "" {
			contract.Provider = discovery.Provider
		}
		if discovery.UpdateCycle != "" {
			contract.UpdateCycle = discovery.UpdateCycle
		}
		if discovery.ModifiedAt != "" {
			contract.ModifiedAt = discovery.ModifiedAt
		}
		if discovery.Description != "" {
			contract.Metadata["description"] = discovery.Description
		}
	}
	res, err := i.http.Get(ctx, sourceURL)
	if err != nil {
		return nil, err
	}
	if res.Status != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %d", sourceURL, res.Status)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(res.Body))
	if err != nil {
		return nil, err
	}
	infSeq := strings.TrimSpace(doc.Find(`form[name="frmFile"] input[name="infSeq"]`).AttrOr("value", "3"))
	assets := make([]Asset, 0)
	doc.Find("table tbody tr").Each(func(_ int, row *goquery.Selection) {
		trigger := row.Find(`[onclick*="downloadFile"]`).First()
		onclick, _ := trigger.Attr("onclick")
		seqMatch := seoulDownload.FindStringSubmatch(onclick)
		if len(seqMatch) != 2 {
			return
		}
		name, _ := trigger.Attr("title")
		if name == "" {
			name = strings.TrimSpace(trigger.Text())
		}
		cells := row.Find("td")
		size, _ := strconv.ParseFloat(strings.TrimSpace(cells.Eq(3).Text()), 64)
		assets = append(assets, Asset{
			Name: name, Format: strings.ToUpper(strings.TrimPrefix(path.Ext(name), ".")),
			SizeBytes: int64(size * 1_000_000), ModifiedAt: strings.TrimSpace(cells.Eq(4).Text()),
			Request: Request{
				Method: http.MethodPost,
				URL:    "https://datafile.seoul.go.kr/bigfile/iot/inf/nio_download.do?useCache=false",
				Form:   url.Values{"infId": {infID}, "infSeq": {infSeq}, "seq": {seqMatch[1]}},
			},
		})
	})
	contract.SourceURL = sourceURL
	contract.AdapterID = "seoul-file"
	contract.AdapterRevision = 1
	contract.VerifiedAt = "2026-09-01"
	contract.Evidence = append(contract.Evidence, Evidence{
		Purpose: "asset-list", Kind: EvidenceFirstPartyWebContract,
		URL: sourceURL, Stability: StabilityFallback,
	})
	contract.Assets = assets
	if len(assets) > 0 {
		contract.Capability = CapabilityRetrievable
	} else {
		contract.Warnings = append(contract.Warnings, "서울 열린데이터광장 파일 목록을 확인하지 못했습니다")
	}
	return contract, nil
}

func mustParseURLPath(raw string) string {
	target, _ := url.Parse(raw)
	if target == nil {
		return ""
	}
	return target.Path
}
