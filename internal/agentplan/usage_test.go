package agentplan

import (
	"context"
	"github.com/JungHoonGhae/odeduck/internal/modelusage"
	"testing"
	"time"
)

func TestProviderUsageAccountsOnlyEnvelopes(t *testing.T) {
	tests := []struct {
		name, provider, body        string
		input, output, cache, write int64
		known                       bool
	}{
		{"codex", ProviderCodex, "{\"type\":\"item.completed\",\"item\":{\"text\":\"ignore this\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1234,\"output_tokens\":56,\"cached_input_tokens\":1000}}", 1234, 56, 1000, 0, true},
		{"claude", ProviderClaude, "{\n\"type\":\"result\",\"usage\":{\"input_tokens\":20,\"output_tokens\":3,\"cache_read_input_tokens\":100,\"cache_creation_input_tokens\":50}}", 170, 3, 100, 50, true},
		{"model text is not accounting", ProviderCodex, `{"type":"item.completed","item":{"text":"{\"usage\":{\"input_tokens\":0}}"}}`, 0, 0, 0, 0, false},
		{"unknown", ProviderGemini, `{"response":"ok"}`, 0, 0, 0, 0, false},
		{"invalid counter", ProviderCodex, `{"type":"turn.completed","usage":{"input_tokens":-1,"output_tokens":50}}`, 0, 0, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tracker modelusage.Tracker
			ctx := modelusage.WithStage(modelusage.WithRecorder(context.Background(), tracker.Record), "planning")
			recordProviderUsage(ctx, tt.provider, "hidden prompt", []byte(tt.body), time.Second, false)
			s := tracker.Snapshot()
			if s.Calls != 1 || (s.ReportedCalls == 1) != tt.known || s.InputTokens != tt.input || s.OutputTokens != tt.output || s.CachedInputTokens != tt.cache || s.CacheWriteTokens != tt.write || s.Records[0].Stage != "planning" {
				t.Fatalf("incorrect accounting: %+v", s)
			}
		})
	}
}
