package goalwork

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompositionExecutesWholeThreeSourcePath(t *testing.T) {
	inputs := map[string][]Row{
		"people":   {{"dong": "001", "year": "2025", "people": "100"}},
		"map":      {{"legal": "001", "admin": "A", "year": "2025"}},
		"shelters": {{"area": "A", "name": "쉼터"}},
	}
	recipe := Composition{ID: "mapped", Purpose: "쉼터와 인구를 같은 구역으로 비교", Base: "people", Assumptions: []string{"동일 연도 공식 코드 대응표"},
		Joins:  []Join{{Right: "map", LeftKeys: []string{"people.dong", "people.year"}, RightKeys: []string{"legal", "year"}}, {Right: "shelters", LeftKeys: []string{"map.admin"}, RightKeys: []string{"area"}}},
		Select: []string{"people.people", "shelters.name"}}
	rows, metrics, err := execute(recipe, inputs, 100)
	if err != nil || len(rows) != 1 || len(metrics) != 2 || rows[0]["shelters.name"] != "쉼터" {
		t.Fatalf("%v %v %v", rows, metrics, err)
	}
	inputs["shelters"] = []Row{{"area": "B", "name": "다른 쉼터"}}
	if _, _, err = execute(recipe, inputs, 100); err == nil {
		t.Fatal("empty full join must not succeed")
	}
}

func TestCompositionRejectsFalseEvidenceAndExpansion(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right []Row
		lk, rk      []string
		limit       int
		want        string
	}{
		{"marginal overlap", []Row{{"a": "1", "b": "2"}, {"a": "2", "b": "1"}}, []Row{{"a": "1", "b": "1"}, {"a": "2", "b": "2"}}, []string{"l.a", "l.b"}, []string{"a", "b"}, 10, "empty"},
		{"null", []Row{{"a": nil}}, []Row{{"a": nil}}, []string{"l.a"}, []string{"a"}, 10, "empty"},
		{"leading zeros", []Row{{"a": "01"}}, []Row{{"a": "1"}}, []string{"l.a"}, []string{"a"}, 10, "empty"},
		{"types", []Row{{"a": "1"}}, []Row{{"a": float64(1)}}, []string{"l.a"}, []string{"a"}, 10, "empty"},
		{"unknown field", []Row{{"a": "1"}}, []Row{{"a": "1"}}, []string{"l.missing"}, []string{"a"}, 10, "field"},
		{"expansion", []Row{{"a": "1"}, {"a": "1"}}, []Row{{"a": "1"}, {"a": "1"}}, []string{"l.a"}, []string{"a"}, 3, "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := execute(Composition{Base: "l", Joins: []Join{{Right: "r", LeftKeys: tc.lk, RightKeys: tc.rk}}}, map[string][]Row{"l": tc.left, "r": tc.right}, tc.limit)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestExtractRowsRequiresUnambiguousBoundedTupleCollection(t *testing.T) {
	body := map[string]any{"a": []any{map[string]any{"code": "001", "year": "2025"}}, "b": []any{map[string]any{"code": "002"}}}
	if _, err := ExtractRows(body, "", 10); err == nil {
		t.Fatal("ambiguous arrays accepted")
	}
	rows, err := ExtractRows(body, "/a", 10)
	if err != nil || len(rows) != 1 || rows[0]["code"] != "001" {
		t.Fatalf("%v %v", rows, err)
	}
	if _, err := ExtractRows(body, "/missing", 10); err == nil {
		t.Fatal("missing pointer accepted")
	}
	if _, err := ExtractRows([]any{map[string]any{"a": "1"}, map[string]any{"a": "2"}}, "", 1); err == nil {
		t.Fatal("silent truncation")
	}
}

func TestPairwiseOverlapsCannotProveWholePath(t *testing.T) {
	inputs := map[string][]Row{"a": {{"id": "1"}}, "b": {{"left": "1", "right": "X"}, {"left": "2", "right": "Y"}}, "c": {{"id": "Y"}}}
	if _, _, err := execute(Composition{Base: "b", Joins: []Join{{Right: "c", LeftKeys: []string{"b.right"}, RightKeys: []string{"id"}}}}, inputs, 10); err != nil {
		t.Fatal(err)
	}
	_, _, err := execute(Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.id"}, RightKeys: []string{"left"}}, {Right: "c", LeftKeys: []string{"b.right"}, RightKeys: []string{"id"}}}}, inputs, 10)
	if err == nil {
		t.Fatal("pairwise overlaps falsely proved globally empty join")
	}
}

func TestAggregationRequiresExplicitNumericEvidence(t *testing.T) {
	inputs := map[string][]Row{"a": {{"code": "1", "amount": float64(2)}, {"code": "1", "amount": float64(3)}}, "b": {{"code": "1"}}}
	p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.code"}, RightKeys: []string{"code"}}}, GroupBy: []string{"a.code"}, Aggregates: []Aggregate{{As: "total", Op: "sum", Field: "a.amount"}, {As: "n", Op: "count"}}}
	rows, _, err := execute(p, inputs, 10)
	if err != nil || rows[0]["total"] != json.Number("5") || rows[0]["n"] != float64(2) {
		t.Fatalf("%v %v", rows, err)
	}
	inputs["a"][0]["amount"] = "2"
	if _, _, err = execute(p, inputs, 10); err == nil {
		t.Fatal("implicit string numeric conversion")
	}
}

func TestCountFieldSkipsNullAndRequiresObservedField(t *testing.T) {
	inputs := map[string][]Row{"a": {{"id": "1", "value": nil}, {"id": "1", "value": "present"}}, "b": {{"id": "1"}}}
	p := Composition{Base: "a", Joins: []Join{{Right: "b", LeftKeys: []string{"a.id"}, RightKeys: []string{"id"}}}, Aggregates: []Aggregate{{As: "nonNull", Op: "count", Field: "a.value"}, {As: "all", Op: "count"}}}
	rows, _, err := execute(p, inputs, 10)
	if err != nil || len(rows) != 1 || rows[0]["nonNull"] != float64(1) || rows[0]["all"] != float64(2) {
		t.Fatalf("%v %v", rows, err)
	}
	p.Aggregates[0].Field = "b.invented"
	if _, _, err := execute(p, inputs, 10); err == nil {
		t.Fatal("count accepted fabricated field lineage")
	}
}
