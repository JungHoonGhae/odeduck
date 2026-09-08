package goalwork_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func retryEngine(t *testing.T, sample func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error)) *goalwork.Engine {
	t.Helper()
	e, err := goalwork.Start("preserve the original requested comparison", goalwork.Policy{}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) { return goalwork.Inspection{PK: "123"}, nil },
		Sample:  sample,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []goalwork.Decision{
		{Action: "define", Contract: &goalwork.GoalContract{Outcome: "comparison", Region: "fixture", Period: "2025", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "observed role"}}, Outputs: []goalwork.OutputRequirement{{ID: "x", Role: "r", Type: "string", Description: "observed value"}}}},
		{Action: "search", Query: "fixture", Role: "r"}, {Action: "inspect", PK: "123"},
	} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func TestExplicitRetryPreservesFailedRequestAndAppendsObservation(t *testing.T) {
	calls := 0
	var requests []goalwork.SampleRequest
	e := retryEngine(t, func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
		calls++
		requests = append(requests, s)
		if calls == 1 {
			return goalwork.Acquired{}, &goalwork.AcquisitionError{Kind: goalwork.FailureAccessRequired, Cause: errors.New("PRIVATE_CALLER_SECRET")}
		}
		return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001"}}, Delivery: "REST"}, nil
	})
	s := goalwork.SampleRequest{PK: "123", Delivery: "api", Operation: "official-operation", Params: map[string]string{"year": "2025"}, RowPath: "/items"}
	before := e.View()
	v, err := e.Advance(context.Background(), before.Revision, goalwork.Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.SampleAttempts) != 1 || v.SampleAttempts[0].FailureKind != goalwork.FailureAccessRequired {
		t.Fatalf("missing classified failure: %+v %v", v.SampleAttempts, err)
	}
	failed := v.SampleAttempts[0]
	if _, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "sample", Sample: &s}); !errors.Is(err, goalwork.ErrActionAlreadyAttempted) {
		t.Fatal("ordinary replay must remain blocked")
	}
	if _, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: failed.Revision, Sample: &s}); err == nil {
		t.Fatal("retry accepted a caller-supplied replacement request")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: failed.Revision, Reason: "access prerequisite checked externally"})
	if err != nil || calls != 2 || len(v.Observations) != 1 || len(v.SampleAttempts) != 2 {
		t.Fatalf("retry did not reacquire: %+v %v calls=%d", v.SampleAttempts, err, calls)
	}
	if !reflect.DeepEqual(requests[0], requests[1]) || !reflect.DeepEqual(failed, v.SampleAttempts[0]) || v.SampleAttempts[1].RetryOf != failed.Revision || v.SampleAttempts[1].Status != "acquired" || v.SampleAttempts[1].FailureKind != "" || v.SampleAttempts[1].RequestSHA256 != failed.RequestSHA256 {
		t.Fatal("retry changed request or rewrote history")
	}
	if v.Goal != before.Goal || !reflect.DeepEqual(v.Contract, before.Contract) || v.Budget.SamplesRemaining != 6 || v.Status != "exploring" {
		t.Fatal("retry weakened goal or bypassed budget/acceptance")
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "PRIVATE_CALLER_SECRET") {
		t.Fatal("typed acquisition error exposed private cause")
	}
	if _, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: failed.Revision}); err == nil {
		t.Fatal("superseded attempt remained retryable")
	}
	if _, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: v.SampleAttempts[1].Revision}); err == nil {
		t.Fatal("successful observation was retried")
	}
}

func TestXLSXRetryRetainsRectangleDespiteAdapterAndViewMutation(t *testing.T) {
	calls := 0
	table := &dataset.TableProvenance{Sheet: "districts", Range: "A27:B27", RowNumbers: []int{27}, FormulaCells: []string{"B27"}}
	e := retryEngine(t, func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
		calls++
		if s.XLSX == nil || s.XLSX.Range != "A27:B27" {
			t.Error("retry changed original rectangle")
		}
		s.XLSX.Range = "A1:B1"
		if calls == 1 {
			return goalwork.Acquired{}, &goalwork.AcquisitionError{Kind: goalwork.FailureTransient, Cause: errors.New("timeout")}
		}
		return goalwork.Acquired{Rows: []goalwork.Row{{"A": "001", "B": "148"}}, Delivery: "FILE", Table: table}, nil
	})
	s := goalwork.SampleRequest{PK: "123", Delivery: "file", Asset: "source.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "districts", Range: "A27:B27"}}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
	if err != nil {
		t.Fatal(err)
	}
	failed := v.SampleAttempts[0]
	s.XLSX.Range = "A2:B2"
	v.SampleAttempts[0].Request.XLSX.Range = "A3:B3"
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: failed.Revision})
	if err != nil || len(v.Observations) != 1 || v.SampleAttempts[0].RequestSHA256 != v.SampleAttempts[1].RequestSHA256 || v.SampleAttempts[1].Request.XLSX.Range != "A27:B27" {
		t.Fatalf("XLSX retry contract changed: %+v %v", v.SampleAttempts, err)
	}
	table.RowNumbers[0] = 1
	table.FormulaCells[0] = "B1"
	fresh := e.View()
	if fresh.Observations[0].Table.RowNumbers[0] != 27 || fresh.Observations[0].Table.FormulaCells[0] != "B27" {
		t.Fatal("adapter mutated retained provenance")
	}
}

func TestRetryBudgetCannotBeResetByFollowingNewFailures(t *testing.T) {
	calls := 0
	e := retryEngine(t, func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
		calls++
		return goalwork.Acquired{}, &goalwork.AcquisitionError{Kind: goalwork.FailureTransient, Cause: errors.New("timeout")}
	})
	v, _ := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "api"}})
	for n := 0; n < 2; n++ {
		var err error
		v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: v.SampleAttempts[len(v.SampleAttempts)-1].Revision})
		if err != nil {
			t.Fatal(err)
		}
	}
	before := v.Revision
	_, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: v.SampleAttempts[2].Revision})
	if err == nil || calls != 3 || e.View().Revision != before || e.View().Budget.SamplesRemaining != 5 {
		t.Fatal("request-chain limit did not stop fourth attempt without consuming another revision")
	}
}

func TestUnknownAcquisitionFailureCannotBeRetriedByChangingProse(t *testing.T) {
	calls := 0
	e := retryEngine(t, func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
		calls++
		return goalwork.Acquired{}, errors.New("unsupported contract, not transient")
	})
	v, _ := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "file", Asset: "observed.zip"}})
	// Returned planning state is detached; forged classifications have no authority.
	v.SampleAttempts[0].FailureKind = goalwork.FailureAccessRequired
	for _, d := range []goalwork.Decision{{Action: "retry_sample", RetryOf: 0}, {Action: "retry_sample", RetryOf: 999}, {Action: "retry_sample", RetryOf: v.Revision, Reason: "I declare access fixed"}} {
		if _, err := e.Advance(context.Background(), v.Revision, d); err == nil {
			t.Fatal("unknown failure or fake attempt became retryable")
		}
	}
	if calls != 1 || e.View().Revision != v.Revision {
		t.Fatal("rejected retry consumed acquisition")
	}
}

func TestExplicitRetryAlsoHonoursOverallSampleBudget(t *testing.T) {
	calls := 0
	e := retryEngine(t, func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
		calls++
		return goalwork.Acquired{}, &goalwork.AcquisitionError{Kind: goalwork.FailureTransient}
	})
	for i := 0; i < 8; i++ {
		_, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "api", Params: map[string]string{"page": strconv.Itoa(i + 1)}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if _, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "retry_sample", RetryOf: v.SampleAttempts[7].Revision}); err == nil || calls != 8 || e.View().Revision != v.Revision || e.View().Budget.SamplesRemaining != 0 {
		t.Fatal("one-attempt request bypassed overall eight-acquisition budget")
	}
}

func TestCancelledAcquisitionRetriesOnlyWithLiveContextAndCurrentRevision(t *testing.T) {
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	e := retryEngine(t, func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
		calls++
		if calls == 1 {
			cancel()
			return goalwork.Acquired{}, context.Canceled
		}
		return goalwork.Acquired{Rows: []goalwork.Row{{"id": "001"}}}, nil
	})
	v, err := e.Advance(ctx, e.View().Revision, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "api"}})
	if !errors.Is(err, context.Canceled) || v.SampleAttempts[0].FailureKind != goalwork.FailureCancelled {
		t.Fatal("cancelled attempt or context error lost")
	}
	d := goalwork.Decision{Action: "retry_sample", RetryOf: v.Revision}
	if _, err = e.Advance(ctx, v.Revision, d); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled context accepted for retry")
	}
	if _, err = e.Advance(context.Background(), v.Revision-1, d); err == nil {
		t.Fatal("stale revision accepted for retry")
	}
	if calls != 1 || e.View().Revision != v.Revision {
		t.Fatal("rejected retry consumed request or revision")
	}
	v, err = goalwork.Run(context.Background(), e, func(_ context.Context, view goalwork.View) (goalwork.Decision, error) {
		if len(view.Observations) == 0 {
			return d, nil
		}
		return goalwork.Decision{Action: "abstain", Reason: "acquisition recovered; comparison still incomplete"}, nil
	}, nil)
	if err != nil || calls != 2 || len(v.Observations) != 1 || v.Status != "abstained" {
		t.Fatal("standalone loop did not recover acquisition without claiming completion")
	}
}
