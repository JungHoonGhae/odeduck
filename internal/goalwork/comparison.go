package goalwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

const comparisonMethod = "source_comparison_v1"

var comparisonSummaryMetrics = []string{"leftRows", "rightRows", "matchedPairs", "leftOnly", "rightOnly", "leftUnresolved", "rightUnresolved", "checks", "comparisons", "equal", "different", "missing", "invalid"}

type ComparisonKey struct {
	Field  string `json:"field"`
	Rule   string `json:"rule"`
	Digits int    `json:"digits,omitempty"`
}

type ComparisonSide struct {
	Observation string          `json:"observation"`
	RowsSHA256  string          `json:"rowsSha256"`
	Keys        []ComparisonKey `json:"keys"`
}

type NumericComparison struct {
	ID    string  `json:"id"`
	Left  Measure `json:"left"`
	Right Measure `json:"right"`
}

// SourceComparison proposes explicit keys and arithmetic, never expected values
// or an identity/applicability verdict. Operands use unqualified source fields.
type SourceComparison struct {
	Left   ComparisonSide      `json:"left"`
	Right  ComparisonSide      `json:"right"`
	Checks []NumericComparison `json:"checks"`
}

type ComparisonRevision struct {
	Observation    string `json:"observation"`
	RowsSHA256     string `json:"rowsSha256"`
	ContentSHA256  string `json:"contentSha256,omitempty"`
	RequestSHA256  string `json:"requestSha256"`
	ContractSHA256 string `json:"contractSha256,omitempty"`
}

// Positions are 1-based retained source rows. Resolve physical CSV/XLSX/API
// addresses in these immutable source observations; never treat them as IDs.
type ComparisonOrigin struct {
	AllRows bool   `json:"allRows,omitempty"`
	Left    []int  `json:"left,omitempty"`
	Right   []int  `json:"right,omitempty"`
	Check   string `json:"check,omitempty"`
}

type ComparisonProvenance struct {
	Method  string             `json:"method"`
	Recipe  SourceComparison   `json:"recipe"`
	Left    ComparisonRevision `json:"left"`
	Right   ComparisonRevision `json:"right"`
	Pairs   [][2]int           `json:"pairs"`
	Records []ComparisonOrigin `json:"records"`
}

func (e *Engine) sampleComparison(ctx context.Context, request SampleRequest) (Acquired, error) {
	r := request.Compare
	if r == nil || r.Left.Observation == r.Right.Observation || len(r.Checks) < 1 || len(r.Checks) > 64 || len(r.Left.Keys) < 1 || len(r.Left.Keys) > 8 || len(r.Left.Keys) != len(r.Right.Keys) {
		return Acquired{}, fmt.Errorf("compare requires two distinct original revisions, 1–8 paired keys and 1–64 numeric checks")
	}
	observed := map[string]Observation{}
	for _, o := range e.state.Observations {
		observed[o.ID] = o
	}
	var sources [2]Observation
	var inputs [2][]Row
	for i, side := range []ComparisonSide{r.Left, r.Right} {
		o, exists := observed[side.Observation]
		if !exists || side.RowsSHA256 == "" || side.RowsSHA256 != o.RowsSHA256 || o.Document != nil || o.Reduction != nil || o.Spatial != nil || o.Comparison != nil || o.Delivery == "DERIVED" || o.Delivery == "DOCUMENT" {
			return Acquired{}, fmt.Errorf("compare requires exact retained original observation revisions; derived/document inputs are unsupported")
		}
		if _, err := prepareLineage(Composition{Base: o.ID}, e.rows, observed); err != nil {
			return Acquired{}, err
		}
		sources[i], inputs[i] = o, e.rows[o.ID]
		seen := map[string]bool{}
		for _, key := range side.Keys {
			if !slices.Contains(o.Columns, key.Field) || credentialField(key.Field) || seen[key.Field] {
				return Acquired{}, fmt.Errorf("comparison keys require distinct observed non-credential fields")
			}
			seen[key.Field] = true
			switch key.Rule {
			case "exact", "trim":
				if key.Digits != 0 {
					return Acquired{}, fmt.Errorf("digit width applies only to explicit terminal extraction")
				}
			case "trailing_parenthesized_digits_v1":
				if key.Digits < 1 || key.Digits > 32 {
					return Acquired{}, fmt.Errorf("terminal key extraction requires 1–32 ASCII digits")
				}
			default:
				return Acquired{}, fmt.Errorf("unsupported comparison key rule")
			}
		}
	}
	if sources[0].PK != request.PK || e.requests[sources[0].ID].Delivery != request.Delivery {
		return Acquired{}, fmt.Errorf("compare must retain the left source PK and original request delivery")
	}
	// Qualify copies for the shared numeric interpretation contract; originals and
	// public recipes remain unchanged, and each operand is evaluated independently.
	checks := make([][2]Measure, len(r.Checks))
	ids := map[string]bool{}
	for i, check := range r.Checks {
		if !validRequirementID(check.ID) || ids[check.ID] || credentialField(check.ID) || check.Left.Unit != check.Right.Unit {
			return Acquired{}, fmt.Errorf("comparison checks require distinct IDs and equal declared units")
		}
		ids[check.ID] = true
		for side, operand := range []Measure{check.Left, check.Right} {
			if operand.As != "" {
				return Acquired{}, fmt.Errorf("comparison operands have no output alias; name the check instead")
			}
			m := operand
			m.As = "comparison_value"
			m.Fields = slices.Clone(m.Fields)
			qualifyField := func(field string) (string, error) {
				if !slices.Contains(sources[side].Columns, field) || credentialField(field) {
					return "", fmt.Errorf("comparison references an absent or credential numeric field")
				}
				return sources[side].ID + "." + field, nil
			}
			var err error
			if m.Field != "" {
				m.Field, err = qualifyField(m.Field)
				if err != nil {
					return Acquired{}, err
				}
			}
			for j, field := range m.Fields {
				m.Fields[j], err = qualifyField(field)
				if err != nil {
					return Acquired{}, err
				}
			}
			if _, err := validateMeasureDefinition(m); err != nil {
				return Acquired{}, err
			}
			checks[i][side] = m
		}
	}
	qualified := [2][]Row{qualify(sources[0].ID, inputs[0]), qualify(sources[1].ID, inputs[1])}
	revision := func(o Observation) ComparisonRevision {
		return ComparisonRevision{Observation: o.ID, RowsSHA256: o.RowsSHA256, ContentSHA256: o.ContentSHA256, RequestSHA256: o.RequestSHA256, ContractSHA256: o.ContractSHA256}
	}
	p := &ComparisonProvenance{Method: comparisonMethod, Recipe: *r, Left: revision(sources[0]), Right: revision(sources[1])}
	alignment := Row{"kind": "alignment", "leftRows": len(inputs[0]), "rightRows": len(inputs[1]), "matchedPairs": 0, "leftOnly": 0, "rightOnly": 0, "leftUnresolved": 0, "rightUnresolved": 0}
	numeric := Row{"kind": "numeric", "checks": len(checks), "comparisons": 0, "equal": 0, "different": 0, "missing": 0, "invalid": 0}
	var rows []Row
	for _, check := range r.Checks {
		rows = append(rows, Row{"kind": "check", "check": check.ID, "equal": 0, "different": 0, "missing": 0, "invalid": 0})
		p.Records = append(p.Records, ComparisonOrigin{AllRows: true, Check: check.ID})
	}
	add := func(row Row, origin ComparisonOrigin) error {
		if len(rows)+len(comparisonSummaryMetrics) >= 1000 {
			return fmt.Errorf("comparison report exceeds 1000 rows; no discrepancies were truncated")
		}
		rows = append(rows, row)
		p.Records = append(p.Records, origin)
		return nil
	}
	inc := func(row Row, field string, n int) { row[field] = row[field].(int) + n }
	indexes := [2]map[string][]int{{}, {}}
	keys := map[string]bool{}
	for side, selection := range []ComparisonSide{r.Left, r.Right} {
		for ordinal, row := range inputs[side] {
			if err := ctx.Err(); err != nil {
				return Acquired{}, err
			}
			key, issue := comparisonTuple(row, selection.Keys)
			if issue != "" {
				origin, prefix := ComparisonOrigin{Left: []int{ordinal + 1}}, "left"
				if side == 1 {
					origin, prefix = ComparisonOrigin{Right: []int{ordinal + 1}}, "right"
				}
				inc(alignment, prefix+"Unresolved", 1)
				if err := add(Row{"kind": prefix + "_" + issue, "members": 1}, origin); err != nil {
					return Acquired{}, err
				}
				continue
			}
			indexes[side][key] = append(indexes[side][key], ordinal+1)
			keys[key] = true
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		if err := ctx.Err(); err != nil {
			return Acquired{}, err
		}
		left, right := indexes[0][key], indexes[1][key]
		if len(left) > 1 || len(right) > 1 {
			inc(alignment, "leftUnresolved", len(left))
			inc(alignment, "rightUnresolved", len(right))
			if err := add(Row{"kind": "ambiguous_key", "key": key, "leftMembers": len(left), "rightMembers": len(right)}, ComparisonOrigin{Left: left, Right: right}); err != nil {
				return Acquired{}, err
			}
			continue
		}
		if len(left) == 0 || len(right) == 0 {
			kind, field := "left_only", "leftOnly"
			if len(left) == 0 {
				kind, field = "right_only", "rightOnly"
			}
			inc(alignment, field, 1)
			if err := add(Row{"kind": kind, "key": key, "members": 1}, ComparisonOrigin{Left: left, Right: right}); err != nil {
				return Acquired{}, err
			}
			continue
		}
		inc(alignment, "matchedPairs", 1)
		p.Pairs = append(p.Pairs, [2]int{left[0], right[0]})
		for i, check := range checks {
			if err := ctx.Err(); err != nil {
				return Acquired{}, err
			}
			lv, le := evaluateMeasure(qualified[0][left[0]-1], check[0])
			rv, re := evaluateMeasure(qualified[1][right[0]-1], check[1])
			ln, _, leftRangeErr := exactNumber(lv)
			rn, _, rightRangeErr := exactNumber(rv)
			if errors.Is(le, errMeasureResultRange) || errors.Is(re, errMeasureResultRange) ||
				(lv != nil && leftRangeErr != nil) || (rv != nil && rightRangeErr != nil) {
				return Acquired{}, fmt.Errorf("comparison check %s exceeds the supported exact numeric result range", r.Checks[i].ID)
			}
			status := "equal"
			switch {
			case le != nil || re != nil:
				status = "invalid"
			case lv == nil || rv == nil:
				status = "missing"
			default:
				if ln.Cmp(rn) != 0 {
					status = "different"
				}
			}
			inc(numeric, "comparisons", 1)
			inc(numeric, status, 1)
			inc(rows[i], status, 1)
			if status != "equal" {
				row := Row{"kind": status, "check": r.Checks[i].ID, "leftValue": lv, "rightValue": rv}
				if le != nil {
					row["leftInvalid"] = true
				}
				if re != nil {
					row["rightInvalid"] = true
				}
				if err := add(row, ComparisonOrigin{Left: left, Right: right, Check: r.Checks[i].ID}); err != nil {
					return Acquired{}, err
				}
			}
		}
	}
	// All summary counts fit one ordinary 13-row/2-field evidence packet. This
	// keeps negative outcomes visible without enlarging any disclosure limit.
	summaries := make([]Row, 0, len(comparisonSummaryMetrics))
	origins := make([]ComparisonOrigin, 0, len(comparisonSummaryMetrics))
	for _, metric := range comparisonSummaryMetrics {
		value, exists := alignment[metric]
		if !exists {
			value = numeric[metric]
		}
		summaries = append(summaries, Row{"kind": "summary", "metric": metric, "value": value})
		origins = append(origins, ComparisonOrigin{AllRows: true})
	}
	rows = append(summaries, rows...)
	p.Records = append(origins, p.Records...)
	return Acquired{Rows: rows, Delivery: "DERIVED", Comparison: p, Warnings: []string{"Local numeric comparison of all retained input records only. Exact key/measure agreement is not identity, field-definition applicability, period alignment or population approval. Follow comparison revisions, pairs and report origins to both original sources. Selected computed evidence is support-only."}}, nil
}

func comparisonTuple(row Row, keys []ComparisonKey) (string, string) {
	values := make([]string, len(keys))
	for i, key := range keys {
		value, exists := row[key.Field]
		if !exists || value == nil {
			return "", "missing_key"
		}
		text, ok := value.(string)
		if !ok || len(text) > 2048 || !utf8.ValidString(text) {
			return "", "invalid_key"
		}
		if strings.TrimSpace(text) == "" {
			return "", "missing_key"
		}
		switch key.Rule {
		case "trim":
			text = strings.TrimSpace(text)
		case "trailing_parenthesized_digits_v1":
			start := len(text) - key.Digits - 2
			if start < 0 || text[start] != '(' || text[len(text)-1] != ')' {
				return "", "invalid_key"
			}
			text = text[start+1 : len(text)-1]
			for _, digit := range []byte(text) {
				if digit < '0' || digit > '9' {
					return "", "invalid_key"
				}
			}
		}
		values[i] = text
	}
	b, _ := json.Marshal(values)
	return string(b), ""
}
