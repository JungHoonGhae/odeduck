package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

// SourceReduction groups one immutable retained source before later joins.
// Fields are original unqualified columns; aggregate fields may name a measure.
// It reduces observed records, not a certified complete population.
type SourceReduction struct {
	Observation string      `json:"observation"`
	RowsSHA256  string      `json:"rowsSha256"`
	GroupBy     []string    `json:"groupBy,omitempty"`
	Measures    []Measure   `json:"measures,omitempty"`
	Aggregates  []Aggregate `json:"aggregates"`
}

type ReductionProvenance struct {
	Method              string          `json:"method"`
	Recipe              SourceReduction `json:"recipe"`
	SourceContentSHA256 string          `json:"sourceContentSha256,omitempty"`
	SourceRequestSHA256 string          `json:"sourceRequestSha256"`
	Groups              [][]int         `json:"groups"` // contributing 1-based retained source rows, not physical ordinals
}

func (e *Engine) sampleReduction(ctx context.Context, request SampleRequest) (Acquired, error) {
	r := request.Reduce
	var source *Observation
	for i := range e.state.Observations {
		if e.state.Observations[i].ID == r.Observation {
			source = &e.state.Observations[i]
		}
	}
	if source == nil || source.PK != request.PK || source.Document != nil || source.Spatial != nil || source.Reduction != nil || r.RowsSHA256 == "" || r.RowsSHA256 != source.RowsSHA256 {
		return Acquired{}, fmt.Errorf("reduce requires this PK's exact retained original observation revision; nested/spatial reductions are unsupported")
	}
	if request.Delivery != e.requests[source.ID].Delivery {
		return Acquired{}, fmt.Errorf("reduce must retain the original request delivery")
	}
	if len(r.GroupBy) > 8 || len(r.Measures) > 16 || len(r.Aggregates) < 1 || len(r.Aggregates) > 8 {
		return Acquired{}, fmt.Errorf("reduce requires at most 8 group fields, 16 measures and 1–8 aggregates")
	}
	rows := e.rows[source.ID]
	p := Composition{Base: source.ID}
	field := func(name string) (string, error) {
		if !slices.Contains(source.Columns, name) || credentialField(name) {
			return "", fmt.Errorf("reduce references an absent or credential source field")
		}
		return source.ID + "." + name, nil
	}
	aliases := map[string]bool{}
	for _, name := range r.GroupBy {
		qualified, err := field(name)
		if err != nil || aliases[name] {
			return Acquired{}, fmt.Errorf("reduce needs distinct observed group fields")
		}
		aliases[name] = true
		p.GroupBy = append(p.GroupBy, qualified)
	}
	measureNames := map[string]bool{}
	for _, m := range r.Measures {
		if !validRequirementID(m.As) || measureNames[m.As] || slices.Contains(source.Columns, m.As) || credentialField(m.As) {
			return Acquired{}, fmt.Errorf("reduce measure aliases must be unique and cannot shadow original columns")
		}
		measureNames[m.As] = true
		var err error
		if m.Field != "" {
			m.Field, err = field(m.Field)
			if err != nil {
				return Acquired{}, err
			}
		}
		m.Fields = slices.Clone(m.Fields)
		for i, name := range m.Fields {
			m.Fields[i], err = field(name)
			if err != nil {
				return Acquired{}, err
			}
		}
		p.Measures = append(p.Measures, m)
	}
	for _, a := range r.Aggregates {
		if !validRequirementID(a.As) || aliases[a.As] || credentialField(a.As) {
			return Acquired{}, fmt.Errorf("reduce output aliases must be distinct from group fields and each other")
		}
		aliases[a.As] = true
		if a.Field != "" && !measureNames[a.Field] {
			var err error
			a.Field, err = field(a.Field)
			if err != nil {
				return Acquired{}, err
			}
		}
		p.Aggregates = append(p.Aggregates, a)
	}
	if err := ctx.Err(); err != nil {
		return Acquired{}, err
	}
	computed, _, err := execute(p, map[string][]Row{source.ID: rows}, 1000, *source)
	if err != nil {
		return Acquired{}, err
	}
	// Bind each computed group to ALL retained inputs, preserving equal-valued
	// records. Membership contains positions, never raw group keys.
	groupKey := func(row Row, fields []string) string {
		values := make([]any, len(fields))
		for i, f := range fields {
			values[i] = row[f]
		}
		b, _ := json.Marshal(values)
		return string(b)
	}
	members := map[string][]int{}
	for i, row := range rows {
		key := groupKey(row, r.GroupBy)
		members[key] = append(members[key], i+1)
	}
	provenance := &ReductionProvenance{Method: "source_group_v1", Recipe: *r, SourceContentSHA256: source.ContentSHA256, SourceRequestSHA256: source.RequestSHA256}
	for _, row := range computed {
		key := groupKey(row, p.GroupBy)
		provenance.Groups = append(provenance.Groups, members[key])
		for i, qualified := range p.GroupBy {
			row[r.GroupBy[i]] = row[qualified]
			delete(row, qualified)
		}
	}
	if err := ctx.Err(); err != nil {
		return Acquired{}, err
	}
	return Acquired{Rows: computed, Delivery: "DERIVED", Reduction: provenance, Warnings: []string{"Local aggregation of every retained source row, not a new source download or population approval. Follow reduction.recipe and groups to the original observation, request, selection and record addresses."}}, nil
}
