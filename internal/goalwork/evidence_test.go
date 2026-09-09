package goalwork_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func evidenceEngine(t *testing.T, recipient string, acquired goalwork.Acquired) *goalwork.Engine {
	t.Helper()
	var policy goalwork.Policy
	encoded, _ := json.Marshal(map[string]any{"evidenceRecipient": recipient})
	if err := json.Unmarshal(encoded, &policy); err != nil {
		t.Fatal(err)
	}
	e, err := goalwork.Start("read observed source evidence", policy, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: "records"}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: "records"}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return acquired, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := resultContract()
	for _, d := range []goalwork.Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "records", Role: "records"}, {Action: "inspect", PK: "records"}, {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}}} {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("acquire: %v %v", v.Gaps, err)
		}
	}
	return e
}

func readEvidence(t *testing.T, e *goalwork.Engine, request map[string]any) goalwork.View {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"action": "read_evidence", "evidence": request})
	var d goalwork.Decision
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	v, err := e.Advance(context.Background(), e.View().Revision, d)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func evidenceJSON(t *testing.T, v goalwork.View) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSelectedEvidenceContainsOnlyRequestedObservedValues(t *testing.T) {
	e := evidenceEngine(t, "claude", goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{
		{"name": "SELECTED_OBSERVATION", "amount": json.Number("9007199254740993"), "unused": "DO_NOT_SHARE"},
		{"name": "UNSELECTED_ROW", "amount": json.Number("0.10")},
	}})
	v := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": e.View().Observations[0].RowsSHA256, "rows": []int{1}, "fields": []string{"name", "amount"}})
	if len(v.Gaps) > 0 {
		t.Fatalf("selected evidence failed: %+v", v.Gaps)
	}
	encoded := evidenceJSON(t, e.PlanningView())
	for _, want := range []string{"SELECTED_OBSERVATION", "9007199254740993", `"retainedRow":1`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("missing exact evidence %s", want)
		}
	}
	for _, forbidden := range []string{"DO_NOT_SHARE", "UNSELECTED_ROW"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("unselected source disclosed: %s", forbidden)
		}
	}
	if v.Status != "exploring" || v.Evaluation != nil {
		t.Fatal("evidence read became semantic approval")
	}
}

func TestSelectedEvidenceRejectsUnauthorizedStaleAndOversizedSelections(t *testing.T) {
	for _, tc := range []struct {
		name, recipient, field, value, hash string
		rows                                []int
		message                             string
	}{
		{"default private", "", "name", "private", "current", []int{1}, "disabled"},
		{"stale", "claude", "name", "private", "old", []int{1}, "rowsSha256"},
		{"outside", "claude", "name", "private", "current", []int{2}, "positions"},
		{"duplicate rows", "claude", "name", "private", "current", []int{1, 1}, "distinct"},
		{"credential field", "claude", "serviceKey", "private", "current", []int{1}, "credential"},
		{"credential Korean field", "claude", "일반인증키", "private", "current", []int{1}, "credential"},
		{"credential URL", "claude", "name", "serviceKey%3Dfixture-secret-key-value", "current", []int{1}, "credential"},
		{"credential JSON", "claude", "name", `{"api_key":"fixture-secret-key-value"}`, "current", []int{1}, "credential"},
		{"oversized cell", "claude", "name", strings.Repeat("x", 2047), "current", []int{1}, "2048"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := evidenceEngine(t, tc.recipient, goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{tc.field: tc.value}}})
			hash := tc.hash
			if hash == "current" {
				hash = e.View().Observations[0].RowsSHA256
			}
			v := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": hash, "rows": tc.rows, "fields": []string{tc.field}})
			if len(v.Evidence) != 0 || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, tc.message) {
				t.Fatalf("invalid disclosure accepted: %+v", v.Gaps)
			}
			if strings.Contains(evidenceJSON(t, v), tc.value) {
				t.Fatal("rejection leaked selected value")
			}
		})
	}
}

func TestSelectedEvidencePreservesNullMissingAndDetachedViews(t *testing.T) {
	e := evidenceEngine(t, "claude", goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{
		{"id": "0001", "zero": json.Number("0"), "false": false, "empty": "", "null": nil},
		{"optional": "UNSELECTED"},
	}})
	v := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": e.View().Observations[0].RowsSHA256, "rows": []int{1}, "fields": []string{"id", "zero", "false", "empty", "null", "optional"}})
	if len(v.Gaps) > 0 {
		t.Fatal(v.Gaps)
	}
	r := v.Evidence[0].Records[0]
	if len(r.Values) != 5 || r.Values["id"] != "0001" || r.Values["zero"] != json.Number("0") || r.Values["false"] != false || r.Values["empty"] != "" || len(r.Missing) != 1 || r.Missing[0] != "optional" {
		t.Fatalf("scalar states changed: %+v", r)
	}
	if value, present := r.Values["null"]; !present || value != nil {
		t.Fatal("null became missing")
	}
	r.Values["id"] = "MUTATED"
	v.Policy.EvidenceRecipient = "gemini"
	if fresh := e.PlanningView(); fresh.Evidence[0].Records[0].Values["id"] != "0001" || fresh.Policy.EvidenceRecipient != "claude" {
		t.Fatal("caller mutated retained evidence/policy")
	}
}

func TestSelectedEvidencePreservesOriginalCSVAndWorksheetAddresses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		acquired goalwork.Acquired
		kind     string
		ordinal  int
	}{
		{"csv", goalwork.Acquired{Delivery: "FILE", CSV: &dataset.CSVProvenance{DataRecords: []int{17, 501}, StartLines: []int{21, 508}}}, "csv_data_record", 501},
		{"xlsx", goalwork.Acquired{Delivery: "FILE", Table: &dataset.TableProvenance{Sheet: "원본", RowNumbers: []int{203, 220}}}, "worksheet_row", 220},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.acquired.Rows = []goalwork.Row{{"A": "first"}, {"A": "selected"}}
			e := evidenceEngine(t, "claude", tc.acquired)
			v := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": e.View().Observations[0].RowsSHA256, "rows": []int{2}, "fields": []string{"A"}})
			if len(v.Gaps) > 0 {
				t.Fatal(v.Gaps)
			}
			a := v.Evidence[0].Records[0].Origins["A"]
			if a.Kind != tc.kind || a.Ordinal != tc.ordinal || a.Observation != "o1" || a.Field != "A" {
				t.Fatalf("original address changed: %+v", a)
			}
		})
	}
}

func TestSelectedEvidenceBudgetsCannotBeRefilledByReadingViews(t *testing.T) {
	for _, cellBytes := range []int{1, 500} {
		t.Run(fmt.Sprint(cellBytes), func(t *testing.T) {
			var rows []goalwork.Row
			for i := 0; i < 200; i++ {
				rows = append(rows, goalwork.Row{"value": strings.Repeat("x", cellBytes)})
			}
			e := evidenceEngine(t, "claude", goalwork.Acquired{Delivery: "REST", Rows: rows})
			var last goalwork.View
			for batch := 0; batch < 9; batch++ {
				positions := make([]int, 20)
				for i := range positions {
					positions[i] = batch*20 + i + 1
				}
				last = readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": e.View().Observations[0].RowsSHA256, "rows": positions, "fields": []string{"value"}})
				_ = e.PlanningView()
				if len(last.Gaps) > 0 {
					break
				}
			}
			if len(last.Gaps) != 1 || !strings.Contains(last.Gaps[0].Detail, "budget exhausted") {
				t.Fatalf("missing cumulative disclosure limit: %+v", last.Gaps)
			}
			if cellBytes == 1 && len(last.Evidence) != 8 {
				t.Fatal("packet count must allow exactly eight bounded packets")
			}
			if cellBytes == 500 && len(last.Evidence) >= 8 {
				t.Fatal("bytes must stop disclosure before packet count")
			}
			if last.Budget.EvidenceBytesRemaining < 0 {
				t.Fatal("negative disclosure budget")
			}
		})
	}
}

func TestSelectedEvidenceExpiresWithItsObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := evidenceEngine(t, "claude", goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"name": "EXPIRED_VALUE"}}})
		v := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": e.View().Observations[0].RowsSHA256, "rows": []int{1}, "fields": []string{"name"}})
		if len(v.Evidence) != 1 {
			t.Fatal("setup failed")
		}
		time.Sleep(time.Hour + time.Second)
		for _, expired := range []goalwork.View{e.View(), e.PlanningView()} {
			if expired.Status != "expired" || len(expired.Evidence) != 0 || strings.Contains(evidenceJSON(t, expired), "EXPIRED_VALUE") {
				t.Fatal("expired evidence retained")
			}
		}
	})
}

func TestSelectedEvidenceRejectsWholeOversizedPacketAndUnobservedFields(t *testing.T) {
	var rows []goalwork.Row
	positions := make([]int, 20)
	for i := range positions {
		positions[i] = i + 1
		rows = append(rows, goalwork.Row{"value": strings.Repeat("x", 1000)})
	}
	for _, tc := range []struct {
		name      string
		fields    []string
		positions []int
		want      string
	}{
		{"packet bytes", []string{"value"}, positions, "16 KiB"},
		{"unobserved field", []string{"invented"}, []int{1}, "observed"},
		{"duplicate field", []string{"value", "value"}, []int{1}, "distinct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := evidenceEngine(t, "claude", goalwork.Acquired{Delivery: "REST", Rows: rows})
			before := e.View().Budget
			v := readEvidence(t, e, map[string]any{"observation": "o1", "rowsSha256": e.View().Observations[0].RowsSHA256, "rows": tc.positions, "fields": tc.fields})
			if len(v.Evidence) != 0 || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, tc.want) || v.Budget.EvidenceBytesRemaining != before.EvidenceBytesRemaining || v.Budget.EvidencePacketsRemaining != before.EvidencePacketsRemaining {
				t.Fatalf("partial disclosure or incorrect rejection: %+v", v.Gaps)
			}
		})
	}
}
