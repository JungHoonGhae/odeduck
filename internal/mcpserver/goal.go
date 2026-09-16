package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"sync"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type goalIn struct {
	FullState bool   `json:"fullState,omitempty" jsonschema:"request a full state snapshot after lost context or for older clients; otherwise subsequent actions return JSON Patch changes from baseRevision, plus current revision/status/budget"`
	Context   string `json:"context,omitempty" jsonschema:"start only: relevant user conversation needed to interpret the goal, such as prior examples to exclude. At most 8000 UTF-8 bytes, no credentials. Not source evidence or approval. Passed unchanged to planning and authorized independent review"`

	Goal            string             `json:"goal,omitempty" jsonschema:"start a goal with the user's natural-language intended output; no keywords or PKs required. Omit for subsequent actions"`
	SessionID       string             `json:"sessionId,omitempty" jsonschema:"opaque ID from this MCP session's prior goal response"`
	Revision        int                `json:"revision,omitempty" jsonschema:"exact latest state.revision; stale/replayed actions fail"`
	RequireSemantic *bool              `json:"requireSemantic,omitempty" jsonschema:"start only; default true. False explicitly allows degraded lexical retrieval, never use it to silently retry a semantic error"`
	Decision        *goalwork.Decision `json:"decision,omitempty" jsonschema:"one engine action: read_guide (topic from brief), define, search, retry_search, inspect, layout, sample, retry_sample, read_evidence, compose, execute, review_result, or abstain. Read odeduck://guide for the shared action contract and limits before planning. Start with define; reference only actual inspected sources and retained observation IDs. Never submit rows, computed evidence or approval flags"`
}
type goalOut struct {
	SessionID string        `json:"sessionId"`
	State     goalwork.View `json:"state"`
	Guide     string        `json:"guide,omitempty"`
	previous  *goalwork.View
}
type goalSession struct {
	owner  *mcp.ServerSession
	engine *goalwork.Engine
}

func registerGoalTool(server *mcp.Server, startupPolicy goalwork.Policy, deps func(goalwork.Policy) goalwork.Dependencies) {
	var mu sync.Mutex
	sessions := map[string]goalSession{}
	tool := &mcp.Tool{
		Name:        "goal",
		Description: "[목표 기반 실행] 자연어 goal로 시작하고 최신 revision과 decision으로 검색·검사·표본·조회·분석·필요한 결합을 진행한다. 상세 행동은 odeduck://guide의 공통 목표 계약을 먼저 읽는다. state.gaps와 evaluation.checks로 미충족 조건을 확인한다. sample_executed는 표본 실행이지 목표 완료·인과·sample_verified가 아니다. 보고·분석 검토는 서버 --review-with codex|claude|gemini 설정으로 허용한다. 별도 검토는 선택 사항이다. 기본 MCP는 host가 계산된 artifact를 해석하며 추가 agent CLI 설치가 필요 없다. 미검증 의미와 누락은 그대로 밝히고 독립 검증 완료를 주장하지 않는다. 응답은 최초 snapshot, 이후 baseRevision 기준 JSON Patch changes다. 맥락을 잃으면 fullState:true로 복구한다. read_guide로 필요한 상세 topic만 읽는다. 접근권한이 부족하면 list_applications로 확인하고 사용자에게 로그인을 요청하거나 허용된 apply 후 같은 목표의 retry_sample로 이어간다. 세션은 소유 MCP 연결에 묶이고 1시간 뒤 만료한다. read_evidence는 서버 시작 시 선택 근거 공유를 허용한 경우에만 가능하며 모델은 권한을 켤 수 없다. 이 설정은 기존 사용자 Artifact/call_api 반환을 차단하지 않는다.",
		Annotations: &mcp.ToolAnnotations{Title: "목표 → 탐색·검증·조합", ReadOnlyHint: false, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	}
	handle := func(ctx context.Context, req *mcp.CallToolRequest, in goalIn) (*mcp.CallToolResult, *goalOut, error) {
		if in.SessionID == "" {
			if in.Decision != nil || in.Revision != 0 {
				return errResult("start with goal only, then advance using returned revision"), nil, nil
			}
			policy := startupPolicy
			policy.RequireSemantic = in.RequireSemantic == nil || *in.RequireSemantic
			engine, err := goalwork.StartRequest(goalwork.Request{Goal: in.Goal, Context: in.Context}, policy, deps(policy))
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			view, checkErr := engine.Preflight(ctx)
			if checkErr != nil {
				return &mcp.CallToolResult{IsError: true}, &goalOut{State: view}, nil
			}
			mu.Lock()
			if len(sessions) >= 8 {
				mu.Unlock()
				return errResult("goal session capacity reached (8); retained observations expire after one hour"), nil, nil
			}
			id := rand.Text()
			sessions[id] = goalSession{owner: req.Session, engine: engine}
			mu.Unlock()
			time.AfterFunc(time.Until(view.ExpiresAt), func() { mu.Lock(); delete(sessions, id); mu.Unlock() })
			return nil, &goalOut{SessionID: id, State: view}, nil
		}
		if in.Goal != "" || in.Context != "" || in.RequireSemantic != nil {
			return errResult("goal, context and search policy are immutable within a session"), nil, nil
		}
		mu.Lock()
		entry, ok := sessions[in.SessionID]
		mu.Unlock()
		if !ok || entry.owner != req.Session {
			return errResult("unknown or expired goal session"), nil, nil
		}
		before := entry.engine.View()
		if in.Decision == nil {
			return nil, &goalOut{SessionID: in.SessionID, State: before, previous: &before}, nil
		}
		view, err := entry.engine.Advance(ctx, in.Revision, *in.Decision)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		out := &goalOut{SessionID: in.SessionID, State: view, previous: &before}
		if in.Decision.Action == "read_guide" && view.GuideTopic == in.Decision.Topic {
			out.Guide, _ = goalwork.PlanningSection(view.GuideTopic)
		}
		return nil, out, nil
	}
	// SDK v1.6.1's inferred-output-schema validation unmarshals numbers into
	// float64, then re-marshals them. Keep typed input validation, but serialize
	// this engine-owned output ourselves. Out=any + nil output avoids that lossy
	// pass. TextContent also lets clients use a precision-preserving JSON decoder
	// when their SDK eagerly decodes StructuredContent into floating-point values.
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, in goalIn) (*mcp.CallToolResult, any, error) {
		res, out, err := handle(ctx, req, in)
		if err != nil || out == nil {
			return res, nil, err
		}
		body, err := goalResponse(out, in.FullState)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{IsError: res != nil && res.IsError, StructuredContent: json.RawMessage(body), Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
	})
}
