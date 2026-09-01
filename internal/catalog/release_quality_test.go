package catalog

import "testing"

func TestReleaseGoldenSetPassesRepresentativeCatalogue(t *testing.T) {
	golden, err := DefaultReleaseGoldenSet()
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]Entry, 0, len(golden.Cases))
	for _, item := range golden.Cases {
		entries = append(entries, Entry{PK: item.ExpectedPK, Title: item.Query, SvcType: item.ExpectedSvcType})
	}
	report := ValidateReleaseQuality(&Catalog{Entries: entries}, golden)
	if !report.Passed || len(report.Cases) != len(golden.Cases) {
		t.Fatalf("report = %+v", report)
	}
	for _, result := range report.Cases {
		if !result.Passed || result.Rank != 1 {
			t.Fatalf("case = %+v", result)
		}
	}
}

func TestReleaseGoldenSetFailsWhenExpectedDatasetDisappears(t *testing.T) {
	golden := ReleaseGoldenSet{Version: 1, ReviewedAt: "2026-09-01", Cases: []ReleaseGoldenCase{{
		ID: "missing", Query: "온비드 공매 물건", ExpectedPK: "15157207", ExpectedSvcType: SvcREST, MaxRank: 5,
	}}}
	report := ValidateReleaseQuality(&Catalog{Entries: []Entry{{PK: "other", Title: "온비드 공매 물건", SvcType: SvcREST}}}, golden)
	if report.Passed || len(report.Cases) != 1 || report.Cases[0].Failure == "" {
		t.Fatalf("report = %+v", report)
	}
}
