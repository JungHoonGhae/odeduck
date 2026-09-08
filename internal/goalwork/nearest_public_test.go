package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

// Seeded source/calculation replay. Selection of the four anchor records and
// source URLs comes from the independent reference, not autonomous discovery.
func TestLiveNearestMatchesIndependentMobilityDistances(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_NEAREST") != "1" {
		t.Skip("opt-in public whole-source nearest replay")
	}
	b, err := os.ReadFile("testdata/goalbench-v1/mobility-comparative-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Sources []struct {
			PK        string            `json:"pk"`
			URL       string            `json:"url"`
			SHA       string            `json:"contentSha256"`
			Selection map[string]string `json:"selection"`
			Records   []struct {
				Member  string            `json:"member"`
				Ordinal int               `json:"csvRecord"`
				Values  map[string]string `json:"values"`
			} `json:"records"`
		} `json:"sources"`
		Expected struct {
			Rows []struct {
				FacilityOrdinal int `json:"facilityCSVRecord"`
				Stops           []struct {
					Ordinal  int     `json:"stopCSVRecord"`
					ID       string  `json:"stopID"`
					Manager  string  `json:"manager"`
					Distance float64 `json:"sphericalDistanceMeters"`
				} `json:"nearbyTransitRecords"`
			} `json:"rows"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(b, &ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := fetch.New(fetch.WithDelay(0))
	files := dataset.NewInspector(client, "")
	anchorSource, candidateSource := ref.Sources[0], ref.Sources[1]
	anchorAsset := dataset.Asset{Name: "facilities.zip", Format: "ZIP", Request: dataset.Request{Method: http.MethodGet, URL: anchorSource.URL}}
	candidateAsset := dataset.Asset{Name: "stops.csv", Format: "CSV", Request: dataset.Request{Method: http.MethodGet, URL: candidateSource.URL}}
	member := anchorSource.Records[0].Member
	anchorSample, err := files.SampleZIPCSV(ctx, anchorAsset, member, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if anchorSample.SHA256 != anchorSource.SHA {
		t.Fatal("anchor source changed; preserve oracle")
	}
	anchors := Acquired{Delivery: "FILE", ContentSHA256: anchorSample.SHA256, ContractSHA256: digest(anchorAsset), Archive: anchorSample.Archive, CSV: &dataset.CSVProvenance{Encoding: anchorSample.CSV.Encoding}}
	for _, expected := range ref.Expected.Rows {
		found := false
		for i, ordinal := range anchorSample.CSV.DataRecords {
			if ordinal+1 != expected.FacilityOrdinal {
				continue
			}
			anchors.Rows = append(anchors.Rows, anchorSample.Rows[i])
			anchors.CSV.DataRecords = append(anchors.CSV.DataRecords, ordinal)
			anchors.CSV.StartLines = append(anchors.CSV.StartLines, anchorSample.CSV.StartLines[i])
			found = true
		}
		if !found {
			t.Fatalf("frozen facility record %d unavailable", expected.FacilityOrdinal)
		}
	}
	if len(anchors.Rows) != 4 {
		t.Fatal("reference anchors incomplete")
	}
	contract := &dataset.Contract{Ref: dataset.Ref{PK: candidateSource.PK, Delivery: "FILE"}, Assets: []dataset.Asset{candidateAsset}}
	live := LiveDependencies(client, "", nil, catalog.Searcher{}, Policy{})
	deps := Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (Inspection, error) {
			return Inspection{PK: pk, Assets: []string{"stops.csv", "facilities.zip"}, handle: &dataset.InspectionResult{File: contract}}, nil
		},
		Sample: func(ctx context.Context, s SampleRequest, _ Inspection) (Acquired, error) {
			if s.PK == anchorSource.PK {
				return anchors, nil
			}
			sample, err := files.SampleCSVScanned(ctx, candidateAsset, 1000, candidateSource.Selection)
			if err != nil {
				return Acquired{}, err
			}
			if sample.SHA256 != candidateSource.SHA {
				return Acquired{}, fmt.Errorf("candidate source changed; preserve oracle")
			}
			return Acquired{Delivery: "FILE", Rows: sample.Rows, ContentSHA256: sample.SHA256, ContractSHA256: digest(contract), CSV: sample.CSV, Selection: sample.Selection}, nil
		},
		ScanCSV: live.ScanCSV,
	}
	e, err := Start("frozen four-facility conditional nearest comparison", Policy{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	c := GoalContract{Outcome: "comparison", Region: "Incheon reference facilities", Period: "2022/2025 snapshots", Coverage: "sample", Roles: []RoleRequirement{{ID: "facility", Description: "facility"}, {ID: "transit", Description: "stops"}}, Outputs: []OutputRequirement{{ID: "distance", Role: "transit", Description: "conditional spherical distance", Type: "number"}}}
	s := SampleRequest{PK: candidateSource.PK, Delivery: "file", Asset: "stops.csv", ScanCSV: true, Where: candidateSource.Selection, Nearest: &NearestSelection{Method: "spherical_nearest_records_v1", Anchor: "o1", Candidate: "o2", AnchorLatitude: "위도", AnchorLongitude: "경도", Latitude: "위도", Longitude: "경도", K: 3}}
	p := Composition{ID: "comparison", Purpose: "conditional spherical distances", Base: "o3", Select: []string{"o3.anchor.업체명", "o3.candidate.정류장번호", "o3.candidate.관리도시명", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"conditional decimal-degree spherical calculation, shared datum/accuracy/current operation/route unknown"}}
	steps := []Decision{{Action: "define", Contract: &c}, {Action: "search", Query: anchorSource.PK, Role: "facility"}, {Action: "inspect", PK: anchorSource.PK}, {Action: "sample", Sample: &SampleRequest{PK: anchorSource.PK, Delivery: "file", Asset: "facilities.zip", Member: member}}, {Action: "search", Query: candidateSource.PK, Role: "transit"}, {Action: "inspect", PK: candidateSource.PK}, {Action: "sample", Sample: &SampleRequest{PK: candidateSource.PK, Delivery: "file", Asset: "stops.csv", ScanCSV: true, Where: candidateSource.Selection}}, {Action: "sample", Sample: &s}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}}
	var v View
	for _, d := range steps {
		v, err = e.Advance(ctx, e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("live replay failed: %+v %v", v.Gaps, err)
		}
	}
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 12 {
		t.Fatal("wrong result size or unsupported semantic approval")
	}
	spatial := v.Observations[2].Spatial
	if spatial == nil || spatial.Scan.MatchedRows != 9278 || spatial.Scan.ScannedRows != 227065 || spatial.Comparisons != 4*9278 || spatial.MeaningVerified {
		t.Fatal("nearest used a prefix or asserted geographic validity")
	}
	for a, facility := range ref.Expected.Rows {
		for rank, expected := range facility.Stops {
			i := a*3 + rank
			row := v.Artifact.Rows[i]
			distance, err := row["o3.distance_m"].(json.Number).Float64()
			if err != nil || math.Abs(distance-expected.Distance) > 1e-6 || row["o3.candidate.정류장번호"] != expected.ID || row["o3.candidate.관리도시명"] != expected.Manager || spatial.Pairs[i].CandidateDataRecord+1 != expected.Ordinal {
				t.Fatalf("reference pair %d differs: %+v %+v", i, row, spatial.Pairs[i])
			}
		}
	}
	t.Logf("12 independent nearest records/distances matched over %d candidates and %d comparisons; 4 original ZIP anchor records; status=%s", spatial.Scan.MatchedRows, spatial.Comparisons, v.Status)
}
