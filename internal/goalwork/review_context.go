package goalwork

import "encoding/hex"

// SourceContext contains already-disclosed records from another observation of
// the same file revision. Targets attest the file association, not applicability
// of a heading to a table, field meaning, identity, or computational participation.
type SourceContext struct {
	Targets  []string       `json:"targets"`
	Source   Observation    `json:"source"`
	Request  SampleRequest  `json:"request"`
	Evidence EvidencePacket `json:"evidence"`
}

func (e *Engine) reviewSourceContext(a *Artifact) []SourceContext {
	if a == nil {
		return nil
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
	return context
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
