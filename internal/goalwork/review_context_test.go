package goalwork_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestReviewUsesExplicitSeparateSourceSupportWithoutChangingCalculation(t *testing.T) {
	e := contextReviewEngine(t, "pk", func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		if in.Analysis == nil || len(in.Analysis.SourceContext) != 1 {
			t.Fatal("separately acquired and disclosed supporting record did not reach review")
		}
		c := in.Analysis.SourceContext[0]
		if c.Source.PK != "other" || c.Source.ID != "o2" || len(c.Targets) != 1 || c.Targets[0] != "o1" || reviewPacket(t, in, c.PacketID).Records[0].Values["A"] != "Snapshot 2024-04-01; geography effective 2024-07-01" {
			t.Fatal("support lost actual source, selected wording or target")
		}
		b, _ := json.Marshal(c)
		if !strings.Contains(string(b), `"proposed":true`) || !strings.Contains(string(b), `"purpose":"Check the stated reference dates"`) || strings.Contains(string(b), "UNSELECTED_PRIVATE") || strings.Contains(string(b), "OTHER_REVISION") {
			t.Fatal("support hid the proposed relationship or widened disclosure")
		}
		if len(in.Artifact.Sources) != 1 || len(in.Artifact.Rows) != 1 || in.Artifact.Rows[0]["o1.A"] != "12" {
			t.Fatal("support changed the calculated result or source participation")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.Verdict = "insufficient" // Boundary fixture, not source applicability approval.
		a.GoalFit.PacketIDs = []string{in.Evidence.ID, c.PacketID}
		return a, nil
	})
	readContext(t, e)
	v := e.View()
	p := v.Artifact.Recipe
	p.ID = "with-support"
	// Exercise the same nested wire contract used by CLI and MCP decisions.
	b, _ := json.Marshal(map[string]any{"support": []any{map[string]any{"packetId": v.Evidence[1].ID, "targets": []string{"o1"}, "purpose": "Check the stated reference dates"}}})
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if v.Status != "review_required" || len(v.Reviews) != 1 {
		t.Fatal("proposed support was treated as approval")
	}
}

func TestSupportRequiresExistingEvidenceAndParticipatingTargets(t *testing.T) {
	for _, scenario := range []string{"unknown packet", "duplicate packet", "no target", "unknown target", "nonparticipating target", "duplicate target", "too many targets", "empty purpose", "long purpose", "credential purpose", "participating support", "too many packets", "disabled", "derived support"} {
		t.Run(scenario, func(t *testing.T) {
			variant := "pk"
			if scenario == "disabled" {
				variant = "source only"
			}
			e := contextReviewEngine(t, variant, func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				t.Fatal("invalid support reached the reviewer")
				return goalwork.ReviewAssessment{}, nil
			})
			readContext(t, e)
			v := e.View()
			b := goalwork.SupportBinding{PacketID: v.Evidence[1].ID, Targets: []string{"o1"}, Purpose: "Check applicability of these definitions"}
			switch scenario {
			case "unknown packet":
				b.PacketID = "ep_from_another_goal"
			case "no target":
				b.Targets = nil
			case "unknown target":
				b.Targets = []string{"invented"}
			case "nonparticipating target":
				b.Targets = []string{"o2"}
			case "duplicate target":
				b.Targets = []string{"o1", "o1"}
			case "too many targets":
				b.Targets = make([]string, 9)
			case "empty purpose":
				b.Purpose = " "
			case "long purpose":
				b.Purpose = strings.Repeat("x", 1001)
			case "credential purpose":
				b.Purpose = "serviceKey=fixture-secret-never-return"
			case "participating support":
				b.PacketID = v.Evidence[0].ID
			case "derived support":
				source := v.Observations[1]
				v = advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: source.PK, Delivery: "file", Reduce: &goalwork.SourceReduction{Observation: source.ID, RowsSHA256: source.RowsSHA256, Aggregates: []goalwork.Aggregate{{As: "n", Op: "count", Field: "A"}}}}})
				derived := v.Observations[2]
				v = advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: derived.ID, RowsSHA256: derived.RowsSHA256, Rows: []int{1}, Fields: []string{"n"}}})
				b.PacketID = v.Evidence[2].ID
			}
			p := v.Artifact.Recipe
			p.ID, p.Support = "invalid-support", []goalwork.SupportBinding{b}
			if scenario == "duplicate packet" {
				p.Support = append(p.Support, b)
			}
			if scenario == "too many packets" {
				p.Support = make([]goalwork.SupportBinding, 9)
			}
			before := len(v.Compositions)
			v, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "compose", Composition: &p})
			if err != nil || len(v.Gaps) != 1 || len(v.Compositions) != before || len(v.Reviews) != 0 || v.Artifact.Rows[0]["o1.A"] != "12" {
				t.Fatalf("invalid support replaced the existing result or entered a composition: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestExplicitSameFileSupportIsUniqueAndProposalMutationCannotApprove(t *testing.T) {
	calls := 0
	e := contextReviewEngine(t, "same file", func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		if in.Analysis == nil || len(in.Analysis.SourceContext) != 1 || !in.Analysis.SourceContext[0].Proposed || in.Analysis.SourceContext[0].Purpose != "Check the heading" {
			t.Fatal("explicit packet was duplicated or its proposal was replaced")
		}
		a := supportedAnalysisReview(in)
		in.Analysis.SourceContext[0].Purpose = "forged applicability"
		return a, nil
	})
	readContext(t, e)
	v := e.View()
	p := v.Artifact.Recipe
	p.ID, p.Support = "explicit-header", []goalwork.SupportBinding{{PacketID: v.Evidence[1].ID, Targets: []string{"o1"}, Purpose: "Check the heading"}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	// Mutating the caller-owned recipe cannot alter the stored proposal.
	p.Support[0].Purpose, p.Support[0].Targets[0] = "caller mutation", "invented target"
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if v.Artifact.Recipe.Support[0].Purpose != "Check the heading" || v.Artifact.Recipe.Support[0].Targets[0] != "o1" {
		t.Fatal("stored proposal aliases caller-owned recipe")
	}
	v, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || calls != 1 || len(v.Gaps) != 1 || len(v.Reviews) != 1 || v.Reviews[0].Status != "failed" || v.Status == "output_ready" || v.Artifact.Recipe.Support[0].Purpose != "Check the heading" {
		t.Fatal("reviewer proposal mutation affected source or approval")
	}
}

func TestSupportingEvidenceCannotSubstituteForARequiredDataRole(t *testing.T) {
	e := contextReviewEngine(t, "pk", func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		t.Fatal("support-only role reached semantic review")
		return goalwork.ReviewAssessment{}, nil
	})
	readContext(t, e)
	v := e.View()
	p := v.Artifact.Recipe
	p.ID, p.Support = "support-role", []goalwork.SupportBinding{{PacketID: v.Evidence[1].ID, Targets: []string{"o1"}, Purpose: "Check definition applicability"}}
	p.Roles = []goalwork.RoleBinding{{Role: "r", Observation: "o2"}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	if err != nil || len(v.Gaps) != 1 || v.Evaluation.Status != "partial" || len(v.Artifact.Sources) != 1 || v.Artifact.Rows[0]["o1.A"] != "12" {
		t.Fatal("support substituted for a required role or changed the output")
	}
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	if err != nil || len(v.Gaps) != 2 || len(v.Reviews) != 0 || v.Status == "output_ready" {
		t.Fatalf("partial support-only role was reviewed or approved: %s %+v %v", v.Status, v.Gaps, err)
	}
}

func TestReviewUsesSelectedSameFileContextWithoutJoiningHeaderRows(t *testing.T) {
	calls := 0
	e := contextReviewEngine(t, "same file", func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		if calls == 1 {
			if in.Analysis != nil {
				t.Fatal("unread header was included")
			}
			a := supportedSourceReview(in)
			a.Outputs[0].Output = "n"
			a.GoalFit.Verdict = "insufficient"
			return a, nil
		}
		if in.Analysis == nil || len(in.Analysis.SourceContext) != 1 || len(in.Artifact.Sources) != 1 || len(in.Artifact.Rows) != 1 || len(in.Analysis.Sources) != 1 {
			t.Fatal("header missing or treated as contributing data")
		}
		c := in.Analysis.SourceContext[0]
		if len(c.Targets) != 1 || c.Targets[0] != "o1" || c.Source.ID != "o2" || c.Request.XLSX.Range != "A1:C1" || reviewPacket(t, in, c.PacketID).Records[0].Values["A"] != "Snapshot 2024-04-01; geography effective 2024-07-01" {
			t.Fatal("context lost source request, selected values or target revision")
		}
		b, _ := json.Marshal(in)
		if len(c.Source.Columns) != 1 || len(c.Source.ColumnProfiles) != 1 || len(c.Source.ColumnTypes) != 1 {
			t.Fatal("context metadata was not projected to selected fields")
		}
		if strings.Contains(string(b), "UNSELECTED_PRIVATE") || strings.Contains(string(b), "OTHER_REVISION") {
			t.Fatal("context widened disclosure")
		}
		a := supportedAnalysisReview(in)
		a.GoalFit.PacketIDs = []string{in.Evidence.ID, c.PacketID}
		return a, nil
	})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if v.Status != "review_required" || len(v.Reviews) != 1 {
		t.Fatal("fixture pre-context review did not require missing evidence")
	}
	readContext(t, e)
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
	if calls != 2 || v.Status != "output_ready" || v.Evaluation.Review.Method != goalwork.AnalysisReviewMethod || v.Reviews[0].EvidenceSHA256 == v.Reviews[1].EvidenceSHA256 {
		t.Fatalf("new context did not enter bounded review: %+v", v.Gaps)
	}
}

func contextReviewEngine(t *testing.T, variant string, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	policy := goalwork.Policy{EvidenceRecipient: "claude", ReviewRecipient: "claude", ReviewAnalyses: variant != "source only"}
	e, err := goalwork.Start("Report the source counts and distinguish census date from geography date", policy, goalwork.Dependencies{
		Search: func(_ context.Context, q string) (catalog.Result, error) {
			return catalog.Result{Hits: []catalog.Hit{{PK: q}}}, nil
		},
		Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
			return goalwork.Inspection{PK: pk}, nil
		},
		Sample: func(_ context.Context, s goalwork.SampleRequest, _ goalwork.Inspection) (goalwork.Acquired, error) {
			a := goalwork.Acquired{Rows: []goalwork.Row{{"A": "12"}}, Delivery: "FILE", ContentSHA256: strings.Repeat("a", 64), ContractSHA256: strings.Repeat("b", 64)}
			if s.Member != "" || s.XLSX.Range == "A1:C1" {
				a.Rows = []goalwork.Row{{"A": "Snapshot 2024-04-01; geography effective 2024-07-01", "B": "UNSELECTED_PRIVATE", "C": "OTHER_REVISION"}}
				if variant == "content" {
					a.ContentSHA256 = strings.Repeat("c", 64)
				}
				if variant == "contract" {
					a.ContractSHA256 = strings.Repeat("d", 64)
				}
				if variant == "missing content" {
					a.ContentSHA256 = ""
				}
				if variant == "missing contract" {
					a.ContractSHA256 = ""
				}
				if variant == "delivery" {
					a.Delivery = "REST"
				}
			}
			if variant == "invalid hash" {
				a.ContentSHA256 = strings.Repeat("x", 64)
			}
			return a, nil
		}, Review: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "source counts and dates", Region: "observed source", Period: "declared source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "published count"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	for i, selection := range []string{"A2:A2", "A1:C1"} {
		pk, asset := "source", "table.xlsx"
		if i == 1 && variant == "pk" {
			pk = "other"
		}
		if i == 1 && variant == "asset" {
			asset = "other.xlsx"
		}
		if i == 0 || pk == "other" {
			advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: pk, Role: "r"})
			advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: pk})
		}
		s := goalwork.SampleRequest{PK: pk, Delivery: "file", Asset: asset, XLSX: &dataset.XLSXSelection{Sheet: "table", Range: selection}}
		if i == 1 && variant == "member" {
			s.Member = "other.csv"
			s.XLSX = nil
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	}
	p := goalwork.Composition{ID: "report", Base: "o1", Purpose: "report actual counts", Select: []string{"o1.A"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "n", Field: "o1.A"}}, Assumptions: []string{"source report, not present-day capacity"}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
	advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
	o := e.View().Observations[0]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"A"}}})
	return e
}

func readContext(t *testing.T, e *goalwork.Engine) {
	t.Helper()
	o := e.View().Observations[1]
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"A"}}})
}

func TestReviewContextRequiresMatchingFileRevisionAndAnalysisAuthority(t *testing.T) {
	for _, variant := range []string{"content", "contract", "missing content", "missing contract", "invalid hash", "pk", "asset", "member", "delivery", "source only"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			e := contextReviewEngine(t, variant, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				calls++
				b, _ := json.Marshal(in)
				if in.Analysis != nil || strings.Contains(string(b), "geography effective") {
					t.Fatal("unassociated or unauthorized context reached reviewer")
				}
				a := supportedSourceReview(in)
				a.Outputs[0].Output = "n"
				a.GoalFit.Verdict = "insufficient"
				return a, nil
			})
			readContext(t, e)
			v := advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if calls != 1 || v.Status != "review_required" {
				t.Fatal("unresolved source context was approved")
			}
		})
	}
}

func TestReviewContextCannotBeMutatedOrRepackedForAnotherReview(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "repacked", true: "mutated"}[mutate], func(t *testing.T) {
			calls := 0
			e := contextReviewEngine(t, "same file", func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
				calls++
				a := supportedAnalysisReview(in)
				a.GoalFit.Verdict = "insufficient"
				if mutate {
					reviewPacket(t, in, in.Analysis.SourceContext[0].PacketID).Records[0].Values["A"] = "forged context"
				}
				return a, nil
			})
			readContext(t, e)
			if !mutate {
				o := e.View().Observations[1]
				advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"C"}}})
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if err != nil || calls != 1 || v.Status == "output_ready" || v.Evidence[1].Records[0].Values["A"] == "forged context" {
				t.Fatal("context mutation affected source or approval")
			}
			if mutate {
				if len(v.Gaps) != 1 || v.Reviews[0].Status != "failed" {
					t.Fatal("review input mutation was not rejected")
				}
				return
			}
			// A genuinely new packet containing the same context cells cannot
			// fund a second review of this execution.
			o := v.Observations[1]
			advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"C", "A"}}})
			v, err = e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "review_result", CompositionID: "report"})
			if err != nil || calls != 1 || len(v.Reviews) != 1 || len(v.Gaps) != 1 || v.Status == "output_ready" {
				t.Fatal("repacking bypassed canonical evidence dedup")
			}
		})
	}
}
