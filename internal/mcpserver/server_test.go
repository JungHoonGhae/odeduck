package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/gongctl/internal/catalog"
	"github.com/JungHoonGhae/gongctl/internal/fetch"
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
		{name: "describe_api", stage: "2단계"},
		{name: "call_api", stage: "3단계"},
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
}

func TestCatalogSearchDefaultsToCallableDatasets(t *testing.T) {
	if !((catalogIn{}).restOnly()) {
		t.Fatal("omitting restOnly should search callable REST datasets")
	}
	no := false
	if (catalogIn{RESTOnly: &no}).restOnly() {
		t.Fatal("restOnly=false should include non-callable catalogue entries")
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
		if _, ok := props["concepts"]; !ok {
			t.Fatal("catalog_search must let the host model provide semantic concept queries")
		}
		return
	}
	t.Fatal("catalog_search tool missing")
}

func TestCatalogSearchToolReturnsCompactCallableCandidatesByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	if got.Total != 1 || len(got.Hits) != 1 || got.Hits[0].PK != "rest" {
		t.Fatalf("default catalog result = %+v, want only callable REST candidate", got)
	}
}

func TestCatalogSearchToolExecutesAndDiversifiesSemanticConcepts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

func containsString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
