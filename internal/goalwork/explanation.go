package goalwork

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// ExplanationRequirement is a requested evidence explanation, not a pretend
// data column or another dataset role. Execution explanations are engine-written;
// source explanations additionally need a cited proposal and separate review.
type ExplanationRequirement struct {
	ID          string `json:"id"`
	Topic       string `json:"topic"` // provenance, temporal, coverage, identity, measurement
	Description string `json:"description"`
	Basis       string `json:"basis,omitempty"` // execution (default) or source
}

type EvidenceExplanation struct {
	ID           string   `json:"id"`
	Topic        string   `json:"topic"`
	Text         string   `json:"text"`
	Observations []string `json:"observations"`
}

// ExplanationDraft is a proposed answer to a source-based explanation
// requirement, not publisher text or a semantic approval. Its citations resolve
// through View.Evidence (or ReviewInput.EvidencePackets) to original addresses.
type ExplanationDraft struct {
	ID        string             `json:"id"`
	Text      string             `json:"text"`
	Citations []EvidenceCitation `json:"citations"`
}

type EvidenceCitation struct {
	PacketID  string `json:"packetId"`
	PacketRow int    `json:"packetRow"` // packet's retained row, not a physical source row
	Field     string `json:"field"`
}

func (e *Engine) validateExplanationDrafts(ctx context.Context, p Composition) error {
	if len(p.Explanations) == 0 {
		return nil // Missing required drafts leave an executable partial result.
	}
	if e.state.Policy.EvidenceRecipient == "" || len(p.Explanations) > 8 {
		return fmt.Errorf("source explanations require enabled disclosure and at most 8 drafts")
	}
	seen := map[string]bool{}
	for _, draft := range p.Explanations {
		if seen[draft.ID] || !slices.ContainsFunc(e.state.Contract.Explanations, func(r ExplanationRequirement) bool { return r.ID == draft.ID && r.Basis == "source" }) {
			return fmt.Errorf("source explanations need distinct IDs from source-based contract requirements")
		}
		seen[draft.ID] = true
		if !utf8.ValidString(draft.Text) || strings.TrimSpace(draft.Text) == "" || len(draft.Text) > 2000 || credentialText(draft.Text) {
			return fmt.Errorf("source explanation text needs 1–2000 UTF-8 bytes without credential material; it remains a proposal")
		}
	}
	if err := validateExplanationCitations(p.Explanations, e.state.Evidence); err != nil {
		return err
	}
	used := map[string]bool{}
	for _, draft := range p.Explanations {
		for _, citation := range draft.Citations {
			used[citation.PacketID] = true
		}
	}
	var packets []EvidencePacket
	for _, packet := range e.state.Evidence {
		if used[packet.ID] {
			packets = append(packets, packet)
		}
	}
	_, _, err := e.indexReviewDisclosure(ctx, packets)
	return err
}

// A citation may describe selected missing/null values, but never undisclosed
// cells. Revision/value validation is owned by the Engine's disclosure index.
func validateExplanationCitations(drafts []ExplanationDraft, packets []EvidencePacket) error {
	byID := map[string]EvidencePacket{}
	for _, packet := range packets {
		byID[packet.ID] = packet
	}
	for _, draft := range drafts {
		if len(draft.Citations) < 1 || len(draft.Citations) > 16 {
			return fmt.Errorf("source explanation requires 1–16 distinct selected cell citations")
		}
		seen := map[EvidenceCitation]bool{}
		for _, citation := range draft.Citations {
			packet, exists := byID[citation.PacketID]
			if !exists || seen[citation] || !slices.Contains(packet.Selection.Fields, citation.Field) {
				return fmt.Errorf("source explanation cites an unavailable, duplicated or undisclosed cell; use participating or supported evidence for review")
			}
			seen[citation] = true
			found := false
			for _, record := range packet.Records {
				if record.RetainedRow == citation.PacketRow {
					_, present := record.Values[citation.Field]
					found = present != slices.Contains(record.Missing, citation.Field)
				}
			}
			if !found {
				return fmt.Errorf("source explanation citation needs an actual selected packet row and field, including its presence state")
			}
		}
	}
	return nil
}

func validateExplanationReviews(a ReviewAssessment, in ReviewInput) error {
	required := map[string]bool{}
	for _, requirement := range in.Contract.Explanations {
		if requirement.Basis == "source" {
			required[requirement.ID] = true
		}
	}
	if len(required) != len(a.Explanations) || len(required) != len(in.Artifact.Explanations) {
		return fmt.Errorf("review requires every source-based explanation and its finding, separate from execution notes")
	}
	if len(required) == 0 {
		return nil
	}
	if in.Analysis == nil {
		return fmt.Errorf("source explanations need analysis review; source report v1 cannot approve them")
	}
	if err := validateExplanationCitations(in.Artifact.Explanations, in.EvidencePackets()); err != nil {
		return err
	}
	drafts := map[string]ExplanationDraft{}
	for _, draft := range in.Artifact.Explanations {
		if _, duplicate := drafts[draft.ID]; !required[draft.ID] || duplicate {
			return fmt.Errorf("review requires distinct source explanation drafts matching the contract")
		}
		drafts[draft.ID] = draft
	}
	seen := map[string]bool{}
	for _, review := range a.Explanations {
		if seen[review.Explanation] || !required[review.Explanation] || !validReviewFinding(review.Finding, in) {
			return fmt.Errorf("source explanation review has a missing, duplicated, unknown or invalid finding")
		}
		seen[review.Explanation] = true
		if review.Finding.Verdict == "supported" {
			for _, citation := range drafts[review.Explanation].Citations {
				if !slices.Contains(review.Finding.PacketIDs, citation.PacketID) {
					return fmt.Errorf("supported explanation must cite every packet proposed as its basis")
				}
			}
		}
	}
	return nil
}

func validExplanationTopic(topic string) bool {
	switch topic {
	case "provenance", "temporal", "coverage", "identity", "measurement":
		return true
	}
	return false
}

func explainEvidence(c GoalContract, p Composition, sources []Observation, rowCount int, temporal TemporalEvaluation, metrics []JoinMetric) []EvidenceExplanation {
	var ids []string
	inputRows := 0
	for _, source := range sources {
		ids = append(ids, source.ID)
		if source.Spatial == nil && source.Reduction == nil {
			inputRows += source.RowCount
		}
	}
	var out []EvidenceExplanation
	for _, requirement := range c.Explanations {
		if requirement.Basis == "source" {
			continue // Source answers are explicit artifact drafts, not generic execution notes.
		}
		report := EvidenceExplanation{ID: requirement.ID, Topic: requirement.Topic, Observations: append([]string(nil), ids...)}
		switch requirement.Topic {
		case "provenance":
			report.Text = fmt.Sprintf("%d개 원천 관측을 사용했습니다. artifact.sources의 PK·제공형·요청/계약/내용/행 해시·observedAt과 artifact.requests를 추적하세요. 과거 원천 bytes를 보관하지 않으므로 재호출 결과가 달라질 수 있습니다.", len(sources))
		case "coverage":
			report.Text = fmt.Sprintf("원천 %d개에서 읽은 표본 %d행으로 결과 %d행을 만들었습니다. 원천 행 수의 합은 동일 모집단의 크기가 아닙니다. 대표성·전체 coverage·미발견 대상의 부재는 증명하지 않습니다.", len(sources), inputRows, rowCount)
			if len(p.Joins) > 0 {
				report.Text += " inner join은 불일치 행을 제외합니다."
				for _, metric := range metrics {
					report.Text += fmt.Sprintf(" %s 결합 단계에서 왼쪽 입력 %d개·오른쪽 원천 기록 %d개가 대응하지 않았습니다.", metric.Right, len(metric.UnmatchedLeft), len(metric.UnmatchedRight))
				}
				report.Text += " metrics의 unmatchedLeft/unmatchedRight는 단계별 보유 행 주소입니다. 선택 근거 읽기로 원문을 확인할 수 있으며, 단계 간 중복·후속 제외가 가능하므로 합산한 고유 대상 수나 사건 부재로 해석하지 않습니다."
				if len(p.ReportUnmatched) > 0 {
					report.Text += " 요청한 미대응 원천 값은 artifact.unmatched의 별도 표에 보존했습니다. 이 값은 결합·시간·집계 계산에 추가되지 않으며, 상대 기록이 없다고 원천 값 0이나 null을 만들지 않습니다."
				}
			}
		case "temporal":
			if temporal.Status == "checked" {
				report.Text = fmt.Sprintf("모든 참여 원천의 명시된 시간 필드에 common_overlap_v1을 실행했습니다. 시간 조건으로 후보 쌍 %d개를 제외했고, 이 중 %d개는 시간 값이 없었습니다. 날짜의 의미와 기간 안의 정확한 사건 시점은 검증되지 않았습니다.", temporal.RejectedPairs, temporal.UnknownPairs)
				if temporal.Window != nil {
					report.Text += " 요청한 비교 범위: " + temporal.Window.From + " ~ " + temporal.Window.Through + " (양 끝 날짜 포함)."
				}
			} else {
				report.Text = "원천 행의 시간 정합성을 계산하지 않았습니다. observedAt은 조회 시각이며 사건일·기준일·유효 기간을 대신하지 않습니다. 카탈로그 수정일만으로 현재 유효한 데이터라고 판단할 수 없습니다."
			}
		case "identity":
			if len(p.Joins) > 0 {
				report.Text = "선택한 필드의 exact/trim 복합키 일치를 계산했습니다."
			} else {
				report.Text = "기존 관측을 사용했으며 추가 키 결합은 실행하지 않았습니다."
			}
			report.Text += " 발급기관·namespace·코드 재사용·실세계 동일성은 검증되지 않았습니다. 동일 이름이나 값만으로 canonical entity를 합치거나 연결 장부의 sample_verified로 승격하지 않습니다."
			checks, conflicts, unknowns := 0, 0, 0
			claims := map[string]bool{}
			for _, metric := range metrics {
				for _, check := range metric.ScopeChecks {
					checks++
					conflicts += check.ConflictPairs
					unknowns += check.UnknownPairs
					for _, claim := range []*CitedScopeClaim{check.LeftClaim, check.RightClaim} {
						if claim != nil {
							claims[claim.ClaimSHA256] = true
						}
					}
				}
			}
			if checks > 0 {
				report.Text += fmt.Sprintf(" 원천 텍스트에 묶인 scope 검사 %d개에서 불일치 %d건·판정 불가 %d건을 집계했습니다. 판정 불가는 값 누락 또는 선택 사전의 미등록 표기를 포함합니다. 검사별 후보 쌍의 합이며 서로 중복될 수 있습니다. 텍스트 일치는 주소 해석이나 실제 지역 동일성의 검증이 아닙니다.", checks, conflicts, unknowns)
			} else {
				report.Text += " 추가적인 원천 행의 scope 조건은 검사하지 않았습니다."
			}
			if len(claims) > 0 {
				report.Text += fmt.Sprintf(" 원천 설명·범위의 인용 가설 %d개를 사용했습니다. 인용 문구의 존재·위치·해시만 확인했으며, 부정 표현·시점·혼합 범위 해석이나 개별 행의 범위는 검증하지 않았습니다. 원천 선언과 주의사항을 함께 검토해야 합니다.", len(claims))
			}
		case "measurement":
			report.Text = explainMeasurements(p, sources)
		default:
			continue
		}
		out = append(out, report)
	}
	return out
}

// Called only for an executed artifact. Describe its recipe and computed source
// provenance, never infer measurement semantics from role or output names.
func explainMeasurements(p Composition, sources []Observation) string {
	var parts, units []string
	conversions, fieldSums := 0, 0
	for _, measure := range p.Measures {
		units = append(units, measure.As+"="+measure.Unit)
		if measure.Op == "sum_fields" {
			fieldSums++
		} else {
			conversions++
		}
	}
	if conversions+fieldSums > 0 {
		parts = append(parts, fmt.Sprintf("명시된 형식으로 수치 변환 %d개·행 내부 필드 합산 %d개를 실행했습니다. 필드 합산은 같은 원천 행의 서로 다른 필드에 대한 정확한 십진 계산이며, 단위나 측정 정의의 호환성을 증명하지 않습니다.", conversions, fieldSums))
	}
	hasSum, hasCount := false, false
	for _, aggregate := range p.Aggregates {
		hasSum = hasSum || aggregate.Op == "sum"
		hasCount = hasCount || aggregate.Op == "count"
	}
	if hasSum {
		parts = append(parts, "수치 sum은 정확한 십진 계산이며 같은 그룹 안에서 동일 원천 행의 반복 기여는 거부합니다. 값이 같은 별개 원천 행은 별도로 기여합니다. 그룹 간 합산 가능성은 별도 검토가 필요합니다.")
	}
	if hasCount {
		parts = append(parts, "count는 조합 결과의 행(필드를 지정하면 null 제외)을 세며, 고유 시설·실세계 개체 수를 증명하지 않습니다.")
	}
	if len(p.Measures) == 0 && len(p.Aggregates) == 0 {
		parts = append(parts, "조합 단계에서 수치 변환·집계를 실행하지 않았습니다.")
	}
	if len(units) > 0 {
		parts = append(parts, "선언된 측정 단위(미검증): "+strings.Join(units, ", ")+".")
	}
	byID := map[string]Observation{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	for _, source := range sources {
		if r := source.Reduction; r != nil {
			parts = append(parts, fmt.Sprintf("관측 %s는 %s의 보유 원본 %d행을 %d그룹으로 선집계했습니다. reduction.recipe에 필드·수치 형식·단위·집계 규칙을, groups에 각 그룹의 모든 기여 행을 보존합니다. 원천 전체·모집단 완전성이나 그룹 의미를 승인하지 않습니다.", source.ID, r.Recipe.Observation, byID[r.Recipe.Observation].RowCount, len(r.Groups)))
		}
		s := source.Spatial
		if s == nil {
			continue
		}
		candidate := byID[s.CandidateObservation]
		parts = append(parts, fmt.Sprintf("공간 관측 %s는 %s의 보유 행과 후보 %s(PK=%s)에 %s를 실행했습니다. 후보 파일 %d행을 스캔하여 정확한 필터에 일치한 %d행과 %d회 비교했고, 구 반지름 %.1f m로 대권거리(m)를 계산해 %d쌍을 보유했습니다. 이는 후속 조합·시간 조건 적용 전 공간 관측의 수치입니다.", source.ID, s.AnchorObservation, s.CandidateObservation, candidate.PK, s.Method, s.Scan.ScannedRows, s.Scan.MatchedRows, s.Comparisons, s.RadiusMeters, len(s.Pairs)))
		parts = append(parts, "가장 가까움은 선택한 후보 원천과 필터 안에서만 성립합니다. 전체 교통망·전체 시설의 포괄성, 일상적 의미의 가까움, 서로 다른 실제 목적지라는 판단은 검증하지 않았습니다. 공통 좌표계·좌표 정확도·현재 운영 여부·휠체어 이동 경로·경사·단차·승하차 가능성은 미확인입니다. 이 값은 경로 거리나 실제 이동 가능성을 뜻하지 않습니다.")
		if p.Time != nil {
			parts = append(parts, "시간 조건은 이미 선택한 공간 쌍을 걸러내며, 시간 조건에 맞는 전체 후보를 대상으로 순위를 다시 채우지 않았습니다.")
		}
	}
	parts = append(parts, "원천 수치·단위·측정 정의의 의미는 별도 검토가 필요합니다.")
	return strings.Join(parts, " ")
}
