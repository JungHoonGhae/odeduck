package goalwork

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
)

func TestGoalObservationReportsValueFreeColumnShapes(t *testing.T) {
	ctx := context.Background()
	e, _ := Start("지역 단위가 다른 자료 연결", Policy{}, Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "s"}}}, nil
		},
		Inspect: func(context.Context, string) (Inspection, error) { return Inspection{PK: "s"}, nil },
		Sample: func(context.Context, SampleRequest, Inspection) (Acquired, error) {
			return Acquired{Rows: []Row{{"region": "공주시", "secret": "RAW_VALUE_NOT_FOR_PLANNER"}, {"region": "공주시 계룡면", "n": " 1,000 "}, {"region": " \u3000"}, {"region": nil}, {}}, Delivery: "FILE"}, nil
		},
	})
	for _, d := range []Decision{
		{Action: "define", Contract: &GoalContract{Outcome: "비교", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "name", Role: "role", Type: "string", Description: "test"}}}},
		{Action: "search", Query: "source", Role: "role"}, {Action: "inspect", PK: "s"}, {Action: "sample", Sample: &SampleRequest{PK: "s", Delivery: "file"}},
	} {
		v, err := e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("%+v %v", v.Gaps, err)
		}
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "RAW_VALUE_NOT_FOR_PLANNER") || strings.Contains(string(b), "공주시") {
		t.Fatal("raw values leaked into planning metadata")
	}
	var got struct {
		Observations []struct {
			ColumnProfiles map[string]struct {
				Version        string `json:"version"`
				Missing        int    `json:"missing"`
				Null           int    `json:"null"`
				Strings        int    `json:"strings"`
				Blank          int    `json:"blank"`
				Trimmed        int    `json:"trimmed"`
				MinTokens      int    `json:"minTokens"`
				MaxTokens      int    `json:"maxTokens"`
				Decimal        int    `json:"decimal"`
				GroupedDecimal int    `json:"groupedDecimal"`
			} `json:"columnProfiles"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	p := got.Observations[0].ColumnProfiles["region"]
	if p.Version != "shape_v1" || p.Missing != 1 || p.Null != 1 || p.Strings != 3 || p.Blank != 1 || p.Trimmed != 1 || p.MinTokens != 1 || p.MaxTokens != 2 {
		t.Fatalf("incorrect observed shape: %+v", p)
	}
	n := got.Observations[0].ColumnProfiles["n"]
	if n.Decimal != 0 || n.GroupedDecimal != 1 || n.Trimmed != 1 || n.Missing != 4 {
		t.Fatalf("incorrect numeric shape: %+v", n)
	}
	// Statistics are not implicit conversion or a mutation of the private source.
	if e.rows["o1"][1]["n"] != " 1,000 " {
		t.Fatal("profile changed source")
	}
}
