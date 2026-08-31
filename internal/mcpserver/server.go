// Package mcpserver exposes gongctl over the Model Context Protocol (stdio):
// dataset search, 활용신청, spec surfacing, and authenticated calls as tools.
// It only assembles — the deterministic work lives in portal/apicall.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JungHoonGhae/gongctl/internal/apicall"
	"github.com/JungHoonGhae/gongctl/internal/catalog"
	"github.com/JungHoonGhae/gongctl/internal/fetch"
	"github.com/JungHoonGhae/gongctl/internal/portal"
	"github.com/JungHoonGhae/gongctl/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps carries the collaborators the server needs.
type Deps struct {
	Fetch         *fetch.Client
	BaseURL       string // data.go.kr root for search/describe (override in tests)
	SemanticIndex *catalog.SemanticIndex
	Embedder      catalog.Embedder
}

type searchIn struct {
	Keyword string `json:"keyword" jsonschema:"free-text query for data.go.kr datasets"`
}
type searchOut struct {
	Datasets []portal.Dataset `json:"datasets"`
}
type applyIn struct {
	PK       string `json:"pk" jsonschema:"publicDataPk of the dataset to apply for"`
	Purpose  string `json:"purpose" jsonschema:"활용목적 — why you need this data (required)"`
	Category string `json:"category" jsonschema:"portal purpose category inferred from the intended use: web | app | research | ref | etc"`
}
type describeIn struct {
	PK string `json:"pk" jsonschema:"publicDataPk of an OpenAPI dataset"`
}
type callIn struct {
	PK     string            `json:"pk" jsonschema:"publicDataPk returned by catalog_search and inspected with describe_api; raw endpoint URLs are intentionally not accepted by MCP"`
	Op     string            `json:"op,omitempty" jsonschema:"which operation, named by the last path segment of its endpoint (e.g. getHeatWaveCasualtiesRegionList). Omit when the dataset has only one"`
	Params map[string]string `json:"params,omitempty" jsonschema:"request variables as key/value"`
	// Bounded well below apicall.MaxPropagationWait: a tool call that blocks for an
	// hour looks like a hung agent, and the caller can simply ask again.
	WaitSeconds int `json:"waitSeconds,omitempty" jsonschema:"wait up to this many seconds for a just-approved API to propagate, retrying the 403 (max 300). Use it right after apply; omit it otherwise"`
}
type catalogIn struct {
	Query string `json:"query" jsonschema:"the user's original natural-language need. For a concrete lookup this is also searched directly; for a broad or implicit goal, preserve it here and provide model-inferred concepts below"`
	// Semantic interpretation belongs to the MCP host model that already
	// understands the conversation. gongctl then executes the plan against all
	// 11k+ rows locally; this avoids both a brittle synonym dictionary and a
	// second embedding/LLM credential inside the CLI.
	Concepts []string `json:"concepts,omitempty" jsonschema:"for broad, exploratory or implicit intent: 2-8 concrete Korean catalogue queries inferred from the user's goal. Cover distinct direct, adjacent, leading-indicator or constraint axes rather than mere synonyms; omit only for a concrete dataset lookup"`
	Limit    int      `json:"limit,omitempty" jsonschema:"max rows to return (default 20) — keep it small, the total match count comes back separately"`
	Ranking  string   `json:"ranking,omitempty" jsonschema:"planned-search ranking: balanced (default, interleaves proven demand and recently modified data), demand, or recent"`
	// Pointer distinguishes omission from an explicit false. Exploratory planned
	// search defaults previews on; a concrete lookup stays compact by default.
	IncludePreviews *bool `json:"includePreviews,omitempty" jsonschema:"include a short official-description preview. Defaults true when concepts are provided and false for a concrete lexical lookup"`
	Semantic        *bool `json:"semantic,omitempty" jsonschema:"default true: use the optional local Ollama vector index when it is built; false forces deterministic lexical/planned retrieval"`
	// Pointer distinguishes omission (the safe, callable default) from an explicit
	// false requested by a caller doing broad discovery rather than describe→call.
	RESTOnly *bool `json:"restOnly,omitempty" jsonschema:"default true: only datasets whose spec is published on the portal (REST). Set false only for broad discovery that does not need describe_api/call_api"`
}

func (in catalogIn) restOnly() bool {
	return in.RESTOnly == nil || *in.RESTOnly
}

func (in catalogIn) includePreviews() bool {
	if in.IncludePreviews != nil {
		return *in.IncludePreviews
	}
	return len(in.Concepts) > 0
}

func (in catalogIn) semanticEnabled() bool { return in.Semantic == nil || *in.Semantic }

type catalogOut struct {
	Mode     string                `json:"mode"`              // lexical | planned
	Intent   string                `json:"intent,omitempty"`  // the user's original goal
	Queries  []string              `json:"queries,omitempty"` // model-inferred concrete data axes
	Terms    []string              `json:"terms,omitempty"`   // what a single query was reduced to
	Relaxed  bool                  `json:"relaxed,omitempty"` // true = no entry had every term, so any-term matches are shown
	Total    int                   `json:"total"`             // matches found
	Shown    int                   `json:"shown"`             // rows returned
	SyncedAt string                `json:"syncedAt"`          // when the catalogue was built
	Stale    bool                  `json:"stale"`             // true = re-sync, results may be incomplete
	Hits     []catalog.Hit         `json:"hits"`
	Semantic *catalog.SemanticInfo `json:"semantic,omitempty"`
}
type appsOut struct {
	Applications []portal.Application `json:"applications"`
}

// maxToolWait caps how long call_api will block. Propagation can take longer than
// this; the agent is told to call again rather than have a tool hold the session.
const maxToolWait = 5 * time.Minute

// emptyIn is the input type for tools that take no arguments. mcp.AddTool
// infers a JSON schema from the struct even with zero fields, so this is
// just a named `struct{}` for readability at call sites.
type emptyIn struct{}

func boolPtr(v bool) *bool { return &v }

func readOnlyAnnotations(title string, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:         title,
		ReadOnlyHint:  true,
		OpenWorldHint: boolPtr(openWorld),
	}
}

// New builds the MCP server. Three data tools deliberately form the compact
// progressive-discovery surface: search a small catalogue result, inspect one
// specification, then call it. apply is the explicit 2.5-stage access action
// between inspection and call when approval is missing. MCP clients may sort tool
// names, so titles and descriptions carry the stage while unrelated account and
// live-search tools say "support".
func New(deps Deps) *mcp.Server {
	base := deps.BaseURL
	if base == "" {
		base = portal.BaseURL
	}
	// One shared transport → one throttle across search/describe/call.
	pc := portal.New(deps.Fetch, portal.WithBaseURL(base))
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "gongctl",
		Title:   "gongctl — 공공데이터포털(data.go.kr) 자동화",
		Version: version.Version,
	}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_search",
		Annotations: readOnlyAnnotations("1단계 · 공공데이터 카탈로그 검색", false),
		Description: "[1단계: 검색] 자연어 요청으로 로컬 카탈로그에서 호출 가능한 API 후보를 찾는다. 포털 검색과 달리 전체 목록을 한 번에 " +
			"훑으므로 '이런 데이터가 있나?'를 키워드를 추측해가며 여러 번 물을 필요가 없다. " +
			"구체적인 데이터명을 찾을 때는 query 만 쓴다. 하지만 '돈 될 만한 것', '새 서비스를 기획하고 싶다', " +
			"'대한민국이 어떻게 변하고 있나'처럼 의미 해석이 필요한 목표는 원문을 query 에 보존하고, **호출하기 전에 " +
			"스스로 2~8개의 구체적인 데이터 축을 추론해 concepts 에 넣어라**. concepts 는 동의어 나열이 아니라 직접 대상, " +
			"인접 시장, 선행지표, 제약·위험, 다른 기관 관점을 포함해야 한다. gongctl 은 각 축을 전체 카탈로그에서 독립 검색해 " +
			"중복을 제거하고 골고루 섞는다. 이것이 언어모델의 의미 이해와 결정적 로컬 검색을 결합하는 경계다. " +
			"planned 결과는 matchedQuery 로 왜 발견됐는지 설명하며, 짧은 공식 preview 를 기본 포함한다. ranking=balanced 는 " +
			"활용 수요가 검증된 데이터와 최근 수정된 저활용 데이터를 함께 보여준다. 데이터 탐색은 반드시 이 도구로 시작하고, " +
			"고른 pk 하나를 describe_api 로 넘겨라. " +
			"svcType 이 LINK 면 포털에 명세가 없어 describe_api/call_api 로 갈 수 없다(전체의 약 40%가 LINK다) — " +
			"그래서 restOnly 는 생략해도 기본 true 다. 호출 목적이 아닌 전체 탐색일 때만 false 로 둬라. " +
			"svcType 이 비어 있으면 유형이 확인되지 않은 것이다. " +
			"relaxed=true 면 모든 단어를 포함하는 데이터가 없어 일부만 일치하는 것까지 보여준 것이므로 " +
			"matched 가 낮은 결과는 무관할 수 있다. terms 로 실제 검색된 단어를 확인하라. " +
			"stale=true 면 스냅샷이 오래되어 최근 신설 API 가 누락될 수 있다. " +
			"카탈로그가 없으면 사람에게 `gongctl catalog sync` 를 안내하라.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in catalogIn) (*mcp.CallToolResult, *catalogOut, error) {
		if in.Limit < 0 || in.Limit > catalog.MaxSearchLimit {
			return errResult(fmt.Sprintf("limit은 생략하거나 1~%d 사이여야 합니다", catalog.MaxSearchLimit)), nil, nil
		}
		cat, err := catalog.Load()
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		plan := catalog.QueryPlan{
			Intent: in.Query, Concepts: in.Concepts, Limit: in.Limit,
			RESTOnly: in.restOnly(), IncludePreviews: in.includePreviews(), Ranking: in.Ranking,
		}
		var res catalog.Result
		if !in.semanticEnabled() {
			res = cat.SearchPlan(plan)
		} else {
			index, embedder := deps.SemanticIndex, deps.Embedder
			if index == nil {
				loaded, loadErr := catalog.LoadSemanticIndex(cat)
				switch {
				case loadErr == nil:
					index = loaded
				case errors.Is(loadErr, catalog.ErrSemanticIndexStale):
					res = cat.SearchPlan(plan)
					res.Semantic = &catalog.SemanticInfo{Status: catalog.SemanticUnavailable, Detail: "카탈로그 갱신 후 semantic-build 가 필요함"}
				case errors.Is(loadErr, catalog.ErrSemanticIndexNotBuilt):
					res = cat.SearchHybrid(ctx, plan, nil, nil)
				default:
					res = cat.SearchPlan(plan)
					res.Semantic = &catalog.SemanticInfo{Status: catalog.SemanticUnavailable, Detail: "의미 인덱스 로드 실패: " + loadErr.Error()}
				}
			}
			if index != nil {
				if embedder == nil {
					embedder = catalog.NewOllamaEmbedder(os.Getenv("GONGCTL_OLLAMA_URL"), index.Model)
				}
				res = cat.SearchHybrid(ctx, plan, index, embedder)
			}
		}
		hits := res.Hits
		return nil, &catalogOut{
			Mode: res.Mode, Intent: res.Intent, Queries: res.Queries,
			Terms: res.Terms, Relaxed: res.Relaxed,
			Total: res.Total, Shown: len(hits),
			SyncedAt: cat.SyncedAt.Format("2006-01-02"), Stale: cat.Stale(),
			Hits: hits, Semantic: res.Semantic,
		}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "describe_api",
		Annotations: readOnlyAnnotations("2단계 · OpenAPI 상세 및 파라미터 확인", true),
		Description: "[2단계: 상세] catalog_search 가 반환한 pk 하나의 OpenAPI 상세기능·엔드포인트·요청변수를 확인한다. call_api 전에 반드시 호출하고 params 를 여기 나온 명세로 구성하라. params 가 비고 rawHtml 만 있으면 표 구조가 불확실하다는 뜻 — rawHtml 을 읽어라. apiType 이 LINK 면 linkUrl 을 직접 열어 읽어라(제공기관 사이트에 명세가 있고, 그곳은 별도 인증키를 요구한다). operations 가 비고 note 가 있으면 명세가 참고문서에만 있는 API이므로 guideDocUrl 을 내려받아 읽어라 (파라미터 추측 금지).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in describeIn) (*mcp.CallToolResult, *apicall.APISpec, error) {
		spec, err := apicall.Describe(ctx, deps.Fetch, base, in.PK)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, spec, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "call_api",
		Annotations: readOnlyAnnotations("3단계 · 승인된 OpenAPI 호출", true),
		Description: "[3단계: 호출] describe_api 로 확인한 승인 API 를 pk·op·params 로 호출한다. " +
			"MCP에서는 endpoint URL과 인증키를 받지 않는다. gongctl 이 pk로 포털 명세에서 엔드포인트를 다시 조회하고, " +
			"로그인 세션의 키를 안전하게 주입하며, 명세의 필수 요청변수가 빠졌는지 호출 전에 " +
			"확인한다(빠지면 data.go.kr 은 에러 대신 빈 결과를 주므로 스스로 알아채기 어렵다). " +
			"상세기능이 여럿이면 op 로 지정하라(엔드포인트 마지막 경로 조각). " +
			"**방금 apply 한 API 라면 waitSeconds=300 을 줘라** — 승인은 즉시지만 게이트웨이 반영에 " +
			"보통 7~10분 걸려 403 이 오고, gongctl 이 그 동안 1분 간격으로 재시도한다. " +
			"그래도 403 이면 실패가 아니라 아직 반영 전이니 잠시 후 다시 호출하라(키를 바꾸거나 " +
			"다시 신청하지 마라). 응답 XML 은 JSON 으로 변환한다. body 의 resultCode 로 성공(00) 여부를 확인하라.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in callIn) (*mcp.CallToolResult, *apicall.CallResult, error) {
		if strings.TrimSpace(in.PK) == "" {
			return errResult("pk 가 필요합니다 — catalog_search → describe_api 순서로 먼저 확인하세요"), nil, nil
		}
		resolved, rerr := apicall.Resolve(ctx, deps.Fetch, base, in.PK, in.Op)
		if rerr != nil {
			return errResult(rerr.Error()), nil, nil
		}
		endpoint := resolved.Endpoint
		if missing := apicall.MissingRequired(resolved, in.Params); len(missing) > 0 {
			return errResult(fmt.Sprintf("필수 요청변수가 빠졌습니다: %s — describe_api(pk=%s) 로 확인하세요",
				strings.Join(missing, ", "), in.PK)), nil, nil
		}
		key, kerr := portal.APIKey(ctx)
		if kerr != nil {
			return errResult("인증키를 얻지 못했습니다: " + kerr.Error()), nil, nil
		}
		doCall := func(k string) (*apicall.CallResult, error) {
			if in.WaitSeconds <= 0 {
				return apicall.Call(ctx, deps.Fetch, endpoint, in.Params, k)
			}
			w := time.Duration(in.WaitSeconds) * time.Second
			if w > maxToolWait {
				w = maxToolWait
			}
			return apicall.CallWaiting(ctx, deps.Fetch, endpoint, in.Params, k, w, nil)
		}
		res, err := doCall(key)
		// A rejected key may just be a stale cached copy (the user reissued it).
		// Drop it and read the key again — once, so a genuinely bad key still fails.
		if errors.Is(err, apicall.ErrKeyRejected) {
			portal.InvalidateCachedKey()
			if fresh, kerr := portal.APIKey(ctx); kerr == nil && fresh != key {
				res, err = doCall(fresh)
			}
		}
		if err != nil {
			// surface the hint but still return the body
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, res, nil
		}
		return nil, res, nil
	})

	// Fallback discovery reaches the live portal and only sees its current result
	// page. Keep it after the three primary tools so models do not mistake it for
	// the exhaustive catalogue entry point.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_datasets",
		Annotations: readOnlyAnnotations("보조 · 포털 라이브 검색", true),
		Description: "보조용 data.go.kr 라이브 키워드 검색이다. 기본 탐색에는 쓰지 말고 catalog_search 결과가 stale 이거나 최신 신설 항목을 재확인할 때만 사용하라. 한 페이지 결과만 반환하므로 전체 카탈로그 검색을 대신하지 못한다.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, *searchOut, error) {
		ds, err := pc.SearchDatasets(ctx, portal.SearchOptions{Keyword: in.Keyword})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &searchOut{Datasets: ds}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_applications",
		Annotations: readOnlyAnnotations("보조 · 활용신청 현황", true),
		Description: "보조 계정 도구. call_api 가 미승인 오류를 반환했을 때 내 활용신청 상태·인증키 만료일을 확인한다. 로그인 세션 필요.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, *appsOut, error) {
		apps, err := portal.Applications(ctx)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &appsOut{Applications: apps}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "apply",
		Annotations: &mcp.ToolAnnotations{
			Title:           "2.5단계 · AI OpenAPI 활용신청 자동 제출",
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(true),
		},
		Description: "[2.5단계: 자동 활용신청] describe_api 로 명세·개발단계 심의유형을 확인했지만 아직 승인되지 않은 OpenAPI라면 AI가 활용신청을 실제 제출한다. " +
			"purpose 에 사용자의 목표를 구체적으로 요약하고 category 는 실제 용도에 맞춰 web | app | research | ref | etc 중 하나로 분류하라. 개발단계 자동승인 API는 승인 확인 뒤 call_api(pk, op, params, waitSeconds=300)로 즉시 이어가고, " +
			"심의승인은 제공기관 승인을 기다린다. 이미 신청한 API는 다시 신청하지 말고 list_applications 로 상태를 확인하라. " +
			"계정에 실제 신청 기록을 남기는 외부 변경이며 MCP 클라이언트의 도구 승인 정책을 따른다. 로그인 세션 필요.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in applyIn) (*mcp.CallToolResult, *portal.ApplyResult, error) {
		category, categoryErr := portal.NormalizePurposeCategory(in.Category)
		if categoryErr != nil {
			return errResult(categoryErr.Error()), nil, nil
		}
		res, err := portal.Apply(ctx, in.PK, in.Purpose, category, nil) // MCP host approval guards the side effect.
		if err != nil {
			return errResult(err.Error()), res, nil
		}
		if !res.Submitted {
			return errResult(res.Message), res, nil
		}
		return nil, res, nil
	})

	s.AddResource(&mcp.Resource{
		Name:        "guide",
		URI:         "gongctl://guide",
		MIMEType:    "text/markdown",
		Description: "gongctl 도구 사용 순서와 인증키 Encoding/Decoding 주의. 먼저 읽으세요.",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      "gongctl://guide",
			MIMEType: "text/markdown",
			Text:     GuideDoc,
		}}}, nil
	})

	return s
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// Serve runs the server over stdio until the client disconnects or ctx cancels.
func Serve(ctx context.Context, deps Deps) error {
	return New(deps).Run(ctx, &mcp.StdioTransport{})
}
