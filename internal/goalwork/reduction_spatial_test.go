package goalwork

import (
	"context"
	"strings"
	"testing"
)

func TestSourceGroupCannotBecomeAnOriginalSpatialAnchor(t *testing.T) {
	e, s := nearestFixture(t, "")
	r := SourceReduction{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, GroupBy: []string{"id", "lat", "lon"}, Aggregates: []Aggregate{{As: "records", Op: "count"}}}
	v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &SampleRequest{PK: e.View().Observations[0].PK, Delivery: "file", Reduce: &r}})
	if err != nil || len(v.Gaps) != 0 {
		t.Fatalf("reduce setup: %v %+v", err, v.Gaps)
	}
	s.Nearest.Anchor = "o3"
	v, err = e.Advance(context.Background(), v.Revision, Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Observations) != 3 || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "original") {
		t.Fatalf("group mistaken for original point: %v %+v", err, v.Gaps)
	}
}
