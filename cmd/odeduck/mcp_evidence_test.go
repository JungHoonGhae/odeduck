package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/mcpserver"
)

func TestMCPReviewNeedsSeparateStartupDisclosure(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"--review-goals-with=claude"}, false},
		{[]string{"--share-goal-evidence", "--review-goals-with=auto"}, false},
		{[]string{"--share-goal-evidence", "--review-goals-with=claude"}, true},
	} {
		called := false
		cmd := mcpCommand(func(_ context.Context, d mcpserver.Deps) error {
			called = true
			if d.GoalReviewProvider != "claude" || !d.ShareGoalEvidence {
				t.Fatal("lost review authority")
			}
			return nil
		})
		cmd.SetArgs(tc.args)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if (err == nil) != tc.valid || called != tc.valid {
			t.Fatalf("%v: %v", tc.args, err)
		}
	}
}

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
