package goalwork

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

type LayoutRequest struct {
	PK        string `json:"pk"`
	Asset     string `json:"asset"`
	Sheet     string `json:"sheet,omitempty"`
	RefreshOf string `json:"refreshOf,omitempty"`
}
type LayoutObservation struct {
	ID            string             `json:"id"`
	Request       LayoutRequest      `json:"request"`
	RequestSHA256 string             `json:"requestSha256"`
	ObservedAt    string             `json:"observedAt"`
	Layout        dataset.FileLayout `json:"layout"`
}

func (e *Engine) discoverLayout(ctx context.Context, r *LayoutRequest) error {
	if r == nil || r.PK == "" || r.Asset == "" || len(r.Asset) > 512 || len(r.Sheet) > 256 {
		return fmt.Errorf("layout requires known PK, exact inspected asset and optional exact sheet")
	}
	n := e.node(r.PK)
	if n == nil || n.Inspection == nil {
		return fmt.Errorf("inspect known PK before layout discovery")
	}
	found := false
	for _, a := range n.Inspection.Assets {
		found = found || a == r.Asset
	}
	if !found {
		return fmt.Errorf("layout asset not in inspected contract")
	}
	if r.RefreshOf != "" {
		previous := e.layoutByID(r.RefreshOf)
		if previous == nil || previous.Request.PK != r.PK || previous.Request.Asset != r.Asset || previous.Request.Sheet != r.Sheet {
			return fmt.Errorf("refreshOf must match an earlier layout request for the same asset and sheet")
		}
	}
	if e.layouts >= 6 {
		return fmt.Errorf("layout acquisition budget exhausted")
	}
	e.layouts++
	if e.deps.Layout == nil {
		return fmt.Errorf("layout discovery unavailable")
	}
	result, err := e.deps.Layout(ctx, *r, *n.Inspection)
	if err != nil {
		return err
	}
	h, err := hex.DecodeString(result.SHA256)
	if err != nil || len(h) != 32 {
		return fmt.Errorf("layout adapter did not provide a complete content hash")
	}
	b, err := json.Marshal(result)
	if err != nil || len(b) > 32<<10 {
		return fmt.Errorf("layout metadata exceeds 32 KiB")
	}
	var detached dataset.FileLayout
	if err := json.Unmarshal(b, &detached); err != nil {
		return err
	}
	e.state.Layouts = append(e.state.Layouts, LayoutObservation{ID: fmt.Sprintf("l%d", len(e.state.Layouts)+1), Request: *r, RequestSHA256: digest(r), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Layout: detached})
	return nil
}

func (e *Engine) layoutByID(id string) *LayoutObservation {
	for i := range e.state.Layouts {
		if e.state.Layouts[i].ID == id {
			return &e.state.Layouts[i]
		}
	}
	return nil
}

func (e *Engine) sampleLayoutHash(s SampleRequest) (string, error) {
	if s.LayoutID == "" {
		return "", nil
	}
	if s.Delivery != "file" || (s.XLSX == nil && s.Member == "") {
		return "", fmt.Errorf("layoutId is supported only for XLSX or ZIP member sampling")
	}
	l := e.layoutByID(s.LayoutID)
	if l == nil || l.Request.PK != s.PK || l.Request.Asset != s.Asset {
		return "", fmt.Errorf("layoutId must identify the same inspected asset")
	}
	if s.Member != "" {
		if l.Layout.Format != "ZIP" || l.Request.Sheet != "" {
			return "", fmt.Errorf("member sampling requires a ZIP layout")
		}
		for _, m := range l.Layout.Members {
			if m.Name == s.Member && m.Format == "CSV" {
				return l.Layout.SHA256, nil
			}
		}
		return "", fmt.Errorf("CSV member was not discovered in the referenced layout")
	}
	if l.Request.Sheet != "" && l.Request.Sheet != s.XLSX.Sheet {
		return "", fmt.Errorf("sample sheet differs from selected layout")
	}
	for _, sheet := range l.Layout.Sheets {
		if sheet.Name == s.XLSX.Sheet {
			return l.Layout.SHA256, nil
		}
	}
	return "", fmt.Errorf("sample sheet was not discovered in the referenced layout")
}
