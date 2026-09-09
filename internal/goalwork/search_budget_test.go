package goalwork

import (
	"context"
	"fmt"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestCandidateCapacitySupportsEveryAllowedSearchWithoutIncreasingCalls(t *testing.T) {
	calls := 0
	e, _ := Start("search counterpart without discarding earlier alternatives", Policy{}, Dependencies{Search: func(_ context.Context, q string) (catalog.Result, error) {
		calls++
		hits := make([]catalog.Hit, 9)
		for i := range hits {
			hits[i] = catalog.Hit{PK: fmt.Sprintf("%s-%d", q, i)}
		}
		return catalog.Result{Hits: hits}, nil
	}})
	ctx := context.Background()
	_, err := e.Advance(ctx, 0, Decision{Action: "define", Contract: &GoalContract{Outcome: "test", Region: "test", Period: "test", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "value", Description: "test", Role: "role", Type: "string"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		v, err := e.Advance(ctx, e.View().Revision, Decision{Action: "search", Query: fmt.Sprint(i), Role: "role"})
		if err != nil || len(v.Gaps) > 0 || len(v.Nodes) != (i+1)*8 {
			t.Fatalf("allowed search %d blocked: candidates=%d gaps=%+v err=%v", i+1, len(v.Nodes), v.Gaps, err)
		}
	}
	v, err := e.Advance(ctx, e.View().Revision, Decision{Action: "search", Query: "thirteenth", Role: "role"})
	if err != nil || calls != 12 || len(v.Nodes) != 96 || len(v.Gaps) != 1 || v.Budget.SearchesRemaining != 0 || v.Budget.CandidatesRemaining != 0 {
		t.Fatalf("budgets escaped: %+v calls=%d err=%v", v.Budget, calls, err)
	}
}
