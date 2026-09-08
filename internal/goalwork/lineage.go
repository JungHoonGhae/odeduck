package goalwork

import (
	"fmt"
	"strings"
)

// Record addresses are scoped to an immutable observation, not an inferred
// canonical entity. CSV addresses refer to original data records; row addresses
// refer to retained API/table rows or computed pair rows.
type recordSlot struct {
	observation string
	kind        string
}
type rowLineage map[recordSlot]int

func originalSlot(id string, observed map[string]Observation) recordSlot {
	kind := "row"
	if o := observed[id]; o.CSV != nil && len(o.CSV.DataRecords) > 0 {
		kind = "csv"
	}
	return recordSlot{observation: id, kind: kind}
}

func fieldSlot(field string, observed map[string]Observation) recordSlot {
	id, name, qualified := strings.Cut(field, ".")
	if !qualified || id == "" || name == "" {
		return recordSlot{}
	}
	if spatial := observed[id].Spatial; spatial != nil {
		if strings.HasPrefix(name, "anchor.") {
			return originalSlot(spatial.AnchorObservation, observed)
		}
		if strings.HasPrefix(name, "candidate.") {
			return recordSlot{observation: spatial.CandidateObservation, kind: "csv"}
		}
	}
	return originalSlot(id, observed)
}

// Direct and transitive inputs are evidence dependencies, not added joins or
// canonical identity merges. Used consistently for artifact and role attribution.
func compositionSources(p Composition, observed map[string]Observation) map[string]bool {
	used := map[string]bool{}
	var add func(string)
	add = func(id string) {
		if used[id] {
			return
		}
		used[id] = true
		if spatial := observed[id].Spatial; spatial != nil {
			add(spatial.AnchorObservation)
			add(spatial.CandidateObservation)
		}
	}
	add(p.Base)
	for _, j := range p.Joins {
		add(j.Right)
	}
	return used
}

func prepareLineage(p Composition, inputs map[string][]Row, observed map[string]Observation) (map[string][]rowLineage, error) {
	used := compositionSources(p, observed)
	out := map[string][]rowLineage{}
	// Build original addresses first so pair origins can inherit both retained
	// row and physical CSV addresses without treating a prefix index as a CSV ID.
	for id := range used {
		rows, ok := inputs[id]
		if !ok || len(rows) == 0 {
			return nil, fmt.Errorf("source record lineage requires retained input %s", id)
		}
		o := observed[id]
		if o.RowsSHA256 != "" && o.RowsSHA256 != digest(rows) {
			return nil, fmt.Errorf("source record lineage row revision mismatch for %s", id)
		}
		if o.Spatial != nil {
			continue
		}
		if o.CSV != nil && len(o.CSV.DataRecords) > 0 && len(o.CSV.DataRecords) != len(rows) {
			return nil, fmt.Errorf("CSV source record lineage length mismatch")
		}
		for i := range rows {
			trace := rowLineage{{observation: id, kind: "row"}: i}
			if o.CSV != nil && len(o.CSV.DataRecords) > 0 {
				if o.CSV.DataRecords[i] < 1 {
					return nil, fmt.Errorf("CSV source record lineage requires positive data record positions")
				}
				trace[recordSlot{observation: id, kind: "csv"}] = o.CSV.DataRecords[i]
			}
			out[id] = append(out[id], trace)
		}
	}
	for id := range used {
		o := observed[id]
		s := o.Spatial
		if s == nil {
			continue
		}
		a, aOK := observed[s.AnchorObservation]
		c, cOK := observed[s.CandidateObservation]
		if !aOK || !cOK || a.Spatial != nil || c.Spatial != nil || s.AnchorObservation == s.CandidateObservation || len(s.Pairs) != len(inputs[id]) || c.CSV == nil || len(c.CSV.DataRecords) == 0 {
			return nil, fmt.Errorf("spatial source record lineage needs original anchor/candidate observations and one pair address per row")
		}
		if s.AnchorRowsSHA256 != a.RowsSHA256 || s.AnchorContentSHA256 != a.ContentSHA256 || s.AnchorRequestSHA256 != a.RequestSHA256 || s.CandidateRequestSHA256 != c.RequestSHA256 || s.Scan.SHA256 != c.ContentSHA256 || o.ContentSHA256 != c.ContentSHA256 || !s.Scan.Exhausted {
			return nil, fmt.Errorf("spatial source record lineage revision mismatch")
		}
		for i, pair := range s.Pairs {
			if pair.AnchorRow < 1 || pair.AnchorRow > len(out[a.ID]) || pair.CandidateDataRecord < 1 || pair.CandidateDataRecord > s.Scan.ScannedRows {
				return nil, fmt.Errorf("spatial source record lineage position outside observed range")
			}
			trace := rowLineage{{observation: id, kind: "row"}: i, {observation: c.ID, kind: "csv"}: pair.CandidateDataRecord}
			for slot, ordinal := range out[a.ID][pair.AnchorRow-1] {
				trace[slot] = ordinal
			}
			out[id] = append(out[id], trace)
		}
	}
	return out, nil
}

func compatibleLineage(left, right rowLineage) bool {
	for slot, ordinal := range right {
		if prior, exists := left[slot]; exists && prior != ordinal {
			return false
		}
	}
	return true
}

func mergeLineage(left, right rowLineage) rowLineage {
	out := make(rowLineage, len(left)+len(right))
	for slot, ordinal := range left {
		out[slot] = ordinal
	}
	for slot, ordinal := range right {
		out[slot] = ordinal
	}
	return out
}

func validateMeasureLineage(specs []Measure, observed map[string]Observation) error {
	for _, m := range specs {
		fields, err := measureFields(m)
		if err != nil {
			return err
		}
		origin := fieldSlot(fields[0], observed)
		for _, field := range fields[1:] {
			if fieldSlot(field, observed) != origin {
				return fmt.Errorf("measure fields must come from the same original record; copied anchor/candidate/computed values have distinct origins")
			}
		}
	}
	return nil
}

func outputMatchesRole(field, bound string, observed map[string]Observation) bool {
	origin := fieldSlot(field, observed).observation
	if origin == bound {
		return true
	}
	// A nearest-derived container represents the candidate role. Its copied
	// candidate fields can fulfill that role; copied anchor fields cannot.
	if s := observed[bound].Spatial; s != nil {
		return origin == s.CandidateObservation
	}
	return false
}
