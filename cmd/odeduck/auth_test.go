package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/portal"
)

func TestStatusReportsAuthenticationAndProbeFailures(t *testing.T) {
	unavailable := errors.New("portal unavailable")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"valid", nil},
		{"missing", portal.ErrNotLoggedIn},
		{"unavailable", unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			cmd := statusCmdWith(func(context.Context) error { return tc.err })
			cmd.SetArgs(nil)
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			err := cmd.Execute()
			if !errors.Is(err, tc.err) {
				t.Fatalf("status returned %v; want %v", err, tc.err)
			}
			if tc.err == nil && !strings.Contains(output.String(), "세션이 살아있습니다") {
				t.Fatalf("missing success message: %q", output.String())
			}
			if tc.err != nil && strings.Contains(output.String(), "세션이 살아있습니다") {
				t.Fatal("failed probe reported success")
			}
			if errors.Is(err, unavailable) && errors.Is(err, portal.ErrNotLoggedIn) {
				t.Fatal("transport failure was classified as a missing login")
			}
		})
	}
}
