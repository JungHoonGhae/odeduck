package goalwork

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const (
	maxEvidencePackets = 8
	maxEvidenceBytes   = 64 << 10
)

// EvidenceRequest selects immutable observations, never caller-provided values.
type EvidenceRequest struct {
	Observation string   `json:"observation"`
	RowsSHA256  string   `json:"rowsSha256"`
	Rows        []int    `json:"rows"` // 1-based within the retained observation, not original IDs
	Fields      []string `json:"fields"`
}

type EvidencePacket struct {
	ID        string           `json:"id"`
	Status    string           `json:"status"` // observed_untrusted; not semantic approval
	Selection EvidenceRequest  `json:"selection"`
	Records   []EvidenceRecord `json:"records"`
}

type EvidenceRecord struct {
	RetainedRow int                        `json:"retainedRow"`
	Values      Row                        `json:"values"`
	Missing     []string                   `json:"missing,omitempty"`
	Origins     map[string]EvidenceAddress `json:"origins"`
}

// Observation references carry source, request and contract revisions in View.
// An original CSV ordinal may lie beyond the retained candidate prefix.
type EvidenceAddress struct {
	Observation string `json:"observation"`
	Field       string `json:"field"`
	Kind        string `json:"kind"`    // retained_row | csv_data_record | worksheet_row | document_block | computed_pair | computed_group | computed_comparison
	Ordinal     int    `json:"ordinal"` // 1-based in the named coordinate system
	Sheet       string `json:"sheet,omitempty"`
	Locator     string `json:"locator,omitempty"` // original DOM path for document_block
}

func (e *Engine) readEvidence(request *EvidenceRequest) error {
	if e.state.Policy.EvidenceRecipient == "" {
		return fmt.Errorf("selected evidence is disabled by the trusted caller's start policy")
	}
	if request == nil || len(request.Rows) < 1 || len(request.Rows) > 20 || len(request.Fields) < 1 || len(request.Fields) > 8 {
		return fmt.Errorf("evidence requires an observation revision, 1–20 distinct retained rows and 1–8 distinct observed fields")
	}
	if len(e.state.Evidence) >= maxEvidencePackets {
		return fmt.Errorf("evidence packet budget exhausted")
	}
	observed := map[string]Observation{}
	for _, o := range e.state.Observations {
		observed[o.ID] = o
	}
	o, exists := observed[request.Observation]
	if !exists || request.RowsSHA256 == "" || request.RowsSHA256 != o.RowsSHA256 {
		return fmt.Errorf("evidence requires the exact retained observation rowsSha256")
	}
	fields := map[string]bool{}
	for _, field := range request.Fields {
		if fields[field] || !slices.Contains(o.Columns, field) || credentialField(field) {
			return fmt.Errorf("evidence fields must be distinct observed non-credential fields")
		}
		fields[field] = true
	}
	lineage, err := prepareLineage(Composition{Base: o.ID}, e.rows, observed)
	if err != nil {
		return fmt.Errorf("evidence source record lineage is unavailable or inconsistent")
	}
	packet := EvidencePacket{Status: "observed_untrusted", Selection: *request}
	positions := map[int]bool{}
	for _, position := range request.Rows {
		if position < 1 || position > len(e.rows[o.ID]) || positions[position] {
			return fmt.Errorf("evidence rows must be distinct 1-based positions within the retained observation")
		}
		positions[position] = true
		record := EvidenceRecord{RetainedRow: position, Values: Row{}, Origins: map[string]EvidenceAddress{}}
		for _, field := range request.Fields {
			value, present := e.rows[o.ID][position-1][field]
			if !present {
				record.Missing = append(record.Missing, field)
				continue
			}
			cell, err := json.Marshal(value)
			if err != nil || len(cell) > 2048 {
				return fmt.Errorf("evidence cell exceeds 2048 JSON bytes")
			}
			switch v := value.(type) {
			case nil, bool, json.Number, float64:
			case string:
				if credentialText(v) {
					return fmt.Errorf("evidence contains credential material; packet withheld")
				}
			default:
				return fmt.Errorf("evidence supports JSON scalars only")
			}
			address, err := evidenceAddress(o, field, lineage[o.ID][position-1], observed)
			if err != nil {
				return err
			}
			record.Values[field] = value
			record.Origins[field] = address
		}
		packet.Records = append(packet.Records, record)
	}
	packet.ID = "ep_" + digest(packet)
	b, err := json.Marshal(packet)
	if err != nil || len(b) > 16<<10 {
		return fmt.Errorf("evidence packet exceeds 16 KiB; select fewer cells")
	}
	if e.evidenceBytes+len(b) > maxEvidenceBytes {
		return fmt.Errorf("session evidence byte budget exhausted")
	}
	e.evidenceBytes += len(b)
	e.state.Evidence = append(e.state.Evidence, packet)
	return nil
}

func evidenceAddress(o Observation, field string, trace rowLineage, observed map[string]Observation) (EvidenceAddress, error) {
	slot := fieldSlot(o.ID+"."+field, observed)
	ordinal, exists := trace[slot]
	if !exists {
		return EvidenceAddress{}, fmt.Errorf("evidence field has no original record address")
	}
	address := EvidenceAddress{Observation: slot.observation, Field: field, Kind: "retained_row", Ordinal: ordinal + 1}
	if o.Comparison != nil {
		address.Kind = "computed_comparison"
		return address, nil
	}
	if o.Reduction != nil {
		address.Kind = "computed_group"
		return address, nil
	}
	if o.Spatial != nil {
		switch slot.observation {
		case o.Spatial.AnchorObservation:
			address.Field = strings.TrimPrefix(field, "anchor.")
		case o.Spatial.CandidateObservation:
			address.Field = strings.TrimPrefix(field, "candidate.")
		default:
			address.Kind = "computed_pair"
		}
	}
	if slot.kind == "csv" {
		address.Kind, address.Ordinal = "csv_data_record", ordinal
	} else if document := observed[slot.observation].Document; document != nil {
		if len(document.Blocks) != observed[slot.observation].RowCount || ordinal < 0 || ordinal >= len(document.Blocks) {
			return EvidenceAddress{}, fmt.Errorf("evidence document block provenance is inconsistent")
		}
		block := document.Blocks[ordinal]
		address.Kind, address.Ordinal, address.Locator = "document_block", block.Ordinal, block.Locator
	} else if table := observed[slot.observation].Table; table != nil {
		if len(table.RowNumbers) != observed[slot.observation].RowCount || ordinal < 0 || ordinal >= len(table.RowNumbers) || table.RowNumbers[ordinal] < 1 || table.Sheet == "" {
			return EvidenceAddress{}, fmt.Errorf("evidence worksheet row provenance is inconsistent")
		}
		address.Kind, address.Ordinal, address.Sheet = "worksheet_row", table.RowNumbers[ordinal], table.Sheet
	}
	return address, nil
}

func credentialField(field string) bool {
	// Spatial prefixes and publisher-style dotted names do not bypass the guard.
	for _, part := range strings.FieldsFunc(field, func(r rune) bool { return r == '.' || r == '/' || r == ':' }) {
		if sensitiveParameter(part) {
			return true
		}
		if strings.Contains(part, "인증키") || strings.Contains(part, "비밀번호") || strings.Contains(part, "접근토큰") {
			return true
		}
	}
	return false
}

// A supplemental label guard, not a general PII or arbitrary-secret detector.
var credentialMarker = regexp.MustCompile(`(?i)(service[_ -]?key|api[_ -]?key|auth[_ -]?key|access[_ -]?token|refresh[_ -]?token|authorization|cookie|password|secret|bearer|인증키|비밀번호|접근토큰)\s*["']?\s*[:=]?\s*["']?\s*[a-z0-9%._~+/=-]{8,}`)

func credentialText(value string) bool {
	for i := 0; i < 3; i++ {
		if credentialMarker.MatchString(value) {
			return true
		}
		decoded, err := url.PathUnescape(value)
		if err != nil || decoded == value {
			break
		}
		value = decoded
	}
	return false
}
