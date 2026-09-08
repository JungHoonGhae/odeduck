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
	Goal            string             `json:"goal,omitempty" jsonschema:"start a goal with the user's natural-language intended output; no keywords or PKs required. Omit for subsequent actions"`
	SessionID       string             `json:"sessionId,omitempty" jsonschema:"opaque ID from this MCP session's prior advance_goal response"`
	Revision        int                `json:"revision,omitempty" jsonschema:"exact latest state.revision; stale/replayed actions fail"`
	RequireSemantic *bool              `json:"requireSemantic,omitempty" jsonschema:"start only; default true. False explicitly allows degraded lexical retrieval, never use it to silently retry a semantic error"`
	Decision        *goalwork.Decision `json:"decision,omitempty" jsonschema:"one engine action: define, search, inspect, layout, sample, retry_sample, read_evidence, compose, execute, or abstain. Read odeduck://guide for the shared action contract and limits before planning. Start with define; reference only actual inspected sources and retained observation IDs. Never submit rows, computed evidence or approval flags"`
}
type goalOut struct {
	SessionID string        `json:"sessionId"`
	State     goalwork.View `json:"state"`
}
type goalSession struct {
	owner  *mcp.ServerSession
	engine *goalwork.Engine
}

func registerGoalTool(server *mcp.Server, shareEvidence bool, deps func(goalwork.Policy) goalwork.Dependencies) {
	var mu sync.Mutex
	sessions := map[string]goalSession{}
	tool := &mcp.Tool{
		Name:        "advance_goal",
		Description: "[목표 기반 실행] 자연어 goal로 시작하고 최신 revision과 decision으로 검색·검사·표본·조회·분석·필요한 결합을 진행한다. 상세 행동은 odeduck://guide의 공통 목표 계약을 먼저 읽는다. state.gaps와 evaluation.checks로 미충족 조건을 확인한다. sample_executed는 표본 실행이지 목표 완료·인과·sample_verified가 아니다. 자동 의미 승인·자동 신청은 없다. 세션은 소유 MCP 연결에 묶이고 1시간 뒤 만료한다. read_evidence는 서버 시작 시 --share-goal-evidence를 허용한 경우에만 가능하며 모델은 권한을 켤 수 없다. 이 설정은 기존 사용자 Artifact/call_api 반환을 차단하지 않는다.",
		Annotations: &mcp.ToolAnnotations{Title: "목표 → 탐색·검증·조합", ReadOnlyHint: false, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	}
	handle := func(ctx context.Context, req *mcp.CallToolRequest, in goalIn) (*mcp.CallToolResult, *goalOut, error) {
		if in.SessionID == "" {
			if in.Decision != nil || in.Revision != 0 {
				return errResult("start with goal only, then advance using returned revision"), nil, nil
			}
			policy := goalwork.Policy{RequireSemantic: in.RequireSemantic == nil || *in.RequireSemantic}
			if shareEvidence {
				policy.EvidenceRecipient = "mcp_host"
			}
			engine, err := goalwork.Start(in.Goal, policy, deps(policy))
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			mu.Lock()
			if len(sessions) >= 8 {
				mu.Unlock()
				return errResult("goal session capacity reached (8); retained observations expire after one hour"), nil, nil
			}
			id := rand.Text()
			sessions[id] = goalSession{owner: req.Session, engine: engine}
			mu.Unlock()
			view := engine.View()
			time.AfterFunc(time.Until(view.ExpiresAt), func() { mu.Lock(); delete(sessions, id); mu.Unlock() })
			return nil, &goalOut{SessionID: id, State: view}, nil
		}
		if in.Goal != "" || in.RequireSemantic != nil {
			return errResult("goal and search policy are immutable within a session"), nil, nil
		}
		mu.Lock()
		entry, ok := sessions[in.SessionID]
		mu.Unlock()
		if !ok || entry.owner != req.Session {
			return errResult("unknown or expired goal session"), nil, nil
		}
		if in.Decision == nil {
			return nil, &goalOut{SessionID: in.SessionID, State: entry.engine.View()}, nil
		}
		view, err := entry.engine.Advance(ctx, in.Revision, *in.Decision)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &goalOut{SessionID: in.SessionID, State: view}, nil
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
		body, err := json.Marshal(out)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{StructuredContent: json.RawMessage(body), Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
	})
}
