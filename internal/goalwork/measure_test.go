package goalwork

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMeasuresConvertExplicitlyWithoutChangingSourceOrJoinKeys(t *testing.T) {
	inputs := map[string][]Row{"a": {{"code": "001", "capacity": " 1,234.50 "}}, "b": {{"code": "001"}}}
	p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.code"}, RightKeys: []string{"code"}}}, Measures: []Measure{{As: "capacity_number", Field: "a.capacity", Format: "grouped_decimal_v1", Trim: true, Unit: "persons"}}}
	rows, _, err := execute(p, inputs, 10)
	if err != nil || len(rows) != 1 || rows[0]["capacity_number"] != json.Number("1234.5") || rows[0]["a.capacity"] != " 1,234.50 " || inputs["a"][0]["code"] != "001" {
		t.Fatalf("%+v %v", rows, err)
	}
	p.Joins[0].LeftKeys = []string{"capacity_number"}
	if _, _, err := execute(p, inputs, 10); err == nil {
		t.Fatal("measure used as an identity join key")
	}
}

func TestMeasureFormatsAndMissingValuesAreNotGuessed(t *testing.T) {
	for _, tc := range []struct {
		value   any
		format  string
		trim    bool
		missing []string
		want    any
		bad     bool
	}{
		{"0012.50", "decimal_v1", false, nil, json.Number("12.5"), false},
		{"-0.00", "decimal_v1", false, nil, json.Number("0"), false},
		{"9007199254740993", "decimal_v1", false, nil, json.Number("9007199254740993"), false},
		{"1,234.50", "grouped_decimal_v1", false, nil, json.Number("1234.5"), false},
		{nil, "decimal_v1", false, nil, nil, false},
		{"미상", "decimal_v1", false, []string{"미상"}, nil, false},
		{"", "decimal_v1", false, []string{""}, nil, false},
		{"", "decimal_v1", false, nil, nil, true},
		{" 12 ", "decimal_v1", false, nil, nil, true},
		{"1,234", "decimal_v1", false, nil, nil, true},
		{"12,34", "grouped_decimal_v1", false, nil, nil, true},
		{"1.234,50", "grouped_decimal_v1", false, nil, nil, true},
		{"1e3", "decimal_v1", false, nil, nil, true},
		{"NaN", "decimal_v1", false, nil, nil, true},
		{"12명", "decimal_v1", false, nil, nil, true},
		{true, "decimal_v1", false, nil, nil, true},
		{strings.Repeat("9", 257), "decimal_v1", false, nil, nil, true},
	} {
		t.Run(stringifyMeasureCase(tc.value)+tc.format, func(t *testing.T) {
			p := Composition{Base: "a", Measures: []Measure{{As: "n", Field: "a.value", Format: tc.format, Trim: tc.trim, NullTokens: tc.missing, Unit: "persons"}}}
			rows, _, err := execute(p, map[string][]Row{"a": {{"value": tc.value}}}, 10)
			if tc.bad {
				if err == nil {
					t.Fatal("accepted ambiguous conversion")
				}
				return
			}
			if err != nil || rows[0]["n"] != tc.want {
				t.Fatalf("%v %v want %v", rows, err, tc.want)
			}
		})
	}
}

func stringifyMeasureCase(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 50 {
		b = b[:50]
	}
	return string(b)
}

func TestMeasureContractRejectsAliasOverwriteAndUnprovenLineage(t *testing.T) {
	for _, specs := range [][]Measure{
		{{As: "a.value", Field: "a.value", Format: "decimal_v1", Unit: "persons"}},
		{{As: "n", Field: "a.missing", Format: "decimal_v1", Unit: "persons"}},
		{{As: "n", Field: "a.value", Format: "guess", Unit: "persons"}},
		{{As: "n", Field: "a.value", Format: "decimal_v1"}},
		{{As: "n", Field: "a.value", Format: "decimal_v1", Unit: "persons"}, {As: "n", Field: "a.value", Format: "decimal_v1", Unit: "persons"}},
		{{As: "n", Field: "a.value", Format: "decimal_v1", Unit: "persons"}, {As: "m", Field: "n", Format: "decimal_v1", Unit: "persons"}},
	} {
		if _, _, err := execute(Composition{Base: "a", Measures: specs}, map[string][]Row{"a": {{"value": "1"}}}, 10); err == nil {
			t.Fatalf("invalid contract accepted: %+v", specs)
		}
	}
}

func TestDecimalSumsPreserveLargeIntegersAndFractionalPrecision(t *testing.T) {
	for _, tc := range []struct {
		values []any
		want   string
	}{
		{[]any{json.Number("9007199254740993"), json.Number("1")}, "9007199254740994"},
		{[]any{json.Number("0.1"), json.Number("0.2")}, "0.3"},
		{[]any{json.Number("1.25e2"), json.Number("-0.25")}, "124.75"},
	} {
		var input []Row
		for _, v := range tc.values {
			input = append(input, Row{"n": v})
		}
		p := Composition{Base: "a", Aggregates: []Aggregate{{As: "total", Op: "sum", Field: "a.n"}}}
		rows, _, err := execute(p, map[string][]Row{"a": input}, 10)
		if err != nil || len(rows) != 1 || rows[0]["total"] != json.Number(tc.want) {
			t.Fatalf("%v %v want %s", rows, err, tc.want)
		}
	}
}

func TestSumRejectsRepeatedSourceRecordsAfterOneToManyJoin(t *testing.T) {
	p := Composition{Base: "people", Joins: []Join{{Right: "shelters", LeftKeys: []string{"people.code"}, RightKeys: []string{"code"}}}, Measures: []Measure{{As: "people_number", Field: "people.n", Format: "decimal_v1", Unit: "persons"}}, Aggregates: []Aggregate{{As: "population", Op: "sum", Field: "people_number"}}}
	inputs := map[string][]Row{"people": {{"code": "001", "n": "100"}}, "shelters": {{"code": "001", "name": "A"}, {"code": "001", "name": "B"}}}
	if _, _, err := execute(p, inputs, 10); err == nil || !strings.Contains(err.Error(), "repeats source record") {
		t.Fatalf("inflated population accepted: %v", err)
	}
	// Distinct original rows with equal values are not duplicates to collapse.
	inputs["people"] = []Row{{"code": "001", "n": "100"}, {"code": "001", "n": "100"}}
	inputs["shelters"] = inputs["shelters"][:1]
	rows, _, err := execute(p, inputs, 10)
	if err != nil || rows[0]["population"] != json.Number("200") {
		t.Fatalf("collapsed distinct source rows: %v %v", rows, err)
	}
}
