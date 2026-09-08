package goalwork

import (
	"context"
	"fmt"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestStandaloneReplayRepairDoesNotRepeatAcquisition(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		t.Run(fmt.Sprint(persistent), func(t *testing.T) {
			calls, plans := 0, 0
			e, _ := Start("test", Policy{}, Dependencies{Search: func(context.Context, string) (catalog.Result, error) {
				calls++
				return catalog.Result{}, fmt.Errorf("source unavailable")
			}})
			define := Decision{Action: "define", Contract: &GoalContract{Outcome: "test", Region: "test", Period: "test", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "value", Description: "test", Role: "role", Type: "string"}}}}
			v, err := Run(context.Background(), e, func(_ context.Context, v View) (Decision, error) {
				plans++
				if plans == 4 && v.PlannerFeedback == "" {
					t.Fatal("correction lacked replay diagnostic")
				}
				if plans == 1 {
					return define, nil
				}
				if plans == 4 && !persistent {
					return Decision{Action: "abstain", Reason: "requires another source"}, nil
				}
				return Decision{Action: "search", Query: "unchanged", Role: "role"}, nil
			}, nil)
			if calls != 1 || plans != 4 {
				t.Fatalf("replay repair must add one plan, no external repeat: calls=%d plans=%d err=%v", calls, plans, err)
			}
			if !persistent && (err != nil || v.Status != "abstained") {
				t.Fatalf("could not change action after replay: %s %v", v.Status, err)
			}
			if persistent && err == nil {
				t.Fatal("persistent replay accepted")
			}
		})
	}
}

func TestStandaloneReplayCorrectionIsNotRearmedByAnotherAction(t *testing.T) {
	plans, calls := 0, 0
	e, _ := Start("test", Policy{}, Dependencies{Search: func(context.Context, string) (catalog.Result, error) {
		calls++
		return catalog.Result{}, fmt.Errorf("unavailable")
	}})
	_, err := Run(context.Background(), e, func(context.Context, View) (Decision, error) {
		plans++
		if plans == 1 {
			return Decision{Action: "define", Contract: &GoalContract{Outcome: "test", Region: "test", Period: "test", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "value", Description: "test", Role: "role", Type: "string"}}}}, nil
		}
		q := "first"
		if plans >= 4 {
			q = "second"
		}
		return Decision{Action: "search", Query: q, Role: "role"}, nil
	}, nil)
	if err == nil || plans != 5 || calls != 2 {
		t.Fatalf("correction allowance was reset: plans=%d calls=%d err=%v", plans, calls, err)
	}
}
