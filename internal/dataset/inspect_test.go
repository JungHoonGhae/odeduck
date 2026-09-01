package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/JungHoonGhae/opendatactl/internal/fetch"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

type fixtureTransport struct {
	gets  map[string]*fetch.Response
	posts map[string]*fetch.Response
}

func (f fixtureTransport) Get(_ context.Context, rawURL string) (*fetch.Response, error) {
	res := f.gets[rawURL]
	if res == nil {
		return nil, fmt.Errorf("unexpected GET %s", rawURL)
	}
	return res, nil
}

func (f fixtureTransport) PostForm(_ context.Context, rawURL string, _ url.Values) (*fetch.Response, error) {
	res := f.posts[rawURL]
	if res == nil {
		return nil, fmt.Errorf("unexpected POST %s", rawURL)
	}
	return res, nil
}

func htmlResponse(body string) *fetch.Response {
	return &fetch.Response{Status: 200, ContentType: "text/html; charset=utf-8", Body: []byte(body)}
}

func jsonResponse(body string) *fetch.Response {
	return &fetch.Response{Status: 200, ContentType: "application/json", Body: []byte(body)}
}

func TestInspectorResolvesPortalHostedFileIntoActionableAsset(t *testing.T) {
	const base = "https://portal.test"
	const pk = "15151047"
	metadataURL := base + "/catalog/" + pk + "/fileData.json"
	detailURL := base + "/data/" + pk + "/fileData.do"
	resolveURL := base + "/tcs/dss/selectFileDataDownload.do"
	transport := fixtureTransport{
		gets: map[string]*fetch.Response{
			metadataURL: jsonResponse(`{
				"name":"공식 성장상권 이름",
				"alternateName":"성장상권_20250831",
				"description":"공식 표준 메타데이터 설명",
				"dateModified":"2025-11-10",
				"creator":{"name":"소상공인시장진흥공단"},
				"datasetTimeInterval":"수시 (1회성 데이터)",
				"encodingFormat":"SHP",
				"license":"이용허락범위 제한 없음",
				"@type":"Dataset"
			}`),
			detailURL: htmlResponse(`
			<h1>성장상권</h1>
			<button onclick="fileDetailObj.fn_fileDataDown('15151047', 'uddi:growth', '', '1', '3')">다운로드</button>
			<ul class="info-ul">
				<li><strong class="key">파일데이터명</strong><div class="value">오래된 HTML 이름</div></li>
				<li><strong class="key">제공기관</strong><div class="value">오래된 HTML 기관</div></li>
				<li><strong class="key">관리부서 전화번호</strong><div class="value"><span>02-1234-5678</span><script>secret implementation text</script></div></li>
				<li><strong class="key">업데이트 주기</strong><div class="value">수시 (1회성 데이터)</div></li>
				<li><strong class="key">확장자</strong><div class="value">SHP</div></li>
				<li><strong class="key">수정일</strong><div class="value">2025-11-10</div></li>
			</ul>`)},
		posts: map[string]*fetch.Response{resolveURL: jsonResponse(`{
			"status":true,
			"atchFileId":"FILE_123",
			"fileDetailSn":"1",
			"fileDataRegistVO":{"dataNm":"성장상권_20250831","orginlFileNm":"성장상권.zip","atchFileExtsn":"shp"}
		}`)},
	}

	contract, err := NewInspector(transport, base).Inspect(context.Background(), Ref{PK: pk, Delivery: DeliveryFile})
	if err != nil {
		t.Fatal(err)
	}
	if contract.Name != "성장상권_20250831" || contract.Provider != "소상공인시장진흥공단" || contract.UpdateCycle != "수시 (1회성 데이터)" {
		t.Fatalf("metadata = %+v", contract)
	}
	if contract.Metadata["description"] != "공식 표준 메타데이터 설명" || contract.Metadata["license"] != "이용허락범위 제한 없음" {
		t.Fatalf("standard metadata = %+v", contract.Metadata)
	}
	if len(contract.Evidence) != 2 || contract.Evidence[0].Kind != EvidenceStandardMetadata || contract.Evidence[0].Stability != StabilityDocumented || contract.Evidence[1].Kind != EvidenceFirstPartyWebContract || contract.Evidence[1].Stability != StabilityFallback {
		t.Fatalf("evidence = %+v", contract.Evidence)
	}
	if got := contract.Metadata["관리부서 전화번호"]; got != "02-1234-5678" {
		t.Fatalf("metadata must exclude script content, got %q", got)
	}
	if len(contract.Assets) != 1 {
		t.Fatalf("assets = %+v", contract.Assets)
	}
	asset := contract.Assets[0]
	if asset.Name != "성장상권.zip" || asset.Format != "SHP" || asset.Request.Method != "GET" {
		t.Fatalf("asset = %+v", asset)
	}
	if !strings.Contains(asset.Request.URL, "/cmm/cmm/fileDownload.do?atchFileId=FILE_123") || !strings.Contains(asset.Request.URL, "fileDetailSn=1") {
		t.Fatalf("download URL = %q", asset.Request.URL)
	}
	if contract.Capability != CapabilityRetrievable || contract.AdapterID != "datagokr-file" || contract.AdapterRevision != 1 {
		t.Fatalf("capability/adapter = %q %q@%d", contract.Capability, contract.AdapterID, contract.AdapterRevision)
	}
}

func TestInspectorFallsBackToLabelledHTMLWhenStandardMetadataFails(t *testing.T) {
	const (
		base = "https://portal.test"
		pk   = "15151046"
	)
	transport := fixtureTransport{gets: map[string]*fetch.Response{
		base + "/catalog/" + pk + "/fileData.json": {Status: http.StatusBadGateway},
		base + "/data/" + pk + "/fileData.do": htmlResponse(`
			<ul class="info-ul">
				<li><strong class="key">파일데이터명</strong><div class="value">HTML 대체 데이터</div></li>
				<li><strong class="key">제공기관</strong><div class="value">대체 기관</div></li>
				<li><strong class="key">업데이트 주기</strong><div class="value">월간</div></li>
				<li><strong class="key">확장자</strong><div class="value">CSV</div></li>
			</ul>`),
	}}
	contract, err := NewInspector(transport, base).Inspect(context.Background(), Ref{PK: pk, Delivery: DeliveryFile})
	if err != nil {
		t.Fatal(err)
	}
	if contract.Name != "HTML 대체 데이터" || contract.Provider != "대체 기관" || contract.UpdateCycle != "월간" || contract.DeclaredFormat != "CSV" {
		t.Fatalf("fallback contract = %+v", contract)
	}
	if len(contract.Evidence) != 1 || contract.Evidence[0].Kind != EvidenceFirstPartyWebContract || contract.Evidence[0].Stability != StabilityFallback {
		t.Fatalf("fallback evidence = %+v", contract.Evidence)
	}
	if len(contract.Warnings) != 2 || !strings.Contains(contract.Warnings[0], "HTTP 502") || !strings.Contains(contract.Warnings[1], "다운로드 계약") {
		t.Fatalf("fallback warnings = %+v", contract.Warnings)
	}
}

func TestInspectorExpandsSeoulHandoffIntoVersionedFileAssets(t *testing.T) {
	const base = "https://portal.test"
	const pk = "15076355"
	metadataURL := base + "/catalog/" + pk + "/fileData.json"
	detailURL := base + "/data/" + pk + "/fileData.do"
	seoulCatalogURL := "http://openapi.seoul.go.kr:8088/sample/json/SearchOpenDataServiceList/1/5/OA-15572/"
	seoulURL := "https://data.seoul.go.kr/dataList/OA-15572/F/1/datasetView.do"
	transport := fixtureTransport{gets: map[string]*fetch.Response{
		metadataURL: jsonResponse(`{
			"name":"서울시 상권 추정매출",
			"creator":{"name":"서울특별시"},
			"encodingFormat":"CSV",
			"@type":"Dataset"
		}`),
		detailURL: htmlResponse(`
			<ul class="info-ul">
				<li><strong class="key">파일데이터명</strong><div class="value">서울시 상권 추정매출</div></li>
				<li><strong class="key">제공기관</strong><div class="value">서울특별시</div></li>
				<li><strong class="key">제공형태</strong><div class="value">기관자체에서 다운로드(제공데이터URL기재)</div></li>
				<li><strong class="key">URL</strong><div class="value"><a href="http://data.seoul.go.kr/dataList/OA-15572/S/1/datasetView.do">바로가기</a></div></li>
			</ul>`),
		seoulCatalogURL: jsonResponse(`{
			"SearchOpenDataServiceList":{"list_total_count":3,"RESULT":{"CODE":"INFO-000","MESSAGE":"정상 처리되었습니다"},"row":[
				{"INF_ID":"OA-15572","INF_NM":"서울시 상권분석서비스(추정매출-상권)","MNG_ORGAN_NAME":"서울신용보증재단","CHNG_LOAD_CYCLE":"분기별","INF_EXP":"서울시 공식 최신 설명","LAST_UPD_DTTM":"2026-06-19","SRV_TYPE":"SHEET","SHORT_URL":"https://data.seoul.go.kr/dataList/OA-15572/S/1/datasetView.do"},
				{"INF_ID":"OA-15572","INF_NM":"서울시 상권분석서비스(추정매출-상권)","MNG_ORGAN_NAME":"서울신용보증재단","CHNG_LOAD_CYCLE":"분기별","INF_EXP":"서울시 공식 최신 설명","LAST_UPD_DTTM":"2026-06-19","SRV_TYPE":"FILE","SHORT_URL":"https://data.seoul.go.kr/dataList/OA-15572/F/1/datasetView.do"},
				{"INF_ID":"OA-15572","INF_NM":"서울시 상권분석서비스(추정매출-상권)","MNG_ORGAN_NAME":"서울신용보증재단","CHNG_LOAD_CYCLE":"분기별","INF_EXP":"서울시 공식 최신 설명","LAST_UPD_DTTM":"2026-06-19","SRV_TYPE":"OPENAPI","SHORT_URL":"https://data.seoul.go.kr/dataList/OA-15572/A/1/datasetView.do"}
			]}
		}`),
		seoulURL: htmlResponse(`
			<form name="frmFile"><input name="infId" value="OA-15572"><input name="infSeq" value="3"></form>
			<table><tbody>
			<tr><td>1</td><td>데이터</td><td><span title="추정매출_2025년.zip" onclick="downloadFile('51')">추정매출_2025년.zip</span></td><td>13.71</td><td>2026.05.18.</td></tr>
			<tr><td>2</td><td>데이터</td><td><span title="추정매출_2024년.zip" onclick="downloadFile('50')">추정매출_2024년.zip</span></td><td>14.21</td><td>2025.03.14.</td></tr>
			</tbody></table>`),
	}}

	contract, err := NewInspector(transport, base).Inspect(context.Background(), Ref{PK: pk, Delivery: DeliveryFile})
	if err != nil {
		t.Fatal(err)
	}
	if contract.AdapterID != "seoul-file" || contract.AdapterRevision != 1 || contract.Capability != CapabilityRetrievable || len(contract.Assets) != 2 {
		t.Fatalf("contract = %+v", contract)
	}
	if len(contract.Evidence) != 4 || contract.Evidence[2].Kind != EvidenceOfficialAPI || contract.Evidence[2].Purpose != "provider-representations" || contract.Evidence[3].Kind != EvidenceFirstPartyWebContract || contract.Evidence[3].Purpose != "asset-list" {
		t.Fatalf("evidence = %+v", contract.Evidence)
	}
	if contract.SourceURL != seoulURL || len(contract.Alternatives) != 3 || contract.Alternatives[2].Delivery != "OPENAPI" {
		t.Fatalf("official representations = source %q alternatives %+v", contract.SourceURL, contract.Alternatives)
	}
	if contract.Name != "서울시 상권분석서비스(추정매출-상권)" || contract.Provider != "서울신용보증재단" || contract.UpdateCycle != "분기별" || contract.ModifiedAt != "2026-06-19" || contract.Metadata["description"] != "서울시 공식 최신 설명" {
		t.Fatalf("provider metadata must override stale aggregator metadata: %+v", contract)
	}
	asset := contract.Assets[0]
	if asset.Name != "추정매출_2025년.zip" || asset.SizeBytes != 13_710_000 || asset.ModifiedAt != "2026.05.18." {
		t.Fatalf("asset metadata = %+v", asset)
	}
	if asset.Request.Method != "POST" || asset.Request.URL != "https://datafile.seoul.go.kr/bigfile/iot/inf/nio_download.do?useCache=false" {
		t.Fatalf("request = %+v", asset.Request)
	}
	if asset.Request.Form.Get("infId") != "OA-15572" || asset.Request.Form.Get("infSeq") != "3" || asset.Request.Form.Get("seq") != "51" {
		t.Fatalf("form = %+v", asset.Request.Form)
	}
}

func TestInspectorObservesCSVColumnsInsideZIPWithoutExtractingFiles(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	encodedName, _, err := transform.Bytes(korean.EUCKR.NewEncoder(), []byte("상권/매출.csv"))
	if err != nil {
		t.Fatal(err)
	}
	header := &zip.FileHeader{Name: string(encodedName), Method: zip.Deflate, NonUTF8: true}
	entry, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("기준_년분기_코드,상권_코드,당월_매출_금액\n20251,3110131,1000\n"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	const downloadURL = "https://files.test/sales.zip"
	transport := fixtureTransport{gets: map[string]*fetch.Response{
		downloadURL: {Status: 200, ContentType: "application/zip", Body: archive.Bytes()},
	}}

	observed, err := NewInspector(transport, "https://portal.test").Observe(context.Background(), Asset{
		Name: "sales.zip", Request: Request{Method: "GET", URL: downloadURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observed.SHA256 == "" || observed.Bytes != int64(archive.Len()) || len(observed.Files) != 1 {
		t.Fatalf("observation = %+v", observed)
	}
	file := observed.Files[0]
	want := []string{"기준_년분기_코드", "상권_코드", "당월_매출_금액"}
	if file.Name != "상권/매출.csv" || strings.Join(file.Columns, ",") != strings.Join(want, ",") || file.SampleRows != 1 || file.Format != "CSV" {
		t.Fatalf("observed file = %+v", file)
	}
}

func TestInspectorObservesDBFColumnsInsideShapefileZIP(t *testing.T) {
	dbf := minimalDBF([]string{"CRTR_YM", "MJR_BZZNNO", "AREA"})
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entry, err := zw.Create("growth.dbf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write(dbf)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	const downloadURL = "https://files.test/growth.zip"
	transport := fixtureTransport{gets: map[string]*fetch.Response{
		downloadURL: {Status: 200, ContentType: "application/zip", Body: archive.Bytes()},
	}}

	observed, err := NewInspector(transport, "https://portal.test").Observe(context.Background(), Asset{
		Name: "growth.zip", Format: "SHP", Request: Request{Method: "GET", URL: downloadURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Files) != 1 || strings.Join(observed.Files[0].Columns, ",") != "CRTR_YM,MJR_BZZNNO,AREA" || observed.Files[0].Format != "DBF" {
		t.Fatalf("DBF observation = %+v", observed)
	}
}

func TestInspectorRejectsUnsafeArchiveShapes(t *testing.T) {
	build := func(t *testing.T, write func(*zip.Writer)) []byte {
		t.Helper()
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		write(writer)
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return archive.Bytes()
	}

	tooMany := build(t, func(writer *zip.Writer) {
		for index := 0; index <= maxArchiveEntries; index++ {
			entry, err := writer.Create(fmt.Sprintf("%03d.csv", index))
			if err != nil {
				t.Fatal(err)
			}
			_, _ = entry.Write([]byte("column\n"))
		}
	})
	tooLarge := build(t, func(writer *zip.Writer) {
		header := &zip.FileHeader{Name: "oversized.csv", Method: zip.Store, UncompressedSize64: maxExpandedBytes + 1}
		header.CompressedSize64 = 0
		if _, err := writer.CreateRaw(header); err != nil {
			t.Fatal(err)
		}
	})

	for name, body := range map[string][]byte{
		"too-many":  tooMany,
		"too-large": tooLarge,
		"malformed": []byte("PK\x03\x04not-a-zip"),
	} {
		t.Run(name, func(t *testing.T) {
			const downloadURL = "https://files.test/archive.zip"
			transport := fixtureTransport{gets: map[string]*fetch.Response{
				downloadURL: {Status: http.StatusOK, ContentType: "application/zip", Body: body},
			}}
			_, err := NewInspector(transport, "https://portal.test").Observe(context.Background(), Asset{
				Name: "archive.zip", Request: Request{Method: http.MethodGet, URL: downloadURL},
			})
			if err == nil {
				t.Fatal("unsafe archive must fail")
			}
		})
	}
}

func TestInspectorRejectsUnsupportedMethodAndNon200Asset(t *testing.T) {
	inspector := NewInspector(fixtureTransport{gets: map[string]*fetch.Response{
		"https://files.test/missing.csv": {Status: http.StatusNotFound},
	}}, "https://portal.test")
	if _, err := inspector.Observe(context.Background(), Asset{
		Name: "missing.csv", Request: Request{Method: http.MethodGet, URL: "https://files.test/missing.csv"},
	}); err == nil || !strings.Contains(err.Error(), "unexpected status 404") {
		t.Fatalf("non-200 error = %v", err)
	}
	if _, err := inspector.Observe(context.Background(), Asset{
		Name: "bad.csv", Request: Request{Method: http.MethodDelete, URL: "https://files.test/bad.csv"},
	}); err == nil || !strings.Contains(err.Error(), "unsupported asset request method") {
		t.Fatalf("unsupported method error = %v", err)
	}
}

func TestInspectorStreamsProductionAssetPastBufferedLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("column_a,column_b\n1,2\n"))
	}))
	defer server.Close()

	client := fetch.New(fetch.WithDelay(0), fetch.WithMaxResponseBytes(8))
	observed, err := NewInspector(client, "https://portal.test").Observe(context.Background(), Asset{
		Name: "streamed.csv", Request: Request{Method: http.MethodGet, URL: server.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Files) != 1 || strings.Join(observed.Files[0].Columns, ",") != "column_a,column_b" {
		t.Fatalf("streamed observation = %+v", observed)
	}
}

func minimalDBF(fields []string) []byte {
	headerLength := 32 + 32*len(fields) + 1
	recordLength := 1 + 12*len(fields)
	out := make([]byte, headerLength+recordLength+1)
	out[0] = 0x03
	binary.LittleEndian.PutUint32(out[4:8], 1)
	binary.LittleEndian.PutUint16(out[8:10], uint16(headerLength))
	binary.LittleEndian.PutUint16(out[10:12], uint16(recordLength))
	for index, field := range fields {
		offset := 32 + index*32
		copy(out[offset:offset+11], field)
		out[offset+11] = 'C'
		out[offset+16] = 12
	}
	out[headerLength-1] = 0x0D
	out[len(out)-1] = 0x1A
	return out
}
