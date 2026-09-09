package goalwork

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func TestLiveAdapterXLSXRectangleJoinsCSVAndRetainsArtifactCoordinates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	entries := []catalog.Entry{{PK: "111", Title: "교육", SvcType: "FILE"}, {PK: "222", Title: "인구", SvcType: "FILE"}}
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: entries}).Save(); err != nil {
		t.Fatal(err)
	}
	var workbook bytes.Buffer
	z := zip.NewWriter(&workbook)
	for name, content := range map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="districts" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row r="27"><c r="A27" t="inlineStr"><is><t> 001 </t></is></c><c r="B27"><f>SUM(C27:D27)</f><v>148</v></c></row></sheetData></worksheet>`,
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/catalog/"):
			fmt.Fprint(w, `{"name":"fixture","description":"Fixture code namespace, no identity certification"}`)
		case strings.HasPrefix(r.URL.Path, "/data/"):
			fmt.Fprintf(w, `<button onclick="fileDetailObj.fn_fileDataDown('%s','uddi:x','','1','3')">download</button>`, strings.Split(r.URL.Path, "/")[2])
		case r.URL.Path == "/tcs/dss/selectFileDataDownload.do":
			_ = r.ParseForm()
			pk := r.Form.Get("publicDataPk")
			ext := "csv"
			if pk == "111" {
				ext = "xlsx"
			}
			fmt.Fprintf(w, `{"status":true,"atchFileId":"%s","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"fixture","orginlFileNm":"%s.%s","atchFileExtsn":"%s"}}`, pk, pk, ext, ext)
		case r.URL.Path == "/cmm/cmm/fileDownload.do":
			downloads++
			if r.URL.Query().Get("atchFileId") == "111" {
				_, _ = w.Write(workbook.Bytes())
			} else {
				fmt.Fprint(w, "id,population\n001,40981\n")
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e, err := Start("two fixture observations", Policy{}, LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{}))
	if err != nil {
		t.Fatal(err)
	}
	step := func(d Decision) View {
		t.Helper()
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) > 0 {
			t.Fatalf("%s: %+v %v", d.Action, v.Gaps, err)
		}
		return v
	}
	step(Decision{Action: "define", Contract: &GoalContract{Outcome: "compare fixture counts", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []RoleRequirement{{ID: "schools", Description: "education"}, {ID: "people", Description: "population"}}, Outputs: []OutputRequirement{{ID: "schools", Role: "schools", Description: "count", Type: "number"}, {ID: "people", Role: "people", Description: "count", Type: "number"}}}})
	for index, entry := range entries {
		role, ext := "schools", "xlsx"
		if index == 1 {
			role, ext = "people", "csv"
		}
		step(Decision{Action: "search", Query: entry.Title, Role: role})
		step(Decision{Action: "inspect", PK: entry.PK})
		s := SampleRequest{PK: entry.PK, Delivery: "file", Asset: entry.PK + "." + ext}
		if index == 0 {
			v := step(Decision{Action: "layout", Layout: &LayoutRequest{PK: entry.PK, Asset: s.Asset}})
			sheet := v.Layouts[0].Layout.Sheets[0].Name
			v = step(Decision{Action: "layout", Layout: &LayoutRequest{PK: entry.PK, Asset: s.Asset, Sheet: sheet}})
			if v.Layouts[1].Layout.Sheet.Extent != "A27:B27" {
				t.Fatal("live adapter lost physical layout")
			}
			s.LayoutID = v.Layouts[1].ID
			s.XLSX = &dataset.XLSXSelection{Sheet: "districts", Range: "A27:B27"}
		}
		v := step(Decision{Action: "sample", Sample: &s})
		if index == 0 {
			if v.Observations[0].Table == nil || v.Observations[0].Table.RowNumbers[0] != 27 || v.Observations[0].Table.FormulaCells[0] != "B27" {
				t.Fatal("rectangle provenance missing")
			}
			s.XLSX.Range = "A1:B1"
			v.Observations[0].Table.RowNumbers[0] = 1
			v.SampleAttempts[0].Request.XLSX.Range = "A1:B1"
			fresh := e.PlanningView()
			if fresh.Observations[0].Table.RowNumbers[0] != 27 || fresh.SampleAttempts[0].Request.XLSX.Range != "A27:B27" {
				t.Fatal("caller mutated retained XLSX evidence")
			}
		}
	}
	p := Composition{ID: "comparison", Purpose: "fixture comparison", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.A"}, RightKeys: []string{"id"}, Normalization: "trim"}}, Measures: []Measure{{As: "school_count", Field: "o1.B", Format: "decimal_v1", Unit: "schools"}, {As: "population", Field: "o2.population", Format: "decimal_v1", Unit: "persons"}}, Select: []string{"school_count", "population"}, Roles: []RoleBinding{{Role: "schools", Observation: "o1"}, {Role: "people", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "schools", Field: "school_count"}, {Output: "people", Field: "population"}}, Assumptions: []string{"Fixture code namespaces and cached formula meaning need review"}}
	step(Decision{Action: "compose", Composition: &p})
	v := step(Decision{Action: "execute", CompositionID: p.ID})
	if downloads != 4 || v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 1 {
		t.Fatalf("unexpected execution: status=%s downloads=%d", v.Status, downloads)
	}
	if v.Artifact.Rows[0]["school_count"] != json.Number("148") || v.Artifact.Rows[0]["population"] != json.Number("40981") {
		t.Fatal("XLSX numeric storage values were not converted through explicit measures")
	}
	if len(v.Artifact.Layouts) != 2 || v.Artifact.Requests[0].LayoutID != "l2" {
		t.Fatal("artifact lost layout binding")
	}
	if v.Artifact.Requests[0].XLSX.Range != "A27:B27" || v.Artifact.Sources[0].Table.RowNumbers[0] != 27 || v.Artifact.Sources[0].Table.FormulaCells[0] != "B27" {
		t.Fatal("artifact lost original selection")
	}
	b, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(b), "40981") || strings.Contains(string(b), " 001 ") {
		t.Fatal("raw values leaked into planning context")
	}
}
