package goalwork_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestHistoricalAcquisitionSurvivesCurrentReinspectionAndLocalReduction(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(configRoot, ".config"))
	t.Setenv("APPDATA", filepath.Join(configRoot, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "snapshot", SvcType: catalog.SvcFILE}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := map[string]string{"/catalog/123/fileData.json": "current.json", "/data/123/fileData.do": "current.html", "/tcs/dss/selectHistAndCsvData.do": "list.html", "/tcs/dss/selectDpkDetailInfo.do": "past.html", "/tcs/dss/selectFileDataDownload.do": "resolved.json", "/cmm/cmm/fileDownload.do": "past.csv"}[r.URL.Path]
		b, err := os.ReadFile("../dataset/testdata/file-history/" + name)
		if err != nil || name == "" {
			t.Errorf("unexpected fixture request %s: %v", r.URL.Path, err)
			http.Error(w, "missing fixture", 500)
			return
		}
		fmt.Fprint(w, string(b))
	}))
	defer srv.Close()
	deps := goalwork.LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, goalwork.Policy{})
	// Fixed catalog candidate: this test covers actual inspection/acquisition and
	// retained execution, not semantic discovery or model judgment.
	deps.Search = func(context.Context, string) (catalog.Result, error) {
		return catalog.Result{Hits: []catalog.Hit{{PK: "123"}}}, nil
	}
	e, err := goalwork.Start("Sum the prior publication", goalwork.Policy{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "source sum", Region: "fixture", Period: "prior publication", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "total", Description: "sum", Role: "r", Type: "number"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "snapshot", Role: "r"})
	v := advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "123", FileHistory: true})
	version := v.Nodes[0].Inspection.FileVersions[0].ID
	advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "123", FileVersion: version})
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "file", Asset: "snapshot.csv", FileVersion: version, ScanCSV: true}})
	o := v.Observations[0]
	if o.RowCount != 2 || o.Declaration == nil || o.Declaration.FileVersionID != version || o.Declaration.Description != "" || o.Selection == nil || !o.Selection.Exhausted || o.Selection.ScannedRows != 2 {
		t.Fatal("selected edition lost original metadata or full scan provenance")
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "123"})
	v = advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "123", Delivery: "file", Reduce: &goalwork.SourceReduction{Observation: o.ID, RowsSHA256: o.RowsSHA256, Measures: []goalwork.Measure{{As: "n", Field: "count", Format: "decimal_v1", Unit: "persons"}}, Aggregates: []goalwork.Aggregate{{As: "total", Op: "sum", Field: "n"}}}}})
	p := goalwork.Composition{ID: "prior-total", Base: "o2", Purpose: "sum retained prior records", Select: []string{"o2.total"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "total", Field: "o2.total"}}, Assumptions: []string{"Source arithmetic only; not independently verified meaning or population."}}
	v = executeResult(t, e, p)
	if v.Artifact == nil || len(v.Artifact.Rows) != 1 || v.Artifact.Rows[0]["o2.total"] != json.Number("10") || v.Status != "review_required" {
		t.Fatalf("historical source did not reach a grounded calculation: %+v", v.Gaps)
	}
	if v.Observations[0].Declaration.FileVersionID != version || v.Observations[0].ContentSHA256 != o.ContentSHA256 || v.SampleAttempts[0].Request.FileVersion != version || v.Observations[1].Declaration != nil {
		t.Fatal("reinspection replaced retained historical provenance or labelled a local sum with current metadata")
	}
}
