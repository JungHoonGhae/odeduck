package goalwork

import (
	"encoding/json"
	"testing"
)

func TestRowSumCombinesDisjointAgeFieldsWithoutSubstitutingTotalPopulation(t *testing.T) {
	input := map[string][]Row{"a": {{"region": "001", "전체인구": "9999", "65-69세": "1,000", "70-74세": "200", "100세이상": "3"}}, "b": {{"region": "001", "name": "shelter"}}}
	p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.region"}, RightKeys: []string{"region"}}}, Measures: []Measure{{As: "elderly", Op: "sum_fields", Fields: []string{"a.65-69세", "a.70-74세", "a.100세이상"}, Format: "grouped_decimal_v1", Unit: "persons"}}, Select: []string{"elderly", "a.region", "b.name"}}
	rows, _, err := execute(p, input, 10)
	if err != nil || rows[0]["elderly"] != json.Number("1203") || input["a"][0]["전체인구"] != "9999" {
		t.Fatalf("%v %v", rows, err)
	}
	input["a"][0]["70-74세"] = nil
	rows, _, err = execute(p, input, 10)
	if err != nil || rows[0]["elderly"] != nil {
		t.Fatalf("missing age band became zero: %v %v", rows, err)
	}
}

func TestRowSumCannotDoubleCountFieldsMixSourcesOrChainAliases(t *testing.T) {
	input := map[string][]Row{"a": {{"id": "001", "n": "1", "m": "2"}}, "b": {{"id": "001", "n": "3"}}}
	for _, measure := range []Measure{
		{As: "sum", Op: "sum_fields", Fields: []string{"a.n", "a.n"}, Format: "decimal_v1", Unit: "persons"},
		{As: "sum", Op: "sum_fields", Fields: []string{"a.n", "b.n"}, Format: "decimal_v1", Unit: "persons"},
		{As: "sum", Op: "sum_fields", Field: "a.n", Fields: []string{"a.n", "a.m"}, Format: "decimal_v1", Unit: "persons"},
		{As: "sum", Op: "sum_fields", Fields: []string{"a.n"}, Format: "decimal_v1", Unit: "persons"},
		{As: "sum", Op: "sum_fields", Fields: []string{"a.n", "a.unknown"}, Format: "decimal_v1", Unit: "persons"},
		{As: "sum", Op: "sum_fields", Fields: []string{"a.n", "n_alias"}, Format: "decimal_v1", Unit: "persons"},
		{As: "sum", Fields: []string{"a.n", "a.m"}, Format: "decimal_v1", Unit: "persons"},
	} {
		p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.id"}, RightKeys: []string{"id"}}}, Measures: []Measure{{As: "n_alias", Field: "a.n", Format: "decimal_v1", Unit: "persons"}, measure}}
		if _, _, err := execute(p, input, 10); err == nil {
			t.Fatalf("unsafe row sum accepted: %+v", measure)
		}
	}
}

func TestRowSumPreservesPrecisionAndDuplicateSourceGuardThroughAggregation(t *testing.T) {
	p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.id"}, RightKeys: []string{"id"}}}, Measures: []Measure{{As: "row_total", Op: "sum_fields", Fields: []string{"a.n", "a.m"}, Format: "decimal_v1", Unit: "persons"}}, Aggregates: []Aggregate{{As: "total", Op: "sum", Field: "row_total"}}}
	input := map[string][]Row{"a": {{"id": "001", "n": "9007199254740993", "m": "1"}}, "b": {{"id": "001"}}}
	rows, _, err := execute(p, input, 10)
	if err != nil || rows[0]["total"] != json.Number("9007199254740994") {
		t.Fatalf("%v %v", rows, err)
	}
	input["b"] = append(input["b"], Row{"id": "001"})
	if _, _, err := execute(p, input, 10); err == nil {
		t.Fatal("row sum hid duplicated source contribution")
	}
}
