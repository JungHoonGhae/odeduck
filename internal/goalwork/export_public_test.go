package goalwork_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func exportEngine(t *testing.T) *goalwork.Engine {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "3033304", Title: "export fixture", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: documentHTTP(func(r *http.Request) (*http.Response, error) {
		body, contentType := "", "text/html; charset=UTF-8"
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = `{}`
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = `<ul><li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li></ul>`
		case "https://jumin.mois.go.kr/ageStatMonth.do", "https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3":
			name := "mois_monthly_export.html"
			if r.URL.Path == "/downloadCsvAge.do" {
				name, contentType = "mois_monthly_export.csv", "text/csv"
			}
			b, err := os.ReadFile(filepath.Join("..", "dataset", "testdata", name))
			if err != nil {
				return nil, err
			}
			body = string(b)
		default:
			return nil, fmt.Errorf("unexpected HTTP: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	policy := goalwork.Policy{EvidenceRecipient: "claude"}
	e, err := goalwork.Start("Read the selected official monthly records, preserving their scope and provenance", policy, goalwork.LiveDependencies(client, "", nil, catalog.Searcher{}, policy))
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "original records", Region: "source province", Period: "source month", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "original records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "selected interval count"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: "export fixture", Role: "r"})
	advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: "3033304"})
	return e
}

func TestMonthlyExportRejectsMixedAndUnknownSelectorsBeforeAcquisition(t *testing.T) {
	for _, variant := range []string{"asset", "fileVersion", "where", "scan", "document", "reduce", "endpoint", "missing", "age range", "registration month", "unregistered"} {
		t.Run(variant, func(t *testing.T) {
			e := exportEngine(t)
			var s goalwork.SampleRequest
			if err := json.Unmarshal([]byte(exportSampleWire), &s); err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "asset":
				s.Asset = "invented.csv"
			case "fileVersion":
				s.FileVersion = "invented edition"
			case "where":
				s.Where = map[string]string{"행정구역": "invented"}
			case "scan":
				s.ScanCSV = true
			case "document":
				s.Document = &dataset.DocumentSelection{ReferenceID: "mois-monthly-help"}
			case "reduce":
				s.Reduce = &goalwork.SourceReduction{}
			case "endpoint":
				s.Params["endpoint"] = "https://unregistered.example/"
			case "missing":
				delete(s.Params, "registration")
			case "age range":
				s.Params["ageTo"] = "110"
			case "registration month":
				s.Params["registration"], s.Params["month"] = "resident", "2010-09"
			case "unregistered":
				s.Operation = "other-export"
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.SampleAttempts) != 0 || len(v.Observations) != 0 {
				t.Fatalf("invalid selector reached acquisition: %+v %v", v, err)
			}
		})
	}
}

const exportSampleWire = `{"pk":"3033304","delivery":"file","operation":"mois-monthly-age-csv","params":{"month":"2026-07","registration":"all","provinceCode":"2800000000","ageFrom":"6","ageTo":"7"}}`

func TestMonthlyExportThroughSharedEngineKeepsChoicesPrivateRowsAndPositions(t *testing.T) {
	e := exportEngine(t)
	wire, _ := json.Marshal(e.PlanningView())
	if !strings.Contains(string(wire), `"exports"`) || !strings.Contains(string(wire), "mois-monthly-age-csv") {
		t.Fatal("inspection did not advertise the export operation to the planner")
	}
	var sample goalwork.SampleRequest
	if err := json.Unmarshal([]byte(exportSampleWire), &sample); err != nil {
		t.Fatal(err)
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &sample})
	o := e.View().Observations[0]
	if o.Operation != "mois-monthly-age-csv" || o.RowCount != 2 || o.CSV == nil || o.CSV.Export == nil || o.CSV.Export.Choices.Month != "2026-07" || o.Selection.ScannedRows != 3 || o.CSV.DataRecords[0] != 2 {
		t.Fatalf("acquisition lost export provenance: %+v", o)
	}
	wire, _ = json.Marshal(e.PlanningView())
	if strings.Contains(string(wire), "Province (") || strings.Contains(string(wire), "Child (") {
		t.Fatal("raw export values bypassed explicit evidence selection")
	}
	advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2}, Fields: []string{"행정구역"}}})
	p := e.View().Evidence[0]
	if p.Records[0].Values["행정구역"] != "Child (2811051000)" || p.Records[0].Origins["행정구역"].Ordinal != 3 {
		t.Fatalf("export was not preserved as original CSV evidence: %+v", p)
	}
}
