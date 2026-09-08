package goalwork

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Measure makes numeric interpretation explicit without rewriting source
// records. Measures run after identity joins and cannot become their keys.
// Unit is a declared interpretation, not evidence that the source uses it.
type Measure struct {
	As         string   `json:"as,omitempty"` // Required for output measures; absent for comparison operands.
	Field      string   `json:"field,omitempty"`
	Op         string   `json:"op,omitempty"` // convert (default) | sum_fields
	Fields     []string `json:"fields,omitempty"`
	Format     string   `json:"format"` // decimal_v1 | grouped_decimal_v1
	Unit       string   `json:"unit"`
	Trim       bool     `json:"trim,omitempty"`
	NullTokens []string `json:"nullTokens,omitempty"`
}

var plainMeasure = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
var groupedMeasure = regexp.MustCompile(`^-?(?:[0-9]+|[1-9][0-9]{0,2}(?:,[0-9]{3})+)(?:\.[0-9]+)?$`)
var jsonDecimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func measureRows(rows []Row, specs []Measure) error {
	if len(specs) > 16 {
		return fmt.Errorf("at most 16 explicit measures are supported")
	}
	aliases := map[string]bool{}
	for _, s := range specs {
		if aliases[s.As] {
			return fmt.Errorf("measure aliases must be unique")
		}
		fields, err := validateMeasureDefinition(s)
		if err != nil {
			return err
		}
		if err := requireFields(rows, fields); err != nil {
			return err
		}
		for _, r := range rows {
			if _, ok := r[s.As]; ok {
				return fmt.Errorf("measure cannot overwrite an existing field")
			}
		}
		aliases[s.As] = true
	}
	for _, s := range specs {
		for index, r := range rows {
			v, err := evaluateMeasure(r, s)
			if err != nil {
				return fmt.Errorf("measure %s failed at joined row %d: %w", s.As, index+1, err)
			}
			r[s.As] = v
		}
	}
	return nil
}

func validateMeasureDefinition(s Measure) ([]string, error) {
	if !validRequirementID(s.As) || strings.TrimSpace(s.Unit) == "" || len(s.Unit) > 100 || len(s.NullTokens) > 16 || (s.Format != "decimal_v1" && s.Format != "grouped_decimal_v1") {
		return nil, fmt.Errorf("measure requires an unqualified alias, original qualified field, declared unit and versioned decimal format")
	}
	for _, token := range s.NullTokens {
		if len(token) > 100 {
			return nil, fmt.Errorf("measure null token exceeds 100 bytes")
		}
	}
	return measureFields(s)
}

func measureFields(s Measure) ([]string, error) {
	fields := []string{s.Field}
	switch s.Op {
	case "", "convert":
		if len(s.Fields) != 0 {
			return nil, fmt.Errorf("convert uses field; multiple fields require explicit op=sum_fields")
		}
	case "sum_fields":
		if s.Field != "" || len(s.Fields) < 2 || len(s.Fields) > 32 {
			return nil, fmt.Errorf("sum_fields needs 2–32 distinct original fields and no singular field")
		}
		fields = s.Fields
	default:
		return nil, fmt.Errorf("measure op must be convert or sum_fields")
	}
	seen := map[string]bool{}
	source := ""
	for _, field := range fields {
		id, name, qualified := strings.Cut(field, ".")
		if !qualified || id == "" || name == "" || seen[field] || (source != "" && source != id) {
			return nil, fmt.Errorf("measure fields must be distinct qualified original fields from one source record; no cross-source arithmetic or alias chaining")
		}
		source = id
		seen[field] = true
	}
	return fields, nil
}

// Valid sum_fields inputs all belong to one source record. The full recipe
// retains every contributing field; one field suffices to locate row lineage.
func measureSourceField(s Measure) string {
	if s.Op == "sum_fields" && len(s.Fields) > 0 {
		return s.Fields[0]
	}
	return s.Field
}

func evaluateMeasure(row Row, s Measure) (any, error) {
	if s.Op != "sum_fields" {
		return convertMeasure(row[s.Field], s)
	}
	total := new(big.Rat)
	scale := 0
	missing := false
	for _, field := range s.Fields {
		value, err := convertMeasure(row[field], s)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field, err)
		}
		if value == nil {
			missing = true
			continue
		}
		n, precision, err := exactNumber(value)
		if err != nil {
			return nil, err
		}
		total.Add(total, n)
		if precision > scale {
			scale = precision
		}
	}
	if missing {
		return nil, nil
	}
	return decimalNumber(total, scale), nil
}

func convertMeasure(value any, s Measure) (any, error) {
	if value == nil {
		return nil, nil
	}
	if text, ok := value.(string); ok {
		if len(text) > 256 {
			return nil, fmt.Errorf("numeric token exceeds 256 bytes")
		}
		if s.Trim {
			text = strings.TrimSpace(text)
		}
		for _, token := range s.NullTokens {
			if text == token {
				return nil, nil
			}
		}
		pattern := plainMeasure
		if s.Format == "grouped_decimal_v1" {
			pattern = groupedMeasure
		}
		if !pattern.MatchString(text) {
			return nil, fmt.Errorf("value does not match declared numeric format; no implicit locale, unit stripping or missing-value substitution")
		}
		text = strings.ReplaceAll(text, ",", "")
		// Remove leading zeroes only from the new measure, never the source.
		negative := strings.HasPrefix(text, "-")
		text = strings.TrimPrefix(text, "-")
		parts := strings.SplitN(text, ".", 2)
		parts[0] = strings.TrimLeft(parts[0], "0")
		if parts[0] == "" {
			parts[0] = "0"
		}
		text = strings.Join(parts, ".")
		if negative {
			text = "-" + text
		}
		value = json.Number(text)
	}
	n, scale, err := exactNumber(value)
	if err != nil {
		return nil, err
	}
	return decimalNumber(n, scale), nil
}

// A bounded decimal rational avoids float64 loss for source JSON numbers and
// explicitly parsed measurements. Existing float64 inputs retain only the
// precision their acquisition already preserved; it cannot be reconstructed.
func exactNumber(value any) (*big.Rat, int, error) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = string(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, 0, fmt.Errorf("numeric value is not finite")
		}
		text = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return nil, 0, fmt.Errorf("numeric operation requires JSON numbers or an explicit measure conversion")
	}
	if len(text) > 256 || !jsonDecimal.MatchString(text) {
		return nil, 0, fmt.Errorf("invalid or overlong JSON number")
	}
	mantissa, exponent := text, 0
	if pos := strings.IndexAny(text, "eE"); pos >= 0 {
		mantissa = text[:pos]
		var err error
		exponent, err = strconv.Atoi(text[pos+1:])
		if err != nil || exponent < -1000 || exponent > 1000 {
			return nil, 0, fmt.Errorf("decimal exponent exceeds supported range")
		}
	}
	scale := 0
	if pos := strings.IndexByte(mantissa, '.'); pos >= 0 {
		scale = len(mantissa) - pos - 1
	}
	scale -= exponent
	if scale < 0 {
		scale = 0
	}
	if scale > 1000 {
		return nil, 0, fmt.Errorf("decimal scale exceeds supported range")
	}
	n, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, 0, fmt.Errorf("invalid decimal number")
	}
	return n, scale, nil
}

func decimalNumber(n *big.Rat, scale int) json.Number {
	text := n.FloatString(scale)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "-0" {
		text = "0"
	}
	return json.Number(text)
}
