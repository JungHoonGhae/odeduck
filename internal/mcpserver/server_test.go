package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/connectionledger"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

func isolateConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

type datasetCallFunc func(context.Context, apicall.DatasetCallRequest) (*apicall.CallResult, error)

func (f datasetCallFunc) Call(ctx context.Context, request apicall.DatasetCallRequest) (*apicall.CallResult, error) {
	return f(ctx, request)
}

func TestCallAPIDispatchesThroughUnifiedDatasetCaller(t *testing.T) {
	called := false
	caller := datasetCallFunc(func(_ context.Context, request apicall.DatasetCallRequest) (*apicall.CallResult, error) {
		called = true
		if request.PK != "15116894" || request.Operation != "certificationDetail" || request.Params["certNum"] != "SU123" {
			t.Fatalf("request = %#v", request)
		}
		if request.Wait != maxToolWait {
			t.Fatalf("wait = %s, want clamp %s", request.Wait, maxToolWait)
		}
		return &apicall.CallResult{Status: 200, ContentType: "application/json", Body: map[string]any{"items": []any{map[string]any{"certNum": "SU123"}}}}, nil
	})
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), Caller: caller}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "call_api", Arguments: map[string]any{
		"pk": "15116894", "op": "certificationDetail", "params": map[string]any{"certNum": "SU123"},
		"waitSeconds": 3600, "profileFields": []string{"certNum"},
	}})
	if err != nil || res.IsError || !called {
		t.Fatalf("call_api err=%v result=%#v called=%v", err, res, called)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var result apicall.CallResult
	if err := json.Unmarshal(raw, &result); err != nil || result.Profile == nil || len(result.Profile.Fields) != 1 || result.Profile.Fields[0].DistinctCount != 1 {
		t.Fatalf("profiled result = %#v decode error=%v", result, err)
	}
}

func TestSearchToolRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write([]byte(`<div class="apply-result-item">
			<div class="apply-result-category"><span class="krds-badge">공공행정</span><span class="krds-badge">국가기관</span></div>
			<div class="apply-result-link"><a href="/data/12345/openapi.do">테스트 OpenAPI</a></div>
			<p class="apply-result-summary">설명</p>
			<div class="in-result-item"><ul><li><strong>제공기관</strong>테스트청</li></ul></div>
		</div>`))
	}))
	defer srv.Close()

	server := New(Deps{
		Fetch:   fetch.New(fetch.WithDelay(0)),
		BaseURL: srv.URL,
	})

	ctx := context.Background()
	sess := connectTestClient(t, server)

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_datasets",
		Arguments: map[string]any{"keyword": "테스트"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got searchOut
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Datasets) != 1 || got.Datasets[0].PublicDataPk != "12345" || got.Datasets[0].Title != "테스트 OpenAPI" {
		t.Fatalf("search datasets = %+v", got.Datasets)
	}
}

func TestDescribeLinkHandoffRoundTrip(t *testing.T) {
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<html><body>
				<h1 class="h-tit">제품 안전인증 및 리콜 정보</h1>
				<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
			</body></html>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), BaseURL: srv.URL}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "describe_api", Arguments: map[string]any{"pk": pk},
	})
	if err != nil {
		t.Fatalf("call describe_api: %v", err)
	}
	if res.IsError {
		t.Fatalf("describe_api returned error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got apicall.APISpec
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.APIType != "LINK" || got.Handoff == nil || got.Handoff.State != apicall.HandoffContractKnown ||
		got.Handoff.Trust != apicall.HandoffPublisherUntrusted ||
		got.Handoff.FetchPolicy != apicall.HandoffSafeFetcherRequired ||
		got.Handoff.NextAction != apicall.HandoffRequestAccess || got.Handoff.Contract == nil ||
		got.Handoff.Contract.AdapterID != "safetykorea" || got.Handoff.Contract.AdapterRevision != 2 ||
		got.Handoff.Contract.InvocationState != apicall.InvocationImplemented || len(got.Handoff.Contract.Operations) != 5 ||
		got.Handoff.Contract.Auth == nil || got.Handoff.Contract.Auth.Name != "AuthKey" {
		t.Fatalf("LINK structured content = %+v", got)
	}
}

func TestInspectDatasetResolvesAndObservesFileData(t *testing.T) {
	isolateConfigHome(t)
	const pk = "15151047"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{
		PK: pk, Title: "성장상권", SvcType: catalog.SvcFILE, DataTypes: []string{"FILE"},
	}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/" + pk + "/fileData.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"alternateName":"성장상권","creator":{"name":"소상공인시장진흥공단"},"encodingFormat":"CSV","@type":"Dataset"}`))
		case "/data/" + pk + "/fileData.do":
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15151047','uddi:growth','','1','3')">다운로드</button>
				<li><strong class="key">파일데이터명</strong><div class="value">성장상권</div></li>
				<li><strong class="key">제공기관</strong><div class="value">소상공인시장진흥공단</div></li>
				<li><strong class="key">확장자</strong><div class="value">CSV</div></li>`))
		case "/tcs/dss/selectFileDataDownload.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE_123","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"성장상권","orginlFileNm":"growth.csv","atchFileExtsn":"csv"}}`))
		case "/cmm/cmm/fileDownload.do":
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte("CRTR_YM,MJR_BZZNNO,AREA\n202401,100,42\n"))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer srv.Close()

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), BaseURL: srv.URL}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "inspect_dataset", Arguments: map[string]any{"pk": pk, "observe": true},
	})
	if err != nil {
		t.Fatalf("inspect_dataset transport: %v", err)
	}
	if res.IsError {
		t.Fatalf("inspect_dataset error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Delivery    string               `json:"delivery"`
		File        *dataset.Contract    `json:"file"`
		Observation *dataset.Observation `json:"observation"`
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Delivery != catalog.SvcFILE || got.File == nil || got.File.Capability != dataset.CapabilityRetrievable {
		t.Fatalf("inspection = %+v", got)
	}
	if got.Observation == nil || len(got.Observation.Files) != 1 || strings.Join(got.Observation.Files[0].Columns, ",") != "CRTR_YM,MJR_BZZNNO,AREA" {
		t.Fatalf("observation = %+v", got.Observation)
	}
}

func TestApplyRejectsLinkBeforeDataGoKRSideEffect(t *testing.T) {
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			t.Fatalf("unexpected request before LINK apply rejection: %s", r.URL)
		}
	}))
	defer srv.Close()

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), BaseURL: srv.URL}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply", Arguments: map[string]any{
		"pk": pk, "purpose": "제품 안전 분석", "category": "research",
	}})
	if err != nil {
		t.Fatalf("apply transport: %v", err)
	}
	raw, _ := json.Marshal(res.Content)
	if !res.IsError || !strings.Contains(string(raw), "LINK") || !strings.Contains(string(raw), "https://www.safetykorea.kr/release/openapi2") {
		t.Fatalf("LINK apply result = %#v", res)
	}
}

func TestDescribeProviderServiceAdapterRoundTrip(t *testing.T) {
	const pk = "15058359"
	const target = "https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=I-0040&svc_type_cd=API_TYPE06"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<html><body>
				<h1 class="h-tit">건강기능식품 기능성 원료인정 현황</h1>
				<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>
			</body></html>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"` + target + `","status":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), BaseURL: srv.URL}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "describe_api", Arguments: map[string]any{"pk": pk},
	})
	if err != nil {
		t.Fatalf("call describe_api: %v", err)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var got apicall.APISpec
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Handoff == nil || got.Handoff.Contract == nil ||
		got.Handoff.Contract.AdapterID != "foodsafetykorea" || got.Handoff.Contract.AdapterRevision != 2 ||
		got.Handoff.Contract.ProviderServiceID != "I-0040" || got.Handoff.Contract.Auth == nil ||
		got.Handoff.Contract.Auth.Placement != "path" ||
		got.Handoff.Contract.Auth.CredentialScope != "https://openapi.foodsafetykorea.go.kr/api/" ||
		got.Handoff.Contract.InvocationState != apicall.InvocationImplemented || len(got.Handoff.Contract.Operations) != 1 {
		t.Fatalf("FoodSafetyKorea structured content = %+v", got.Handoff)
	}
}

func TestToolCatalogPresentsProgressiveDiscoveryWorkflow(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	var names []string
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		byName[tool.Name] = tool
	}
	wantSteps := []struct {
		name  string
		stage string
	}{
		{name: "catalog_search", stage: "1단계"},
		{name: "inspect_dataset", stage: "2단계"},
		{name: "call_api", stage: "3단계"},
	}
	if byName["describe_api"] == nil {
		t.Fatal("describe_api compatibility tool must remain available")
	}
	if !strings.Contains(byName["inspect_dataset"].Description, "observe=true") ||
		!strings.Contains(byName["inspect_dataset"].Description, "SHA-256") {
		t.Fatal("inspect_dataset must explain observed FILE schema evidence")
	}
	for _, want := range wantSteps {
		name := want.name
		tool := byName[name]
		if tool == nil {
			t.Fatalf("missing primary tool %q; tools = %v", name, names)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("primary read tool %q must be annotated read-only", name)
		}
		if !strings.Contains(tool.Title+tool.Annotations.Title+tool.Description, want.stage) {
			t.Errorf("primary tool %q does not identify itself as %s", name, want.stage)
		}
	}
	if !strings.Contains(byName["describe_api"].Description, "invocationState=implemented") ||
		!strings.Contains(byName["describe_api"].Description, "call_api") ||
		!strings.Contains(byName["describe_api"].Description, "safe_fetcher_required") {
		t.Fatal("describe_api must explain typed LINK invocation")
	}

	callSchema, ok := byName["call_api"].InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("call_api schema type = %T", byName["call_api"].InputSchema)
	}
	props, ok := callSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("call_api properties type = %T", callSchema["properties"])
	}
	if _, ok := props["endpoint"]; ok {
		t.Error("MCP call_api must not bypass describe/resolve with a raw endpoint")
	}
	if _, ok := props["key"]; ok {
		t.Error("MCP call_api should use the session key instead of exposing a credential input")
	}
	if _, ok := props["profileFields"]; !ok {
		t.Error("MCP call_api should expose bounded response profiling for connection evidence")
	}
	required, ok := callSchema["required"].([]any)
	if !ok || !containsString(required, "pk") {
		t.Fatalf("call_api must require pk; required = %#v", callSchema["required"])
	}

	apply := byName["apply"]
	if apply == nil || apply.Annotations == nil || apply.Annotations.DestructiveHint == nil ||
		!*apply.Annotations.DestructiveHint || apply.Annotations.ReadOnlyHint {
		t.Fatal("apply must advertise its irreversible external side effect to MCP clients")
	}
	if !strings.Contains(apply.Title+apply.Annotations.Title+apply.Description, "2.5단계") ||
		!strings.Contains(apply.Description, "자동 활용신청") {
		t.Fatal("apply must present automatic access application as the bridge between describe and call")
	}
	applySchema, ok := apply.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("apply schema type = %T", apply.InputSchema)
	}
	applyProps, ok := applySchema["properties"].(map[string]any)
	if !ok || applyProps["category"] == nil {
		t.Fatal("apply must require an explicit purpose category instead of silently filing everything as research")
	}
	applyRequired, ok := applySchema["required"].([]any)
	if !ok || !containsString(applyRequired, "category") {
		t.Fatalf("apply category must be required; schema = %#v", applySchema)
	}
	if byName["get_api_key"] != nil {
		t.Fatal("MCP must not expose the account-wide serviceKey to model context")
	}
	record := byName["record_connection_assessment"]
	if record == nil || record.Annotations == nil || record.Annotations.ReadOnlyHint ||
		record.Annotations.DestructiveHint == nil || *record.Annotations.DestructiveHint || !record.Annotations.IdempotentHint {
		t.Fatal("connection assessment must advertise a local additive idempotent write")
	}
}

func TestConnectionAssessmentRoundTripDoesNotStoreRawValues(t *testing.T) {
	store := connectionledger.New(filepath.Join(t.TempDir(), "connections.jsonl"))
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), Ledger: store}))
	field := map[string]any{"selector": "lawdCd", "namespace": "법정동코드 10자리", "dataType": "string", "grain": "행정구역"}
	hash := strings.Repeat("a", 64)
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_connection_assessment",
		Arguments: map[string]any{
			"left":     map[string]any{"pk": "15000001", "delivery": "REST", "source": map[string]any{"url": "https://www.data.go.kr/data/15000001/openapi.do"}, "record": map[string]any{"kind": "api_profile", "evidenceHash": hash}, "fields": []any{field}},
			"right":    map[string]any{"pk": "15000002", "delivery": "FILE", "source": map[string]any{"url": "https://www.data.go.kr/data/15000002/fileData.do"}, "record": map[string]any{"kind": "file_observation", "evidenceHash": hash}, "fields": []any{field}},
			"relation": "SHARES_LEGAL_DISTRICT", "edgeKinds": []string{"spatial"}, "expectedKeys": []string{"lawdCd"},
			"matchMethod": "deterministic", "status": "sample_verified", "incrementalValue": "수요와 공급 비교", "reason": "표본 일치",
			"observedAt": "2026-09-04T00:00:00Z", "sample": map[string]any{"leftDistinct": 10, "rightDistinct": 8, "overlapDistinct": 7, "joinedRows": 12, "leftMaxRowsPerKey": 2, "rightMaxRowsPerKey": 1},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("record err=%v result=%+v", err, res)
	}
	listed, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_connection_assessments", Arguments: map[string]any{"pk": "15000002"}})
	if err != nil || listed.IsError {
		t.Fatalf("list err=%v result=%+v", err, listed)
	}
	raw, _ := json.Marshal(listed.StructuredContent)
	if !bytes.Contains(raw, []byte("sample_verified")) || bytes.Contains(raw, []byte("values")) {
		t.Fatalf("ledger response = %s", raw)
	}
}

func TestGuideResourceUsesOdeduckURI(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	seen := map[string]bool{}
	for _, resource := range res.Resources {
		seen[resource.URI] = true
	}
	const uri = "odeduck://guide"
	if !seen[uri] {
		t.Fatalf("missing guide resource %q; resources = %+v", uri, res.Resources)
	}
	read, err := sess.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("read %s: %v", uri, err)
	}
	if len(read.Contents) != 1 || !strings.Contains(read.Contents[0].Text, "odeduck") {
		t.Fatalf("guide %s content = %+v", uri, read.Contents)
	}
}

func TestCatalogSearchDefaultsToAllDiscoverableDatasets(t *testing.T) {
	if (catalogIn{}).restOnly() {
		t.Fatal("omitting restOnly should keep LINK datasets discoverable")
	}
	yes := true
	if !((catalogIn{RESTOnly: &yes}).restOnly()) {
		t.Fatal("restOnly=true should explicitly restrict search to REST datasets")
	}
}

func TestCatalogSearchRejectsContextFloodingLimit(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "catalog_search",
		Arguments: map[string]any{"query": "데이터", "limit": 100000},
	})
	if err != nil {
		t.Fatalf("catalog_search transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("oversized catalogue limit must be rejected before loading or returning the catalogue")
	}
}

func TestCatalogSearchSchemaSupportsSemanticQueryPlanning(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "catalog_search" {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("catalog_search schema type = %T", tool.InputSchema)
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("catalog_search properties type = %T", schema["properties"])
		}
		for _, name := range []string{"concepts", "axes", "anchorPks", "bridgeSelections", "maxConnections"} {
			if _, ok := props[name]; !ok {
				t.Fatalf("catalog_search schema missing %s", name)
			}
		}
		return
	}
	t.Fatal("catalog_search tool missing")
}

func TestCatalogSearchReturnsConnectionsOnlyAfterExplicitBridgeSelection(t *testing.T) {
	isolateConfigHome(t)
	cat := &catalog.Catalog{
		SyncedAt: time.Now(),
		Type:     "API",
		Source:   catalog.SourceOfficial,
		Entries: []catalog.Entry{
			{PK: "anchor", Title: "온비드 공매 물건", SvcType: catalog.SvcREST},
			{PK: "bridge", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
		},
	}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "catalog_search",
		Arguments: map[string]any{
			"query": "공매 투자 판단", "anchorPks": []string{"anchor"}, "semantic": false,
			"axes": []map[string]any{
				{"role": "anchor", "query": "온비드 공매 물건"},
				{
					"role": "상권 수요", "query": "상권 점포 개폐업",
					"contribution": "저가 매물과 쇠퇴 상권을 구분",
					"edge": map[string]any{
						"kinds":        []string{"spatial", "temporal"},
						"expectedKeys": []string{"법정동코드", "기준연월"},
					},
				},
			},
			"bridgeSelections": []map[string]any{{
				"pk": "bridge", "role": "상권 수요",
				"incrementalValue": "저가 매물과 쇠퇴 상권을 구분",
				"whyCandidate":     "점포 개폐업 title이 역할을 직접 뒷받침",
				"edge": map[string]any{
					"kinds":        []string{"spatial", "temporal"},
					"expectedKeys": []string{"법정동코드", "기준연월"},
				},
			}},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("catalog_search err=%v result=%+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var got catalogOut
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Connections) != 1 {
		t.Fatalf("connections = %+v", got.Connections)
	}
	if got.Source != catalog.SourceOfficial {
		t.Fatalf("catalog source = %q, want official", got.Source)
	}
	connection := got.Connections[0]
	if connection.Status != catalog.ConnectionStatusCandidate || connection.Anchor.PK != "anchor" || connection.Bridge.PK != "bridge" {
		t.Fatalf("connection = %+v", connection)
	}
	if strings.Contains(strings.ToLower(connection.Status), "verified") {
		t.Fatalf("metadata search overclaimed verification: %+v", connection)
	}
}

func TestCatalogSearchReturnsAConnectionOptionPoolBeforeSelection(t *testing.T) {
	isolateConfigHome(t)
	cat := &catalog.Catalog{
		SyncedAt: time.Now(), Type: "ALL",
		Entries: []catalog.Entry{
			{PK: "anchor", Title: "요양시설 매물", SvcType: catalog.SvcREST},
			{PK: "demand-api", Title: "시군구 장기요양 인정자", SvcType: catalog.SvcREST},
			{PK: "demand-file", Title: "시군구 고령인구 전망", SvcType: catalog.SvcFILE,
				DataTypes: []string{"FILE", "API"}, Formats: []string{"CSV", "JSON", "XML"}},
		},
	}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "catalog_search",
		Arguments: map[string]any{
			"query": "요양시설 인수", "anchorPks": []string{"anchor"}, "semantic": false,
			"axes": []map[string]any{
				{"role": "anchor", "query": "요양시설 매물"},
				{
					"role": "지역 수요", "query": "시군구 장기요양 고령인구",
					"contribution": "공급 대비 잠재 수요 비교",
					"edge":         map[string]any{"kinds": []string{"spatial"}, "expectedKeys": []string{"시군구코드"}},
				},
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("catalog_search err=%v result=%+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var got catalogOut
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Connections) != 0 || len(got.ConnectionOptions) != 1 || len(got.ConnectionOptions[0].Nodes) != 2 {
		t.Fatalf("options=%+v connections=%+v", got.ConnectionOptions, got.Connections)
	}
	var fileNode *catalog.Hit
	for i := range got.ConnectionOptions[0].Nodes {
		if got.ConnectionOptions[0].Nodes[i].PK == "demand-file" {
			fileNode = &got.ConnectionOptions[0].Nodes[i]
		}
	}
	if fileNode == nil || fileNode.NextAction != "inspect_dataset" || fileNode.DetailURL == "" ||
		strings.Join(fileNode.DataTypes, ",") != "FILE,API" || strings.Join(fileNode.Formats, ",") != "CSV,JSON,XML" {
		t.Fatalf("FILE handoff = %+v", fileNode)
	}
}

func TestDiscoveryAndProfileInputBoundsFailBeforeIO(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	manyAxes := make([]map[string]any, 9)
	for i := range manyAxes {
		manyAxes[i] = map[string]any{"role": "role", "query": "query"}
	}
	tests := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"too many axes", "catalog_search", map[string]any{"query": "x", "axes": manyAxes}, "axes는 최대 8개"},
		{"too many anchors", "catalog_search", map[string]any{"query": "x", "anchorPks": []string{"1", "2", "3", "4"}}, "anchorPks는 최대 3개"},
		{"too many selections", "catalog_search", map[string]any{"query": "x", "bridgeSelections": []map[string]any{
			{"pk": "1", "whyCandidate": "a"}, {"pk": "2", "whyCandidate": "b"},
			{"pk": "3", "whyCandidate": "c"}, {"pk": "4", "whyCandidate": "d"},
		}}, "bridgeSelections는 최대 3개"},
		{"too many cards", "catalog_search", map[string]any{"query": "x", "maxConnections": 4}, "maxConnections는 생략하거나 1~3"},
		{"too many profile fields", "call_api", map[string]any{"pk": "x", "profileFields": []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}}, "profile field는 최대 8개"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Fatalf("expected tool error: %+v", res)
			}
			raw, _ := json.Marshal(res.Content)
			if !strings.Contains(string(raw), tt.want) {
				t.Fatalf("error %s does not contain %q", raw, tt.want)
			}
		})
	}
}

func TestCatalogSearchToolKeepsLinkCandidatesDiscoverableByDefault(t *testing.T) {
	isolateConfigHome(t)
	cat := &catalog.Catalog{
		SyncedAt: time.Now(),
		Type:     "API",
		Entries: []catalog.Entry{
			{PK: "rest", Title: "기상 관측", SvcType: catalog.SvcREST, Desc: "기온 관측값"},
			{PK: "link", Title: "기상 연계", SvcType: catalog.SvcLINK, Desc: "기온 관측값"},
		},
	}
	if err := cat.Save(); err != nil {
		t.Fatalf("save catalog: %v", err)
	}

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "catalog_search",
		Arguments: map[string]any{"query": "기온"},
	})
	if err != nil {
		t.Fatalf("catalog_search: %v", err)
	}
	if res.IsError {
		t.Fatalf("catalog_search returned error: %+v", res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var got catalogOut
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.Total != 2 || len(got.Hits) != 2 {
		t.Fatalf("default catalog result = %+v, want REST and LINK candidates", got)
	}
	seen := map[string]bool{}
	for _, hit := range got.Hits {
		seen[hit.PK] = true
	}
	if !seen["rest"] || !seen["link"] {
		t.Fatalf("default catalog result = %+v, want REST and LINK candidates", got)
	}
}

func TestCatalogSearchToolExecutesAndDiversifiesSemanticConcepts(t *testing.T) {
	isolateConfigHome(t)
	cat := &catalog.Catalog{
		SyncedAt: time.Now(),
		Type:     "API",
		Entries: []catalog.Entry{
			{PK: "onbid", Title: "온비드 공매 물건", SvcType: catalog.SvcREST, ApplyCount: 1200},
			{PK: "bid", Title: "나라장터 입찰 공고", SvcType: catalog.SvcREST, ApplyCount: 9000},
			{PK: "auction", Title: "도매시장 실시간 경매", SvcType: catalog.SvcREST, ApplyCount: 800},
			{PK: "noise", Title: "갯벌 체험 지수", SvcType: catalog.SvcREST, ApplyCount: 50000},
		},
	}
	if err := cat.Save(); err != nil {
		t.Fatalf("save catalog: %v", err)
	}

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "catalog_search",
		Arguments: map[string]any{
			"query":    "돈 벌 수 있는 데이터를 찾아줘",
			"concepts": []string{"온비드 공매", "나라장터 입찰", "도매시장 경매"},
			"limit":    3,
		},
	})
	if err != nil {
		t.Fatalf("catalog_search: %v", err)
	}
	if res.IsError {
		t.Fatalf("catalog_search returned error: %+v", res.Content)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var got catalogOut
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.Mode != catalog.SearchModePlanned || len(got.Hits) != 3 {
		t.Fatalf("semantic catalogue result = %+v", got)
	}
	want := []string{"onbid", "bid", "auction"}
	for i, pk := range want {
		if got.Hits[i].PK != pk || got.Hits[i].MatchedQuery == "" {
			t.Errorf("hit[%d] = %+v, want pk=%s with matchedQuery", i, got.Hits[i], pk)
		}
	}
}

func TestServerAdvertisesTriggerAndNoSilentSemanticFallbackInstructions(t *testing.T) {
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	got := sess.InitializeResult().Instructions
	for _, want := range []string{"catalog_search", "requireSemantic", "semantic=false"} {
		if !strings.Contains(got, want) {
			t.Fatalf("server instructions = %q, want %q", got, want)
		}
	}
}

func TestCatalogSearchCanRequireSemanticInsteadOfFallingBack(t *testing.T) {
	isolateConfigHome(t)
	cat := &catalog.Catalog{SyncedAt: time.Now(), Type: "FILE", Entries: []catalog.Entry{{
		PK: "jeju", Title: "제주 실종 데이터", SvcType: catalog.SvcFILE,
	}}}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}

	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0))}))
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "catalog_search",
		Arguments: map[string]any{
			"query": "제주 실종", "requireSemantic": true,
		},
	})
	if err != nil {
		t.Fatalf("catalog_search transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("catalog_search silently fell back: %+v", res.StructuredContent)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var strictOut catalogOut
	if err := json.Unmarshal(raw, &strictOut); err != nil {
		t.Fatal(err)
	}
	if len(strictOut.Hits) != 0 || strictOut.Semantic != nil {
		t.Fatalf("strict semantic failure must not expose fallback hits: %+v", res.StructuredContent)
	}
	var message string
	if len(res.Content) > 0 {
		if content, ok := res.Content[0].(*mcp.TextContent); ok {
			message = content.Text
		}
	}
	if !strings.Contains(message, "semantic-build") {
		t.Fatalf("error content = %s, want recovery action", message)
	}
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
