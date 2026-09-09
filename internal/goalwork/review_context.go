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
	Comparison        *ComparisonReviewTrace    `json:"comparison,omitempty"`
	ComparisonSources []ComparisonContextSource `json:"comparisonSources,omitempty"`
	Targets           []string                  `json:"targets"`
	Source            Observation               `json:"source"`
	Request           SampleRequest             `json:"request"`
	PacketID          string                    `json:"packetId"`
	Proposed          bool                      `json:"proposed,omitempty"`
	Purpose           string                    `json:"purpose,omitempty"`
}

// ComparisonReviewTrace retains complete positional provenance; its one recipe
// is the context's actual Request.Compare. Runtime observations are unchanged.
type ComparisonReviewTrace struct {
	Method  string             `json:"method"`
	Left    ComparisonRevision `json:"left"`
	Right   ComparisonRevision `json:"right"`
	Pairs   [][2]int           `json:"pairs"`
	Records []ComparisonOrigin `json:"records"`
}

// A comparison parent is either an existing Artifact source ID (and its paired
// request), or an inline original source/request. Neither form discloses values
// or changes computational participation.
type ComparisonContextSource struct {
	ArtifactSource string         `json:"artifactSource,omitempty"`
	Source         *Observation   `json:"source,omitempty"`
	Request        *SampleRequest `json:"request,omitempty"`
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
				request := e.requests[parent.ID]
				for field := range request.Where {
					fields[field] = true
				}
				for field := range request.WhereIn {
					fields[field] = true
				}
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
				projected := projectReviewSource(parent, names)
				c.ComparisonSources = append(c.ComparisonSources, ComparisonContextSource{Source: &projected, Request: &request})
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

// Normalize only comparison context in the detached review projection. Shared
// original metadata is unioned, not promoted into computational field usage.
func (e *Engine) projectComparisonReview(in *ReviewInput) error {
	observed := map[string]Observation{}
	for _, source := range e.state.Observations {
		observed[source.ID] = source
	}
	for i := range in.Analysis.SourceContext {
		c := &in.Analysis.SourceContext[i]
		p := c.Source.Comparison
		if p == nil {
			continue
		}
		if c.Request.Compare == nil || digest(c.Request.Compare) != digest(p.Recipe) || digest(c.Request) != c.Source.RequestSHA256 || len(c.ComparisonSources) != 2 {
			return fmt.Errorf("comparison review lost its exact request or original revisions")
		}
		c.Comparison = &ComparisonReviewTrace{Method: p.Method, Left: p.Left, Right: p.Right, Pairs: p.Pairs, Records: p.Records}
		c.Source.Comparison = nil // Request.Compare is the one recipe in this wire contract.
		for j := range c.ComparisonSources {
			parent := &c.ComparisonSources[j]
			if parent.Source == nil || parent.Request == nil || parent.ArtifactSource != "" {
				return fmt.Errorf("comparison review requires both original source/request pairs")
			}
			source := *parent.Source
			raw, exists := observed[source.ID]
			if !exists || digest(projectReviewSource(raw, source.Columns)) != digest(source) || digest(parent.Request) != source.RequestSHA256 {
				return fmt.Errorf("comparison review original metadata or request changed")
			}
			for k, shared := range in.Artifact.Sources {
				if shared.ID != source.ID {
					continue
				}
				if k >= len(in.Artifact.Requests) || digest(in.Artifact.Requests[k]) != source.RequestSHA256 || digest(projectReviewSource(raw, shared.Columns)) != digest(shared) {
					return fmt.Errorf("comparison review cannot reference inconsistent artifact metadata")
				}
				fields := append(slices.Clone(shared.Columns), source.Columns...)
				slices.Sort(fields)
				in.Artifact.Sources[k] = projectReviewSource(raw, slices.Compact(fields))
				*parent = ComparisonContextSource{ArtifactSource: source.ID}
				break
			}
		}
	}
	return nil
}
