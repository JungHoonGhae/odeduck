package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestSamplingHistoryKeepsRequestsAndOutcomesWithoutCallerSecretsOrRawRows(t *testing.T) {
	ctx := context.Background()
	calls := 0
	e, _ := Start("sample comparison", Policy{}, Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
		},
		Inspect: func(context.Context, string) (Inspection, error) { return Inspection{PK: "123"}, nil },
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			calls++
			if s.Delivery == "api" {
				s.Params["serviceKey"] = "CALLER_SECRET"
				return Acquired{}, fmt.Errorf("API unavailable")
			}
			s.Where["city"] = "CALLER_MUTATION"
			return Acquired{Delivery: "FILE", Rows: []Row{{"private": "SOURCE_ROW_NOT_FOR_PLANNER"}}}, nil
		},
	})
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "test", Region: "target", Period: "2025", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "value", Description: "test", Role: "role", Type: "string"}}}})
	step(Decision{Action: "search", Query: "test", Role: "role"})
	step(Decision{Action: "inspect", PK: "123"})
	api := SampleRequest{PK: "123", Delivery: "api", Operation: "observed operation", Params: map[string]string{"year": "2025"}, RowPath: "/items"}
	file := SampleRequest{PK: "123", Delivery: "file", Asset: "observed.csv", Where: map[string]string{"city": "target"}}
	step(Decision{Action: "sample", Sample: &api})
	step(Decision{Action: "sample", Sample: &file})
	b, _ := json.Marshal(e.PlanningView())
	for _, bad := range []string{"CALLER_SECRET", "CALLER_MUTATION", "SOURCE_ROW_NOT_FOR_PLANNER"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("private source or caller mutation leaked: %s", bad)
		}
	}
	var got struct {
		Attempts []struct {
			Revision    int           `json:"revision"`
			Request     SampleRequest `json:"request"`
			Hash        string        `json:"requestSha256"`
			Status      string        `json:"status"`
			Observation string        `json:"observationId"`
		} `json:"sampleAttempts"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Attempts) != 2 {
		t.Fatalf("missing immutable acquisition history: %+v", got.Attempts)
	}
	a, z := got.Attempts[0], got.Attempts[1]
	if a.Status != "failed" || a.Request.Operation != api.Operation || a.Request.Params["year"] != "2025" || a.Hash != digest(api) || a.Revision != 4 || a.Observation != "" {
		t.Fatalf("failed attempt lost: %+v", a)
	}
	if z.Status != "acquired" || z.Request.Where["city"] != "target" || z.Hash != digest(file) || z.Observation != "o1" || e.View().Observations[0].RequestSHA256 != digest(file) {
		t.Fatalf("selected sample scope lost: %+v", z)
	}
	if _, err := e.Advance(ctx, e.View().Revision, Decision{Action: "sample", Sample: &api}); err == nil {
		t.Fatal("history allowed replay")
	}
	step(Decision{Action: "sample", Sample: &SampleRequest{PK: "123", Delivery: "api", Params: map[string]string{"serviceKey": "REJECTED_SECRET"}}})
	b, _ = json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "REJECTED_SECRET") || calls != 2 || len(e.View().SampleAttempts) != 2 {
		t.Fatal("credential rejection entered history or acquisition")
	}
	view := e.PlanningView()
	view.SampleAttempts[0].Status = "acquired"
	view.SampleAttempts[1].Request.Where["city"] = "FORGED_SCOPE"
	view = e.PlanningView()
	if view.SampleAttempts[0].Status != "failed" || view.SampleAttempts[1].Request.Where["city"] != "target" {
		t.Fatal("returned view can mutate trusted acquisition history")
	}
}
