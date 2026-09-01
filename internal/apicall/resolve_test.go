package apicall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JungHoonGhae/oddsock/internal/fetch"
)

// describeFromHTML runs Describe against a page body, for assertions about a single
// row of the summary table.
func describeFromHTML(t *testing.T, body string) *APISpec {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body>" + body + "</body></html>"))
	}))
	t.Cleanup(srv.Close)
	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "1")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	return spec
}

// The same field arrives with two vocabularies depending on which source described
// the API: the embedded Swagger document says 필수/옵션, the rendered HTML table
// says the portal's own 필/옵. A check written against one of them silently passes
// everything from the other.
func TestIsRequiredAcceptsBothVocabularies(t *testing.T) {
	for v, want := range map[string]bool{
		"필수": true, "필": true, "  필  ": true, "Y": true,
		"옵션": false, "옵": false, "": false, "N": false,
	} {
		if got := isRequired(v); got != want {
			t.Errorf("isRequired(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestMissingRequired(t *testing.T) {
	op := &Operation{Params: []Param{
		{Name: "ServiceKey", Required: "필"}, // oddsock injects this one
		{Name: "pageNo", Required: "필"},
		{Name: "numOfRows", Required: "필수"},
		{Name: "bas_yy", Required: "옵"},
	}}
	missing := MissingRequired(op, map[string]string{"pageNo": "1"})
	if len(missing) != 1 || missing[0] != "numOfRows" {
		t.Errorf("missing = %v, want [numOfRows]", missing)
	}
	if m := MissingRequired(op, map[string]string{"pageNo": "1", "NUMOFROWS": "10"}); len(m) != 0 {
		t.Errorf("parameter names should match case-insensitively, still missing %v", m)
	}
	if m := MissingRequired(op, map[string]string{"pageNo": "1", "numOfRows": "10", "bas_yy": "2019"}); len(m) != 0 {
		t.Errorf("all required supplied, still missing %v", m)
	}
	if m := MissingRequired(op, map[string]string{"pageNo": " ", "numOfRows": "10"}); len(m) != 1 || m[0] != "pageNo" {
		t.Errorf("blank required value → missing %v, want pageNo", m)
	}
}

// A dataset whose parameters the portal never published must stay callable: an
// undocumented spec is not evidence that nothing is required.
func TestMissingRequiredSilentWhenSpecHasNoParams(t *testing.T) {
	if m := MissingRequired(&Operation{}, nil); m != nil {
		t.Errorf("undocumented operation should not block a call, got %v", m)
	}
	if m := MissingRequired(nil, nil); m != nil {
		t.Errorf("nil operation should not block a call, got %v", m)
	}
}

func TestValidateDataGoKRApplicationRoutesOnlyREST(t *testing.T) {
	tests := []struct {
		name string
		spec *APISpec
		want string
	}{
		{"nil", nil, "dataset 명세"},
		{"REST", &APISpec{PublicDataPk: "rest", APIType: "REST"}, ""},
		{"mixed REST LINK", &APISpec{PublicDataPk: "mixed", APIType: "REST/LINK"}, "LINK 유형"},
		{"known LINK", &APISpec{PublicDataPk: "link", APIType: "LINK", Handoff: &ExternalHandoff{Contract: &ExternalContract{ApplicationURL: "https://provider.example/apply"}}}, "https://provider.example/apply"},
		{"LINK without URL", &APISpec{PublicDataPk: "link", APIType: "LINK"}, "handoff.nextAction"},
		{"unknown", &APISpec{PublicDataPk: "unknown", APIType: "OTHER"}, "REST로 확인"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDataGoKRApplication(tt.spec)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("REST rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestOperationName(t *testing.T) {
	for endpoint, want := range map[string]string{
		"http://apis.data.go.kr/1741000/HeatWaveCasualtiesRegion/getHeatWaveCasualtiesRegionList": "getHeatWaveCasualtiesRegionList",
		"http://apis.data.go.kr/a/b/getX/": "getX",
		"":                                 "",
	} {
		if got := OperationName(Operation{Endpoint: endpoint}); got != want {
			t.Errorf("OperationName(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

// The portal renders some 상세기능 blocks twice (a PC table and a mobile one), which
// made a single-operation dataset report two identical operations — and then refuse
// to call it as "ambiguous", listing the same name twice as the choices.
func TestDedupeOperationsCollapsesIdenticalDuplicates(t *testing.T) {
	dup := Operation{
		Name:     "지역별 폭염 인명피해를 조회한다.",
		Endpoint: "http://apis.data.go.kr/1741000/HeatWaveCasualtiesRegion/getHeatWaveCasualtiesRegionList",
		Params:   []Param{{Name: "pageNo", Required: "필"}, {Name: "numOfRows", Required: "필"}},
	}
	if got := dedupeOperations([]Operation{dup, dup}); len(got) != 1 {
		t.Errorf("identical duplicates → %d operations, want 1", len(got))
	}
	// Same endpoint, different documented variables: genuinely two operations.
	other := dup
	other.Params = []Param{{Name: "pageNo", Required: "필"}}
	if got := dedupeOperations([]Operation{dup, other}); len(got) != 2 {
		t.Errorf("operations differing in parameters → %d, want 2", len(got))
	}
}

// oddsock applies for a development account, and the portal grades the two stages
// separately: every sampled dataset auto-approves at 개발단계 while a third of them
// require review at 운영단계. Conflating the two would either promise a key that
// needs a human or warn about review that never applies here.
func TestApprovalParsing(t *testing.T) {
	spec := describeFromHTML(t, `<table><tr>
		<th>심의유형</th><td> 개발단계 : 자동승인 / 운영단계 : 심의승인 </td>
	</tr></table>`)
	if spec.Approval == nil {
		t.Fatal("심의유형 row not surfaced")
	}
	if spec.Approval.Dev != "자동승인" || spec.Approval.Ops != "심의승인" {
		t.Errorf("dev=%q ops=%q, want 자동승인/심의승인", spec.Approval.Dev, spec.Approval.Ops)
	}
	if !spec.Approval.AutoApproved() {
		t.Error("dev 자동승인 should report AutoApproved")
	}
	if spec.Approval.Raw == "" {
		t.Error("raw row should be kept in case the wording changes")
	}
}

// A page without the row must not be reported as auto-approving.
func TestApprovalAbsentIsNotAssumedAutomatic(t *testing.T) {
	spec := describeFromHTML(t, `<table><tr><th>제공기관</th><td>어딘가</td></tr></table>`)
	if spec.Approval != nil {
		t.Errorf("no 심의유형 row → approval should stay nil, got %+v", spec.Approval)
	}
	if spec.Approval.AutoApproved() {
		t.Error("unknown approval must not read as automatic")
	}
}

// A review stage the portal words differently must not silently read as automatic.
func TestApprovalUnparsedKeepsRaw(t *testing.T) {
	spec := describeFromHTML(t, `<table><tr><th>심의유형</th><td>전면 심의 대상</td></tr></table>`)
	if spec.Approval == nil || spec.Approval.Raw != "전면 심의 대상" {
		t.Fatalf("unexpected approval %+v", spec.Approval)
	}
	if spec.Approval.AutoApproved() {
		t.Error("unrecognised wording must not read as auto-approved")
	}
}

// Every LINK dataset sampled (70/70) carries the publisher's address in the URL
// row, so a note telling the caller to consult the publisher without handing over
// that address withholds the only actionable thing on the page.
func TestLinkURLSurfacedAndNamedInNote(t *testing.T) {
	spec := describeFromHTML(t, `<table>
		<tr><th>API 유형</th><td>LINK</td></tr>
		<tr><th>URL</th><td><a href="https://www.safetydata.go.kr/disaster-data/view?dataSn=1326">https://www.safetydata.go.kr/disaster-data/view?dataSn=1326</a></td></tr>
		<tr><th>참고문서</th><td></td></tr>
	</table>`)
	if spec.LinkURL != "https://www.safetydata.go.kr/disaster-data/view?dataSn=1326" {
		t.Fatalf("linkUrl = %q, want the publisher's href", spec.LinkURL)
	}
	if spec.Handoff == nil || spec.Handoff.Host != "www.safetydata.go.kr" ||
		spec.Handoff.State != HandoffInspectionRequired || spec.Handoff.NextAction != HandoffInspectContract {
		t.Fatalf("handoff = %+v, want an explicitly uninspected external handoff", spec.Handoff)
	}
	if !strings.Contains(spec.Note, spec.LinkURL) {
		t.Errorf("note should hand over the address, got: %s", spec.Note)
	}
	// The publishers are a long tail with different contracts; treating every
	// handoff as an endpoint would send a caller down a dead end.
	if !strings.Contains(spec.Note, "단정할 수 없") || !strings.Contains(spec.Note, "call_api") {
		t.Errorf("note should forbid treating a heterogeneous link as an API endpoint, got: %s", spec.Note)
	}
}

// KRDS no longer renders the publisher URL in a labeled row. LINK pages show a
// button whose JavaScript asks selectApiLinkUrl.do for the target only when the
// user clicks it. Describe must follow that portal-owned lookup so callers still
// receive the one actionable address without having to drive a browser.
func TestLinkURLResolvedFromKRDSLookup(t *testing.T) {
	const pk = "15116894"
	lookupCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<html><body>
				<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
				<button onclick="fn_goUrlLink('15116894')">바로가기</button>
			</body></html>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			lookupCount++
			if got := r.URL.Query().Get("publicDataPk"); got != pk {
				t.Errorf("publicDataPk = %q, want %q", got, pk)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"publicDataDetailPk":"detail-1","linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, pk)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if lookupCount != 1 {
		t.Fatalf("link lookups = %d, want 1", lookupCount)
	}
	if spec.LinkURL != "https://www.safetykorea.kr/release/openapi" {
		t.Fatalf("linkUrl = %q", spec.LinkURL)
	}
	if spec.Handoff == nil || spec.Handoff.URL != spec.LinkURL || spec.Handoff.Host != "www.safetykorea.kr" {
		t.Fatalf("handoff = %+v, want resolved SafetyKorea handoff", spec.Handoff)
	}
	if spec.Handoff.State != HandoffContractKnown || spec.Handoff.NextAction != HandoffRequestAccess {
		t.Fatalf("handoff state = %+v, want known contract awaiting provider access", spec.Handoff)
	}
	if spec.Handoff.Trust != HandoffPublisherUntrusted {
		t.Fatalf("handoff trust = %q", spec.Handoff.Trust)
	}
	contract := spec.Handoff.Contract
	if contract == nil || contract.Provider != "SafetyKorea" || contract.AccessMode != "manual_approval" ||
		contract.Auth == nil || contract.Auth.Placement != "header" || contract.Auth.Name != "AuthKey" ||
		contract.InvocationState != InvocationImplemented || len(contract.Operations) != 5 {
		t.Fatalf("contract = %+v, want documented typed SafetyKorea contract", contract)
	}
	if !strings.Contains(spec.Note, spec.LinkURL) {
		t.Errorf("note should hand over the resolved address, got: %s", spec.Note)
	}
}

func TestLinkSkipsBrokenRESTOperationFragments(t *testing.T) {
	const pk = "15116894"
	fragmentCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<html><body>
				<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
				<input id="publicDataDetailPk" value="detail-1">
				<select id="open_api_detail_select"><option value="1">stale operation</option></select>
				<div id="apiDetailFunctionDiv"><div class="data-report-group"></div></div>
			</body></html>`))
		case "/tcs/dss/selectApiDetailFunction.do":
			fragmentCalls++
			w.WriteHeader(http.StatusInternalServerError)
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, pk)
	if err != nil {
		t.Fatalf("Describe LINK: %v", err)
	}
	if fragmentCalls != 0 || len(spec.Operations) != 0 || spec.Handoff == nil {
		t.Fatalf("LINK branch touched REST fragments or lost handoff: calls=%d spec=%+v", fragmentCalls, spec)
	}
}

func TestUnknownLinkHostStaysInspectionRequired(t *testing.T) {
	spec := describeFromHTML(t, `<table>
		<tr><th>API 유형</th><td>LINK</td></tr>
		<tr><th>URL</th><td><a href="https://provider.example/dataset/1">provider</a></td></tr>
		<tr><th>참고문서</th><td></td></tr>
	</table>`)
	if spec.Handoff == nil || spec.Handoff.State != HandoffInspectionRequired ||
		spec.Handoff.NextAction != HandoffInspectContract || spec.Handoff.Contract != nil {
		t.Fatalf("unknown provider must not receive an invented contract: %+v", spec.Handoff)
	}
}

func TestLegacyPlainTextLinkURLIsSurfaced(t *testing.T) {
	spec := describeFromHTML(t, `<table>
		<tr><th>API 유형</th><td>LINK</td></tr>
		<tr><th>URL</th><td>https://provider.example/open-api</td></tr>
	</table>`)
	if spec.LinkURL != "https://provider.example/open-api" || spec.Handoff == nil {
		t.Fatalf("plain-text handoff = %+v", spec)
	}
}

func TestSafetyKoreaContractNormalizesWWWHostAndSurfacesFullMetadata(t *testing.T) {
	spec := describeFromHTML(t, `<table>
		<tr><th>API 유형</th><td>LINK</td></tr>
		<tr><th>URL</th><td><a href="https://WWW.SafetyKorea.KR./release/openapi">provider</a></td></tr>
	</table>`)
	if spec.Handoff == nil {
		t.Fatal("normalized SafetyKorea URL did not produce a handoff")
	}
	if spec.Handoff.Host != "www.safetykorea.kr" {
		t.Fatalf("normalized handoff host = %q", spec.Handoff.Host)
	}
	contract := spec.Handoff.Contract
	if spec.Handoff.State != HandoffContractKnown || contract == nil {
		t.Fatalf("normalized SafetyKorea handoff = %+v", spec.Handoff)
	}
	if contract.DocumentationURL == "" || contract.DocumentationVersion != "2.0 (2025-06-30)" ||
		contract.ApplicationURL != "https://www.safetykorea.kr/release/openapi2" ||
		contract.VerifiedAt != "2026-09-01" || contract.Auth.Type != "api_key" ||
		contract.Auth.CredentialScope != "https://www.safetykorea.kr/openapi/api/" {
		t.Fatalf("contract metadata = %+v", contract)
	}
}

func TestSafetyKoreaUnverifiedPathStaysInspectionRequired(t *testing.T) {
	spec := describeFromHTML(t, `<table>
		<tr><th>API 유형</th><td>LINK</td></tr>
		<tr><th>URL</th><td><a href="https://www.safetykorea.kr/another-service">provider</a></td></tr>
	</table>`)
	if spec.Handoff == nil || spec.Handoff.State != HandoffInspectionRequired || spec.Handoff.Contract != nil {
		t.Fatalf("unverified provider path received an over-broad contract: %+v", spec.Handoff)
	}
}

func TestSafetyKoreaInsecureOriginStaysInspectionRequired(t *testing.T) {
	for _, raw := range []string{
		"http://www.safetykorea.kr/release/openapi",
		"https://www.safetykorea.kr:8443/release/openapi",
		"https://www.safetykorea.kr/release%2Fopenapi",
		"https://www.safetykorea.kr/release/openapi?mode=other",
		"https://www.safetykorea.kr/release/openapi#other",
	} {
		spec := describeFromHTML(t, `<table>
			<tr><th>API 유형</th><td>LINK</td></tr>
			<tr><th>URL</th><td><a href="`+raw+`">provider</a></td></tr>
		</table>`)
		if spec.Handoff == nil || spec.Handoff.State != HandoffInspectionRequired || spec.Handoff.Contract != nil {
			t.Errorf("insecure origin %q received a contract: %+v", raw, spec.Handoff)
		}
	}
}

func TestResolveRejectsEndpointLookingMarkupOnLinkDataset(t *testing.T) {
	isolateResolveCatalog(t)
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/"+pk+"/openapi.do" {
			_, _ = w.Write([]byte(`<html><body>
				<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
				<p>참고 예시: https://apis.data.go.kr/example/getItems</p>
				<table><tr><th>URL</th><td><a href="https://provider.example/dataset">provider</a></td></tr></table>
			</body></html>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	fc := fetch.New(fetch.WithDelay(0))
	spec, err := Describe(context.Background(), fc, srv.URL, pk)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(spec.Operations) != 0 || spec.EndpointOnly {
		t.Fatalf("LINK describe exposed call-shaped operations: %+v", spec)
	}

	_, err = Resolve(context.Background(), fc, srv.URL, pk, "")
	if err == nil || !strings.Contains(err.Error(), "LINK 유형") || !strings.Contains(err.Error(), "DatasetCaller") {
		t.Fatalf("Resolve error = %v, want enforced LINK invocation boundary", err)
	}
}

func TestUnknownAPITypeFailsClosed(t *testing.T) {
	isolateResolveCatalog(t)
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
			<p>markup drift hid the API type: https://apis.data.go.kr/example/getItems</p>
		</body></html>`))
	}))
	defer srv.Close()

	fc := fetch.New(fetch.WithDelay(0))
	spec, err := Describe(context.Background(), fc, srv.URL, pk)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(spec.Operations) != 0 || spec.EndpointOnly || !strings.Contains(spec.Note, "API 유형") {
		t.Fatalf("unknown API type did not fail closed: %+v", spec)
	}
	if _, err := Resolve(context.Background(), fc, srv.URL, pk, ""); err == nil || !strings.Contains(err.Error(), "REST로 확인") {
		t.Fatalf("Resolve error = %v, want unknown-type rejection", err)
	}
}

func isolateResolveCatalog(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func TestLinkURLLookupRejectsUnsafeTarget(t *testing.T) {
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/"+pk+"/openapi.do" {
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"linkUrl":"javascript:alert(1)","status":true}`))
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, pk)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if spec.LinkURL != "" || spec.Handoff == nil || spec.Handoff.URL != "" ||
		spec.Handoff.State != HandoffResolutionFailed || spec.Handoff.NextAction != HandoffChooseAnother ||
		spec.Handoff.FetchPolicy != "" || spec.Handoff.Failure == nil ||
		spec.Handoff.Failure.Code != "link_unsafe_target" || spec.Handoff.Failure.Retryable {
		t.Fatalf("unsafe target must surface only as a structured failure: %+v", spec)
	}
	if !strings.Contains(spec.Note, "유효한 HTTP(S)") {
		t.Fatalf("note should explain rejected handoff, got: %s", spec.Note)
	}
}

func TestLinkURLLookupSurfacesRetryableFailure(t *testing.T) {
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data/"+pk+"/openapi.do" {
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	spec, err := Describe(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, pk)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if spec.Handoff == nil || spec.Handoff.State != HandoffResolutionFailed ||
		spec.Handoff.NextAction != HandoffRetryResolution || spec.Handoff.Failure == nil ||
		spec.Handoff.FetchPolicy != "" || spec.Handoff.Failure.Code != "link_http_error" || !spec.Handoff.Failure.Retryable {
		t.Fatalf("retryable handoff failure = %+v", spec.Handoff)
	}
}

func TestResolvePortalLinkURLFailureModes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{name: "http status", statusCode: http.StatusBadGateway, body: `upstream failed`, want: "HTTP 502"},
		{name: "malformed json", statusCode: http.StatusOK, body: `{`, want: "응답 해석 실패"},
		{name: "portal error", statusCode: http.StatusOK, body: `{"status":false,"errorDc":"서비스 종료"}`, want: "서비스 종료"},
		{name: "empty portal error", statusCode: http.StatusOK, body: `{"status":false}`, want: "실패 상태"},
		{name: "empty target", statusCode: http.StatusOK, body: `{"status":true}`, want: "유효한 HTTP(S)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/tcs/dss/selectApiLinkUrl.do" {
					t.Fatalf("path = %q", r.URL.Path)
				}
				if got := r.URL.Query().Get("publicDataPk"); got != "15116894" {
					t.Fatalf("publicDataPk = %q", got)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := resolvePortalLinkURL(context.Background(), fetch.New(fetch.WithDelay(0)), srv.URL, "15116894")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestValidatePublisherLinkURLRejectsUntrustedShapes(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)",
		"/relative/provider/page",
		"ftp://provider.example/spec",
		"https://user" + "@provider.example/spec",
		"http://localhost/admin",
		"http://service." + "local/admin",
		"http://127.0.0.1/admin",
		"http://10.0.0.1/admin",
		"http://169." + "254.169.254/latest/meta-data",
		"http://[::1]/admin",
		"http://127.1/admin",
		"http://21307" + "06433/admin",
		"http://0x7f000001/admin",
		"http://[fe80::1%25en0]/admin",
		"http://１２７。０。０。１/admin",
		"http://１２７．０．０．１/admin",
		"http://0x７f000001/admin",
	} {
		if _, err := validatePublisherLinkURL(raw); err == nil {
			t.Errorf("validatePublisherLinkURL(%q) succeeded, want rejection", raw)
		}
	}
}

func TestValidatePublisherLinkURLAllowsHexLookingDNSNames(t *testing.T) {
	for _, raw := range []string{
		"https://abc123.de/openapi",
		"https://b2b.de/openapi",
		"https://face1.de/openapi",
		"https://예시.한국/openapi",
	} {
		got, err := validatePublisherLinkURL(raw)
		if err != nil {
			t.Errorf("validatePublisherLinkURL(%q) = %v, want legitimate DNS name", raw, err)
		}
		if strings.Contains(raw, "예시") && !strings.Contains(got, "xn--") {
			t.Errorf("international hostname was not normalized: %q", got)
		}
	}
}

func TestResolvePortalLinkURLDoesNotFollowRedirect(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		_, _ = w.Write([]byte(`must not be fetched`))
	}))
	defer target.Close()

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer portal.Close()

	_, err := resolvePortalLinkURL(context.Background(), fetch.New(fetch.WithDelay(0)), portal.URL, "15116894")
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("redirect error = %v", err)
	}
	if got := targetRequests.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want 0", got)
	}
}

// A REST page has no URL row, and must not acquire an empty linkUrl.
func TestLinkURLAbsentOnRestPage(t *testing.T) {
	spec := describeFromHTML(t, `<table><tr><th>API 유형</th><td>REST</td></tr></table>`)
	if spec.LinkURL != "" {
		t.Errorf("linkUrl = %q, want empty", spec.LinkURL)
	}
}
