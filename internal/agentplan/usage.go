package agentplan

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/modelusage"
)

func recordProviderUsage(ctx context.Context, provider, prompt string, body []byte, elapsed time.Duration, failed bool) {
	c := modelusage.Call{Provider: provider, PromptBytes: len(prompt), ResponseBytes: len(body), DurationMS: elapsed.Milliseconds(), Failed: failed}
	// Read only documented provider accounting envelopes, never model-written text.
	frames := [][]byte{body}
	if !json.Valid(body) {
		frames = bytes.Split(body, []byte("\n"))
	}
	for _, line := range frames {
		var event struct {
			Type  string          `json:"type"`
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if (provider == ProviderCodex && event.Type == "turn.completed") || (provider == ProviderClaude && event.Type == "result") {
			var u struct {
				Input      *int64 `json:"input_tokens"`
				Output     *int64 `json:"output_tokens"`
				Cached     *int64 `json:"cached_input_tokens"`
				CacheRead  *int64 `json:"cache_read_input_tokens"`
				CacheWrite *int64 `json:"cache_creation_input_tokens"`
			}
			if json.Unmarshal(event.Usage, &u) != nil || !validTokenCount(u.Input) || !validTokenCount(u.Output) {
				continue
			}
			c.InputTokens, c.OutputTokens = u.Input, u.Output
			if validTokenCount(u.Cached) {
				c.CachedInputTokens = u.Cached
			}
			if provider == ProviderClaude {
				// Claude reports uncached input separately; normalize total input while
				// retaining cache-read/write counters. Never add these again to total input.
				if validTokenCount(u.CacheRead) {
					c.CachedInputTokens = u.CacheRead
					*c.InputTokens += *u.CacheRead
				}
				if validTokenCount(u.CacheWrite) {
					c.CacheWriteTokens = u.CacheWrite
					*c.InputTokens += *u.CacheWrite
				}
			}
		}
	}
	modelusage.Record(ctx, c)
}
func validTokenCount(n *int64) bool { return n != nil && *n >= 0 && *n <= 1_000_000_000 }
