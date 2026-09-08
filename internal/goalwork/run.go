package goalwork

import (
	"context"
	"errors"
)

// Run is the standalone planning loop. MCP drives Advance directly using the
// host model; both callers use the same state transitions and computed output.
func Run(ctx context.Context, e *Engine, next func(context.Context, View) (Decision, error), progress func(View)) (View, error) {
	replayCorrectionUsed := false
	feedback := ""
	for {
		view := e.PlanningView()
		view.PlannerFeedback, feedback = feedback, ""
		if view.Status != "exploring" {
			return e.View(), nil
		}
		if err := ctx.Err(); err != nil {
			return e.View(), err
		}
		decision, err := next(ctx, view)
		if err != nil {
			return e.View(), err
		}
		result, err := e.Advance(ctx, view.Revision, decision)
		if errors.Is(err, ErrActionAlreadyAttempted) && !replayCorrectionUsed {
			replayCorrectionUsed = true
			feedback = "The last action was rejected by replay protection without acquisition or revision change. That exact request was already attempted. Choose a DIFFERENT supported action using the recorded gaps (retry_sample only for an eligible latest classified failure), or abstain honestly. This is the one permitted replay correction in this standalone run; do not replay the same input again."
			continue
		}
		if progress != nil {
			progress(e.PlanningView())
		}
		if err != nil {
			return result, err
		}
	}
}
