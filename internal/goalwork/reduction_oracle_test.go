package goalwork_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

type citywideContextRecord struct {
	Sheet string
	Row   int
	Cells goalwork.Row
}

// Full retained citywide reference, not a smaller substitute for G4. External
// acquisition replays independent records; reduction and joins use the Engine.
func TestCitywideSourceReductionMatchesIndependentAgeTotalsBeforeSchoolJoin(t *testing.T) {
	checkCitywideReduction(t, false, nil)
}

func TestLiveCitywideSourceReductionMatchesIndependentAgeTotalsBeforeSchoolJoin(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_REDUCTION") != "1" {
		t.Skip("set ODEDUCK_LIVE_REDUCTION=1 for official metadata and actual CSV/XLSX acquisition")
	}
	checkCitywideReduction(t, true, nil)
}

func checkCitywideReduction(t *testing.T, live bool, reviewer *citywideModelReview) goalwork.View {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var ref referenceSlice
	readReference(t, "education-citywide-reference.json", &ref)
	var oracle struct {
		Sources []struct {
			AgeFields      []string `json:"ageFields"`
			HeaderEvidence []citywideContextRecord
		}
		Expected struct {
			Rows []struct {
				District                              string
				ResidentPopulation                    int
				SchoolPublishedTotalExcludingBranch   int
				SchoolPublishedBranchCount            int
				EnrolledPublishedTotalExcludingBranch int
				EnrolledPublishedBranchCount          int
			}
		}
	}
	readReference(t, "education-citywide-reference.json", &oracle)
	contextRange := "A22:AM26"
	contextRows := []int{1, 2, 3, 4, 5}
	contextRecords := append([]citywideContextRecord(nil), oracle.Sources[1].HeaderEvidence...)
	if reviewer != nil && reviewer.tableContext {
		var extra struct {
			PK, ContentSHA256 string
			Records           []citywideContextRecord
		}
		readReference(t, "education-table-context-reference.json", &extra)
		if extra.PK != ref.Sources[1].PK || extra.ContentSHA256 != ref.Sources[1].ContentSHA256 {
			t.Fatal("additional context belongs to a different source revision")
		}
		contextRange = "A22:AM40"
		for _, record := range extra.Records {
			contextRecords = append(contextRecords, record)
			contextRows = append(contextRows, record.Row-21)
		}
	}
	acquired := map[string]goalwork.Acquired{}
	for _, source := range ref.Sources {
		a := goalwork.Acquired{Delivery: "FILE", ContentSHA256: source.ContentSHA256, ContractSHA256: strings.Repeat("b", 64), Warnings: []string{"Development replay of independent retained reference; synthetic contract revision, not a new file download. Original source URL: " + source.URL, "Original observedAt: " + source.ObservedAt}}
		for _, record := range source.Records {
			values := record.Values
			if record.Sheet != "" {
				values = record.Cells
			}
			row := goalwork.Row{}
			for k, v := range values {
				row[k] = v
			}
			a.Rows = append(a.Rows, row)
		}
		acquired[source.PK] = a
	}
	goal := "인천의 지역별 학령인구와 학교 규모를 연결해 비교표를 만들어줘. 학령인구는 만 6~17세로 구분하고, 인구 기준일과 학교 기준일, 행정구역 개편으로 비교할 수 없는 부분을 명시해줘."
	policy := goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: true}
	if reviewer != nil {
		policy.EvidenceRecipient, policy.ReviewRecipient = reviewer.recipient, reviewer.recipient
		policy.ReviewFullScope = reviewer.fullScope
	}
	reviewCalls := 0
	ageDefinition := reviewer != nil && reviewer.ageDefinition
	var documents goalwork.Dependencies
	if ageDefinition && !live {
		documents = citywideDocumentFixture(t, policy)
	}
	deps := goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(ctx context.Context, pk string) (goalwork.Inspection, error) {
			if ageDefinition && pk == "3033304" {
				return documents.Inspect(ctx, pk)
			}
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(ctx context.Context, s goalwork.SampleRequest, inspected goalwork.Inspection) (goalwork.Acquired, error) {
			if s.Document != nil {
				return documents.Sample(ctx, s, inspected)
			}
			if s.Reduce != nil {
				t.Fatal("reduction called provider")
			}
			if s.XLSX != nil && s.XLSX.Range == contextRange {
				a := acquired[s.PK]
				a.Rows = nil
				a.Table = &dataset.TableProvenance{Sheet: s.XLSX.Sheet, Range: s.XLSX.Range}
				for ordinal := 22; ordinal <= contextRecords[len(contextRecords)-1].Row; ordinal++ {
					row := goalwork.Row{}
					if reviewer != nil && reviewer.tableContext {
						// Replay every row of the rectangle, including the intervening
						// table and explicit blank cells. Disclosure remains sparse.
						row = goalwork.Row{"A": nil, "AG": nil, "AK": nil, "AH": nil, "AL": nil}
						for _, record := range ref.Sources[1].Records {
							if record.Row == ordinal {
								for field, value := range record.Cells {
									row[field] = value
								}
							}
						}
					}
					for _, record := range contextRecords {
						if record.Row == ordinal {
							for field, value := range record.Cells {
								row[field] = value
							}
						}
					}
					a.Rows = append(a.Rows, row)
					a.Table.RowNumbers = append(a.Table.RowNumbers, ordinal)
				}
				return a, nil
			}
			return acquired[s.PK], nil
		},
		Review: func(ctx context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
			reviewCalls++
			contexts := 1
			if ageDefinition {
				contexts++
			}
			if in.Goal != goal || in.Analysis == nil || len(in.Analysis.SourceContext) != contexts || len(in.Artifact.Sources) != 3 || len(in.Artifact.Rows) != 10 || len(in.Artifact.Unmatched) != 2 {
				t.Fatal("original goal, calculation or header context changed at review boundary")
			}
			c := in.Analysis.SourceContext[0]
			packet := reviewPacket(t, in, c.PacketID)
			if c.Source.ID != "o4" || len(c.Targets) != 1 || c.Targets[0] != "o2" || c.Request.XLSX.Range != contextRange || len(packet.Records) != len(contextRecords) {
				t.Fatal("header was not associated with its actual school file")
			}
			for i, record := range packet.Records {
				for _, field := range packet.Selection.Fields {
					if value, present := contextRecords[i].Cells[field]; present {
						if record.Values[field] != value {
							t.Fatal("header differs from independently frozen source cells")
						}
						if address := record.Origins[field]; address.Ordinal != contextRecords[i].Row || address.Sheet != contextRecords[i].Sheet {
							t.Fatal("context lost original worksheet addresses")
						}
					}
				}
			}
			if reviewer != nil {
				return reviewer.call(ctx, in)
			}
			a := supportedAnalysisReview(in)
			a.GoalFit = goalwork.ReviewFinding{Verdict: "insufficient", Reason: "Scripted boundary check, not an actual semantic reviewer: unmatched district, age universes and original full comparison remain unresolved", PacketIDs: []string{in.Evidence.ID, c.PacketID}}
			return a, nil
		},
	}
	if live {
		runtime := goalwork.LiveDependencies(fetch.New(), "https://www.data.go.kr", nil, catalog.Searcher{}, policy)
		deps.Inspect, deps.Sample = runtime.Inspect, runtime.Sample
		deps.InspectFileVersion = runtime.InspectFileVersion
	}
	e, err := goalwork.Start(goal, policy, deps)
	if err != nil {
		t.Fatal(err)
	}
	step := func(d goalwork.Decision) goalwork.View {
		t.Helper()
		v, err := e.Advance(ctx, e.View().Revision, d)
		if reviewer != nil && reviewer.observe != nil {
			reviewer.observe(v)
		}
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("%s: %v %+v", d.Action, err, v.Gaps)
		}
		return v
	}
	c := goalwork.GoalContract{Outcome: goal, Region: "인천광역시", Period: "원천별 인구·학교 기준일", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "people", Description: "만 6–17세 주민 인구"}, {ID: "schools", Description: "학교 규모"}}, Outputs: []goalwork.OutputRequirement{{ID: "population", Description: "학령인구", Role: "people", Type: "number"}, {ID: "schools", Description: "학교 수, 분교 별도", Role: "schools", Type: "string"}, {ID: "enrolled", Description: "재적 학생, 분교 별도", Role: "schools", Type: "string"}}}
	if reviewer != nil && reviewer.fullScope {
		c.Coverage = "population"
	}
	schoolFields := []string{"A", "AG", "AK"}
	if reviewer != nil && reviewer.branches {
		c.Outputs = append(c.Outputs, goalwork.OutputRequirement{ID: "branch_schools", Description: "별도 분교 학교 수", Role: "schools", Type: "string"}, goalwork.OutputRequirement{ID: "branch_students", Description: "별도 분교 학생 수", Role: "schools", Type: "string"})
		schoolFields = append(schoolFields, "AH", "AL")
	}
	step(goalwork.Decision{Action: "define", Contract: &c})
	var schoolRequest goalwork.SampleRequest
	for i, role := range []string{"people", "schools"} {
		pk := ref.Sources[i].PK
		step(goalwork.Decision{Action: "search", Query: pk, Role: role})
		historical := live && i == 0 && reviewer != nil && reviewer.historical
		inspected := step(goalwork.Decision{Action: "inspect", PK: pk, FileHistory: historical})
		s := goalwork.SampleRequest{PK: pk, Delivery: "file"}
		if historical {
			u, err := url.Parse(ref.Sources[i].URL)
			if err != nil || u.Query().Get("dataNm") == "" {
				t.Fatal("original reference has no publication name")
			}
			for _, node := range inspected.Nodes {
				if node.Hit.PK == pk && node.Inspection != nil {
					for _, version := range node.Inspection.FileVersions {
						if version.Name == u.Query().Get("dataNm") {
							if s.FileVersion != "" {
								t.Fatal("ambiguous publication label; retain the old oracle")
							}
							s.FileVersion = version.ID
						}
					}
				}
			}
			if s.FileVersion == "" {
				t.Fatal("original publication not in the actual bounded portal history; no fallback")
			}
			inspected = step(goalwork.Decision{Action: "inspect", PK: pk, FileVersion: s.FileVersion})
		}
		if i == 1 && !live {
			s.Asset = "reference.xlsx"
			s.XLSX = &dataset.XLSXSelection{Sheet: "구·군별", Range: "A27:AM37"}
		}
		if live {
			ext := ".csv"
			if i == 1 {
				ext = ".xlsx"
			}
			for _, node := range inspected.Nodes {
				if node.Hit.PK != pk || node.Inspection == nil {
					continue
				}
				for _, asset := range node.Inspection.Assets {
					if strings.HasSuffix(strings.ToLower(asset), ext) {
						if s.Asset != "" {
							t.Fatal("ambiguous inspected asset; no silent selection")
						}
						s.Asset = asset
					}
				}
			}
			if s.Asset == "" {
				t.Fatal("expected official asset not inspected")
			}
			if i == 0 {
				s.Where = map[string]string{"시도명": "인천광역시"}
				s.ScanCSV = historical // Explicit diagnostic variant; old live/reference trials stay unchanged.
			} else {
				s.XLSX = &dataset.XLSXSelection{Sheet: "구·군별", Range: "A27:AM37"}
			}
		}
		observed := step(goalwork.Decision{Action: "sample", Sample: &s}).Observations[i]
		if i == 1 {
			schoolRequest = s
		}
		if observed.ContentSHA256 != ref.Sources[i].ContentSHA256 {
			t.Fatalf("official source revision changed for %s: got %s, expected %s; retain the old oracle and investigate separately", pk, observed.ContentSHA256, ref.Sources[i].ContentSHA256)
		}
		if live {
			t.Logf("official acquisition pk=%s rows=%d contentSha256=%s", pk, observed.RowCount, observed.ContentSHA256)
		}
		if historical {
			if observed.CSV == nil || observed.Selection == nil || !observed.Selection.Exhausted || observed.Selection.ScannedRows != 3619 || observed.Selection.MatchedRows != 162 || observed.Selection.ReturnedRows != 162 || len(observed.CSV.DataRecords) != 162 {
				t.Fatal("historical acquisition did not preserve full scan coverage")
			}
			for row, record := range ref.Sources[i].Records {
				if observed.CSV.DataRecords[row] != record.CSVRecord-1 {
					t.Fatal("historical selection differs from the independent source row positions")
				}
			}
		}
	}
	r := goalwork.SourceReduction{Observation: "o1", RowsSHA256: e.View().Observations[0].RowsSHA256, GroupBy: []string{"시도명", "시군구명", "기준연월"}, Measures: []goalwork.Measure{{As: "ages_6_17", Op: "sum_fields", Fields: oracle.Sources[0].AgeFields, Format: "decimal_v1", Unit: "persons"}}, Aggregates: []goalwork.Aggregate{{As: "residents", Op: "sum", Field: "ages_6_17"}}}
	v := step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: ref.Sources[0].PK, Delivery: "file", Reduce: &r}})
	if len(v.Observations) != 3 || v.Observations[0].RowCount != 162 || v.Observations[2].RowCount != 11 {
		t.Fatal("citywide source was narrowed")
	}
	positions := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	v = step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o3", RowsSHA256: v.Observations[2].RowsSHA256, Rows: positions, Fields: []string{"시군구명", "기준연월", "residents"}}})
	want := map[string]int{}
	for _, expected := range oracle.Expected.Rows {
		want[expected.District] = expected.ResidentPopulation
	}
	total := 0
	for _, record := range v.Evidence[0].Records {
		name := record.Values["시군구명"].(string)
		n, err := strconv.Atoi(string(record.Values["residents"].(json.Number)))
		if err != nil || n != want[name] || record.Values["기준연월"] != "2026-07-31" {
			t.Fatalf("%s differs from independently frozen publisher reconciliation", name)
		}
		delete(want, name)
		total += n
	}
	if total != 304280 || len(want) != 0 {
		t.Fatal("full citywide oracle mismatch")
	}
	if live {
		t.Logf("actual Engine: 162 population records → 11 source groups, total ages 6–17 = %d; all district counts match independent reference", total)
	}
	p := goalwork.Composition{ID: "compare", Base: "o3", Purpose: "retain actual labels, census counts and population date", Joins: []goalwork.Join{{Right: "o2", LeftKeys: []string{"o3.시군구명"}, RightKeys: []string{"A"}}}, Select: []string{"o3.시군구명", "o3.기준연월", "o3.residents", "o2.A", "o2.AG", "o2.AK"}, Roles: []goalwork.RoleBinding{{Role: "people", Observation: "o3"}, {Role: "schools", Observation: "o2"}}, Outputs: []goalwork.OutputBinding{{Output: "population", Field: "o3.residents"}, {Output: "schools", Field: "o2.AG"}, {Output: "enrolled", Field: "o2.AK"}}, Assumptions: []string{"Known label disagreement 서해구/서구 remains unresolved; no global alias or old-to-new geography equivalence. April census and July population do not share reference dates or age universes."}}
	if reviewer != nil {
		// Do not send the scripted test's expected verdict or interpretation to the model.
		p.Assumptions = []string{"Use the selected source revisions and declared district-label normalization. Report source-defined counts; do not infer capacity, identical age universes or contemporaneous observations."}
		if reviewer.branches {
			p.Select = append(p.Select, "o2.AH", "o2.AL")
			p.Outputs = append(p.Outputs, goalwork.OutputBinding{Output: "branch_schools", Field: "o2.AH"}, goalwork.OutputBinding{Output: "branch_students", Field: "o2.AL"})
		}
	}
	step(goalwork.Decision{Action: "compose", Composition: &p})
	v = step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if len(v.Artifact.Rows) != 5 {
		t.Fatal("original whitespace-bearing school labels were silently rewritten")
	}
	p.ID = "compare-trimmed"
	p.Joins[0].Normalization = "trim"
	step(goalwork.Decision{Action: "compose", Composition: &p})
	v = step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.Status != "review_required" || !v.Evaluation.NeedsSemanticReview || len(v.Artifact.Rows) != 10 || v.Artifact.Metrics[0].LeftRows != 11 || v.Artifact.Metrics[0].MatchedLeftRows != 10 {
		t.Fatalf("unresolved district mismatch hidden or goal incorrectly completed: status=%s review=%t rows=%d metrics=%+v", v.Status, v.Evaluation.NeedsSemanticReview, len(v.Artifact.Rows), v.Artifact.Metrics)
	}
	m := v.Artifact.Metrics[0]
	if len(m.UnmatchedLeft) != 1 || len(m.UnmatchedLeft[0]) != 1 || len(m.UnmatchedRight) != 1 || m.UnmatchedRight[0] != 9 {
		t.Fatalf("excluded districts lack retained source addresses: %+v", m)
	}
	position := m.UnmatchedLeft[0]["o3"]
	if position < 1 || position > 11 || v.Evidence[0].Records[position-1].Values["시군구명"] != "서해구" {
		t.Fatal("excluded population group cannot be traced to the independent district label")
	}
	schoolRows, unmatchedRecord := m.UnmatchedRight, 0
	if ageDefinition {
		// Include the unmatched row in the one complete school disclosure, rather
		// than consuming another packet for the same cells later in this run.
		schoolRows, unmatchedRecord = positions, m.UnmatchedRight[0]-1
	}
	v = step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.Observations[1].RowsSHA256, Rows: schoolRows, Fields: schoolFields}})
	if v.Evidence[1].Records[unmatchedRecord].Values["A"] != " 서구 " || v.Evidence[1].Records[unmatchedRecord].Values["AG"] != "111" {
		t.Fatal("excluded school row was hidden, renamed or confused with pre-split geography")
	}
	for _, row := range v.Artifact.Rows {
		name := row["o3.시군구명"].(string)
		for _, expected := range oracle.Expected.Rows {
			if reviewer != nil && reviewer.branches && expected.District == name && (row["o2.AH"] != strconv.Itoa(expected.SchoolPublishedBranchCount) || row["o2.AL"] != strconv.Itoa(expected.EnrolledPublishedBranchCount)) {
				t.Fatalf("branch counts differ from independent district reference: %s", name)
			}
			if expected.District == name && (row["o2.AG"] != strconv.Itoa(expected.SchoolPublishedTotalExcludingBranch) || row["o2.AK"] != strconv.Itoa(expected.EnrolledPublishedTotalExcludingBranch) || row["o3.residents"] != json.Number(strconv.Itoa(expected.ResidentPopulation))) {
				t.Fatalf("school count multiplied by population source rows: %s", name)
			}
		}
	}
	// Context uses a separate actual file selection, not a join against headers.
	p.ID, p.ReportUnmatched = "compare-with-unmatched", p.Select
	step(goalwork.Decision{Action: "compose", Composition: &p})
	v = step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if len(v.Artifact.Unmatched) != 2 || v.Artifact.Unmatched[0].Values["o3.시군구명"] != "서해구" || v.Artifact.Unmatched[1].Values["o2.A"] != " 서구 " {
		t.Fatal("unmatched original labels are not present in the actual result")
	}
	for _, expected := range oracle.Expected.Rows {
		if expected.District == "서해구" {
			if reviewer != nil && reviewer.branches && (v.Artifact.Unmatched[1].Values["o2.AH"] != strconv.Itoa(expected.SchoolPublishedBranchCount) || v.Artifact.Unmatched[1].Values["o2.AL"] != strconv.Itoa(expected.EnrolledPublishedBranchCount)) {
				t.Fatal("unmatched school branch counts differ from independent reference")
			}
			if v.Artifact.Unmatched[0].Values["o3.residents"] != json.Number(strconv.Itoa(expected.ResidentPopulation)) || v.Artifact.Unmatched[1].Values["o2.AG"] != strconv.Itoa(expected.SchoolPublishedTotalExcludingBranch) || v.Artifact.Unmatched[1].Values["o2.AK"] != strconv.Itoa(expected.EnrolledPublishedTotalExcludingBranch) {
				t.Fatal("unmatched source values differ from independent reference")
			}
		}
	}
	// Existing diagnostic variants retain their separate unmatched packet.
	// The new variant retains the same cells in its complete school packet.
	schoolRequest.XLSX = &dataset.XLSXSelection{Sheet: "구·군별", Range: contextRange}
	v = step(goalwork.Decision{Action: "sample", Sample: &schoolRequest})
	v = step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o4", RowsSHA256: v.Observations[3].RowsSHA256, Rows: contextRows, Fields: []string{"A", "AG", "AK", "AH", "AL"}}})
	if !ageDefinition {
		step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o2", RowsSHA256: v.Observations[1].RowsSHA256, Rows: positions, Fields: schoolFields}})
	}
	fields := append([]string{"시도명", "시군구명", "기준연월"}, oracle.Sources[0].AgeFields...)
	for start := 0; start < len(fields); start += 8 {
		step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: "o1", RowsSHA256: v.Observations[0].RowsSHA256, Rows: []int{1}, Fields: fields[start:min(start+8, len(fields))]}})
	}
	if ageDefinition {
		// Explicit development diagnostic source hint, never autonomous discovery.
		step(goalwork.Decision{Action: "search", Query: "3033304", Role: "people"})
		step(goalwork.Decision{Action: "inspect", PK: "3033304"})
		v = step(goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "3033304", Delivery: "document", Document: &dataset.DocumentSelection{ReferenceID: "kosis-answer-22124"}}})
		doc := v.Observations[len(v.Observations)-1]
		v = step(goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: doc.ID, RowsSHA256: doc.RowsSHA256, Rows: []int{1, 2}, Fields: []string{"text"}}})
		p.ID = "compare-with-definition"
		p.Support = []goalwork.SupportBinding{{PacketID: v.Evidence[len(v.Evidence)-1].ID, Targets: []string{"o1"}, Purpose: "Check whether the registered official answer's age definition applies to this population source revision; distinguish series documentation from a file-specific declaration."}}
		step(goalwork.Decision{Action: "compose", Composition: &p})
		step(goalwork.Decision{Action: "execute", CompositionID: p.ID})
	}
	v, err = e.Advance(ctx, e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if reviewer != nil {
		return v // Retain model errors/gaps as well as successful typed findings.
	}
	if len(v.Gaps) != 0 || reviewCalls != 1 || len(v.Evidence) != 8 || v.Status != "review_required" || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod {
		t.Fatal("same-file context bypassed review or changed disclosure limits")
	}
	if live {
		t.Log("actual Engine: exact labels 5 pairs → explicit trim 10 pairs → 10 pairs plus separately reported population/school unmatched tuples; actual A22:AM26 headers and all result values reach scripted review using 8 evidence packets; review_required, NOT model calibration, G4 completion or autonomous discovery")
	}
	return v
}
