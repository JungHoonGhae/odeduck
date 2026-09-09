package goalwork_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestHistoricalFileSamplingRequiresSelectedEdition(t *testing.T) {
	for _, requested := range []string{"", "other", "past"} {
		t.Run("version="+requested, func(t *testing.T) {
			calls := 0
			e, err := goalwork.Start("Compare a historical source snapshot", goalwork.Policy{}, goalwork.Dependencies{
				Search: func(context.Context, string) (catalog.Result, error) {
					return catalog.Result{Hits: []catalog.Hit{{PK: "source"}}}, nil
				},
				InspectFileVersion: func(_ context.Context, pk, version string) (goalwork.Inspection, error) {
					i := goalwork.Inspection{PK: pk, FileVersions: []dataset.FileVersion{{ID: "past", Name: "past snapshot"}}}
					if version != "" {
						i.SelectedFileVersion = &i.FileVersions[0]
						i.Assets = []string{"same.csv"}
					}
					return i, nil
				},
				Sample: func(_ context.Context, s goalwork.SampleRequest, i goalwork.Inspection) (goalwork.Acquired, error) {
					calls++
					return goalwork.Acquired{Rows: []goalwork.Row{{"count": "7"}}, Delivery: "FILE"}, nil
				},
				Layout: func(context.Context, goalwork.LayoutRequest, goalwork.Inspection) (dataset.FileLayout, error) {
					return dataset.FileLayout{SHA256: strings.Repeat("a", 64)}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			c := goalwork.GoalContract{Outcome: "historical source report", Region: "source scope", Period: "historical snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "source count"}}}
			advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
			advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "source", Role: "r"})
			advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "source", FileHistory: true})
			advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "source", FileVersion: "past"})
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "source", Delivery: "file", Asset: "same.csv", FileVersion: requested}})
			if err != nil {
				t.Fatal(err)
			}
			if requested == "past" {
				if calls != 1 || len(v.Observations) != 1 || len(v.Gaps) != 0 || v.SampleAttempts[0].Request.FileVersion != "past" {
					t.Fatal("historical request was not preserved")
				}
			} else if calls != 0 || len(v.Observations) != 0 || len(v.Gaps) != 1 {
				t.Fatal("missing or wrong edition reached acquisition")
			}
			v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "layout", Layout: &goalwork.LayoutRequest{PK: "source", Asset: "same.csv", FileVersion: requested}})
			if err != nil || (len(v.Layouts) == 1) != (requested == "past") {
				t.Fatal("layout did not enforce the same selected edition as sampling")
			}
		})
	}
}

func TestHistoricalFileInspectionCannotSubstituteAnEdition(t *testing.T) {
	for _, returned := range []string{"", "other"} {
		t.Run("returned="+returned, func(t *testing.T) {
			e, err := goalwork.Start("Read a prior publication", goalwork.Policy{}, goalwork.Dependencies{
				Search: func(context.Context, string) (catalog.Result, error) {
					return catalog.Result{Hits: []catalog.Hit{{PK: "source"}}}, nil
				},
				InspectFileVersion: func(_ context.Context, pk, version string) (goalwork.Inspection, error) {
					i := goalwork.Inspection{PK: pk, FileVersions: []dataset.FileVersion{{ID: "past", Name: "prior publication"}}}
					if version != "" {
						i.Assets = []string{"wrong.csv"}
						if returned != "" {
							i.SelectedFileVersion = &dataset.FileVersion{ID: returned}
						}
					}
					return i, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			c := resultContract()
			advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
			advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "source", Role: c.Roles[0].ID})
			advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "source", FileHistory: true})
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "inspect", PK: "source", FileVersion: "past"})
			if err != nil || len(v.Gaps) != 1 || len(v.Nodes[0].Inspection.Assets) != 0 || v.Nodes[0].Inspection.SelectedFileVersion != nil {
				t.Fatalf("substituted edition replaced the known history: %+v %v", v.Nodes, err)
			}
		})
	}
}
