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

func TestZIPMembersUseSharedDiscoveryPinningSelectionAndComposition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "111", Title: "시설접근", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	z := zip.NewWriter(&body)
	for name, content := range map[string]string{"facilities.csv": "id,scope\n001,target\n", "access.csv": "id,scope,ramp\n999,other,N\n001,target,Y\n"} {
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
			fmt.Fprint(w, `{"name":"fixture","encodingFormat":"ZIP"}`)
		case strings.HasPrefix(r.URL.Path, "/data/"):
			fmt.Fprint(w, `<button onclick="fileDetailObj.fn_fileDataDown('111','uddi:x','','1','3')">download</button>`)
		case r.URL.Path == "/tcs/dss/selectFileDataDownload.do":
			fmt.Fprint(w, `{"status":true,"atchFileId":"111","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"fixture","orginlFileNm":"source.zip","atchFileExtsn":"zip"}}`)
		case r.URL.Path == "/cmm/cmm/fileDownload.do":
			downloads++
			_, _ = w.Write(body.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e, err := Start("source association", Policy{}, LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{}))
	if err != nil {
		t.Fatal(err)
	}
	contract := GoalContract{Outcome: "association", Region: "fixture", Period: "historical", Coverage: "sample", Roles: []RoleRequirement{{ID: "facility", Description: "facility"}, {ID: "access", Description: "accessibility"}}, Outputs: []OutputRequirement{{ID: "id", Role: "facility", Description: "code", Type: "string"}, {ID: "ramp", Role: "access", Description: "reported ramp", Type: "string"}}}
	p := Composition{ID: "association", Purpose: "fixture association", Base: "o1", Joins: []Join{{Right: "o2", LeftKeys: []string{"o1.id"}, RightKeys: []string{"id"}}}, Select: []string{"o1.id", "o2.ramp"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "access", Observation: "o2"}}, Outputs: []OutputBinding{{Output: "id", Field: "o1.id"}, {Output: "ramp", Field: "o2.ramp"}}, Assumptions: []string{"Fixture namespace association, not verified current accessibility"}}
	decisions := []Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "시설접근", Role: "facility"}, {Action: "search", Query: "시설접근", Role: "access"}, {Action: "inspect", PK: "111"}, {Action: "layout", Layout: &LayoutRequest{PK: "111", Asset: "source.zip"}}, {Action: "sample", Sample: &SampleRequest{PK: "111", Delivery: "file", Asset: "source.zip", Member: "facilities.csv", LayoutID: "l1", Where: map[string]string{"scope": "target"}}}, {Action: "sample", Sample: &SampleRequest{PK: "111", Delivery: "file", Asset: "source.zip", Member: "access.csv", LayoutID: "l1", Where: map[string]string{"scope": "target"}}}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: p.ID}}
	n := 0
	decisions = append(decisions, Decision{Action: "abstain", Reason: "source association does not verify current accessibility"})
	v, err := Run(context.Background(), e, func(_ context.Context, v View) (Decision, error) {
		if len(v.Gaps) > 0 {
			return Decision{}, fmt.Errorf("unexpected gaps: %+v", v.Gaps)
		}
		if n >= len(decisions) {
			return Decision{}, fmt.Errorf("fixture exhausted")
		}
		d := decisions[n]
		n++
		return d, nil
	}, nil)
	if err != nil || v.Status != "abstained" || v.Artifact == nil || len(v.Artifact.Rows) != 1 || downloads != 3 || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("ZIP flow failed: %+v %v downloads=%d", v.Gaps, err, downloads)
	}
	if v.Artifact.Rows[0]["o1.id"] != "001" || v.Artifact.Rows[0]["o2.ramp"] != "Y" {
		t.Fatal("member association lost values")
	}
	o1, o2 := v.Artifact.Sources[0], v.Artifact.Sources[1]
	if o1.Archive == nil || o2.Archive == nil || o1.ContentSHA256 != o2.ContentSHA256 || o1.Archive.MemberSHA256 == o2.Archive.MemberSHA256 || o2.CSV.DataRecords[0] != 2 || o2.CSV.StartLines[0] != 3 || !o2.Selection.Exhausted || o1.RequestSHA256 == o2.RequestSHA256 || v.Artifact.Requests[1].Member != "access.csv" {
		t.Fatal("archive/member/record provenance was lost or collapsed")
	}
	v.Artifact.Sources[1].CSV.DataRecords[0] = 99
	if e.View().Artifact.Sources[1].CSV.DataRecords[0] != 2 {
		t.Fatal("caller mutated provenance")
	}
	encoded, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(encoded), `"001"`) {
		t.Fatal("raw row value reached planner")
	}
}

func TestZIPMemberSelectionCannotBypassDeliveryOrWorkbookContract(t *testing.T) {
	for _, s := range []SampleRequest{{Delivery: "api", Member: "data.csv"}, {Delivery: "standard", Member: "data.csv"}, {Delivery: "file", Member: "../data.csv"}, {Delivery: "file", Member: "nested.zip"}, {Delivery: "file", Member: "data.csv", XLSX: &dataset.XLSXSelection{Sheet: "s", Range: "A1:B2"}}} {
		if err := validateSampleSelection(s); err == nil {
			t.Fatalf("invalid member selection accepted: %+v", s)
		}
	}
}
