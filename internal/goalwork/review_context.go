package goalwork

import (
	"encoding/hex"
	"fmt"
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

// SourceContext contains already-disclosed records from another observation of
// the same file revision, or explicitly proposed supporting records. Neither
// association proves applicability, identity, or computational participation.
type SourceContext struct {
	Targets  []string       `json:"targets"`
	Source   Observation    `json:"source"`
	Request  SampleRequest  `json:"request"`
	Evidence EvidencePacket `json:"evidence"`
	Proposed bool           `json:"proposed,omitempty"`
	Purpose  string         `json:"purpose,omitempty"`
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
			return nil, fmt.Errorf("support must reference a nonparticipating original observation revision")
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
		out[binding.PacketID] = SourceContext{Targets: binding.Targets, Source: projectReviewSource(source, packet.Selection.Fields), Request: e.requests[source.ID], Evidence: packet, Proposed: true, Purpose: binding.Purpose}
	}
	return out, nil
}

func (e *Engine) reviewSourceContext(a *Artifact) ([]SourceContext, error) {
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
	for _, packet := range e.state.Evidence {
		if proposed, exists := explicit[packet.ID]; exists {
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
			context = append(context, SourceContext{Targets: targets, Source: projectReviewSource(source, packet.Selection.Fields), Request: e.requests[source.ID], Evidence: packet})
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
