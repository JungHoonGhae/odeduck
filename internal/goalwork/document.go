package goalwork

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

func validateDocumentAcquisition(s SampleRequest, inspected Inspection, a Acquired) error {
	if s.Document == nil {
		if a.Document != nil || a.Delivery == "DOCUMENT" {
			return fmt.Errorf("document provenance requires document acquisition")
		}
		return nil
	}
	p := a.Document
	if p == nil || a.Delivery != "DOCUMENT" || a.Operation != "" || a.Reduction != nil || a.Spatial != nil || a.Selection != nil || a.Table != nil || a.CSV != nil || a.Archive != nil {
		return fmt.Errorf("document acquisition must be an original support-only document")
	}
	known := false
	for _, ref := range inspected.Documents {
		if ref.ID == s.Document.ReferenceID && ref == p.Reference {
			known = true
		}
	}
	hash, err := hex.DecodeString(a.ContentSHA256)
	if !known || a.ContractSHA256 != digest(p.Reference) || err != nil || len(hash) != 32 || p.SourceSHA256 != a.ContentSHA256 || p.Bytes < 1 || p.Bytes > 1<<20 || p.ExtractorRevision != "html-block-space-v1" {
		return fmt.Errorf("document source or contract revision mismatch")
	}
	if len(a.Rows) < 1 || len(a.Rows) > 20 || len(p.Blocks) != len(a.Rows) || p.MatchedSections != len(a.Rows) || p.Sections < p.MatchedSections || p.Sections > 20 {
		return fmt.Errorf("document section extent is inconsistent")
	}
	last := 0
	for i, row := range a.Rows {
		text, ok := row["text"].(string)
		cell, _ := json.Marshal(text)
		block := p.Blocks[i]
		if !ok || len(row) != 1 || text == "" || len(cell) > 2048 || !strings.Contains(text, s.Document.Contains) || block.Ordinal <= last || block.Ordinal > p.Sections || !strings.HasPrefix(block.Locator, "/html[1]/body[1]/") || len(block.Locator) > 2048 || block.TextSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(text))) {
			return fmt.Errorf("document selected text or original block address is inconsistent")
		}
		last = block.Ordinal
	}
	return nil
}

func cloneDocumentProvenance(p *dataset.DocumentProvenance) *dataset.DocumentProvenance {
	if p == nil {
		return nil
	}
	copy := *p
	copy.Blocks = append([]dataset.DocumentBlock(nil), p.Blocks...)
	return &copy
}

func validateComputationalSources(p Composition, observations []Observation) error {
	observed := map[string]Observation{}
	for _, o := range observations {
		observed[o.ID] = o
	}
	for id := range compositionSources(p, observed) {
		o := observed[id]
		if o.Document != nil || o.Delivery == "DOCUMENT" {
			return fmt.Errorf("document observations are support-only, not computational sources")
		}
	}
	return nil
}
