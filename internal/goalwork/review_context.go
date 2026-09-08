package goalwork

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// SupportBinding proposes where an already-disclosed packet may help interpret
// a result. The engine resolves the packet; callers cannot supply its contents.
type SupportBinding struct {
	PacketID string   `json:"packetId"`
	Targets  []string `json:"targets"`
	Purpose  string   `json:"purpose"`
}

// SourceContext references already-disclosed records from another observation of
// the same file revision, or explicitly proposed supporting records. Neither
// association proves applicability, identity, or computational participation.
type SourceContext struct {
	ComparisonSources []ComparisonContextSource `json:"comparisonSources,omitempty"`
	Targets           []string                  `json:"targets"`
	Source            Observation               `json:"source"`
	Request           SampleRequest             `json:"request"`
	PacketID          string                    `json:"packetId"`
	Proposed          bool                      `json:"proposed,omitempty"`
	Purpose           string                    `json:"purpose,omitempty"`
}

// Computed support names both original revisions and acquisition conditions.
// These metadata contain no automatically disclosed original row values.
type ComparisonContextSource struct {
	Source  Observation   `json:"source"`
	Request SampleRequest `json:"request"`
}

func (e *Engine) supportContext(p Composition) (map[string]SourceContext, error) {
	out := map[string]SourceContext{}
	if len(p.Support) == 0 {
		return out, nil
	}
	if !e.state.Policy.ReviewAnalyses || e.state.Policy.EvidenceRecipient == "" || len(p.Support) > maxEvidencePackets {
		return nil, fmt.Errorf("support requires analysis review, enabled disclosure and at most 8 packets")
	}
	observed := map[string]Observation{}
	for _, source := range e.state.Observations {
		observed[source.ID] = source
	}
	used := compositionSources(p, observed)
	packets := map[string]EvidencePacket{}
	for _, packet := range e.state.Evidence {
		packets[packet.ID] = packet
	}
	for _, binding := range p.Support {
		packet, exists := packets[binding.PacketID]
		if _, duplicate := out[binding.PacketID]; !exists || duplicate {
			return nil, fmt.Errorf("support requires distinct packets already disclosed in this goal")
		}
		source, exists := observed[packet.Selection.Observation]
		if !exists || source.RowsSHA256 != packet.Selection.RowsSHA256 || used[source.ID] || source.Reduction != nil || source.Spatial != nil {
			return nil, fmt.Errorf("support must reference a nonparticipating original or support-only comparison revision")
		}
		if !utf8.ValidString(binding.Purpose) || strings.TrimSpace(binding.Purpose) == "" || len(binding.Purpose) > 1000 || credentialText(binding.Purpose) {
			return nil, fmt.Errorf("support purpose needs 1–1000 UTF-8 bytes without credential material; it is a proposal, not evidence")
		}
		if len(binding.Targets) == 0 || len(binding.Targets) > 8 {
			return nil, fmt.Errorf("support requires 1–8 participating target observations")
		}
		seen := map[string]bool{}
		for _, id := range binding.Targets {
			if _, exists := observed[id]; !exists || !used[id] || seen[id] {
				return nil, fmt.Errorf("support targets must be distinct participating observations or their original lineage")
			}
			seen[id] = true
		}
		c := SourceContext{Targets: binding.Targets, Source: projectReviewSource(source, packet.Selection.Fields), Request: e.requests[source.ID], PacketID: packet.ID, Proposed: true, Purpose: binding.Purpose}
		if source.Comparison != nil {
			for side, selection := range []ComparisonSide{source.Comparison.Recipe.Left, source.Comparison.Recipe.Right} {
				parent, exists := observed[selection.Observation]
				if !exists || parent.RowsSHA256 != selection.RowsSHA256 {
					return nil, fmt.Errorf("comparison support lost its original source revision")
				}
				fields := map[string]bool{}
				for _, key := range selection.Keys {
					fields[key.Field] = true
				}
				for _, check := range source.Comparison.Recipe.Checks {
					operand := check.Left
					if side == 1 {
						operand = check.Right
					}
					if operand.Field != "" {
						fields[operand.Field] = true
					}
					for _, field := range operand.Fields {
						fields[field] = true
					}
				}
				var names []string
				for field := range fields {
					names = append(names, field)
				}
				slices.Sort(names)
				c.ComparisonSources = append(c.ComparisonSources, ComparisonContextSource{Source: projectReviewSource(parent, names), Request: e.requests[parent.ID]})
			}
		}
		out[binding.PacketID] = c
	}
	if err := completeComparisonSupport(out, packets); err != nil {
		return nil, err
	}
	return out, nil
}

// Every proposed target must see the complete comparison denominator and
// negative accounting, even if the disclosure was split across packets.
func completeComparisonSupport(contexts map[string]SourceContext, packets map[string]EvidencePacket) error {
	coverage := map[string]map[int]map[string]bool{}
	for _, c := range contexts {
		if c.Source.Comparison == nil {
			continue
		}
		for _, target := range c.Targets {
			key := c.Source.ID + "\x00" + target
			if coverage[key] == nil {
				coverage[key] = map[int]map[string]bool{}
			}
			for _, record := range packets[c.PacketID].Records {
				if record.RetainedRow < 1 || record.RetainedRow > len(comparisonSummaryMetrics) {
					continue
				}
				if coverage[key][record.RetainedRow] == nil {
					coverage[key][record.RetainedRow] = map[string]bool{}
				}
				for _, field := range []string{"metric", "value"} {
					if _, exists := record.Values[field]; exists {
						coverage[key][record.RetainedRow][field] = true
					}
				}
			}
		}
	}
	for _, rows := range coverage {
		for ordinal := 1; ordinal <= len(comparisonSummaryMetrics); ordinal++ {
			if !rows[ordinal]["metric"] || !rows[ordinal]["value"] {
				return fmt.Errorf("comparison support requires all 13 summary metrics/values for each target; disclose denominators, unresolved records and numeric discrepancies")
			}
		}
	}
	return nil
}

func (e *Engine) reviewSourceContext(ctx context.Context, a *Artifact) ([]SourceContext, error) {
	if a == nil {
		return nil, nil
	}
	explicit, err := e.supportContext(a.Recipe)
	if err != nil {
		return nil, err
	}
	participating := map[string]bool{}
	for _, source := range a.Sources {
		participating[source.ID] = true
	}
	observed := map[string]Observation{}
	for _, source := range e.state.Observations {
		observed[source.ID] = source
	}
	var context []SourceContext
	replayed := map[string]bool{}
	for _, packet := range e.state.Evidence {
		if proposed, exists := explicit[packet.ID]; exists {
			if o := proposed.Source; o.Comparison != nil && !replayed[o.ID] {
				computed, err := e.sampleComparison(ctx, e.requests[o.ID])
				if err != nil || digest(computed.Rows) != o.RowsSHA256 || digest(computed.Comparison) != digest(o.Comparison) {
					return nil, fmt.Errorf("comparison support cannot be replayed from both retained original revisions")
				}
				replayed[o.ID] = true
			}
			context = append(context, proposed)
			continue
		}
		source, exists := observed[packet.Selection.Observation]
		if !exists || participating[source.ID] || source.RowsSHA256 != packet.Selection.RowsSHA256 {
			continue
		}
		var targets []string
		for _, target := range a.Sources {
			if sameFileRevision(source, target) && e.requests[source.ID].Member == e.requests[target.ID].Member {
				targets = append(targets, target.ID)
			}
		}
		if len(targets) != 0 {
			context = append(context, SourceContext{Targets: targets, Source: projectReviewSource(source, packet.Selection.Fields), Request: e.requests[source.ID], PacketID: packet.ID})
		}
	}
	return context, nil
}

func sameFileRevision(a, b Observation) bool {
	if a.Delivery != "FILE" || b.Delivery != "FILE" || a.Reduction != nil || b.Reduction != nil || a.Spatial != nil || b.Spatial != nil || a.Asset == "" || a.Asset != b.Asset || a.PK == "" || a.PK != b.PK || a.ContentSHA256 != b.ContentSHA256 || a.ContractSHA256 != b.ContractSHA256 {
		return false
	}
	for _, hash := range []string{a.ContentSHA256, a.ContractSHA256} {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 32 {
			return false
		}
	}
	return true
}

func projectReviewSource(source Observation, fields []string) Observation {
	source.Columns = append([]string(nil), fields...)
	types, profiles := source.ColumnTypes, source.ColumnProfiles
	source.ColumnTypes, source.ColumnProfiles = map[string][]string{}, map[string]ColumnProfile{}
	for _, field := range fields {
		source.ColumnTypes[field], source.ColumnProfiles[field] = types[field], profiles[field]
	}
	return source
}
