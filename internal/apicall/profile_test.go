package apicall

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestProfileBodyPreservesIdentifiersAndCountsJoinEvidence(t *testing.T) {
	body := map[string]any{
		"response": map[string]any{
			"items": []any{
				map[string]any{"lawdCd": "00110", "dealYm": "202601"},
				map[string]any{"lawdCd": "00110", "dealYm": "202602"},
				map[string]any{"lawdCd": "00220", "dealYm": nil},
			},
		},
	}
	profile, err := ProfileBody(body, []string{"lawdCd", "dealYm", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	code := profile.Fields[0]
	if !code.Matched || code.Count != 3 || code.DistinctCount != 2 || code.DuplicateCount != 1 {
		t.Fatalf("code profile = %+v", code)
	}
	if len(code.Values) != 2 || code.Values[0] != "00110" {
		t.Fatalf("leading-zero values = %+v", code.Values)
	}
	month := profile.Fields[1]
	if month.Count != 2 || month.NullCount != 1 || month.DistinctCount != 2 {
		t.Fatalf("month profile = %+v", month)
	}
	if profile.Fields[2].Matched {
		t.Fatalf("missing field = %+v", profile.Fields[2])
	}
	if len(profile.EvidenceHash) != 64 {
		t.Fatalf("evidence hash = %q", profile.EvidenceHash)
	}
	again, err := ProfileBody(body, []string{"lawdCd", "dealYm", "missing"})
	if err != nil || again.EvidenceHash != profile.EvidenceHash {
		t.Fatalf("evidence hash must be deterministic: first=%q second=%q err=%v", profile.EvidenceHash, again.EvidenceHash, err)
	}
	changed, err := ProfileBody(map[string]any{"lawdCd": "99999"}, []string{"lawdCd", "dealYm", "missing"})
	if err != nil || changed.EvidenceHash == profile.EvidenceHash {
		t.Fatalf("different bounded evidence must produce a different hash: first=%q changed=%q err=%v", profile.EvidenceHash, changed.EvidenceHash, err)
	}
}

func TestProfileBodyPreservesWhitespaceInRawStringValues(t *testing.T) {
	profile, err := ProfileBody(map[string]any{"items": []any{
		map[string]any{"code": "00110"},
		map[string]any{"code": "00110 "},
	}}, []string{"code"})
	if err != nil {
		t.Fatal(err)
	}
	got := profile.Fields[0]
	if got.DistinctCount != 2 || len(got.Values) != 2 || got.Values[1] != "00110 " {
		t.Fatalf("raw string whitespace was normalized: %+v", got)
	}
}

func TestProfileBodySupportsDottedPathSuffix(t *testing.T) {
	body := map[string]any{
		"header": map[string]any{"code": "00"},
		"body":   map[string]any{"items": []any{map[string]any{"code": "A"}}},
	}
	profile, err := ProfileBody(body, []string{"items.code"})
	if err != nil {
		t.Fatal(err)
	}
	if got := profile.Fields[0]; got.Count != 1 || len(got.Values) != 1 || got.Values[0] != "A" {
		t.Fatalf("dotted profile = %+v", got)
	}
}

func TestProfileBodyDoesNotMergeAmbiguousLeafPaths(t *testing.T) {
	body := map[string]any{
		"header": map[string]any{"code": "00"},
		"body":   map[string]any{"items": []any{map[string]any{"code": "A"}}},
	}
	profile, err := ProfileBody(body, []string{"code"})
	if err != nil {
		t.Fatal(err)
	}
	got := profile.Fields[0]
	if !got.Matched || !got.Ambiguous || got.PathCount != 2 || got.Count != 0 || len(got.Values) != 0 {
		t.Fatalf("ambiguous leaf must not expose merged evidence: %+v", got)
	}
}

func TestProfileBodyFlattensScalarArraysUnderOwningField(t *testing.T) {
	profile, err := ProfileBody(map[string]any{"codes": []any{"001", "002", "001", nil}}, []string{"codes"})
	if err != nil {
		t.Fatal(err)
	}
	got := profile.Fields[0]
	if !got.Matched || got.Ambiguous || got.Count != 3 || got.NullCount != 1 ||
		got.DistinctCount != 2 || got.DuplicateCount != 1 {
		t.Fatalf("scalar array profile = %+v", got)
	}
}

func TestProfileBodyBoundsFieldsAndValueSamples(t *testing.T) {
	var fields []string
	for i := 0; i <= MaxProfileFields; i++ {
		fields = append(fields, fmt.Sprintf("field%d", i))
	}
	if _, err := ProfileBody(nil, fields); err == nil {
		t.Fatal("too many fields should fail before an API call is spent")
	}

	var items []any
	for i := 0; i < maxProfileValues+5; i++ {
		items = append(items, map[string]any{"id": fmt.Sprintf("%03d", i)})
	}
	profile, err := ProfileBody(map[string]any{"items": items}, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	got := profile.Fields[0]
	if got.DistinctCount != maxProfileValues+5 || len(got.Values) != maxProfileValues || !got.ValuesTruncated {
		t.Fatalf("bounded values = %+v", got)
	}
}

func TestProfileEvidenceHashCoversValuesBeyondReturnedPreviewAndFrequencies(t *testing.T) {
	firstRows, secondRows := make([]any, 0, maxProfileValues+1), make([]any, 0, maxProfileValues+1)
	for i := 0; i < maxProfileValues; i++ {
		row := map[string]any{"id": fmt.Sprintf("%03d", i)}
		firstRows = append(firstRows, row)
		secondRows = append(secondRows, row)
	}
	firstRows = append(firstRows, map[string]any{"id": "tail-a"})
	secondRows = append(secondRows, map[string]any{"id": "tail-b"})
	first, err := ProfileBody(firstRows, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProfileBody(secondRows, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	if first.EvidenceHash == second.EvidenceHash {
		t.Fatal("different values beyond the returned preview shared an evidence hash")
	}

	left, _ := ProfileBody([]any{map[string]any{"id": "A"}, map[string]any{"id": "A"}, map[string]any{"id": "B"}}, []string{"id"})
	right, _ := ProfileBody([]any{map[string]any{"id": "A"}, map[string]any{"id": "B"}, map[string]any{"id": "B"}}, []string{"id"})
	if left.EvidenceHash == right.EvidenceHash {
		t.Fatal("different value frequencies shared an evidence hash")
	}
}

func TestProfileBodyHandlesDecodedScalarTypes(t *testing.T) {
	body := map[string]any{"items": []any{
		map[string]any{"value": json.Number("9007199254740993")},
		map[string]any{"value": 7},
		map[string]any{"value": int64(8)},
		map[string]any{"value": float32(1.5)},
		map[string]any{"value": float64(2.5)},
		map[string]any{"value": true},
	}}
	profile, err := ProfileBody(body, []string{"value"})
	if err != nil {
		t.Fatal(err)
	}
	got := profile.Fields[0]
	if got.Count != 6 || got.DistinctCount != 6 || got.Values[0] != "9007199254740993" || got.Values[5] != "true" {
		t.Fatalf("scalar profile = %+v", got)
	}
}
