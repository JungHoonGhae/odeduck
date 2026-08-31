package apicall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/JungHoonGhae/gongctl/internal/fetch"
)

func TestDescribeReadsKRDSMetadata(t *testing.T) {
	body := `<html><body>
		<h1 class="h-tit">중소기업 지원사업 공고 조회 서비스</h1>
		<ul class="info-ul row">
			<li><strong class="key">API 유형</strong><div class="value">REST</div></li>
			<li><strong class="key">심의유형</strong><div class="value">
				개발단계 : 자동승인 / 운영단계 : 심의승인
			</div></li>
		</ul>
	</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15157820")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if spec.DataName != "중소기업 지원사업 공고 조회 서비스" {
		t.Errorf("dataName = %q", spec.DataName)
	}
	if spec.APIType != "REST" {
		t.Errorf("apiType = %q", spec.APIType)
	}
	if spec.Approval == nil || spec.Approval.Dev != "자동승인" || spec.Approval.Ops != "심의승인" {
		t.Errorf("approval = %+v", spec.Approval)
	}
}

func TestDescribeLoadsEveryKRDSOperationFragment(t *testing.T) {
	operation := func(endpoint, param string) string {
		return `<div class="data-info-tit"><h2>기능 설명</h2></div>
			<div class="data-report-group">
				<div class="data-report"><h4>API 기본 정보</h4><ul class="info-ul"><li>
					<strong class="key">요청주소</strong><div class="value">` + endpoint + `</div>
				</li></ul></div>
				<div class="data-report"><h4>요청변수(Request Parameter)</h4><table>
					<thead><tr><th>항목명(영문)</th><th>항목구분</th><th>샘플데이터</th><th>항목설명</th></tr></thead>
					<tbody><tr><td>` + param + `</td><td>필</td><td>10</td><td>테스트 변수</td></tr></tbody>
				</table></div>
			</div>`
	}
	page := `<html><body>
		<input id="publicDataDetailPk" value="detail-1"><input id="publicDataPk" value="15076352">
		<select id="open_api_detail_select">
			<option value="29456">충전소 상태</option><option value="29457">충전소 정보</option>
		</select>
		<div id="apiDetailFunctionDiv">` + operation("https://apis.data.go.kr/test/getStatus", "pageNo") + `</div>
	</body></html>`

	postCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(page))
			return
		}
		postCount++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.URL.Path != "/tcs/dss/selectApiDetailFunction.do" ||
			r.Form.Get("oprtinSeqNo") != "29457" ||
			r.Form.Get("publicDataDetailPk") != "detail-1" ||
			r.Form.Get("publicDataPk") != "15076352" {
			t.Errorf("unexpected operation request: path=%s form=%v", r.URL.Path, r.Form)
		}
		_, _ = w.Write([]byte(operation("https://apis.data.go.kr/test/getInfo", "numOfRows")))
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15076352")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("operation fragment posts = %d, want 1 for the non-initial option", postCount)
	}
	if len(spec.Operations) != 2 {
		t.Fatalf("operations = %+v", spec.Operations)
	}
	if spec.Operations[0].Name != "충전소 상태" || spec.Operations[0].Params[0].Name != "pageNo" {
		t.Errorf("first operation = %+v", spec.Operations[0])
	}
	if spec.Operations[1].Name != "충전소 정보" || spec.Operations[1].Params[0].Name != "numOfRows" {
		t.Errorf("second operation = %+v", spec.Operations[1])
	}
}

func TestDescribe(t *testing.T) {
	body, err := os.ReadFile("testdata/op-15000908.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(body)
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15000908")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	// This fixture has ONE operation, rendered twice. Its two
	// .open-api-detail-result sections are identical — same title, same endpoint
	// (getPoelpcddRegistSttusInfoInqire), same seven request variables — because the
	// page carries a desktop and a mobile copy of the same table. The fixture holds
	// exactly one distinct apis.data.go.kr operation path, and so does the live page.
	//
	// This assertion previously demanded two, describing them as 예비후보자 + 후보자
	// with separate endpoints, which the fixture never contained: the earlier
	// selector fix moved describe from swallowing content to double-counting it, and
	// the double count was recorded as correct. What must hold is that the real
	// operation survives with its endpoint and parameters, and that the
	// operation-switcher box (a <select> and a button, carrying no data) never
	// appears as an operation of its own.
	if len(spec.Operations) != 1 {
		t.Fatalf("want the 1 real operation, got %d: %+v", len(spec.Operations), spec.Operations)
	}
	for i, op := range spec.Operations {
		if !strings.Contains(op.Endpoint, "apis.data.go.kr") {
			t.Errorf("op %d: expected apis.data.go.kr endpoint, got %q", i, op.Endpoint)
		}
		var hasNumOfRows bool
		for _, p := range op.Params {
			if p.Name == "numOfRows" {
				hasNumOfRows = true
			}
			if p.Name == "resultCode" || p.Name == "resultMsg" {
				t.Errorf("op %d: response-only field %q misattributed as a request param", i, p.Name)
			}
		}
		if !hasNumOfRows {
			t.Errorf("op %d: expected numOfRows request param surfaced, got %+v", i, op.Params)
		}
	}
}

// A malformed page must not invent params — surface RawHTML instead of fabricating.
func TestDescribeSurfaceFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><div class="open-api-detail">
			<h4>테스트기능</h4><p>표 구조가 없는 안내문</p></div></body></html>`))
	}))
	defer srv.Close()
	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "1")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(spec.Operations) != 1 {
		t.Fatalf("want 1 op, got %d", len(spec.Operations))
	}
	if len(spec.Operations[0].Params) != 0 {
		t.Error("must not fabricate params when no request-variable table exists")
	}
	if spec.Operations[0].RawHTML == "" {
		t.Error("expected RawHTML surfaced as fallback")
	}
}

// Some OpenAPI pages document nothing inline — no endpoint, no request-variable
// table — because the whole spec ships in an attached guide document. Returning
// an empty Operations list there is a dead end for an agent, so Describe must
// point at the guide it can actually fetch.
func TestDescribeGuideOnlyPage(t *testing.T) {
	body, err := os.ReadFile("testdata/openapi-guide-only.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(body)
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15012005")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(spec.Operations) != 0 {
		t.Errorf("must not invent operations, got %d", len(spec.Operations))
	}
	if spec.GuideDoc == "" {
		t.Error("guideDoc name missing")
	}
	// The name alone is not actionable — the agent needs a URL it can fetch.
	if !strings.Contains(spec.GuideDocURL, "FILE_000000003547578") {
		t.Errorf("guideDocUrl = %q, want a download URL carrying the atchFileId", spec.GuideDocURL)
	}
	if spec.Note == "" {
		t.Error("expected a note telling the agent where the spec actually lives")
	}
}

// A page that DOES document its operations must not gain a note.
func TestDescribeNoNoteWhenOperationsFound(t *testing.T) {
	body, err := os.ReadFile("testdata/op-15000908.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(body)
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15000908")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if spec.Note != "" {
		t.Errorf("note should be empty when operations exist, got %q", spec.Note)
	}
}

// The portal embeds the authoritative spec as Swagger 2.0 in a JS template
// literal — endpoint, per-parameter name/required/description, the lot. Scraping
// the HTML tables instead misses it entirely, which is how a fully documented API
// came back looking undocumented.
func TestDescribeReadsEmbeddedSwagger(t *testing.T) {
	body, err := os.ReadFile("testdata/openapi-swagger.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(body)
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15127057")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(spec.Operations) != 1 {
		t.Fatalf("want 1 operation from swagger, got %d", len(spec.Operations))
	}
	op := spec.Operations[0]
	if !strings.Contains(op.Endpoint, "apis.data.go.kr/1130000/ClslVioltDtl_2Service/getClslVioltDetailInfo_2") {
		t.Errorf("endpoint = %q, want host+path assembled from swagger", op.Endpoint)
	}
	if op.Name == "" {
		t.Error("operation name (swagger summary) missing")
	}
	if len(op.Params) < 8 {
		t.Fatalf("want the 8 documented params, got %d", len(op.Params))
	}
	var svcKey, bzmn *Param
	for i := range op.Params {
		switch op.Params[i].Name {
		case "serviceKey":
			svcKey = &op.Params[i]
		case "bzmnNm":
			bzmn = &op.Params[i]
		}
	}
	if svcKey == nil || bzmn == nil {
		t.Fatal("expected serviceKey and bzmnNm among params")
	}
	if svcKey.Required != "필수" {
		t.Errorf("serviceKey required = %q, want 필수", svcKey.Required)
	}
	if bzmn.Required != "옵션" {
		t.Errorf("bzmnNm required = %q, want 옵션", bzmn.Required)
	}
	if bzmn.Desc == "" {
		t.Error("param description not carried over")
	}
	// A spec this complete must not be flagged as endpoint-only or noted as missing.
	if spec.EndpointOnly || spec.Note != "" {
		t.Errorf("complete swagger spec should carry no note; got endpointOnly=%v note=%q",
			spec.EndpointOnly, spec.Note)
	}
}
