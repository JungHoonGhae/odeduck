package goalwork

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

const FullScopeReviewMethod = "independent_model_full_scope_analysis_v3"

// FullScopeContext records acquisition eligibility, never semantic completeness.
// Counts, selectors and original positions remain in the referenced observations.
type FullScopeContext struct {
	Method   string         `json:"method"`
	Eligible bool           `json:"eligible"`
	Sources  []SourceExtent `json:"sources"`
}

type SourceExtent struct {
	Observation string `json:"observation"`
	RowsSHA256  string `json:"rowsSha256"`
	Kind        string `json:"kind"`     // complete_csv_selection, xlsx_rectangle, or unproven
	Complete    bool   `json:"complete"` // this acquisition selection, not the requested population
}

type SourceCoverageReview struct {
	Observation string        `json:"observation"`
	Finding     ReviewFinding `json:"finding"`
}

func fullScopeExtents(sources []Observation, used map[string]bool) *FullScopeContext {
	c := &FullScopeContext{Method: "bounded_source_extent_v1", Eligible: true}
	for _, source := range sources {
		if !used[source.ID] || source.Reduction != nil {
			continue
		}
		x := SourceExtent{Observation: source.ID, RowsSHA256: source.RowsSHA256, Kind: "unproven"}
		valid := source.Delivery == "FILE" && source.Spatial == nil && source.RowCount > 0 && source.Asset != ""
		for _, hash := range []string{source.ContentSHA256, source.ContractSHA256} {
			b, err := hex.DecodeString(hash)
			valid = valid && err == nil && len(b) == 32
		}
		if s := source.Selection; valid && s != nil && source.CSV != nil && source.Table == nil {
			x.Complete = slices.Contains([]string{"exact_strings_v1", "exact_strings_full_scan_v1", "exact_string_sets_full_scan_v1"}, s.Mode) && s.Exhausted && s.ScannedRows >= s.MatchedRows && s.MatchedRows == s.ReturnedRows && s.ReturnedRows == source.RowCount && len(source.CSV.DataRecords) == source.RowCount && len(source.CSV.StartLines) == source.RowCount
			last := 0
			for i, record := range source.CSV.DataRecords {
				x.Complete = x.Complete && record > last && record <= s.ScannedRows && i < len(source.CSV.StartLines) && source.CSV.StartLines[i] > 0
				last = record
			}
			if x.Complete {
				x.Kind = "complete_csv_selection"
			}
		} else if table := source.Table; valid && table != nil && source.CSV == nil {
			x.Complete = completeRectangle(table, source.RowCount)
			if x.Complete {
				x.Kind = "xlsx_rectangle"
			}
		}
		c.Sources = append(c.Sources, x)
		c.Eligible = c.Eligible && x.Complete
	}
	c.Eligible = c.Eligible && len(c.Sources) > 0
	return c
}

func completeRectangle(table *dataset.TableProvenance, count int) bool {
	if table.Member == "" || len(table.RowNumbers) != count || dataset.ValidateXLSXSelection(dataset.XLSXSelection{Sheet: table.Sheet, Range: table.Range}) != nil {
		return false
	}
	ends := strings.Split(table.Range, ":")
	first, _ := strconv.Atoi(strings.TrimLeft(ends[0], "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	last, _ := strconv.Atoi(strings.TrimLeft(ends[len(ends)-1], "ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	if last-first+1 != count {
		return false
	}
	for i, row := range table.RowNumbers {
		if row != first+i {
			return false
		}
	}
	return true
}

func validateSourceCoverage(a ReviewAssessment, in ReviewInput) error {
	if in.Analysis == nil || in.Analysis.FullScope == nil {
		if len(a.SourceCoverage) != 0 {
			return fmt.Errorf("source coverage findings require full-scope review authority")
		}
		return nil
	}
	c := in.Analysis.FullScope
	if in.Contract.Coverage != "population" || c.Method != "bounded_source_extent_v1" || !c.Eligible || len(c.Sources) == 0 || len(a.SourceCoverage) != len(c.Sources) {
		return fmt.Errorf("full-scope review requires every eligible original source")
	}
	seen := map[string]bool{}
	for _, review := range a.SourceCoverage {
		if seen[review.Observation] || !slices.ContainsFunc(c.Sources, func(s SourceExtent) bool { return s.Observation == review.Observation && s.Complete }) {
			return fmt.Errorf("source coverage needs distinct actual original observations")
		}
		seen[review.Observation] = true
		f := review.Finding
		if !validReviewFinding(f, in) {
			return fmt.Errorf("source coverage requires bounded findings with actual citations")
		}
		grounded := false
		for _, packet := range in.EvidencePackets() {
			grounded = grounded || packet.Selection.Observation == review.Observation && slices.Contains(f.PacketIDs, packet.ID)
		}
		for _, context := range in.Analysis.SourceContext {
			grounded = grounded || slices.Contains(context.Targets, review.Observation) && slices.Contains(f.PacketIDs, context.PacketID)
		}
		for _, source := range in.Analysis.Sources {
			if source.Observation == review.Observation {
				for _, use := range source.Disclosure {
					grounded = grounded || slices.Contains(f.PacketIDs, use.PacketID)
				}
			}
		}
		if !grounded {
			return fmt.Errorf("source coverage must cite that original observation's disclosed cells or its targeted context")
		}
	}
	return nil
}
