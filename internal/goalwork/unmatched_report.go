package goalwork

import (
	"fmt"
	"slices"
	"strings"
)

// UnmatchedTuple is a separate report, never a joined row or an absence fact.
// Positions use retained observation rows; original CSV/table/group provenance
// lives on those observations. Missing refers only to fields on present sources.
type UnmatchedTuple struct {
	JoinIndex int            `json:"joinIndex"` // 1-based ordered join stage
	Side      string         `json:"side"`      // left or right
	Positions map[string]int `json:"positions"`
	Values    Row            `json:"values"`
	Missing   []string       `json:"missing,omitempty"`
}

func reportUnmatched(p Composition, inputs map[string][]Row, metrics []JoinMetric, limit int, observations ...Observation) ([]UnmatchedTuple, error) {
	if len(p.ReportUnmatched) == 0 {
		return nil, nil
	}
	if len(p.ReportUnmatched) > 16 || len(p.Joins) == 0 {
		return nil, fmt.Errorf("unmatched report requires joins and 1–16 observed direct-source fields")
	}
	direct := map[string]bool{p.Base: true}
	for _, join := range p.Joins {
		direct[join.Right] = true
	}
	columns := map[string][]string{}
	for _, source := range observations {
		columns[source.ID] = source.Columns
	}
	seen := map[string]bool{}
	for _, field := range p.ReportUnmatched {
		id, name, _ := strings.Cut(field, ".")
		if seen[field] || !direct[id] || !slices.Contains(columns[id], name) {
			return nil, fmt.Errorf("unmatched report field %q must be a distinct observed direct-source field", field)
		}
		seen[field] = true
	}
	count := 0
	for _, metric := range metrics {
		count += len(metric.UnmatchedLeft) + len(metric.UnmatchedRight)
	}
	if count > limit {
		return nil, fmt.Errorf("combined result and unmatched report exceed the result row budget")
	}
	var out []UnmatchedTuple
	appendTuple := func(stage int, side string, positions map[string]int) error {
		tuple := UnmatchedTuple{JoinIndex: stage, Side: side, Positions: positions, Values: Row{}}
		for _, field := range p.ReportUnmatched {
			id, name, _ := strings.Cut(field, ".")
			position, participates := positions[id]
			if !participates {
				continue // absent counterpart is not a null source value
			}
			if position < 1 || position > len(inputs[id]) {
				return fmt.Errorf("unmatched report source position is unavailable")
			}
			value, exists := inputs[id][position-1][name]
			if exists {
				tuple.Values[field] = value
			} else {
				tuple.Missing = append(tuple.Missing, field)
			}
		}
		out = append(out, tuple)
		return nil
	}
	for index, metric := range metrics {
		for _, positions := range metric.UnmatchedLeft {
			if err := appendTuple(index+1, "left", positions); err != nil {
				return nil, err
			}
		}
		for _, position := range metric.UnmatchedRight {
			if err := appendTuple(index+1, "right", map[string]int{metric.Right: position}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
