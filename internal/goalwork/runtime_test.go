package goalwork_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
	"github.com/JungHoonGhae/odeduck/internal/version"
)

func TestGoalStartReportsActualBuildAndPermittedActions(t *testing.T) {
	e, err := goalwork.Start("자료를 비교해 표로 보여줘", goalwork.Policy{}, goalwork.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	// Decode the public result, exactly as a CLI or MCP consumer does.
	body, err := json.Marshal(e.PlanningView())
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Runtime struct {
			Build        string   `json:"build"`
			Executable   string   `json:"executable"`
			BinarySHA256 string   `json:"binarySha256"`
			GuideSHA256  string   `json:"guideSha256"`
			Actions      []string `json:"actions"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Runtime.Build != version.String() || len(out.Runtime.GuideSHA256) != 64 {
		t.Fatalf("actual running build and delivered guide identity missing: %s", body)
	}
	if !filepath.IsAbs(out.Runtime.Executable) || len(out.Runtime.BinarySHA256) != 64 {
		t.Fatal("runtime does not identify the actual executable bytes")
	}
	if _, err := os.Stat(out.Runtime.Executable); err != nil {
		t.Fatal(err)
	}
	if len(out.Runtime.Actions) == 0 {
		t.Fatal("caller cannot discover permitted engine actions")
	}
	for _, action := range out.Runtime.Actions {
		if action == "review_result" || action == "read_evidence" || action == "search" {
			t.Fatalf("unavailable or unauthorized action advertised: %s", action)
		}
	}
}

func TestGoalPreflightCancellationAndBlockedChecksDoNotReportReady(t *testing.T) {
	for _, cancelled := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		e, err := goalwork.Start("goal", goalwork.Policy{}, goalwork.Dependencies{Preflight: func(context.Context) (goalwork.Environment, error) {
			if cancelled {
				return goalwork.Environment{}, nil
			}
			return goalwork.Environment{Checks: []goalwork.RuntimeCheck{{Name: "catalog", Status: "blocked"}}}, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		v, err := e.Preflight(ctx)
		cancel()
		if err == nil || v.Runtime.Status != "blocked" || v.Revision != 0 || len(v.Searches) != 0 {
			t.Fatal("failed check was reported ready or consumed goal actions")
		}
		if cancelled && !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancellation cause")
		}
	}
}

func TestGoalRuntimeChecksMissingCatalogueBeforeCallingPlanner(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("APPDATA", filepath.Join(root, "appdata"))
	policy := goalwork.Policy{RequireSemantic: true}
	e, err := goalwork.Start("지역을 비교해 표로 보여줘", policy, goalwork.LiveDependencies(fetch.New(fetch.WithDelay(0)), "", nil, catalog.Searcher{}, policy))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	view, err := goalwork.Run(context.Background(), e, func(context.Context, goalwork.View) (goalwork.Decision, error) {
		called = true
		return goalwork.Decision{Action: "abstain", Reason: "planner should not be called"}, nil
	}, nil)
	if called || err == nil || view.Revision != 0 || len(view.Searches) != 0 {
		t.Fatalf("preflight failed to stop unavailable execution before model/search spend: called=%t err=%v view=%+v", called, err, view)
	}
}
