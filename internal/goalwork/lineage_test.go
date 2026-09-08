package goalwork

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func nearestAggregate(t *testing.T, field string, groups []string) View {
	t.Helper()
	e, s := nearestFixture(t, "")
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "sum", Purpose: "sum observed record values", Base: "o3", Joins: []Join{{Right: "o1", LeftKeys: []string{"o3.anchor.id"}, RightKeys: []string{"id"}}}, GroupBy: groups, Measures: []Measure{{As: "n", Field: field, Format: "decimal_v1", Unit: "fixture units"}}, Aggregates: []Aggregate{{As: "total", Op: "sum", Field: "n"}}, Assumptions: []string{"fixture quantities only, not population or spatial meaning approval"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	return e.View()
}

func TestSpatialLineageAllowsDistinctCandidateRecordsWithEqualValues(t *testing.T) {
	// Both candidates have longitude "1", but data records 1001 and 1002
	// are distinct observations; this deliberately tests arithmetic, not a
	// meaningful sum of geographic longitudes or two physical destinations.
	v := nearestAggregate(t, "o3.candidate.lon", nil)
	if len(v.Executions) != 1 || v.Executions[0].Status == "failed" || v.Artifact == nil || v.Artifact.Rows[0]["total"] != json.Number("2") {
		t.Fatalf("distinct records were blocked/collapsed: %+v", v.Gaps)
	}
}

func TestSpatialLineageRejectsRepeatedAnchorButAllowsSeparateGroups(t *testing.T) {
	v := nearestAggregate(t, "o3.anchor.lon", nil)
	if v.Artifact != nil || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "repeats source record") {
		t.Fatalf("repeated anchor not identified: %+v", v.Gaps)
	}
	v = nearestAggregate(t, "o3.anchor.lon", []string{"o3.candidate.id"})
	if v.Artifact == nil || len(v.Artifact.Rows) != 2 {
		t.Fatalf("one contribution per separate group rejected: %+v", v.Gaps)
	}
}

func TestSpatialRowMeasureCannotMixTwoOriginalRecords(t *testing.T) {
	e, s := nearestFixture(t, "")
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "mixed", Purpose: "invalid mixed-origin sum", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.id"}, RightKeys: []string{"anchor.id"}}}, Measures: []Measure{{As: "mixed", Op: "sum_fields", Fields: []string{"o3.anchor.lon", "o3.candidate.lon"}, Format: "decimal_v1", Unit: "fixture"}}, Select: []string{"mixed"}, Assumptions: []string{"same container does not mean same original record"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	if v := e.View(); v.Artifact != nil || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "same original record") {
		t.Fatalf("two source records hidden under one observation: %+v", v.Gaps)
	}
}

func TestSpatialOutputRoleFollowsCopiedFieldOrigin(t *testing.T) {
	e, s := nearestFixture(t, "")
	v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s})
	if err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	c := GoalContract{Outcome: "facility identifier", Region: "fixture", Period: "historic", Coverage: "sample", Roles: []RoleRequirement{{ID: "facility", Description: "facility"}}, Outputs: []OutputRequirement{{ID: "code", Role: "facility", Description: "facility code", Type: "string"}}}
	p := Composition{Base: "o3", Roles: []RoleBinding{{Role: "facility", Observation: "o1"}}, Outputs: []OutputBinding{{Output: "code", Field: "o3.anchor.id"}}}
	result := evaluateRequirements(c, p, []Row{{"o3.anchor.id": "001"}}, v.Observations, v.Nodes)
	if result.Status != "requirements_met" || !result.NeedsSemanticReview {
		t.Fatal("indirect original role was lost or meaning approved")
	}
	// A transit-derived container must not claim that its copied facility code
	// originated in the transit source, even though the outer column prefix fits.
	c.Roles[0].ID, c.Outputs[0].Role = "transit", "transit"
	p.Roles = []RoleBinding{{Role: "transit", Observation: "o3"}}
	if r := evaluateRequirements(c, p, []Row{{"o3.anchor.id": "001"}}, v.Observations, v.Nodes); r.Status != "partial" {
		t.Fatal("copied anchor laundered as transit-origin output")
	}
}

func TestSpatialBackJoinCannotSwitchToAnEqualValuedOriginalRow(t *testing.T) {
	e, s := nearestFixtureRows(t, "", []Row{{"id": "same", "lat": "0", "lon": "0", "n": "5"}, {"id": "same", "lat": "0", "lon": "0", "n": "7"}})
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "back", Purpose: "source-preserving back join", Base: "o3", Joins: []Join{{Right: "o1", LeftKeys: []string{"o3.anchor.id"}, RightKeys: []string{"id"}}}, Select: []string{"o1.id", "o1.n", "o3.anchor.n", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "facility", Field: "o1.id"}, {Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"same observed name does not replace the actual source row"}}
	p.Joins[0].Scopes = []JoinScope{{LeftParts: []string{"o3.anchor.id"}, RightParts: []string{"id"}, Rule: "equal_tokens_v1"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 4 || v.Artifact.Metrics[0].LineageRejectedPairs != 4 {
		t.Fatalf("back join multiplied/switched original records: %+v", v.Gaps)
	}
	if checks := v.Artifact.Metrics[0].ScopeChecks; len(checks) != 1 || checks[0].CandidatePairs != 4 || checks[0].MatchedPairs != 4 {
		t.Fatal("scope counts must exclude the four lineage-rejected key matches")
	}
	for _, row := range v.Artifact.Rows {
		if row["o1.n"] != row["o3.anchor.n"] {
			t.Fatal("back join substituted another anchor's source value")
		}
	}
}

func TestSpatialCandidateBackJoinUsesCSVAddressNotPrefixIndex(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		e, s := nearestFixture(t, "")
		if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
			t.Fatal("nearest setup failed")
		}
		p := Composition{ID: "back", Purpose: "reject unrelated prefix row", Base: "o3", Joins: []Join{{Right: "o2", LeftKeys: []string{"o3.candidate.lat"}, RightKeys: []string{"lat"}}}, Assumptions: []string{"equal latitude is not equal original data record"}}
		if reverse {
			p.Base = "o2"
			p.Joins = []Join{{Right: "o3", LeftKeys: []string{"o2.lat"}, RightKeys: []string{"candidate.lat"}}}
		}
		for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
			if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
				t.Fatal(err)
			}
		}
		v := e.View()
		if v.Artifact != nil || len(v.Executions) != 1 || v.Executions[0].Metrics[0].LineageRejectedPairs != 2 || !strings.Contains(v.Executions[0].Error, "empty full join") {
			t.Fatal("prefix index confused with original CSV record in back join")
		}
	}
}

func TestSpatialCandidateRepeatedAcrossAnchorsCannotBeSummedTwice(t *testing.T) {
	e, s := nearestFixtureRows(t, "", []Row{{"id": "A", "lat": "0", "lon": "0"}, {"id": "B", "lat": "0", "lon": "0"}})
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "sum", Purpose: "reject repeated candidate quantities", Base: "o3", Joins: []Join{{Right: "o1", LeftKeys: []string{"o3.anchor.id"}, RightKeys: []string{"id"}}}, Measures: []Measure{{As: "n", Field: "o3.candidate.lon", Format: "decimal_v1", Unit: "fixture"}}, Aggregates: []Aggregate{{As: "total", Op: "sum", Field: "n"}}, Assumptions: []string{"same candidate source row visited by two anchors remains one original record"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if v.Artifact != nil || len(v.Gaps) != 1 || !strings.Contains(v.Gaps[0].Detail, "repeats source record") {
		t.Fatal("candidate repeated across anchors treated as independent measurements")
	}
}

func TestSpatialResultCanProduceArtifactWithoutArtificialBackJoin(t *testing.T) {
	e, s := nearestFixture(t, "")
	if v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s}); err != nil || len(v.Gaps) != 0 {
		t.Fatal("nearest setup failed")
	}
	p := Composition{ID: "direct", Purpose: "project actual spatial relation", Base: "o3", Select: []string{"o3.anchor.id", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "facility", Field: "o3.anchor.id"}, {Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"the spatial relation already connects two original sources, meaning remains unverified"}}
	for _, d := range []Decision{{Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}} {
		if _, err := e.Advance(context.Background(), e.View().Revision, d); err != nil {
			t.Fatal(err)
		}
	}
	v := e.View()
	if v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 2 || len(v.Artifact.Sources) != 3 || len(v.Artifact.Recipe.Joins) != 0 {
		t.Fatalf("spatial relation required an artificial join or lost origins: %+v", v.Gaps)
	}
}

func TestSpatialLineageRejectsCorruptedVersionAndPairAddresses(t *testing.T) {
	for _, kind := range []string{"rows", "anchor_version", "candidate_version", "pair_count", "anchor_position", "candidate_position", "candidate_csv"} {
		e, s := nearestFixture(t, "")
		v, err := e.Advance(context.Background(), e.View().Revision, Decision{Action: "sample", Sample: &s})
		if err != nil || len(v.Gaps) != 0 {
			t.Fatal("nearest setup failed")
		}
		switch kind {
		case "rows":
			v.Observations[2].RowsSHA256 = strings.Repeat("c", 64)
		case "anchor_version":
			v.Observations[2].Spatial.AnchorRowsSHA256 = strings.Repeat("c", 64)
		case "candidate_version":
			v.Observations[2].Spatial.Scan.SHA256 = strings.Repeat("c", 64)
		case "pair_count":
			v.Observations[2].Spatial.Pairs = nil
		case "anchor_position":
			v.Observations[2].Spatial.Pairs[0].AnchorRow = 999
		case "candidate_position":
			v.Observations[2].Spatial.Pairs[0].CandidateDataRecord = 999999
		case "candidate_csv":
			v.Observations[1].CSV.DataRecords = nil
		}
		if _, _, err := execute(Composition{Base: "o3"}, e.rows, 1000, v.Observations...); err == nil {
			t.Fatalf("corrupt %s lineage accepted", kind)
		}
	}
}
