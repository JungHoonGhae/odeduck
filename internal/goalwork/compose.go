// Package goalwork owns goal-scoped discovery, observations and bounded
// composition. A planner proposes actions; only this module produces evidence.
package goalwork

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

type Row = map[string]any

type Join struct {
	Right         string      `json:"right"`
	LeftKeys      []string    `json:"leftKeys"`
	RightKeys     []string    `json:"rightKeys"`
	Normalization string      `json:"normalization,omitempty"` // exact (default) or trim; never coerce numeric codes
	Scopes        []JoinScope `json:"scopes,omitempty"`
}

type Composition struct {
	ID              string             `json:"id"`
	Purpose         string             `json:"purpose"`
	Base            string             `json:"base"` // observation ID
	Joins           []Join             `json:"joins"`
	Select          []string           `json:"select,omitempty"`          // qualified observation.field
	ReportUnmatched []string           `json:"reportUnmatched,omitempty"` // separate projection of stage-local excluded tuples
	GroupBy         []string           `json:"groupBy,omitempty"`
	Aggregates      []Aggregate        `json:"aggregates,omitempty"`
	Measures        []Measure          `json:"measures,omitempty"`
	Time            *TemporalAlignment `json:"time,omitempty"`
	Assumptions     []string           `json:"assumptions"` // namespace, time, coverage; assertions, not verification
	Roles           []RoleBinding      `json:"roles,omitempty"`
	Outputs         []OutputBinding    `json:"outputs,omitempty"`
	Support         []SupportBinding   `json:"support,omitempty"` // proposed context, never calculation input
}

type Aggregate struct {
	As    string `json:"as"`
	Op    string `json:"op"` // count or sum
	Field string `json:"field,omitempty"`
}

type JoinMetric struct {
	Stage                 string           `json:"stage,omitempty"` // base_temporal for filtering a base without additional joins
	Right                 string           `json:"right"`
	LeftRows              int              `json:"leftRows"`
	RightRows             int              `json:"rightRows"`
	MatchedLeftRows       int              `json:"matchedLeftRows"`
	UnmatchedLeft         []map[string]int `json:"unmatchedLeft,omitempty"`  // one map per excluded left tuple: observation -> 1-based retained row
	UnmatchedRight        []int            `json:"unmatchedRight,omitempty"` // 1-based retained rows of Right; local to this join stage
	OutputRows            int              `json:"outputRows"`
	TemporalRule          string           `json:"temporalRule,omitempty"`
	TemporalRejectedPairs int              `json:"temporalRejectedPairs,omitempty"`
	TemporalUnknownPairs  int              `json:"temporalUnknownPairs,omitempty"`
	LineageRejectedPairs  int              `json:"lineageRejectedPairs,omitempty"`
	ScopeChecks           []ScopeCheck     `json:"scopeChecks,omitempty"`
}

func execute(p Composition, inputs map[string][]Row, limit int, observations ...Observation) ([]Row, []JoinMetric, error) {
	return executeTraced(p, inputs, limit, nil, observations...)
}

// usedRows receives every contributing retained row before arithmetic/grouping,
// including zero/cancelling terms. Result-value equality is not participation.
func executeTraced(p Composition, inputs map[string][]Row, limit int, usedRows map[string]map[int]bool, observations ...Observation) ([]Row, []JoinMetric, error) {
	base, ok := inputs[p.Base]
	if !ok || len(base) == 0 {
		return nil, nil, fmt.Errorf("base observation missing or empty")
	}
	if len(base) > limit {
		return nil, nil, fmt.Errorf("row limit exceeded")
	}
	rows := qualify(p.Base, base)
	used := map[string]bool{p.Base: true}
	observed := map[string]Observation{}
	for _, observation := range observations {
		if _, exists := observed[observation.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate observation metadata")
		}
		observed[observation.ID] = observation
	}
	traces, err := prepareLineage(p, inputs, observed)
	if err != nil {
		return nil, nil, err
	}
	lineage := traces[p.Base]
	if err := validateMeasureLineage(p.Measures, observed); err != nil {
		return nil, nil, err
	}
	temporal, err := prepareTemporal(p, inputs, observations...)
	if err != nil {
		return nil, nil, err
	}
	periods := make([]dateSpan, len(base))
	for index := range base {
		periods[index] = temporal.source(p.Base, index)
	}
	var metrics []JoinMetric
	if len(p.Joins) == 0 && temporal != nil {
		metric := JoinMetric{Stage: "base_temporal", Right: p.Base, LeftRows: len(rows), RightRows: len(rows), TemporalRule: "common_overlap_v1"}
		var kept []Row
		var keptLineage []rowLineage
		for i, span := range periods {
			if !span.known || span.from.After(span.through) {
				metric.TemporalRejectedPairs++
				if !span.known {
					metric.TemporalUnknownPairs++
				}
				continue
			}
			kept = append(kept, rows[i])
			keptLineage = append(keptLineage, lineage[i])
		}
		metric.OutputRows, metric.MatchedLeftRows = len(kept), len(kept)
		metrics = append(metrics, metric)
		if len(kept) == 0 {
			return nil, metrics, fmt.Errorf("empty result after original-source temporal alignment")
		}
		rows, lineage = kept, keptLineage
	}
	for _, j := range p.Joins {
		right, ok := inputs[j.Right]
		if !ok || used[j.Right] || len(j.LeftKeys) == 0 || len(j.LeftKeys) != len(j.RightKeys) || len(j.LeftKeys) > 8 {
			return nil, metrics, fmt.Errorf("invalid connected join or key arity")
		}
		if j.Normalization != "" && j.Normalization != "exact" && j.Normalization != "trim" {
			return nil, metrics, fmt.Errorf("unsupported normalization")
		}
		if err := requireFields(rows, j.LeftKeys); err != nil {
			return nil, metrics, err
		}
		if err := requireFields(right, j.RightKeys); err != nil {
			return nil, metrics, err
		}
		scopes, scopeChecks, err := prepareScopes(j.Scopes, rows, right, scopeSources{observations: observed, left: used, right: j.Right})
		if err != nil {
			return nil, metrics, err
		}
		type indexedRow struct {
			row     Row
			ordinal int
		}
		index := map[string][]indexedRow{}
		for ordinal, r := range right {
			if k, ok := tuple(r, j.RightKeys, j.Normalization); ok {
				index[k] = append(index[k], indexedRow{row: r, ordinal: ordinal})
			}
		}
		metric := JoinMetric{Right: j.Right, LeftRows: len(rows), RightRows: len(right), ScopeChecks: scopeChecks}
		if temporal != nil {
			metric.TemporalRule = "common_overlap_v1"
		}
		// Count before allocating the expanded relation.
		matchedRight := make([]bool, len(right))
		for leftIndex, r := range rows {
			n := 0
			if k, ok := tuple(r, j.LeftKeys, j.Normalization); ok {
				for _, candidate := range index[k] {
					if !compatibleLineage(lineage[leftIndex], traces[j.Right][candidate.ordinal]) {
						metric.LineageRejectedPairs++
						continue
					}
					if !scopes.match(leftIndex, candidate.ordinal, metric.ScopeChecks) {
						continue
					}
					span, ok := temporal.combine(periods[leftIndex], j.Right, candidate.ordinal)
					if ok {
						n++
						matchedRight[candidate.ordinal] = true
					} else {
						metric.TemporalRejectedPairs++
						if !span.known {
							metric.TemporalUnknownPairs++
						}
					}
				}
			}
			if n > 0 {
				metric.MatchedLeftRows++
			} else {
				positions := map[string]int{}
				for slot, ordinal := range lineage[leftIndex] {
					if slot.kind == "row" {
						positions[slot.observation] = ordinal + 1
					}
				}
				metric.UnmatchedLeft = append(metric.UnmatchedLeft, positions)
			}
			if n > limit-metric.OutputRows {
				return nil, metrics, fmt.Errorf("join row limit exceeded; aggregate source or narrow sample before retry")
			}
			metric.OutputRows += n
		}
		for position, matched := range matchedRight {
			if !matched {
				metric.UnmatchedRight = append(metric.UnmatchedRight, position+1)
			}
		}
		metrics = append(metrics, metric)
		if metric.OutputRows == 0 {
			return nil, metrics, fmt.Errorf("empty full join at %s; inspect lineageRejectedPairs, scopeChecks and namespace/time coverage or search an intermediate mapping", j.Right)
		}
		next := make([]Row, 0, metric.OutputRows)
		nextLineage := make([]rowLineage, 0, metric.OutputRows)
		nextPeriods := make([]dateSpan, 0, metric.OutputRows)
		for leftIndex, l := range rows {
			if k, ok := tuple(l, j.LeftKeys, j.Normalization); ok {
				for _, r := range index[k] {
					if !compatibleLineage(lineage[leftIndex], traces[j.Right][r.ordinal]) {
						continue
					}
					if !scopes.match(leftIndex, r.ordinal, nil) {
						continue
					}
					period, ok := temporal.combine(periods[leftIndex], j.Right, r.ordinal)
					if !ok {
						continue
					}
					merged := Row{}
					for f, v := range l {
						merged[f] = v
					}
					for f, v := range r.row {
						merged[j.Right+"."+f] = v
					}
					trace := mergeLineage(lineage[leftIndex], traces[j.Right][r.ordinal])
					nextLineage = append(nextLineage, trace)
					nextPeriods = append(nextPeriods, period)
					next = append(next, merged)
				}
			}
		}
		rows = next
		lineage = nextLineage
		periods = nextPeriods
		used[j.Right] = true
	}
	if usedRows != nil {
		for _, trace := range lineage {
			for slot, ordinal := range trace {
				if slot.kind != "row" {
					continue
				}
				if usedRows[slot.observation] == nil {
					usedRows[slot.observation] = map[int]bool{}
				}
				usedRows[slot.observation][ordinal] = true
			}
		}
	}
	if err := measureRows(rows, p.Measures); err != nil {
		return nil, metrics, err
	}
	if len(p.Aggregates) > 0 {
		var err error
		origins := map[string]recordSlot{}
		for _, s := range p.Aggregates {
			field := s.Field
			for _, m := range p.Measures {
				if m.As == field {
					field = measureSourceField(m)
					break
				}
			}
			origins[s.As] = fieldSlot(field, observed)
		}
		rows, err = aggregate(rows, p.GroupBy, p.Aggregates, lineage, origins)
		if err != nil {
			return nil, metrics, err
		}
	} else if len(p.GroupBy) > 0 {
		return nil, metrics, fmt.Errorf("groupBy requires aggregates")
	}
	if len(p.Select) > 0 {
		if err := requireFields(rows, p.Select); err != nil {
			return nil, metrics, err
		}
		projected := make([]Row, len(rows))
		for i, r := range rows {
			projected[i] = Row{}
			for _, f := range p.Select {
				projected[i][f] = r[f]
			}
		}
		rows = projected
	}
	return rows, metrics, nil
}

func qualify(id string, rows []Row) []Row {
	out := make([]Row, len(rows))
	for i, r := range rows {
		out[i] = Row{}
		for f, v := range r {
			out[i][id+"."+f] = v
		}
	}
	return out
}
func requireFields(rows []Row, fields []string) error {
	for _, f := range fields {
		found := false
		for _, r := range rows {
			if _, ok := r[f]; ok {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unobserved field %q", f)
		}
	}
	return nil
}
func tuple(r Row, fields []string, normalization string) (string, bool) {
	values := make([]any, len(fields))
	for i, f := range fields {
		v, ok := r[f]
		if !ok || v == nil {
			return "", false
		}
		switch x := v.(type) {
		case string:
			if normalization == "trim" {
				x = strings.TrimSpace(x)
			}
			if x == "" {
				return "", false
			}
			values[i] = []any{"string", x}
		case json.Number:
			values[i] = []any{"number", string(x)}
		case float64:
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return "", false
			}
			values[i] = []any{"number", strconv.FormatFloat(x, 'g', -1, 64)}
		case bool:
			values[i] = []any{"bool", x}
		default:
			return "", false
		}
	}
	b, err := json.Marshal(values)
	return string(b), err == nil
}

func aggregate(rows []Row, groups []string, specs []Aggregate, lineage []rowLineage, origins map[string]recordSlot) ([]Row, error) {
	if err := requireFields(rows, groups); err != nil {
		return nil, err
	}
	fields := map[string]bool{}
	for _, g := range groups {
		fields[g] = true
	}
	for _, s := range specs {
		if s.As == "" || fields[s.As] || (s.Op != "count" && s.Op != "sum") {
			return nil, fmt.Errorf("invalid aggregate")
		}
		fields[s.As] = true
		if s.Op == "sum" || s.Field != "" {
			if err := requireFields(rows, []string{s.Field}); err != nil {
				return nil, err
			}
		}
	}
	byKey := map[string]Row{}
	type decimalSum struct {
		value *big.Rat
		scale int
		seen  map[int]bool
	}
	sums := map[string]map[string]*decimalSum{}
	var order []string
	for rowIndex, r := range rows {
		// Grouping preserves null groups; joins never equate null keys.
		values := make([]any, len(groups))
		for i, g := range groups {
			values[i] = r[g]
		}
		b, err := json.Marshal(values)
		if err != nil {
			return nil, err
		}
		key := string(b)
		out, ok := byKey[key]
		if !ok {
			out = Row{}
			for _, g := range groups {
				out[g] = r[g]
			}
			for _, s := range specs {
				out[s.As] = float64(0)
			}
			byKey[key] = out
			sums[key] = map[string]*decimalSum{}
			order = append(order, key)
		}
		for _, s := range specs {
			if s.Op == "count" && s.Field != "" && r[s.Field] == nil {
				continue
			}
			if s.Op == "sum" {
				value, scale, err := exactNumber(r[s.Field])
				if err != nil {
					return nil, fmt.Errorf("sum field %q: %w", s.Field, err)
				}
				acc := sums[key][s.As]
				if acc == nil {
					acc = &decimalSum{value: new(big.Rat), seen: map[int]bool{}}
					sums[key][s.As] = acc
				}
				ordinal, ok := lineage[rowIndex][origins[s.As]]
				if !ok {
					return nil, fmt.Errorf("sum field %q has no source record lineage", s.Field)
				}
				if acc.seen[ordinal] {
					return nil, fmt.Errorf("sum %q repeats source record after join; aggregate at source grain or provide an explicit allocation before summing", s.As)
				}
				acc.seen[ordinal] = true
				acc.value.Add(acc.value, value)
				if scale > acc.scale {
					acc.scale = scale
				}
				continue
			}
			out[s.As] = out[s.As].(float64) + 1
		}
	}
	sort.Strings(order)
	out := make([]Row, 0, len(order))
	for _, k := range order {
		for name, acc := range sums[k] {
			byKey[k][name] = decimalNumber(acc.value, acc.scale)
		}
		out = append(out, byKey[k])
	}
	return out, nil
}

// ExtractRows selects a JSON Pointer to an array of flat records. Automatic
// selection is allowed only for a single record array. Arrays are never zipped
// or independently flattened: doing so would fabricate tuple evidence.
func ExtractRows(body any, pointer string, limit int) ([]Row, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid sample row limit")
	}
	var target any
	if pointer != "" {
		if !strings.HasPrefix(pointer, "/") {
			return nil, fmt.Errorf("rowPath must be a JSON Pointer")
		}
		target = body
		for _, part := range strings.Split(pointer[1:], "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			m, ok := target.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("rowPath does not select an object field")
			}
			target, ok = m[part]
			if !ok {
				return nil, fmt.Errorf("rowPath not observed")
			}
		}
	} else {
		var candidates []any
		var paths []string
		var walk func(any, string, int) error
		walk = func(v any, pointer string, depth int) error {
			if depth > 32 {
				return fmt.Errorf("response nesting limit exceeded")
			}
			switch x := v.(type) {
			case []any:
				if len(x) > 0 {
					if _, ok := x[0].(map[string]any); ok {
						candidates = append(candidates, x)
						paths = append(paths, pointer)
					}
				}
			case map[string]any:
				for key, v := range x {
					child := pointer + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
					if err := walk(v, child, depth+1); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := walk(body, "", 0); err != nil {
			return nil, err
		}
		if len(candidates) != 1 {
			sort.Strings(paths)
			if len(paths) > 8 {
				paths = paths[:8]
			}
			return nil, fmt.Errorf("ambiguous or absent record arrays; specify rowPath JSON Pointer from observed paths: %s", bounded(strings.Join(paths, ", "), 1000))
		}
		target = candidates[0]
	}
	array, ok := target.([]any)
	if !ok || len(array) == 0 {
		return nil, fmt.Errorf("rowPath must select a nonempty record array")
	}
	if len(array) > limit {
		return nil, fmt.Errorf("sample row limit exceeded; request fewer rows from provider")
	}
	rows := make([]Row, len(array))
	for i, v := range array {
		r, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("record array contains non-object")
		}
		rows[i] = Row{}
		if len(r) > 256 {
			return nil, fmt.Errorf("column limit exceeded")
		}
		for k, v := range r {
			switch v.(type) {
			case nil, string, bool, float64, json.Number:
				rows[i][k] = v
			default:
				return nil, fmt.Errorf("nested field %q: choose a flat record array", k)
			}
		}
	}
	return rows, nil
}
