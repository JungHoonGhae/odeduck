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
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
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
		Input  goalwork.ReviewInput `json:"input"`
		Result goalwork.View        `json:"result"`
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
