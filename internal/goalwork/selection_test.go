package goalwork

import (
	"context"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

func TestEngineRejectsIgnoredAndCredentialSelectionsBeforeAcquisition(t *testing.T) {
	calls := 0
	e, _ := Start("test", Policy{}, Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
		},
		Inspect: func(context.Context, string) (Inspection, error) { return Inspection{PK: "123"}, nil },
		Sample: func(context.Context, SampleRequest, Inspection) (Acquired, error) {
			calls++
			return Acquired{Rows: []Row{{"region": "target"}}, Delivery: "FILE"}, nil
		},
	})
	for _, d := range []Decision{
		{Action: "define", Contract: &GoalContract{Outcome: "test", Region: "target", Period: "test", Coverage: "sample", Roles: []RoleRequirement{{ID: "role", Description: "test"}}, Outputs: []OutputRequirement{{ID: "region", Role: "role", Description: "test", Type: "string"}}}},
		{Action: "search", Query: "test", Role: "role"}, {Action: "inspect", PK: "123"},
	} {
		if v, err := e.Advance(context.Background(), e.View().Revision, d); err != nil || len(v.Gaps) != 0 {
			t.Fatalf("invalid setup: %+v %v", v.Gaps, err)
		}
	}
	for _, request := range []SampleRequest{
		{Delivery: "api", Where: map[string]string{"region": "target"}},
		{Delivery: "standard", Where: map[string]string{"region": "target"}},
		{Delivery: "file", Where: map[string]string{"serviceKey": "do-not-use"}},
		{Delivery: "file", Where: map[string]string{"region": ""}},
		{Delivery: "file", Params: map[string]string{"region": "target"}},
		{Delivery: "file", Operation: "invented"},
		{Delivery: "api", ScanCSV: true},
		{Delivery: "standard", ScanCSV: true},
		{Delivery: "file", Member: "data.csv", ScanCSV: true},
		{Delivery: "file", XLSX: &dataset.XLSXSelection{Sheet: "s", Range: "A1:B2"}, ScanCSV: true},
		{Delivery: "api", XLSX: &dataset.XLSXSelection{Sheet: "data", Range: "A1:B2"}},
		{Delivery: "standard", XLSX: &dataset.XLSXSelection{Sheet: "data", Range: "A1:B2"}},
		{Delivery: "file", XLSX: &dataset.XLSXSelection{Sheet: "data", Range: "A1:B2"}, Where: map[string]string{"A": "target"}},
		{Delivery: "file", XLSX: &dataset.XLSXSelection{Sheet: "data", Range: "A1:XFD1048576"}},
	} {
		request.PK, request.Asset = "123", "source.csv"
		v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &request})
		if err != nil || len(v.Gaps) == 0 {
			t.Fatalf("request not rejected as acquisition gap: %+v %v", request, err)
		}
	}
	if calls != 0 || len(e.View().Observations) != 0 {
		t.Fatal("invalid selection reached adapter")
	}
	_, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &SampleRequest{PK: "123", Delivery: "file", Asset: "source.csv", Where: map[string]string{"region": "target"}}})
	if err != nil || calls != 1 || len(e.View().Observations) != 1 {
		t.Fatal("valid selection baseline failed")
	}
}
