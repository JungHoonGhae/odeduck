package goalwork

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const AnalysisReviewMethod = "independent_model_relational_analysis_v1"

// AnalysisReviewContext supplements the v1 primary packet without changing its
// wire contract. Replay attests execution, not independent arithmetic/meaning.
type AnalysisReviewContext struct {
	Method             string           `json:"method"`
	OutputSHA256       string           `json:"outputSha256"`
	Sources            []AnalysisSource `json:"sources"`
	AdditionalEvidence []EvidencePacket `json:"additionalEvidence,omitempty"`
}

type AnalysisSource struct {
	Observation string   `json:"observation"`
	RowsSHA256  string   `json:"rowsSha256"`
	RowCount    int      `json:"rowCount"`
	Fields      []string `json:"fields"`               // metadata projection, not a claim that every original cell was disclosed
	DirectRows  []int    `json:"directRows,omitempty"` // actual contributing 1-based retained positions, before any final aggregate
}

type AnalysisCheck struct {
	Topic   string        `json:"topic"`
	Finding ReviewFinding `json:"finding"`
}

func (e *Engine) analysisReviewInput(ctx context.Context, id string) (ReviewInput, error) {
	a := e.state.Artifact
	if a == nil || e.state.Evaluation == nil || id != a.Recipe.ID || e.state.Evaluation.Status != "requirements_met" || e.state.Evaluation.ExecutionRevision < 1 {
		return ReviewInput{}, fmt.Errorf("analysis review requires the current structurally complete execution")
	}
	if len(a.Recipe.Select) < 1 || len(a.Recipe.Select) > 16 {
		return ReviewInput{}, fmt.Errorf("analysis review requires 1–16 explicit result fields")
	}
	direct := map[string]bool{a.Recipe.Base: true}
	for _, j := range a.Recipe.Joins {
		direct[j.Right] = true
	}
	observed := map[string]Observation{}
	for _, o := range a.Sources {
		if o.Spatial != nil {
			return ReviewInput{}, fmt.Errorf("spatial results and streamed candidate evidence need a separate review contract")
		}
		observed[o.ID] = o
		if o.Reduction != nil {
			recomputed, err := e.sampleReduction(ctx, e.requests[o.ID])
			if err != nil || digest(recomputed.Rows) != o.RowsSHA256 || digest(recomputed.Reduction) != digest(o.Reduction) {
				return ReviewInput{}, fmt.Errorf("source reduction cannot be reproduced from its retained original revision")
			}
		}
	}
	usedRows := map[string]map[int]bool{}
	actual, _, err := executeTraced(a.Recipe, e.rows, 1000, usedRows, a.Sources...)
	if err != nil || digest(actual) != digest(a.Rows) {
		return ReviewInput{}, fmt.Errorf("current result cannot be reproduced from retained sources")
	}
	needed := analysisFields(a.Recipe, observed, e.requests)
	selected := map[string][]Row{}
	cells := map[string][]map[string]bool{}
	for _, o := range a.Sources {
		selected[o.ID] = make([]Row, o.RowCount)
		cells[o.ID] = make([]map[string]bool, o.RowCount)
		for i := range o.RowCount {
			selected[o.ID][i], cells[o.ID][i] = Row{}, map[string]bool{}
		}
	}
	var packets []EvidencePacket
	for _, packet := range e.state.Evidence {
		o, exists := observed[packet.Selection.Observation]
		if !exists || packet.Selection.RowsSHA256 != o.RowsSHA256 {
			continue
		}
		packets = append(packets, packet)
		for _, record := range packet.Records {
			for _, field := range packet.Selection.Fields {
				cells[o.ID][record.RetainedRow-1][field] = true
				if value, exists := record.Values[field]; exists {
					selected[o.ID][record.RetainedRow-1][field] = value
				}
			}
		}
	}
	if len(packets) == 0 {
		return ReviewInput{}, fmt.Errorf("read selected evidence before analysis review")
	}
	for id, fields := range needed {
		for field := range fields {
			seen := false
			for position, row := range cells[id] {
				seen = seen || row[field]
				if direct[id] && usedRows[id][position] && !row[field] {
					return ReviewInput{}, fmt.Errorf("analysis needs every contributing direct input row for field %s.%s", id, field)
				}
			}
			if !seen {
				return ReviewInput{}, fmt.Errorf("analysis needs selected semantic evidence for original field %s.%s", id, field)
			}
		}
	}
	// Only direct relation values are reconstructed from disclosed packets.
	// Non-direct originals stay local for group membership and temporal checks;
	// none of their undisclosed values is placed in ReviewInput.
	inputs := map[string][]Row{}
	metadata := slices.Clone(a.Sources)
	for i, o := range metadata {
		inputs[o.ID] = e.rows[o.ID]
		if direct[o.ID] {
			inputs[o.ID] = selected[o.ID]
			metadata[i].RowsSHA256 = digest(selected[o.ID])
		}
	}
	replayed, _, err := execute(a.Recipe, inputs, 1000, metadata...)
	if err != nil || digest(replayed) != digest(a.Rows) {
		return ReviewInput{}, fmt.Errorf("analysis result cannot be reconstructed from disclosed relation values; read missing fields/rows, do not shrink the goal")
	}
	in := ReviewInput{Recipient: e.state.Policy.ReviewRecipient, Goal: e.state.Goal, Contract: *e.state.Contract, Artifact: *a, Evidence: packets[0], Analysis: &AnalysisReviewContext{Method: "engine_relational_replay_v1", OutputSHA256: digest(a.Rows), AdditionalEvidence: packets[1:]}}
	in.Artifact.Sources = slices.Clone(a.Sources)
	for i, o := range in.Artifact.Sources {
		contextFields := map[string]bool{}
		for field := range needed[o.ID] {
			contextFields[field] = true
		}
		for _, packet := range packets {
			if packet.Selection.Observation == o.ID {
				for _, field := range packet.Selection.Fields {
					contextFields[field] = true
				}
			}
		}
		var fields []string
		for field := range contextFields {
			fields = append(fields, field)
		}
		slices.Sort(fields)
		var participating []int
		for position := range usedRows[o.ID] {
			participating = append(participating, position+1)
		}
		slices.Sort(participating)
		in.Analysis.Sources = append(in.Analysis.Sources, AnalysisSource{Observation: o.ID, RowsSHA256: o.RowsSHA256, RowCount: o.RowCount, Fields: fields, DirectRows: participating})
		in.Artifact.Sources[i].Columns = fields
		in.Artifact.Sources[i].ColumnTypes = map[string][]string{}
		in.Artifact.Sources[i].ColumnProfiles = map[string]ColumnProfile{}
		for _, field := range fields {
			in.Artifact.Sources[i].ColumnTypes[field] = o.ColumnTypes[field]
			in.Artifact.Sources[i].ColumnProfiles[field] = o.ColumnProfiles[field]
		}
	}
	if err := ctx.Err(); err != nil {
		return ReviewInput{}, err
	}
	return detachReviewInput(in)
}

// Include semantic context even for fields used only in selection, a measure
// recipe or an original period; selected output columns alone are insufficient.
func analysisFields(p Composition, observations map[string]Observation, requests map[string]SampleRequest) map[string]map[string]bool {
	needed := map[string]map[string]bool{}
	add := func(id, field string) {
		if field == "" {
			return
		}
		if needed[id] == nil {
			needed[id] = map[string]bool{}
		}
		needed[id][field] = true
	}
	qualified := func(field string) {
		if id, name, ok := strings.Cut(field, "."); ok {
			if _, exists := observations[id]; exists {
				add(id, name)
			}
		}
	}
	for _, fields := range [][]string{p.Select, p.GroupBy} {
		for _, field := range fields {
			qualified(field)
		}
	}
	for _, m := range p.Measures {
		qualified(m.Field)
		for _, field := range m.Fields {
			qualified(field)
		}
	}
	for _, a := range p.Aggregates {
		qualified(a.Field)
	}
	for _, j := range p.Joins {
		for _, field := range j.LeftKeys {
			qualified(field)
		}
		for _, field := range j.RightKeys {
			add(j.Right, field)
		}
		for _, scope := range j.Scopes {
			for _, field := range scope.LeftParts {
				qualified(field)
			}
			for _, field := range scope.RightParts {
				add(j.Right, field)
			}
		}
	}
	if p.Time != nil {
		for _, binding := range p.Time.Bindings {
			add(binding.Observation, binding.FromField)
			add(binding.Observation, binding.ThroughField)
		}
	}
	for id, o := range observations {
		for field := range requests[id].Where {
			add(id, field)
		}
		if o.Reduction != nil {
			r := o.Reduction.Recipe
			for _, field := range r.GroupBy {
				add(r.Observation, field)
			}
			for _, m := range r.Measures {
				add(r.Observation, m.Field)
				for _, field := range m.Fields {
					add(r.Observation, field)
				}
			}
			for _, a := range r.Aggregates {
				if slices.Contains(observations[r.Observation].Columns, a.Field) {
					add(r.Observation, a.Field)
				}
			}
		}
	}
	return needed
}

func validAnalysisCitations(f ReviewFinding, in ReviewInput) bool {
	if f.PacketID != "" || len(f.PacketIDs) < 1 || len(f.PacketIDs) > maxEvidencePackets {
		return false
	}
	valid := map[string]bool{in.Evidence.ID: true}
	for _, packet := range in.Analysis.AdditionalEvidence {
		valid[packet.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range f.PacketIDs {
		if id == "" || !valid[id] || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
