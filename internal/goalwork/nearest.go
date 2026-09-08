package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

const sphericalRadiusMeters = 6371008.8

// A proposal names observed columns and revisions, not coordinates or evidence.
type NearestSelection struct {
	Method          string `json:"method"`
	Anchor          string `json:"anchor"`
	Candidate       string `json:"candidate"`
	AnchorLatitude  string `json:"anchorLatitude"`
	AnchorLongitude string `json:"anchorLongitude"`
	Latitude        string `json:"latitude"`
	Longitude       string `json:"longitude"`
	K               int    `json:"k"`
}

// Pair positions are source lineage, not exposed row values. Distances and
// source values remain in private rows until an artifact is explicitly emitted.
type SpatialPair struct {
	AnchorRow           int `json:"anchorRow"`           // 1-based position in retained anchor observation
	CandidateDataRecord int `json:"candidateDataRecord"` // 1-based after CSV header
	CandidateStartLine  int `json:"candidateStartLine"`
	Rank                int `json:"rank"`
}
type SpatialProvenance struct {
	Method                 string                `json:"method"`
	AnchorObservation      string                `json:"anchorObservation"`
	CandidateObservation   string                `json:"candidateObservation"`
	AnchorRowsSHA256       string                `json:"anchorRowsSha256"`
	AnchorContentSHA256    string                `json:"anchorContentSha256"`
	AnchorRequestSHA256    string                `json:"anchorRequestSha256"`
	CandidateRequestSHA256 string                `json:"candidateRequestSha256"`
	RadiusMeters           float64               `json:"radiusMeters"`
	MeaningVerified        bool                  `json:"meaningVerified"`
	Comparisons            int                   `json:"comparisons"`
	Scan                   dataset.CSVScanReport `json:"scan"`
	Pairs                  []SpatialPair         `json:"pairs"`
}

func validateNearestSelection(s NearestSelection) error {
	if s.Method != "spherical_nearest_records_v1" || s.K < 1 || s.K > 10 || s.Anchor == s.Candidate || s.AnchorLatitude == s.AnchorLongitude || s.Latitude == s.Longitude {
		return fmt.Errorf("nearest needs spherical_nearest_records_v1, distinct anchor/candidate observations and coordinate fields, and k=1–10")
	}
	for _, f := range []string{s.Anchor, s.Candidate, s.AnchorLatitude, s.AnchorLongitude, s.Latitude, s.Longitude} {
		if f == "" || len(f) > 256 || sensitiveParameter(f) {
			return fmt.Errorf("nearest requires bounded observed coordinate fields and observation IDs")
		}
	}
	return nil
}

var coordinateDecimal = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)

func coordinate(v any, bound float64) (float64, error) {
	var text string
	switch v := v.(type) {
	case string:
		text = v
	case json.Number:
		text = string(v)
	case float64:
		text = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return 0, fmt.Errorf("coordinate missing or not a decimal degree value")
	}
	if len(text) > 64 || !coordinateDecimal.MatchString(text) {
		return 0, fmt.Errorf("coordinate must be an ungrouped decimal degree value; no guessing, trimming or axis swap")
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < -bound || n > bound {
		return 0, fmt.Errorf("coordinate outside valid degree range")
	}
	return n * math.Pi / 180, nil
}

func sphericalDistance(lat1, lon1, lat2, lon2 float64) float64 {
	a := math.Pow(math.Sin((lat2-lat1)/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin((lon2-lon1)/2), 2)
	a = math.Max(0, math.Min(1, a))
	return sphericalRadiusMeters * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

type rankedRecord struct {
	record   dataset.CSVScanRecord
	distance float64
	bytes    int
}

// Every matched candidate is compared. Only k records per retained anchor are
// stored; invalid coordinates, tail failures, source drift or budgets discard all
// interim results. Ordering is distance, then original data-record ordinal.
func (e *Engine) sampleNearest(ctx context.Context, request SampleRequest, inspected Inspection) (Acquired, error) {
	s := request.Nearest
	var anchor, candidate *Observation
	for i := range e.state.Observations {
		o := &e.state.Observations[i]
		if o.ID == s.Anchor {
			anchor = o
		}
		if o.ID == s.Candidate {
			candidate = o
		}
	}
	if anchor == nil || candidate == nil || anchor.Document != nil || candidate.Document != nil || anchor.Spatial != nil || candidate.Spatial != nil || anchor.Reduction != nil || candidate.Reduction != nil {
		return Acquired{}, fmt.Errorf("nearest needs two retained original observations; nested spatial/group reduction is unsupported")
	}
	prior := e.requests[candidate.ID]
	if prior.PK != request.PK || prior.Asset != request.Asset || prior.Delivery != "file" || prior.Member != "" || prior.XLSX != nil || candidate.CSV == nil || len(candidate.ContentSHA256) != 64 {
		return Acquired{}, fmt.Errorf("nearest candidate must pin an observed direct CSV of this exact PK/asset")
	}
	anchors := e.rows[anchor.ID]
	if len(anchors) == 0 || len(anchors) > 100 || len(anchors)*s.K > 1000 {
		return Acquired{}, fmt.Errorf("nearest supports 1–100 retained anchor rows and at most 1000 output pairs; narrow anchors explicitly")
	}
	if err := requireFields(anchors, []string{s.AnchorLatitude, s.AnchorLongitude}); err != nil {
		return Acquired{}, err
	}
	if err := requireFields(e.rows[candidate.ID], []string{s.Latitude, s.Longitude}); err != nil {
		return Acquired{}, err
	}
	points := make([][2]float64, len(anchors))
	for i, row := range anchors {
		lat, err := coordinate(row[s.AnchorLatitude], 90)
		if err != nil {
			return Acquired{}, fmt.Errorf("anchor row %d latitude: %w", i+1, err)
		}
		lon, err := coordinate(row[s.AnchorLongitude], 180)
		if err != nil {
			return Acquired{}, fmt.Errorf("anchor row %d longitude: %w", i+1, err)
		}
		points[i] = [2]float64{lat, lon}
	}
	if e.deps.ScanCSV == nil {
		return Acquired{}, fmt.Errorf("complete CSV scan unavailable")
	}
	best := make([][]rankedRecord, len(anchors))
	comparisons, visited, storedBytes, lastRecord := 0, 0, 0, 0
	report, contractHash, err := e.deps.ScanCSV(ctx, cloneSampleRequest(request), inspected, func(record dataset.CSVScanRecord) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if record.DataRecord <= lastRecord || record.StartLine < record.DataRecord+1 {
			return fmt.Errorf("scan returned invalid source record positions")
		}
		lastRecord = record.DataRecord
		visited++
		lat, err := coordinate(record.Values[s.Latitude], 90)
		if err != nil {
			return fmt.Errorf("candidate data record %d latitude: %w", record.DataRecord, err)
		}
		lon, err := coordinate(record.Values[s.Longitude], 180)
		if err != nil {
			return fmt.Errorf("candidate data record %d longitude: %w", record.DataRecord, err)
		}
		for i, point := range points {
			comparisons++
			if comparisons > 10_000_000 {
				return fmt.Errorf("nearest exceeds 10 million comparisons; narrow exact scope or retained anchors")
			}
			distance := sphericalDistance(point[0], point[1], lat, lon)
			top := best[i]
			if len(top) == s.K && distance >= top[len(top)-1].distance {
				continue
			} // equal distance keeps earlier ordinal
			b, err := json.Marshal(record.Values)
			if err != nil {
				return err
			}
			if len(top) == s.K {
				storedBytes -= top[len(top)-1].bytes
				top = top[:len(top)-1]
			}
			storedBytes += len(b)
			if storedBytes > 2<<20 {
				return fmt.Errorf("nearest retained candidate budget exceeds 2 MiB")
			}
			copyRecord := record
			copyRecord.Values = make(map[string]string, len(record.Values))
			for k, v := range record.Values {
				copyRecord.Values[k] = v
			}
			top = append(top, rankedRecord{record: copyRecord, distance: distance, bytes: len(b)})
			sort.Slice(top, func(a, b int) bool {
				if top[a].distance == top[b].distance {
					return top[a].record.DataRecord < top[b].record.DataRecord
				}
				return top[a].distance < top[b].distance
			})
			best[i] = top
		}
		return nil
	})
	if err != nil {
		return Acquired{}, err
	}
	if err := ctx.Err(); err != nil {
		return Acquired{}, err
	}
	if !report.Exhausted || report.MatchedRows != visited || report.ScannedRows < lastRecord || report.SHA256 != candidate.ContentSHA256 || contractHash != candidate.ContractSHA256 {
		return Acquired{}, fmt.Errorf("nearest requires complete matching-record scan of the pinned candidate source/contract; source drift or incomplete scan invalidates all results")
	}
	if visited == 0 {
		return Acquired{}, fmt.Errorf("no matching candidate records; not population absence evidence")
	}
	provenance := &SpatialProvenance{Method: s.Method, AnchorObservation: anchor.ID, CandidateObservation: candidate.ID, AnchorRowsSHA256: anchor.RowsSHA256, AnchorContentSHA256: anchor.ContentSHA256, AnchorRequestSHA256: anchor.RequestSHA256, CandidateRequestSHA256: candidate.RequestSHA256, RadiusMeters: sphericalRadiusMeters, Comparisons: comparisons, Scan: report}
	out := Acquired{Delivery: "FILE", ContentSHA256: report.SHA256, ContractSHA256: contractHash, Spatial: provenance, Warnings: []string{"Conditional spherical nearest source records, not distinct physical stops, a route length or accessible/current operation. Coordinates interpreted as decimal degrees; datum, accuracy, times and identity remain unverified. All exact-filter matching candidates scanned; only retained anchor observations are covered. Ties retain original candidate data-record order. Source bytes identified by hash, not archived."}}
	rowBytes := 2
	for i, top := range best {
		for rank, item := range top {
			row := Row{"distance_m": item.distance, "rank": rank + 1}
			for k, v := range anchors[i] {
				row["anchor."+k] = v
			}
			for k, v := range item.record.Values {
				row["candidate."+k] = v
			}
			b, err := json.Marshal(row)
			if err != nil {
				return Acquired{}, err
			}
			rowBytes += len(b) + 1
			if rowBytes > 2<<20 || len(row) > 256 {
				return Acquired{}, fmt.Errorf("nearest output exceeds 2 MiB or 256 columns")
			}
			out.Rows = append(out.Rows, row)
			provenance.Pairs = append(provenance.Pairs, SpatialPair{AnchorRow: i + 1, CandidateDataRecord: item.record.DataRecord, CandidateStartLine: item.record.StartLine, Rank: rank + 1})
		}
	}
	return out, nil
}
