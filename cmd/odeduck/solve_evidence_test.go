package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveEvidenceNeedsExplicitRecipientAndPreservesDefaultPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     []string
		recipient string
		run       bool
	}{
		{"default", nil, "", true},
		{"explicit", []string{"--agent=claude", "--share-evidence"}, "claude", true},
		{"auto", []string{"--share-evidence"}, "", false},
		{"cursor", []string{"--agent=cursor", "--share-evidence"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			cmd := solveCommand(func(_ context.Context, goal string, p goalwork.Policy, provider string, _ func(goalwork.View)) (goalwork.View, error) {
				called = true
				if p.EvidenceRecipient != tc.recipient {
					t.Fatalf("wrong recipient: %s", p.EvidenceRecipient)
				}
				return goalwork.View{Goal: goal, Status: "abstained"}, nil
			})
			var out, stderr bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			cmd.SetArgs(append([]string{"fixture goal"}, tc.flags...))
			err := cmd.Execute()
			if called != tc.run || err == nil {
				t.Fatalf("policy boundary: called=%v err=%v", called, err)
			}
			if tc.recipient != "" && (!strings.Contains(stderr.String(), "claude") || !strings.Contains(stderr.String(), "전송")) {
				t.Fatal("missing disclosure notice")
			}
		})
	}
}
