package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/mcpserver"
)

func TestMCPCommandFixesEvidencePolicyAtServerStartup(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			called := false
			cmd := mcpCommand(func(_ context.Context, deps mcpserver.Deps) error {
				called = true
				if deps.ShareGoalEvidence != enabled {
					t.Fatal("startup policy lost")
				}
				return nil
			})
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"--share-goal-evidence=" + fmt.Sprint(enabled)})
			if err := cmd.Execute(); err != nil || !called {
				t.Fatalf("startup: %v", err)
			}
			if enabled && !strings.Contains(stderr.String(), "MCP host") {
				t.Fatal("missing recipient notice")
			}
		})
	}
}
