package goalwork

import (
	"encoding/json"
	"fmt"
	"time"
)

// DateWindow uses inclusive calendar dates, not acquisition timestamps.
type DateWindow struct {
	From    string `json:"from"`
	Through string `json:"through"`
}

// TemporalAlignment intersects record periods across the complete composition.
// Field meaning is a declared interpretation; this checks observed date values,
// not whether a reference date really establishes real-world validity.
type TemporalAlignment struct {
	Window   *DateWindow   `json:"window,omitempty"`
	Bindings []TimeBinding `json:"bindings"`
}

type TimeBinding struct {
	Observation  string `json:"observation"`
	FromField    string `json:"fromField"` // original unqualified field
	ThroughField string `json:"throughField,omitempty"`
	Format       string `json:"format"`  // date_v1, month_v1, year_v1, compact_date_v1, compact_month_v1
	Meaning      string `json:"meaning"` // validity, reference_period, event
}

// TemporalEvaluation reports executable calendar checks without certifying
// the model's interpretation of event/reference/validity field semantics.
type TemporalEvaluation struct {
	Status          string        `json:"status"` // checked | not_checked
	Rule            string        `json:"rule,omitempty"`
	Window          *DateWindow   `json:"window,omitempty"`
	Bindings        []TimeBinding `json:"bindings,omitempty"`
	RejectedPairs   int           `json:"rejectedPairs,omitempty"`
	UnknownPairs    int           `json:"unknownPairs,omitempty"`
	MeaningVerified bool          `json:"meaningVerified"`
	Limitation      string        `json:"limitation"`
}

func evaluateTemporal(p Composition, metrics []JoinMetric) TemporalEvaluation {
	result := TemporalEvaluation{Status: "not_checked", Limitation: "No field-bound temporal alignment was executed; acquisition/catalogue dates are not record validity evidence."}
	if p.Time == nil {
		return result
	}
	result.Status = "checked"
	result.Rule = "common_overlap_v1"
	result.Window = p.Time.Window
	result.Bindings = p.Time.Bindings
	result.Limitation = "Observed record periods overlap at the declared calendar granularity. Field meanings, intra-period timing and population coverage are not verified; rejected counts are candidate pairs, not distinct source records."
	for _, metric := range metrics {
		result.RejectedPairs += metric.TemporalRejectedPairs
		result.UnknownPairs += metric.TemporalUnknownPairs
	}
	return result
}

type dateSpan struct {
	from, through time.Time
	known         bool
}
type temporalContext struct{ spans map[string][]dateSpan }

func prepareTemporal(p Composition, inputs map[string][]Row, observations ...Observation) (*temporalContext, error) {
	if p.Time == nil {
		return nil, nil
	}
	byID := map[string]Observation{}
	for _, o := range observations {
		byID[o.ID] = o
	}
	direct := map[string]bool{p.Base: true}
	for _, j := range p.Joins {
		direct[j.Right] = true
	}
	used := compositionSources(p, byID)
	readOriginalRows := map[string]bool{}
	for id := range used {
		if r := byID[id].Reduction; r != nil {
			readOriginalRows[r.Recipe.Observation] = true
			delete(used, id)
		} else if s := byID[id].Spatial; s != nil {
			readOriginalRows[s.AnchorObservation] = true
			delete(used, id)
		} else if direct[id] {
			readOriginalRows[id] = true
		}
	}
	if len(p.Time.Bindings) != len(used) || len(p.Time.Bindings) > 8 {
		return nil, fmt.Errorf("time alignment requires exactly one binding for every participating original observation, including reduction and spatial sources")
	}
	var window *dateSpan
	if p.Time.Window != nil {
		span, err := parseDateWindow(*p.Time.Window)
		if err != nil {
			return nil, err
		}
		window = &span
	}
	out := &temporalContext{spans: map[string][]dateSpan{}}
	bindings := map[string]TimeBinding{}
	for _, binding := range p.Time.Bindings {
		rows, ok := inputs[binding.Observation]
		_, duplicate := bindings[binding.Observation]
		if !ok || !used[binding.Observation] || duplicate || len(rows) == 0 {
			return nil, fmt.Errorf("time binding references an absent, unused or duplicate observation")
		}
		bindings[binding.Observation] = binding
		if binding.Meaning != "validity" && binding.Meaning != "reference_period" && binding.Meaning != "event" {
			return nil, fmt.Errorf("time meaning must be validity, reference_period or event")
		}
		if (binding.Meaning == "validity") != (binding.ThroughField != "") {
			return nil, fmt.Errorf("validity needs two observed endpoint fields; reference/event periods must not be extended to a validity range")
		}
		fields := []string{binding.FromField}
		if binding.ThroughField != "" {
			fields = append(fields, binding.ThroughField)
		}
		if err := requireFields(rows, fields); err != nil {
			return nil, err
		}
		// A candidate prefix supplies observed schema, not the dates of nearest
		// records outside that prefix. Parse the actual pair copy below instead.
		if !readOriginalRows[binding.Observation] {
			continue
		}
		for ordinal, row := range rows {
			span, err := recordSpan(row, binding)
			if err != nil {
				return nil, fmt.Errorf("time binding %s at source row %d: %w", binding.Observation, ordinal+1, err)
			}
			if window != nil {
				span = intersectDates(span, *window)
			}
			out.spans[binding.Observation] = append(out.spans[binding.Observation], span)
		}
	}
	for id := range direct {
		if r := byID[id].Reduction; r != nil {
			// A sum already contains every member. Filtering one member later
			// would falsify that sum, so intersect ALL original member periods.
			original := out.spans[r.Recipe.Observation]
			for _, group := range r.Groups {
				span := original[group[0]-1] // membership validated by prepareLineage
				for _, position := range group[1:] {
					span = intersectDates(span, original[position-1])
				}
				out.spans[id] = append(out.spans[id], span)
			}
			continue
		}
		s := byID[id].Spatial
		if s == nil {
			continue
		}
		b := bindings[s.CandidateObservation]
		if len(s.Pairs) != len(inputs[id]) {
			return nil, fmt.Errorf("spatial time alignment pair count mismatch")
		}
		for i, pair := range s.Pairs {
			anchors := out.spans[s.AnchorObservation]
			if pair.AnchorRow < 1 || pair.AnchorRow > len(anchors) {
				return nil, fmt.Errorf("spatial time alignment anchor position mismatch")
			}
			row := inputs[id][i]
			candidate := Row{b.FromField: row["candidate."+b.FromField]}
			if b.ThroughField != "" {
				candidate[b.ThroughField] = row["candidate."+b.ThroughField]
			}
			span, err := recordSpan(candidate, b)
			if err != nil {
				return nil, fmt.Errorf("spatial candidate time at data record %d: %w", pair.CandidateDataRecord, err)
			}
			if window != nil {
				span = intersectDates(span, *window)
			}
			out.spans[id] = append(out.spans[id], intersectDates(anchors[pair.AnchorRow-1], span))
		}
	}
	return out, nil
}

func parseDateWindow(w DateWindow) (dateSpan, error) {
	from, err := parseDatePeriod(w.From, "date_v1")
	if err != nil {
		return dateSpan{}, fmt.Errorf("invalid window from date: %w", err)
	}
	through, err := parseDatePeriod(w.Through, "date_v1")
	if err != nil {
		return dateSpan{}, fmt.Errorf("invalid window through date: %w", err)
	}
	if !from.known || !through.known || from.from.After(through.through) {
		return dateSpan{}, fmt.Errorf("time window requires nonempty ordered inclusive dates")
	}
	return dateSpan{from: from.from, through: through.through, known: true}, nil
}

func recordSpan(row Row, b TimeBinding) (dateSpan, error) {
	from, err := parseDatePeriod(row[b.FromField], b.Format)
	if err != nil {
		return dateSpan{}, err
	}
	if b.ThroughField == "" {
		return from, nil
	}
	through, err := parseDatePeriod(row[b.ThroughField], b.Format)
	if err != nil {
		return dateSpan{}, err
	}
	if !from.known || !through.known {
		return dateSpan{}, nil
	}
	if from.from.After(through.through) {
		return dateSpan{}, fmt.Errorf("record validity endpoints are reversed")
	}
	return dateSpan{from: from.from, through: through.through, known: true}, nil
}

func parseDatePeriod(value any, format string) (dateSpan, error) {
	var layout string
	switch format {
	case "date_v1":
		layout = "2006-01-02"
	case "month_v1":
		layout = "2006-01"
	case "year_v1":
		layout = "2006"
	case "compact_date_v1":
		layout = "20060102"
	case "compact_month_v1":
		layout = "200601"
	default:
		return dateSpan{}, fmt.Errorf("unsupported versioned time format")
	}
	if value == nil {
		return dateSpan{}, nil
	}
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case json.Number:
		text = string(v)
	default:
		return dateSpan{}, fmt.Errorf("time field must be a string or exact JSON number in its declared format")
	}
	if text == "" {
		return dateSpan{}, nil
	}
	if len(text) != len(layout) {
		return dateSpan{}, fmt.Errorf("time field does not match declared format")
	}
	start, err := time.Parse(layout, text)
	if err != nil || start.Year() < 1 || start.Format(layout) != text {
		return dateSpan{}, fmt.Errorf("invalid calendar value for declared format")
	}
	end := start
	switch format {
	case "year_v1":
		end = start.AddDate(1, 0, -1)
	case "month_v1", "compact_month_v1":
		end = start.AddDate(0, 1, -1)
	}
	return dateSpan{from: start, through: end, known: true}, nil
}

func intersectDates(a, b dateSpan) dateSpan {
	if !a.known || !b.known {
		return dateSpan{}
	}
	if b.from.After(a.from) {
		a.from = b.from
	}
	if b.through.Before(a.through) {
		a.through = b.through
	}
	return a
}

func (c *temporalContext) source(id string, ordinal int) dateSpan {
	if c == nil {
		return dateSpan{}
	}
	return c.spans[id][ordinal]
}

func (c *temporalContext) combine(left dateSpan, rightID string, ordinal int) (dateSpan, bool) {
	if c == nil {
		return dateSpan{}, true
	}
	span := intersectDates(left, c.source(rightID, ordinal))
	return span, span.known && !span.from.After(span.through)
}
