package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const SourceReportReviewMethod = "independent_model_source_report_v1"

// ReviewInput is created only from the current executed artifact and already
// disclosed evidence. It carries no planner history or prior judgements.
type ReviewInput struct {
	Recipient string                 `json:"recipient"`
	Goal      string                 `json:"goal"`
	Contract  GoalContract           `json:"contract"`
	Artifact  Artifact               `json:"artifact"`
	Evidence  EvidencePacket         `json:"evidence"`
	Analysis  *AnalysisReviewContext `json:"analysis,omitempty"`
}

type ReviewFinding struct {
	Verdict   string   `json:"verdict"` // supported | unsupported | insufficient
	Reason    string   `json:"reason"`
	PacketID  string   `json:"packetId,omitempty"`
	PacketIDs []string `json:"packetIds,omitempty"` // analysis only; source v1 keeps its single-packet contract
}
type OutputReview struct {
	Output  string        `json:"output"`
	Finding ReviewFinding `json:"finding"`
}
type ReviewAssessment struct {
	GoalFit        ReviewFinding          `json:"goalFit"`
	Outputs        []OutputReview         `json:"outputs"`
	AnalysisChecks []AnalysisCheck        `json:"analysisChecks,omitempty"`
	SourceCoverage []SourceCoverageReview `json:"sourceCoverage,omitempty"`
}
type ResultReview struct {
	Method            string           `json:"method"`
	Provider          string           `json:"provider"`
	InputSHA256       string           `json:"inputSha256"`
	ExecutionRevision int              `json:"executionRevision"`
	Revision          int              `json:"revision"`
	Assessment        ReviewAssessment `json:"assessment"`
}

// ReviewAttempt keeps bounded metadata, not selected values or reviewer prose.
type ReviewAttempt struct {
	CompositionID     string `json:"compositionId"`
	ExecutionRevision int    `json:"executionRevision"`
	Revision          int    `json:"revision"`
	InputSHA256       string `json:"inputSha256"`
	EvidenceSHA256    string `json:"evidenceSha256"` // order-independent selection content, not packet presentation
	Provider          string `json:"provider"`
	Status            string `json:"status"` // failed | needs_evidence | supported
}

func (e *Engine) reviewResult(ctx context.Context, id string) error {
	if e.state.Policy.ReviewRecipient == "" || e.deps.Review == nil {
		return fmt.Errorf("result review is disabled by trusted startup policy")
	}
	if len(e.state.Reviews) >= 3 {
		return fmt.Errorf("result review budget exhausted")
	}
	in, err := e.sourceReviewInput(id)
	if e.state.Policy.ReviewAnalyses {
		sourceContext, contextErr := e.reviewSourceContext(ctx, e.state.Artifact)
		if contextErr != nil {
			return contextErr
		}
		if err != nil || len(sourceContext) > 0 || e.state.Contract != nil && e.state.Contract.Coverage == "population" {
			in, err = e.analysisReviewInput(ctx, id, sourceContext)
		}
	}
	if err != nil {
		return err
	}
	inputHash := digest(in)
	evidenceHash := reviewEvidenceHash(in)
	for _, attempt := range e.state.Reviews {
		if attempt.ExecutionRevision == in.Artifact.Evaluation.ExecutionRevision && attempt.EvidenceSHA256 == evidenceHash {
			return fmt.Errorf("this execution and selected evidence were already reviewed; acquire new relevant evidence or correct the result")
		}
	}
	attempt := len(e.state.Reviews)
	e.state.Reviews = append(e.state.Reviews, ReviewAttempt{CompositionID: id, ExecutionRevision: in.Artifact.Evaluation.ExecutionRevision, Revision: e.state.Revision, InputSHA256: inputHash, EvidenceSHA256: evidenceHash, Provider: in.Recipient, Status: "failed"})
	assessment, err := e.deps.Review(ctx, in)
	e.expire()
	if e.state.Status == "expired" {
		return fmt.Errorf("goal expired during review; selected evidence and result discarded")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("separate model review failed; no approval recorded") // external errors may quote selected values
	}
	if digest(in) != inputHash {
		return fmt.Errorf("review adapter changed its input; no approval recorded")
	}
	if err := ValidateReviewAssessment(assessment, in); err != nil {
		return err
	}
	// Detach callback-owned slices before retaining them in session memory.
	b, _ := json.Marshal(assessment)
	_ = json.Unmarshal(b, &assessment)
	review := &ResultReview{Method: SourceReportReviewMethod, Provider: in.Recipient, InputSHA256: inputHash, ExecutionRevision: in.Artifact.Evaluation.ExecutionRevision, Revision: e.state.Revision, Assessment: assessment}
	if in.Analysis != nil {
		review.Method = AnalysisReviewMethod
		if in.Analysis.FullScope != nil {
			review.Method = FullScopeReviewMethod
		}
	}
	e.state.Evaluation.Review = review
	e.state.Evaluation.ReviewReason = "Separate bounded model judgement of source support and original goal fit; not human/publisher approval, field verification or a guarantee of truth."
	if in.Analysis != nil {
		e.state.Evaluation.ReviewReason = "Separate bounded model judgement of relations, periods, measurements, coverage, output support and original goal fit; not canonical identity, population certification, human/publisher approval or a guarantee of truth."
	}
	supported := assessment.GoalFit.Verdict == "supported"
	for _, output := range assessment.Outputs {
		supported = supported && output.Finding.Verdict == "supported"
	}
	for _, check := range assessment.AnalysisChecks {
		supported = supported && check.Finding.Verdict == "supported"
	}
	for _, check := range assessment.SourceCoverage {
		supported = supported && check.Finding.Verdict == "supported"
	}
	e.state.Evaluation.NeedsSemanticReview = !supported
	e.state.Artifact.Evaluation = *e.state.Evaluation
	e.state.Status = "review_required"
	e.state.Reviews[attempt].Status = "needs_evidence"
	if supported {
		e.state.Status = "output_ready"
		e.state.Reviews[attempt].Status = "supported"
	}
	return nil
}

// Canonical cells ignore ordering, packet splitting and review kind. Repacking
// the same disclosure cannot buy another review of the same execution.
func reviewEvidenceHash(in ReviewInput) string {
	cells := map[string]any{}
	packets := []EvidencePacket{in.Evidence}
	if in.Analysis != nil {
		packets = append(packets, in.Analysis.AdditionalEvidence...)
		for _, context := range in.Analysis.SourceContext {
			packets = append(packets, context.Evidence)
		}
	}
	for _, packet := range packets {
		for _, record := range packet.Records {
			for _, field := range packet.Selection.Fields {
				key, _ := json.Marshal([]any{packet.Selection.Observation, packet.Selection.RowsSHA256, record.RetainedRow, field})
				value, exists := record.Values[field]
				cells[string(key)] = []any{exists, value, record.Origins[field]}
			}
		}
	}
	return digest(cells)
}

// ValidateReviewAssessment validates completeness and real references, not
// natural-language entailment or model accuracy. Live adapters use this too.
func ValidateReviewAssessment(a ReviewAssessment, in ReviewInput) error {
	if err := validateSourceCoverage(a, in); err != nil {
		return err
	}
	if in.Analysis == nil && len(a.AnalysisChecks) != 0 {
		return fmt.Errorf("source report v1 cannot supply analysis findings")
	}
	if in.Analysis != nil {
		topics := map[string]bool{}
		for _, check := range a.AnalysisChecks {
			if topics[check.Topic] || !slices.Contains([]string{"relations", "periods", "measurements", "coverage"}, check.Topic) || !validReviewFinding(check.Finding, in) {
				return fmt.Errorf("analysis requires distinct relation, period, measurement and coverage findings with actual citations")
			}
			topics[check.Topic] = true
		}
		if len(topics) != 4 {
			return fmt.Errorf("analysis requires every semantic dimension, not only output support")
		}
	}
	if !validReviewFinding(a.GoalFit, in) || len(a.Outputs) != len(in.Contract.Outputs) {
		return fmt.Errorf("review requires bounded goal fit and every output, each citing the actual evidence packet")
	}
	seen := map[string]bool{}
	for _, output := range a.Outputs {
		if seen[output.Output] || !validReviewFinding(output.Finding, in) || !slices.ContainsFunc(in.Contract.Outputs, func(r OutputRequirement) bool { return r.ID == output.Output }) {
			return fmt.Errorf("review has a missing, duplicated, unknown or invalid output finding")
		}
		seen[output.Output] = true
	}
	return nil
}

func validReviewFinding(f ReviewFinding, in ReviewInput) bool {
	if !slices.Contains([]string{"supported", "unsupported", "insufficient"}, f.Verdict) || strings.TrimSpace(f.Reason) == "" || len(f.Reason) > 2000 || credentialText(f.Reason) {
		return false
	}
	if in.Analysis != nil {
		return validAnalysisCitations(f, in)
	}
	return len(f.PacketIDs) == 0 && f.PacketID == in.Evidence.ID && f.PacketID != ""
}

func (e *Engine) sourceReviewInput(id string) (ReviewInput, error) {
	a := e.state.Artifact
	if a == nil || e.state.Evaluation == nil || id != a.Recipe.ID || e.state.Evaluation.Status != "requirements_met" || e.state.Evaluation.ExecutionRevision < 1 {
		return ReviewInput{}, fmt.Errorf("review requires the current executed composition with all structural requirements met")
	}
	p := a.Recipe
	if len(a.Sources) != 1 || a.Sources[0].Spatial != nil || len(p.Joins)+len(p.Measures)+len(p.Aggregates)+len(p.GroupBy) != 0 || len(p.Select) == 0 || len(p.Select) > 8 || a.Sources[0].RowCount > 20 {
		return ReviewInput{}, fmt.Errorf("source review supports only explicit source-field reports up to 20 retained rows and 8 fields; derived, joined and aggregate results remain review items")
	}
	o := a.Sources[0]
	for i := len(e.state.Evidence) - 1; i >= 0; i-- {
		packet := e.state.Evidence[i]
		if packet.Selection.Observation != o.ID || packet.Selection.RowsSHA256 != o.RowsSHA256 || len(packet.Records) != o.RowCount {
			continue
		}
		// Reconstruct the exact projected result from already-authorized values.
		// Row order and duplicates remain source-record identities, never value sets.
		rows := make([]Row, o.RowCount)
		for _, record := range packet.Records {
			rows[record.RetainedRow-1] = record.Values
		}
		complete := true
		for _, request := range a.Requests {
			for field := range request.Where {
				complete = complete && slices.Contains(packet.Selection.Fields, field)
			}
			for field := range request.WhereIn {
				complete = complete && slices.Contains(packet.Selection.Fields, field)
			}
		}
		if !complete {
			continue
		}
		// This temporary observation describes the authorized column projection,
		// not the full source row. Keep the original revision in the actual input.
		selected := o
		selected.RowsSHA256 = digest(rows)
		projected, _, err := execute(p, map[string][]Row{o.ID: rows}, 20, selected)
		if err != nil || digest(projected) != digest(a.Rows) {
			continue
		}
		in := ReviewInput{Recipient: e.state.Policy.ReviewRecipient, Goal: e.state.Goal, Contract: *e.state.Contract, Artifact: *a, Evidence: packet}
		return detachReviewInput(in)
	}
	return ReviewInput{}, fmt.Errorf("read one evidence packet covering every retained source row and all output/time/selection fields before review; do not shrink the original goal")
}

// Both review kinds share the same privacy, size and callback-isolation seam.
func detachReviewInput(in ReviewInput) (ReviewInput, error) {
	in.Artifact.Evaluation.Review = nil
	in.Artifact.Evaluation.ReviewReason = ""
	in.Artifact.Evaluation.NeedsSemanticReview = true
	in.Artifact.Layouts = nil // unrelated layout history is not review evidence
	b, err := json.Marshal(in)
	if err != nil || len(b) > 96<<10 {
		return ReviewInput{}, fmt.Errorf("review input exceeds 96 KiB; withheld, never truncated")
	}
	if credentialText(string(b)) {
		return ReviewInput{}, fmt.Errorf("review input contains credential material; withheld")
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.UseNumber()
	var detached ReviewInput
	if err := decoder.Decode(&detached); err != nil {
		return ReviewInput{}, err
	}
	return detached, nil
}
