package modelusage

import (
	"context"
	"testing"
)

func TestUsageKeepsUnknownCallsSeparate(t *testing.T) {
	var tracker Tracker
	ctx := WithRecorder(context.Background(), tracker.Record)
	Record(ctx, Call{Provider: "codex", Stage: "planning", PromptBytes: 100, ResponseBytes: 20})
	n, o := int64(1000), int64(50)
	Record(ctx, Call{Provider: "claude", Stage: "review", InputTokens: &n, OutputTokens: &o})
	got := tracker.Snapshot()
	if got.Calls != 2 || got.ReportedCalls != 1 || got.UnreportedCalls != 1 || got.InputTokens != 1000 || got.OutputTokens != 50 {
		t.Fatalf("unknown usage treated as zero: %+v", got)
	}
}
