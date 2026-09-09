package goalwork

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

func TestLayoutDiscoveryAndPinnedAcquisitionRejectWorkbookDrift(t *testing.T) {
	layoutCalls, sampleCalls := 0, 0
	e, err := Start("compare original source", Policy{}, Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
		},
		Inspect: func(context.Context, string) (Inspection, error) {
			return Inspection{PK: "123", Assets: []string{"source.xlsx"}}, nil
		},
		Layout: func(_ context.Context, r LayoutRequest, _ Inspection) (dataset.FileLayout, error) {
			layoutCalls++
			hash := strings.Repeat("a", 64)
			if r.RefreshOf != "" {
				hash = strings.Repeat("b", 64)
			}
			return dataset.FileLayout{SHA256: hash, Sheets: []dataset.XLSXSheetRef{{Name: "districts", Member: "xl/worksheets/sheet1.xml"}}}, nil
		},
		Sample: func(context.Context, SampleRequest, Inspection) (Acquired, error) {
			sampleCalls++
			return Acquired{Rows: []Row{{"A": "001"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("b", 64)}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, d := range []Decision{{Action: "define", Contract: &GoalContract{Outcome: "comparison", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []RoleRequirement{{ID: "r", Description: "source"}}, Outputs: []OutputRequirement{{ID: "id", Role: "r", Description: "code", Type: "string"}}}}, {Action: "search", Query: "source", Role: "r"}, {Action: "inspect", PK: "123"}} {
		if v := step(d); len(v.Gaps) > 0 {
			t.Fatal(v.Gaps)
		}
	}
	v := step(Decision{Action: "layout", Layout: &LayoutRequest{PK: "123", Asset: "source.xlsx"}})
	if len(v.Layouts) != 1 || layoutCalls != 1 || v.Layouts[0].ID != "l1" || v.Budget.LayoutsRemaining != 5 {
		t.Fatalf("layout missing: %+v", v)
	}
	v.Layouts[0].Layout.Sheets[0].Name = "forged"
	if e.PlanningView().Layouts[0].Layout.Sheets[0].Name != "districts" {
		t.Fatal("layout view mutated state")
	}
	for _, s := range []SampleRequest{
		{PK: "123", Delivery: "file", Asset: "source.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "districts", Range: "A1:A2"}, LayoutID: "nonexistent"},
		{PK: "123", Delivery: "file", Asset: "different.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "districts", Range: "A1:A2"}, LayoutID: "l1"},
		{PK: "123", Delivery: "file", Asset: "source.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "invented", Range: "A1:A2"}, LayoutID: "l1"},
	} {
		v = step(Decision{Action: "sample", Sample: &s})
		if len(v.Gaps) == 0 {
			t.Fatal("invalid binding accepted")
		}
	}
	if sampleCalls != 0 {
		t.Fatal("invalid layout binding reached acquisition")
	}
	v = step(Decision{Action: "sample", Sample: &SampleRequest{PK: "123", Delivery: "file", Asset: "source.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "districts", Range: "A1:A2"}, LayoutID: "l1"}})
	if sampleCalls != 1 || len(v.Observations) != 0 || len(v.SampleAttempts) != 1 || v.SampleAttempts[0].Status != "failed" || !strings.Contains(v.Gaps[len(v.Gaps)-1].Detail, "changed") {
		t.Fatalf("changed workbook accepted: %+v", v)
	}
	v = step(Decision{Action: "layout", Layout: &LayoutRequest{PK: "123", Asset: "source.xlsx", RefreshOf: "l1"}})
	if len(v.Layouts) != 2 || v.Layouts[0].Layout.SHA256 == v.Layouts[1].Layout.SHA256 {
		t.Fatal("refresh lost distinct revisions")
	}
	v = step(Decision{Action: "sample", Sample: &SampleRequest{PK: "123", Delivery: "file", Asset: "source.xlsx", XLSX: &dataset.XLSXSelection{Sheet: "districts", Range: "A1:A2"}, LayoutID: "l2"}})
	if len(v.Observations) != 1 || sampleCalls != 2 || v.SampleAttempts[0].Status != "failed" || v.SampleAttempts[1].Status != "acquired" || v.Status != "exploring" {
		t.Fatal("refreshed layout did not recover acquisition without promotion")
	}
	for n := 2; n <= 6; n++ {
		v = step(Decision{Action: "layout", Layout: &LayoutRequest{PK: "123", Asset: "source.xlsx", RefreshOf: fmt.Sprintf("l%d", n)}})
	}
	if layoutCalls != 6 || len(v.Layouts) != 6 || v.Budget.LayoutsRemaining != 0 || !strings.Contains(v.Gaps[len(v.Gaps)-1].Detail, "budget") {
		t.Fatal("refresh bypassed layout acquisition budget")
	}
}
