package goalwork

import (
	"context"
	"errors"
	"fmt"
)

type FailureKind string

const (
	FailureAccessRequired FailureKind = "access_required"
	FailureTransient      FailureKind = "transient"
	FailureCancelled      FailureKind = "cancelled"
	FailureUnknown        FailureKind = "unclassified"
)

// AcquisitionError is supplied only by trusted acquisition adapters, never by
// a planner. Cause supports errors.Is/As but is not exposed in the safe message.
// In particular, a URL or provider response containing credentials stays private.
type AcquisitionError struct {
	Kind  FailureKind
	Cause error
}

func (e *AcquisitionError) Error() string {
	switch e.Kind {
	case FailureAccessRequired:
		return "acquisition access is not ready; check the existing login/key/approval prerequisite before an explicit retry (no automatic login or application)"
	case FailureTransient:
		return "acquisition encountered a transient transport failure; an explicit bounded retry is available"
	case FailureCancelled:
		return "acquisition was cancelled; an explicit bounded retry needs a live context"
	default:
		return "unclassified acquisition failure; inspect the contract or choose another request"
	}
}
func (e *AcquisitionError) Unwrap() error { return e.Cause }

func acquisitionFailureKind(err error) FailureKind {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return FailureCancelled
	}
	var failure *AcquisitionError
	if errors.As(err, &failure) && failure != nil {
		switch failure.Kind {
		case FailureAccessRequired, FailureTransient, FailureCancelled:
			return failure.Kind
		}
	}
	return FailureUnknown
}

// retryRequest resolves an immutable, latest failed acquisition. A retry does
// not replace the request, contract, previous attempts, observations or budgets.
func (e *Engine) retryRequest(d Decision) (SampleRequest, error) {
	if d.RetryOf <= 0 || d.Sample != nil || d.Layout != nil || d.Contract != nil || d.Composition != nil || d.Query != "" || d.Role != "" || d.PK != "" || d.CompositionID != "" || len(d.Reason) > 1000 {
		return SampleRequest{}, fmt.Errorf("retry_sample accepts only retryOf (failed attempt revision) and an optional bounded reason; the engine reuses the retained request")
	}
	var target *SampleAttempt
	for i := range e.state.SampleAttempts {
		if e.state.SampleAttempts[i].Revision == d.RetryOf {
			target = &e.state.SampleAttempts[i]
			break
		}
	}
	if target == nil || target.Status != "failed" {
		return SampleRequest{}, fmt.Errorf("retryOf must identify a failed acquisition in this goal")
	}
	if target.FailureKind != FailureAccessRequired && target.FailureKind != FailureTransient && target.FailureKind != FailureCancelled {
		return SampleRequest{}, fmt.Errorf("unclassified or invalid acquisition cannot be retried unchanged; inspect the gap or choose a different supported request")
	}
	count := 0
	for _, attempt := range e.state.SampleAttempts {
		if attempt.RequestSHA256 == target.RequestSHA256 {
			count++
			if attempt.Revision > target.Revision {
				return SampleRequest{}, fmt.Errorf("retryOf is superseded by a later attempt of this request")
			}
		}
	}
	if count >= 3 || e.samples >= 8 {
		return SampleRequest{}, fmt.Errorf("retry budget exhausted: at most 3 acquisitions per request and 8 total")
	}
	return cloneSampleRequest(target.Request), nil
}

func (e *Engine) retrySearch(d Decision) (SearchRecord, error) {
	if d.RetryOf <= 0 || d.Sample != nil || d.Layout != nil || d.Contract != nil || d.Composition != nil || d.Evidence != nil || d.Query != "" || d.Role != "" || d.PK != "" || d.CompositionID != "" || d.FileHistory || d.FileVersion != "" || len(d.Reason) > 1000 {
		return SearchRecord{}, fmt.Errorf("retry_search accepts only retryOf and an optional bounded reason; the engine reuses the retained query and role")
	}
	if e.state.Status != "blocked" || len(e.state.Gaps) == 0 || len(e.state.Searches) == 0 || !e.state.Policy.RequireSemantic {
		return SearchRecord{}, fmt.Errorf("retry_search requires a blocked semantic search in this goal")
	}
	gap := e.state.Gaps[len(e.state.Gaps)-1]
	last := e.state.Searches[len(e.state.Searches)-1]
	if gap.Revision != d.RetryOf || gap.Revision != e.state.Revision || (gap.Action != "search" && gap.Action != "retry_search") || (last.Semantic != nil && last.Semantic.Status == "used") {
		return SearchRecord{}, fmt.Errorf("retryOf must identify the latest blocked semantic search")
	}
	count := 0
	for _, s := range e.state.Searches {
		if s.Query == last.Query && s.Role == last.Role {
			count++
		}
	}
	if count >= 3 || len(e.state.Searches) >= maxSearches || e.state.Revision >= e.state.Policy.MaxRounds {
		return SearchRecord{}, fmt.Errorf("search retry budget exhausted: at most 3 attempts per request within the original search/round limits")
	}
	return last, nil
}
