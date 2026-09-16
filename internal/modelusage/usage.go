// Package modelusage records only call metadata and provider-reported counters.
// It never stores prompts, raw responses, keys, or estimates missing usage.
package modelusage

import (
	"context"
	"sync"
)

type Call struct {
	Provider          string `json:"provider"`
	Stage             string `json:"stage"`
	PromptBytes       int    `json:"promptBytes"`
	ResponseBytes     int    `json:"responseBytes"`
	DurationMS        int64  `json:"durationMs"`
	Failed            bool   `json:"failed,omitempty"`
	InputTokens       *int64 `json:"inputTokens,omitempty"`
	CachedInputTokens *int64 `json:"cachedInputTokens,omitempty"`
	CacheWriteTokens  *int64 `json:"cacheWriteTokens,omitempty"`
	OutputTokens      *int64 `json:"outputTokens,omitempty"`
	ReasoningTokens   *int64 `json:"reasoningTokens,omitempty"`
}

type Summary struct {
	CachedReportedCalls     int    `json:"cachedReportedCalls"`
	CacheWriteReportedCalls int    `json:"cacheWriteReportedCalls"`
	Scope                   string `json:"scope"`
	Calls                   int    `json:"calls"`
	ReportedCalls           int    `json:"reportedCalls"`
	UnreportedCalls         int    `json:"unreportedCalls"`
	PromptBytes             int64  `json:"promptBytes"`
	ResponseBytes           int64  `json:"responseBytes"`
	InputTokens             int64  `json:"inputTokens"`
	CachedInputTokens       int64  `json:"cachedInputTokens"`
	CacheWriteTokens        int64  `json:"cacheWriteTokens"`
	OutputTokens            int64  `json:"outputTokens"`
	Records                 []Call `json:"records,omitempty"`
}

type Tracker struct {
	mu    sync.Mutex
	calls []Call
}

func (t *Tracker) Record(c Call) { t.mu.Lock(); defer t.mu.Unlock(); t.calls = append(t.calls, c) }
func (t *Tracker) Snapshot() Summary {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := Summary{Scope: "odeduck child model calls only; host reasoning and embedding usage are not included", Calls: len(t.calls), Records: append([]Call(nil), t.calls...)}
	for _, c := range t.calls {
		s.PromptBytes += int64(c.PromptBytes)
		s.ResponseBytes += int64(c.ResponseBytes)
		if c.InputTokens != nil && c.OutputTokens != nil {
			s.ReportedCalls++
			s.InputTokens += *c.InputTokens
			s.OutputTokens += *c.OutputTokens
		} else {
			s.UnreportedCalls++
		}
		if c.CachedInputTokens != nil {
			s.CachedReportedCalls++
			s.CachedInputTokens += *c.CachedInputTokens
		}
		if c.CacheWriteTokens != nil {
			s.CacheWriteReportedCalls++
			s.CacheWriteTokens += *c.CacheWriteTokens
		}
	}
	return s
}

type recorderKey struct{}
type stageKey struct{}

func WithRecorder(ctx context.Context, f func(Call)) context.Context {
	return context.WithValue(ctx, recorderKey{}, f)
}
func WithStage(ctx context.Context, stage string) context.Context {
	return context.WithValue(ctx, stageKey{}, stage)
}
func Record(ctx context.Context, c Call) {
	if stage, ok := ctx.Value(stageKey{}).(string); ok {
		c.Stage = stage
	}
	if f, ok := ctx.Value(recorderKey{}).(func(Call)); ok {
		f(c)
	}
}
