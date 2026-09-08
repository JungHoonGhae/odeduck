package goalwork

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// EvidenceUse attributes disclosed fields to a participating observation's row.
// PacketRow is a retained row in the original packet, never a synthetic record.
type EvidenceUse struct {
	PacketID    string   `json:"packetId"`
	PacketRow   int      `json:"packetRow"`
	RetainedRow int      `json:"retainedRow"`
	Fields      []string `json:"fields"`
}

// EvidencePackets is the one packet collection for both review contracts.
// SourceContext holds references into this collection, never another body.
func (in ReviewInput) EvidencePackets() []EvidencePacket {
	packets := []EvidencePacket{in.Evidence}
	if in.Analysis != nil {
		packets = append(packets, in.Analysis.AdditionalEvidence...)
	}
	return packets
}

type disclosureCellKey struct {
	scope string
	row   int
	field string
}

type disclosureCell struct {
	value     any
	present   bool
	packetID  string
	packetRow int
}

type disclosureSource struct {
	observation Observation
	scope       string
	physical    bool
}

func newDisclosureSource(o Observation, request SampleRequest) disclosureSource {
	s := disclosureSource{observation: o, scope: digest([]any{"observation_cell_v1", o.ID, o.RowsSHA256})}
	if !sameFileRevision(o, o) || o.Document != nil || o.Comparison != nil || o.CSV != nil || o.Table == nil ||
		!completeRectangle(o.Table, o.RowCount) || request.XLSX == nil || request.Delivery != "file" ||
		request.PK != o.PK || request.Asset != o.Asset || digest(request) != o.RequestSHA256 ||
		request.XLSX.Range != o.Table.Range || request.XLSX.Sheet != o.Table.Sheet {
		return s // No physical-cell correspondence without a verified raw XLSX request.
	}
	ends := strings.Split(o.Table.Range, ":")
	first := worksheetColumn(strings.TrimRight(ends[0], "0123456789"))
	last := worksheetColumn(strings.TrimRight(ends[len(ends)-1], "0123456789"))
	for _, field := range o.Columns {
		column := worksheetColumn(field)
		if column < first || column > last {
			return s
		}
	}
	s.scope = digest([]any{"raw_xlsx_cell_v1", o.PK, o.Asset, o.ContentSHA256, o.ContractSHA256, request.Member, o.Table.Member, o.Table.Sheet})
	s.physical = true
	return s
}

func worksheetColumn(field string) int {
	if len(field) == 0 || len(field) > 3 {
		return 0
	}
	column := 0
	for _, c := range field {
		if c < 'A' || c > 'Z' {
			return 0
		}
		column = column*26 + int(c-'A') + 1
	}
	return column
}

func (s disclosureSource) key(row int, field string) disclosureCellKey {
	if s.physical {
		row = s.observation.Table.RowNumbers[row-1]
	}
	return disclosureCellKey{scope: s.scope, row: row, field: field}
}

func sameDisclosedValue(a any, aPresent bool, b any, bPresent bool) bool {
	return aPresent == bPresent && digest(a) == digest(b)
}

// All values come from successful read_evidence packets, then are checked
// against their immutable retained cells. Unsupported addresses stay local to
// their observation; a value match never creates a source correspondence.
func (e *Engine) indexReviewDisclosure(ctx context.Context, packets []EvidencePacket) (map[disclosureCellKey]disclosureCell, map[string]disclosureSource, error) {
	sources := map[string]disclosureSource{}
	for _, o := range e.state.Observations {
		sources[o.ID] = newDisclosureSource(o, e.requests[o.ID])
	}
	index := map[disclosureCellKey]disclosureCell{}
	checked := map[string]bool{}
	for _, packet := range packets {
		s, exists := sources[packet.Selection.Observation]
		o := s.observation
		if !exists || packet.Selection.RowsSHA256 != o.RowsSHA256 {
			return nil, nil, fmt.Errorf("review disclosure lost its retained source revision")
		}
		if !checked[o.ID] {
			if digest(e.rows[o.ID]) != o.RowsSHA256 || len(e.rows[o.ID]) != o.RowCount {
				return nil, nil, fmt.Errorf("review disclosure source rows changed")
			}
			checked[o.ID] = true
		}
		for _, record := range packet.Records {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if record.RetainedRow < 1 || record.RetainedRow > o.RowCount {
				return nil, nil, fmt.Errorf("review disclosure row is outside its retained source")
			}
			for _, field := range packet.Selection.Fields {
				value, present := record.Values[field]
				original, originalPresent := e.rows[o.ID][record.RetainedRow-1][field]
				if !slices.Contains(o.Columns, field) || present == slices.Contains(record.Missing, field) || !sameDisclosedValue(value, present, original, originalPresent) {
					return nil, nil, fmt.Errorf("review disclosure differs from its retained source cell")
				}
				key := s.key(record.RetainedRow, field)
				if prior, exists := index[key]; exists {
					if !sameDisclosedValue(prior.value, prior.present, value, present) {
						return nil, nil, fmt.Errorf("conflicting disclosures at the same original cell address")
					}
					continue
				}
				index[key] = disclosureCell{value: value, present: present, packetID: packet.ID, packetRow: record.RetainedRow}
			}
		}
	}
	return index, sources, nil
}

type reviewDisclosure struct {
	packets  []EvidencePacket
	selected map[string][]Row
	cells    map[string][]map[string]bool
	uses     map[string][]EvidenceUse
}

func (e *Engine) resolveReviewDisclosure(ctx context.Context, a *Artifact, sourceContext []SourceContext) (reviewDisclosure, error) {
	d := reviewDisclosure{selected: map[string][]Row{}, cells: map[string][]map[string]bool{}, uses: map[string][]EvidenceUse{}}
	participating, contextPackets := map[string]bool{}, map[string]bool{}
	for _, o := range a.Sources {
		participating[o.ID] = true
	}
	for _, c := range sourceContext {
		contextPackets[c.PacketID] = true
	}
	for _, packet := range e.state.Evidence {
		if participating[packet.Selection.Observation] || contextPackets[packet.ID] {
			d.packets = append(d.packets, packet)
		}
	}
	if len(d.packets) == 0 {
		return d, fmt.Errorf("read selected evidence before analysis review")
	}
	index, sources, err := e.indexReviewDisclosure(ctx, d.packets)
	if err != nil {
		return d, err
	}
	for _, o := range a.Sources {
		d.selected[o.ID] = make([]Row, o.RowCount)
		d.cells[o.ID] = make([]map[string]bool, o.RowCount)
		for i := range o.RowCount {
			if err := ctx.Err(); err != nil {
				return d, err
			}
			d.selected[o.ID][i], d.cells[o.ID][i] = Row{}, map[string]bool{}
			uses := map[string]int{}
			for _, field := range o.Columns {
				cell, exists := index[sources[o.ID].key(i+1, field)]
				if !exists {
					continue
				}
				original, present := e.rows[o.ID][i][field]
				if !sameDisclosedValue(cell.value, cell.present, original, present) {
					return d, fmt.Errorf("disclosed original cell conflicts with a participating source")
				}
				d.cells[o.ID][i][field] = true
				if cell.present {
					d.selected[o.ID][i][field] = cell.value
				}
				use, exists := uses[cell.packetID]
				if !exists {
					use = len(d.uses[o.ID])
					uses[cell.packetID] = use
					d.uses[o.ID] = append(d.uses[o.ID], EvidenceUse{PacketID: cell.packetID, PacketRow: cell.packetRow, RetainedRow: i + 1})
				}
				d.uses[o.ID][use].Fields = append(d.uses[o.ID][use].Fields, field)
			}
		}
	}
	return d, nil
}
