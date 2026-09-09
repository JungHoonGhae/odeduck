package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

// Exercises the public engine seam: coordinates are retained observations,
// never caller-supplied points. The closest candidate is beyond a 1000-row prefix.
func nearestFixture(t *testing.T, tail string) (*Engine, SampleRequest) {
	return nearestFixtureRows(t, tail, []Row{{"id": "001", "lat": "0", "lon": "0"}})
}

func nearestFixtureRows(t *testing.T, tail string, anchors []Row, explanations ...ExplanationRequirement) (*Engine, SampleRequest) {
	t.Helper()
	deps := Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) {
			return Inspection{PK: pk, Assets: []string{"source.csv"}}, nil
		},
		Sample: func(_ context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			rows := anchors
			if s.PK == "222" {
				rows = []Row{{"id": "far", "lat": "0", "lon": "90", "year": "2024"}}
			}
			positions := &dataset.CSVProvenance{Encoding: "utf-8"}
			for i := range rows {
				positions.DataRecords = append(positions.DataRecords, i+1)
				positions.StartLines = append(positions.StartLines, i+2)
			}
			return Acquired{Delivery: "FILE", Rows: rows, ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64), CSV: positions}, nil
		},
		ScanCSV: func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
			for n := 1; n <= 1002; n++ {
				values := map[string]string{"id": "far", "lat": "0", "lon": "90", "year": "2024"}
				if n > 1000 {
					values = map[string]string{"id": fmt.Sprintf("BIS%d", n), "lat": "0", "lon": "1", "year": "2024"}
				}
				if tail != "" && n == 1002 {
					values["lat"] = tail
				}
				if err := visit(dataset.CSVScanRecord{DataRecord: n, StartLine: n + 1, Values: values}); err != nil {
					return dataset.CSVScanReport{}, "", err
				}
			}
			return dataset.CSVScanReport{Encoding: "utf-8", SHA256: strings.Repeat("a", 64), Columns: []string{"id", "lat", "lon", "year"}, Bytes: 20000, ScannedRows: 1002, MatchedRows: 1002, Exhausted: true}, strings.Repeat("b", 64), nil
		},
	}
	e, err := Start("compare locations", Policy{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	c := GoalContract{Outcome: "comparison", Region: "fixture", Period: "historic", Coverage: "sample", Roles: []RoleRequirement{{ID: "facility", Description: "facility"}, {ID: "transit", Description: "stops"}}, Outputs: []OutputRequirement{{ID: "facility", Role: "facility", Description: "facility code", Type: "string"}, {ID: "distance", Role: "transit", Description: "conditional distance", Type: "number"}}}
	c.Explanations = explanations
	for _, d := range []Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "111", Role: "facility"}, {Action: "inspect", PK: "111"}, {Action: "sample", Sample: &SampleRequest{PK: "111", Delivery: "file", Asset: "source.csv"}}, {Action: "search", Query: "222", Role: "transit"}, {Action: "inspect", PK: "222"}, {Action: "sample", Sample: &SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv", ScanCSV: true}}} {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("setup: %+v %v", v.Gaps, err)
		}
	}
	return e, SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv", ScanCSV: true, Nearest: &NearestSelection{Anchor: "o1", Candidate: "o2", AnchorLatitude: "lat", AnchorLongitude: "lon", Latitude: "lat", Longitude: "lon", K: 2, Method: "spherical_nearest_records_v1"}}
}

func TestNearestScansAllRecordsAndPreservesConditionalPairProvenance(t *testing.T) {
	e, s := nearestFixture(t, "")
	v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Gaps) != 0 || len(v.Observations) != 3 {
		t.Fatalf("nearest failed: %+v %v", v.Gaps, err)
	}
	o := v.Observations[2]
	if o.Spatial == nil || o.Spatial.MeaningVerified || o.Spatial.Scan.MatchedRows != 1002 || o.Spatial.Pairs[0].CandidateDataRecord != 1001 || o.Spatial.Pairs[1].CandidateDataRecord != 1002 || o.Spatial.AnchorRowsSHA256 != v.Observations[0].RowsSHA256 {
		t.Fatalf("bad spatial evidence: %+v", o.Spatial)
	}
	s.Nearest.K = 1
	v.Observations[2].Spatial.Pairs[0].CandidateDataRecord = 1
	if e.View().Observations[2].Spatial.Pairs[0].CandidateDataRecord != 1001 || e.View().SampleAttempts[2].Request.Nearest.K != 2 {
		t.Fatal("caller mutated spatial request/evidence")
	}
	p := Composition{ID: "near", Purpose: "conditional comparison", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.id", "o1.lat", "o1.lon"}, RightKeys: []string{"anchor.id", "anchor.lat", "anchor.lon"}}}, Select: []string{"o1.id", "o3.candidate.id", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "facility", Field: "o1.id"}, {Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"coordinates interpreted as degrees; common datum, route and current operation unverified"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "near"}} {
		v, err = e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("compose: %+v %v", v.Gaps, err)
		}
	}
	if v.Status != "review_required" || len(v.Artifact.Rows) != 2 || len(v.Artifact.Sources) != 3 {
		t.Fatal("lost transitive source revision or approved proximity")
	}
	distance, err := v.Artifact.Rows[0]["o3.distance_m"].(json.Number).Float64()
	if err != nil || math.Abs(distance-6371008.8*math.Pi/180) > 1e-7 || v.Artifact.Rows[0]["o3.candidate.id"] != "BIS1001" {
		t.Fatal("wrong full-scan nearest distance/order")
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "BIS1001") || strings.Contains(string(b), "111195.") {
		t.Fatal("raw paired values reached planner")
	}
}

func TestNearestInvalidMatchingCoordinateCannotBeSilentlyExcluded(t *testing.T) {
	for _, bad := range []string{"", "NaN", "Inf", "91", "0x1p0", "1,23", " 0", "1e2"} {
		t.Run(bad, func(t *testing.T) {
			e, s := nearestFixture(t, bad)
			// Empty string is also an actual missing coordinate, not the helper's no-tail mode.
			if bad == "" {
				old := e.deps.ScanCSV
				e.deps.ScanCSV = func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
					return old(ctx, s, i, func(r dataset.CSVScanRecord) error { r.Values["lat"] = ""; return visit(r) })
				}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Observations) != 2 || len(v.Gaps) != 1 || v.SampleAttempts[2].Status != "failed" {
				t.Fatal("invalid coordinate became a partial nearest result")
			}
		})
	}
}

func TestNearestRejectsWrongReferencesAndIncompleteOrChangedSource(t *testing.T) {
	for _, kind := range []string{"unknown_anchor", "candidate_asset", "missing_field", "same_axes", "method", "count", "unexhausted", "changed_bytes", "changed_contract", "callback_error"} {
		t.Run(kind, func(t *testing.T) {
			e, s := nearestFixture(t, "")
			switch kind {
			case "unknown_anchor":
				s.Nearest.Anchor = "o999"
			case "candidate_asset":
				s.Asset = "different.csv"
			case "missing_field":
				s.Nearest.Latitude = "invented"
			case "same_axes":
				s.Nearest.Longitude = "lat"
			case "method":
				s.Nearest.Method = "accessible_route"
			case "count":
				s.Nearest.K = 11
			default:
				old := e.deps.ScanCSV
				e.deps.ScanCSV = func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
					r, h, err := old(ctx, s, i, visit)
					switch kind {
					case "unexhausted":
						r.Exhausted = false
					case "changed_bytes":
						r.SHA256 = strings.Repeat("c", 64)
					case "changed_contract":
						h = strings.Repeat("c", 64)
					case "callback_error":
						err = fmt.Errorf("malformed excluded tail")
					}
					return r, h, err
				}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 {
				t.Fatalf("invalid nearest accepted: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestNearestRejectsExcessiveAnchorsAndRetainedCandidateMemory(t *testing.T) {
	for _, kind := range []string{"anchors", "bytes"} {
		e, s := nearestFixture(t, "")
		if kind == "anchors" {
			e.rows["o1"] = make([]Row, 101)
		} else {
			old := e.deps.ScanCSV
			e.deps.ScanCSV = func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
				return old(ctx, s, i, func(r dataset.CSVScanRecord) error {
					r.Values["large"] = strings.Repeat("x", (1<<20)+1)
					return visit(r)
				})
			}
		}
		v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s})
		if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 {
			t.Fatal("nearest exceeded memory/anchor limits")
		}
	}
}

func TestNearestCopiedSourceValuesCannotBypassRepeatedRecordSumGuard(t *testing.T) {
	e, s := nearestFixture(t, "")
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "bad-sum", Purpose: "sum", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.id"}, RightKeys: []string{"anchor.id"}}}, Measures: []Measure{{As: "n", Field: "o3.anchor.lat", Format: "decimal_v1", Unit: "declared"}}, Aggregates: []Aggregate{{As: "total", Op: "sum", Field: "n"}}, Assumptions: []string{"copied anchor is not two independent source records"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "repeats source record") || v.Artifact != nil {
		t.Fatal("paired source copies were summed as independent records")
	}
}

func TestNearestSphericalEndpointsProduceFiniteConditionalDistances(t *testing.T) {
	for _, tc := range []struct {
		lat, lon string
		radians  float64
	}{{"0", "0", 0}, {"0", "180", math.Pi}, {"0", "-180", math.Pi}, {"90", "0", math.Pi / 2}} {
		e, s := nearestFixture(t, "")
		old := e.deps.ScanCSV
		e.deps.ScanCSV = func(ctx context.Context, s SampleRequest, i Inspection, visit func(dataset.CSVScanRecord) error) (dataset.CSVScanReport, string, error) {
			return old(ctx, s, i, func(r dataset.CSVScanRecord) error {
				r.Values["lat"], r.Values["lon"] = tc.lat, tc.lon
				return visit(r)
			})
		}
		p := Composition{ID: "endpoints", Purpose: "conditional distances", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.id"}, RightKeys: []string{"anchor.id"}}}, Select: []string{"o1.id", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "facility", Field: "o1.id"}, {Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"conditional sphere, no CRS approval"}}
		for _, d := range []Decision{{Action: "sample", Sample: &s}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
			v, err := e.Advance(context.Background(), e.View().Revision, d)
			if err != nil || len(v.Gaps) != 0 {
				t.Fatalf("endpoint failed: %+v %v", v.Gaps, err)
			}
		}
		v := e.View()
		for _, row := range v.Artifact.Rows {
			n, err := row["o3.distance_m"].(json.Number).Float64()
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n-sphericalRadiusMeters*tc.radians) > 1e-6 {
				t.Fatal("invalid spherical endpoint distance")
			}
		}
	}
}
