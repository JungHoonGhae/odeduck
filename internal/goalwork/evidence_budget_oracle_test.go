package goalwork_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

// Replays only already-disclosed original cells from the immutable diagnostic.
// This verifies packet feasibility, not new acquisition or computational reuse.
func TestOriginalG4SchoolDisclosureFitsOnePacketWithoutLosingCells(t *testing.T) {
	compressed, err := os.ReadFile("testdata/goalbench-v1/citywide-review-20260908/age-definition-codex.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != "0946308262f2509367e48877de88e2b940986bcd578afd906eb0e0c1a8b9927d" {
		t.Fatal("original diagnostic changed; do not replace the prior evidence")
	}
	var archive struct {
		// Explicit legacy wire decoding: historical context embedded its packet.
		Input struct {
			Goal     string
			Contract goalwork.GoalContract
			Analysis struct {
				SourceContext []struct {
					Source   goalwork.Observation
					Request  goalwork.SampleRequest
					Evidence goalwork.EvidencePacket
				}
			}
		}
		Result goalwork.View `json:"result"`
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := d.Decode(&archive); err != nil {
		t.Fatal(err)
	}
	contextSource := archive.Input.Analysis.SourceContext[0]
	rows := make([]goalwork.Row, 19)
	for _, packet := range archive.Result.Evidence {
		if packet.Selection.Observation != "o2" && packet.Selection.Observation != "o4" {
			continue
		}
		for _, record := range packet.Records {
			position := record.Origins["A"].Ordinal - 22
			if position < 0 || position >= len(rows) || rows[position] != nil || len(record.Missing) != 0 || len(record.Values) != 5 {
				t.Fatal("school evidence no longer consists of 95 distinct explicit original cells")
			}
			rows[position] = record.Values
		}
	}
	e, err := goalwork.Start(archive.Input.Goal, goalwork.Policy{EvidenceRecipient: "codex"}, goalwork.Dependencies{
		Search: func(context.Context, string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: contextSource.Source.PK}}}, nil
		},
		Inspect: func(context.Context, string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: contextSource.Source.PK}, nil
		},
		Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
			return goalwork.Acquired{Rows: rows, Delivery: "FILE", ContentSHA256: contextSource.Source.ContentSHA256, ContractSHA256: contextSource.Source.ContractSHA256, Table: contextSource.Source.Table, Warnings: []string{"Replay of previously disclosed archive cells; not a new file download."}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &archive.Input.Contract})
	advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "archived school evidence", Role: "schools"})
	advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: contextSource.Source.PK})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &contextSource.Request})
	positions := make([]int, len(rows))
	for i, row := range rows {
		if row == nil {
			t.Fatal("an original school row was dropped")
		}
		positions[i] = i + 1
	}
	o := v.Observations[0]
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: positions, Fields: contextSource.Evidence.Selection.Fields}})
	if len(v.Evidence) != 1 || v.Budget.EvidencePacketsRemaining != 7 || len(v.Evidence[0].Records) != 19 || v.Artifact != nil || len(v.Reviews) != 0 || v.Status == "output_ready" {
		t.Fatal("packet feasibility changed disclosure limits or claimed a reviewed result")
	}
	for i, record := range v.Evidence[0].Records {
		if !reflect.DeepEqual(record.Values, rows[i]) || len(record.Missing) != 0 || len(record.Origins) != 5 {
			t.Fatal("packet changed an original value or null")
		}
		for field, address := range record.Origins {
			if address.Observation != o.ID || address.Kind != "worksheet_row" || address.Sheet != contextSource.Source.Table.Sheet || address.Ordinal != i+22 || address.Field != field {
				t.Fatal("packet lost an original worksheet cell address")
			}
		}
	}
	packet, err := json.Marshal(v.Evidence[0])
	if err != nil || len(packet) > 16<<10 {
		t.Fatal("complete school packet exceeds the unchanged limit")
	}
	t.Logf("95 original school cells fit one %d-byte Engine packet; computational reuse and full G4 remain unverified", len(packet))
}

// Replays archived acquisition metadata and independent retained source values,
// not a fresh download or model trial. The original population contract and all
// disclosed cells survive; only duplicate school disclosure is consolidated.
func TestOriginalG4ReviewReusesSchoolCellsWithinDisclosureBudget(t *testing.T) {
	replayOriginalG4Disclosure(t, nil)
}

// Opt-in original CSV byte replay plus archived XLSX/document evidence. No
// network or actual model is called; the full comparison summary is computed.
func TestOriginalG4ReviewIncludesCompleteComparisonWithinDisclosureBudget(t *testing.T) {
	comparison := replayOriginalG4Comparison(t)
	replayOriginalG4Disclosure(t, &comparison)
}

func TestOriginalG4ReviewIncludesSourceExplanationsAndCompleteComparison(t *testing.T) {
	comparison := replayOriginalG4Comparison(t)
	replayOriginalG4Disclosure(t, &comparison, true)
}

func replayOriginalG4Disclosure(t *testing.T, comparison *originalG4ComparisonReplay, explain ...bool) {
	t.Helper()
	compressed, err := os.ReadFile("testdata/goalbench-v1/citywide-review-20260908/age-definition-codex.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	body, err := io.ReadAll(io.LimitReader(z, 1<<20))
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != "0946308262f2509367e48877de88e2b940986bcd578afd906eb0e0c1a8b9927d" {
		t.Fatal("original diagnostic changed")
	}
	var archive struct{ Result goalwork.View }
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err := d.Decode(&archive); err != nil {
		t.Fatal(err)
	}
	old := archive.Result
	contract := *old.Contract
	withExplanations := len(explain) > 0 && explain[0]
	if withExplanations {
		contract.Explanations = []goalwork.ExplanationRequirement{
			{ID: "reference_dates", Topic: "temporal", Basis: "source", Description: "인구 기준일·학교 작성기준일·구역 개편 적용일을 구분한다."},
			{ID: "comparison_limits", Topic: "coverage", Basis: "source", Description: "미대응 지역과 비교 한계를 원천 기록에 연결해 명시한다."},
		}
	}
	var reference referenceSlice
	readReference(t, "education-citywide-reference.json", &reference)
	values := map[string][]goalwork.Row{"o4": make([]goalwork.Row, 19)}
	for i, source := range reference.Sources {
		id := fmt.Sprintf("o%d", i+1)
		if source.ContentSHA256 != old.Observations[i].ContentSHA256 {
			t.Fatal("independent source revision differs from archive")
		}
		for _, record := range source.Records {
			row := record.Values
			if record.Sheet != "" {
				row = record.Cells
			}
			copy := goalwork.Row{}
			for field, value := range row {
				copy[field] = value
			}
			values[id] = append(values[id], copy)
		}
	}
	for _, packet := range old.Evidence {
		for _, record := range packet.Records {
			switch packet.Selection.Observation {
			case "o2", "o4":
				values["o4"][record.Origins["A"].Ordinal-22] = record.Values
			case "o5":
				values["o5"] = append(values["o5"], record.Values)
			}
		}
	}
	if comparison != nil {
		values["o1"] = comparison.Original.Rows
	}
	inspections := map[string]goalwork.Inspection{}
	for _, node := range old.Nodes {
		if node.Inspection != nil {
			inspections[node.Inspection.PK] = *node.Inspection
		}
	}
	var reviewed goalwork.ReviewInput
	e, err := goalwork.Start(old.Goal, old.Policy, goalwork.Dependencies{
		Search: func(_ context.Context, pk string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: pk}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) { return inspections[pk], nil },
		InspectFileVersion: func(_ context.Context, pk, version string) (goalwork.Inspection, error) {
			i := inspections[pk]
			if version == "" {
				return goalwork.Inspection{PK: pk, FileVersions: []dataset.FileVersion{*i.SelectedFileVersion}, FileHistoryCount: 1}, nil
			}
			return i, nil
		},
		Sample: func(_ context.Context, request goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			if comparison != nil && reflect.DeepEqual(request, comparison.ExportRequest) {
				return comparison.Export, nil
			}
			for _, attempt := range old.SampleAttempts {
				if attempt.Request.Reduce == nil && reflect.DeepEqual(attempt.Request, request) {
					for _, o := range old.Observations {
						if o.ID == attempt.ObservationID {
							return goalwork.Acquired{Rows: values[o.ID], Delivery: o.Delivery, ContentSHA256: o.ContentSHA256, ContractSHA256: o.ContractSHA256, Table: o.Table, CSV: o.CSV, Selection: o.Selection, Document: o.Document, Warnings: []string{"Archived metadata and independent reference replay, not a new acquisition."}}, nil
						}
					}
				}
			}
			return goalwork.Acquired{}, fmt.Errorf("request not in the immutable diagnostic")
		},
		Review: func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			packets := 7
			if comparison != nil {
				packets++
				if len(in.Analysis.SourceContext) != 3 || in.Analysis.SourceContext[2].Comparison == nil || len(in.Analysis.SourceContext[2].ComparisonSources) != 2 {
					t.Fatal("full computed comparison and both original revisions did not reach review")
				}
				c := in.Analysis.SourceContext[2]
				if c.Source.Comparison != nil || c.Request.Compare == nil || len(c.Request.Compare.Checks) != 42 || len(c.Comparison.Pairs) != 162 || len(c.Comparison.Records) != 70 || c.ComparisonSources[0].ArtifactSource != "o1" || c.ComparisonSources[1].Source == nil || c.ComparisonSources[1].Source.ID != "o6" || c.ComparisonSources[1].Request == nil || !reflect.DeepEqual(*c.ComparisonSources[1].Request, comparison.ExportRequest) {
					t.Fatal("comparison projection lost recipe, complete positional provenance or original acquisition conditions")
				}
				summary := reviewPacket(t, in, in.Analysis.SourceContext[2].PacketID)
				got := map[string]any{}
				for _, record := range summary.Records {
					got[record.Values["metric"].(string)] = record.Values["value"]
				}
				want := map[string]int{"leftRows": 162, "rightRows": 177, "matchedPairs": 162, "leftOnly": 0, "rightOnly": 15, "leftUnresolved": 0, "rightUnresolved": 0, "checks": 42, "comparisons": 6804, "equal": 6804, "different": 0, "missing": 0, "invalid": 0}
				if !equalJSON(got, want) {
					t.Fatal("review summary omitted or changed comparison denominators")
				}
			}
			if in.Analysis == nil || in.Analysis.FullScope == nil || len(in.EvidencePackets()) != packets || !reflect.DeepEqual(in.Contract, contract) || !reflect.DeepEqual(in.Artifact.Rows, old.Artifact.Rows) || !reflect.DeepEqual(in.Artifact.Unmatched, old.Artifact.Unmatched) {
				t.Fatal("reuse changed original scope, values or unmatched results")
			}
			reviewed = in
			a := supportedAnalysisReview(in)
			a.GoalFit.Verdict = "insufficient"
			if withExplanations {
				if len(in.Artifact.Explanations) != 2 || len(in.Artifact.Evaluation.Explanations) != 0 || in.Contract.Coverage != "population" || in.Goal != old.Goal {
					t.Fatal("source explanations weakened the original goal or became execution disclaimers")
				}
				for _, draft := range in.Artifact.Explanations {
					for _, citation := range draft.Citations {
						packet := reviewPacket(t, in, citation.PacketID)
						if packet.Selection.Observation == "o4" {
							for _, record := range packet.Records {
								if record.RetainedRow == citation.PacketRow && record.Origins[citation.Field].Ordinal != citation.PacketRow+21 {
									t.Fatal("source explanation lost the original school cell address")
								}
							}
						}
					}
					a.Explanations = append(a.Explanations, goalwork.ExplanationReview{Explanation: draft.ID, Finding: a.GoalFit})
				}
			}
			for _, source := range in.Analysis.FullScope.Sources {
				a.SourceCoverage = append(a.SourceCoverage, goalwork.SourceCoverageReview{Observation: source.Observation, Finding: a.GoalFit})
			}
			return a, nil // Boundary/definition applicability still needs actual evidence.
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &contract})
	for _, node := range old.Nodes {
		advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: node.Hit.PK, Role: node.Roles[0]})
	}
	for _, attempt := range old.SampleAttempts {
		r := attempt.Request
		if r.Reduce != nil {
			reduction := *r.Reduce
			r.Reduce = &reduction
			r.Reduce.RowsSHA256 = e.View().Observations[0].RowsSHA256
		} else if attempt.ObservationID != "o4" {
			if r.FileVersion != "" {
				advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: r.PK, FileHistory: true})
			}
			advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: r.PK, FileVersion: r.FileVersion})
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &r})
	}
	for _, packet := range old.Evidence {
		r := packet.Selection
		if r.Observation == "o2" {
			continue
		}
		if r.Observation == "o4" {
			r.Rows = make([]int, 19)
			for i := range r.Rows {
				r.Rows[i] = i + 1
			}
		}
		for _, o := range e.View().Observations {
			if o.ID == r.Observation {
				r.RowsSHA256 = o.RowsSHA256
			}
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &r})
	}
	p := old.Artifact.Recipe
	p.Support = slices.Clone(p.Support)
	p.Support[0].PacketID = e.View().Evidence[6].ID
	if comparison != nil {
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &comparison.ExportRequest})
		left, right := e.View().Observations[0], e.View().Observations[5]
		r := comparison.Comparison
		r.Left.Observation, r.Left.RowsSHA256 = left.ID, left.RowsSHA256
		r.Right.Observation, r.Right.RowsSHA256 = right.ID, right.RowsSHA256
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: left.PK, Delivery: "file", Compare: &r}})
		o := e.View().Observations[6]
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, Fields: []string{"metric", "value"}}})
		p.Support = append(p.Support, goalwork.SupportBinding{PacketID: e.View().Evidence[7].ID, Targets: []string{"o1"}, Purpose: "Compare both retained source revisions and conditions when assessing whether the official series definition applies; numerical agreement alone is not approval."})
	}
	if withExplanations {
		packetFor := func(observation, field string) string {
			for _, packet := range e.View().Evidence {
				if packet.Selection.Observation == observation && slices.Contains(packet.Selection.Fields, field) {
					return packet.ID
				}
			}
			t.Fatalf("original explanation evidence missing: %s.%s", observation, field)
			return ""
		}
		population, school := packetFor("o3", "시군구명"), packetFor("o4", "A")
		p.Explanations = []goalwork.ExplanationDraft{
			{ID: "reference_dates", Text: "인구 원본의 기준연월은 2026-07-31입니다. 학교 표의 작성기준일은 2026-04-01이며, 표 제목의 2026-07-01은 행정개편 시행일입니다. 학교가 7월에 다시 조사되었다는 뜻이 아니므로 같은 날의 관측값으로 해석하지 않습니다.", Citations: []goalwork.EvidenceCitation{{PacketID: packetFor("o1", "기준연월"), PacketRow: 1, Field: "기준연월"}, {PacketID: school, PacketRow: 1, Field: "A"}, {PacketID: school, PacketRow: 2, Field: "A"}}},
			{ID: "comparison_limits", Text: "인구의 서해구와 학교의 서구 표기는 현재 조합에서 대응하지 않았습니다. 두 원천 값을 별도 미대응 표에 남겼으며 같은 지역으로 치환하거나 없는 인구·학교 수를 0으로 채우지 않았습니다. 학교 표는 개편 후 구역을 명시하지만, 이 두 표기의 대응은 이 결과에서 확정하지 않았습니다.", Citations: []goalwork.EvidenceCitation{{PacketID: population, PacketRow: 7, Field: "시군구명"}, {PacketID: school, PacketRow: 1, Field: "A"}, {PacketID: school, PacketRow: 14, Field: "A"}}},
		}
		// Independent archive positions, not a producer-derived expected value.
		if e.View().Evidence[0].Records[6].Values["시군구명"] != "서해구" {
			t.Fatal("original unmatched population position differs")
		}
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	remaining := 1
	if comparison != nil {
		remaining = 0
	}
	if reviewed.Analysis == nil || v.Status == "output_ready" || len(v.Reviews) != 1 || v.Budget.EvidencePacketsRemaining != remaining {
		t.Fatal("original goal was approved or the disclosed packet budget was not preserved")
	}
	// Compare actual original addresses/value presence, not acquisition aliases.
	cells := func(state goalwork.View) map[string]any {
		result := map[string]any{}
		for _, packet := range state.Evidence {
			if packet.Selection.Observation == "o7" {
				continue // Added computed summary, not part of the preserved 157 cells.
			}
			for _, record := range packet.Records {
				for _, field := range packet.Selection.Fields {
					address := record.Origins[field]
					if address.Kind == "worksheet_row" {
						address.Observation = "school-original"
					}
					key, _ := json.Marshal(address)
					value, present := record.Values[field]
					result[string(key)] = []any{value, present, slices.Contains(record.Missing, field)}
				}
			}
		}
		return result
	}
	if !reflect.DeepEqual(cells(old), cells(v)) || len(cells(v)) != 157 {
		t.Fatal("consolidation lost or added original disclosed values, presence or addresses")
	}
	wire, err := json.Marshal(reviewed)
	if err != nil || len(wire) > 96<<10 {
		t.Fatal("review no longer fits the unchanged input budget")
	}
	t.Logf("original G4 replay preserves 157 disclosed cells, 10 matched rows and 2 unmatched tuples in %d packets; complete comparison added=%t; source explanations=%t; review input %d bytes; no new model/G4 completion", len(v.Evidence), comparison != nil, withExplanations, len(wire))
}
