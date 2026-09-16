package goalwork_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func blockedSearch(t *testing.T, rounds int, recover *bool) *goalwork.Engine {
	t.Helper()
	e, err := goalwork.Start("original broad goal", goalwork.Policy{RequireSemantic: true, MaxRounds: rounds}, goalwork.Dependencies{Search: func(_ context.Context, q string) (catalog.Result, error) {
		if q != "unseeded question" {
			t.Fatal("retry changed query")
		}
		status := "unavailable"
		if *recover {
			status = "used"
		}
		return catalog.Result{Semantic: &catalog.SemanticInfo{Status: status}, Hits: []catalog.Hit{{PK: "123"}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	c := &goalwork.GoalContract{Outcome: "verified story", Region: "unspecified", Period: "source period", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "record"}}, Outputs: []goalwork.OutputRequirement{{ID: "x", Role: "r", Type: "string", Description: "value"}}}
	for _, d := range []goalwork.Decision{{Action: "define", Contract: c}, {Action: "search", Query: "unseeded question", Role: "r"}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func TestSearchRecoveryPreservesGoalHistoryAndStrictPolicy(t *testing.T) {
	recovered := false
	e := blockedSearch(t, 32, &recovered)
	before := e.View()
	if before.Status != "blocked" || len(before.Nodes) != 0 {
		t.Fatal("lexical fallback accepted")
	}
	for _, d := range []goalwork.Decision{
		{Action: "search", Query: "replacement", Role: "r"},
		{Action: "retry_search", RetryOf: 2, Query: "replacement"},
		{Action: "retry_search", RetryOf: 1},
		{Action: "retry_search", RetryOf: 2, Evidence: &goalwork.EvidenceRequest{}},
	} {
		if _, err := e.Advance(context.Background(), 2, d); err == nil {
			t.Fatal("invalid recovery accepted", d)
		}
	}
	recovered = true
	v, err := e.Advance(context.Background(), 2, goalwork.Decision{Action: "retry_search", RetryOf: 2})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "exploring" || len(v.Nodes) != 1 || len(v.Searches) != 2 || len(v.Gaps) != 1 || v.Revision != 3 || v.Budget.SearchesRemaining != 10 || v.Goal != before.Goal || !reflect.DeepEqual(v.Contract, before.Contract) || !reflect.DeepEqual(v.Policy, before.Policy) || !v.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatalf("recovery lost state: %+v", v)
	}
	if _, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_search", RetryOf: 2}); err == nil {
		t.Fatal("old failure retried")
	}
}

func TestSearchRecoveryCannotResetBudgetsOrAcceptFallback(t *testing.T) {
	recovered := false
	e := blockedSearch(t, 4, &recovered)
	v, err := e.Advance(context.Background(), 2, goalwork.Decision{Action: "retry_search", RetryOf: 2})
	if err != nil || v.Status != "blocked" || len(v.Nodes) != 0 {
		t.Fatal("retry weakened semantic gate", err)
	}
	v, err = e.Advance(context.Background(), 3, goalwork.Decision{Action: "retry_search", RetryOf: 3})
	if err != nil || v.Status != "budget_exhausted" || len(v.Searches) != 3 {
		t.Fatal("failed retry bypassed round limit", v.Status, err)
	}
	recovered = true
	if _, err = e.Advance(context.Background(), 4, goalwork.Decision{Action: "retry_search", RetryOf: 4}); err == nil {
		t.Fatal("exhausted goal resumed")
	}
}

func TestSearchRetryLimitDoesNotResetWithEachFailure(t *testing.T) {
	recovered := false
	e := blockedSearch(t, 32, &recovered)
	for _, rev := range []int{2, 3} {
		if _, err := e.Advance(context.Background(), rev, goalwork.Decision{Action: "retry_search", RetryOf: rev}); err != nil {
			t.Fatal(err)
		}
	}
	before := e.View()
	if _, err := e.Advance(context.Background(), 4, goalwork.Decision{Action: "retry_search", RetryOf: 4}); err == nil {
		t.Fatal("per-request retry cap bypassed")
	}
	if e.View().Revision != before.Revision || len(e.View().Searches) != 3 {
		t.Fatal("rejected retry consumed acquisition")
	}
}
