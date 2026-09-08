package goalwork

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

func TestNearestUsesSharedHTTPInspectionAcquisitionAndRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("APPDATA", filepath.Join(dir, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "111", Title: "시설", SvcType: "FILE"}, {PK: "222", Title: "정류장", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{"111": "id,lat,lon\n001,0,0\n002,0,1\n", "222": "id,lat,lon,scope\n" + strings.Repeat("far,0,90,target\n", 1001) + "ICB001,0,0,target\nGGB001,0,1,target\n"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/catalog/"):
			fmt.Fprint(w, `{"name":"fixture","encodingFormat":"CSV"}`)
		case strings.HasPrefix(r.URL.Path, "/data/"):
			pk := strings.Split(r.URL.Path, "/")[2]
			fmt.Fprintf(w, `<button onclick="fileDetailObj.fn_fileDataDown('%s','uddi:x','','1','3')">download</button>`, pk)
		case r.URL.Path == "/tcs/dss/selectFileDataDownload.do":
			_ = r.ParseForm()
			pk := r.Form.Get("publicDataPk")
			fmt.Fprintf(w, `{"status":true,"atchFileId":"%s","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"fixture","orginlFileNm":"source.csv","atchFileExtsn":"csv"}}`, pk)
		case r.URL.Path == "/cmm/cmm/fileDownload.do":
			fmt.Fprint(w, bodies[r.URL.Query().Get("atchFileId")])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e, err := Start("compare facility and nearby source stops", Policy{}, LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{}))
	if err != nil {
		t.Fatal(err)
	}
	c := GoalContract{Outcome: "comparison", Region: "fixture", Period: "historic", Coverage: "sample", Roles: []RoleRequirement{{ID: "facility", Description: "facility"}, {ID: "transit", Description: "stops"}}, Outputs: []OutputRequirement{{ID: "distance", Role: "transit", Description: "conditional distance", Type: "number"}}}
	nearest := SampleRequest{PK: "222", Asset: "source.csv", Delivery: "file", ScanCSV: true, Where: map[string]string{"scope": "target"}, Nearest: &NearestSelection{Method: "spherical_nearest_records_v1", Anchor: "o1", Candidate: "o2", AnchorLatitude: "lat", AnchorLongitude: "lon", Latitude: "lat", Longitude: "lon", K: 1}}
	p := Composition{ID: "near", Purpose: "conditional comparison", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.id", "o1.lat", "o1.lon"}, RightKeys: []string{"anchor.id", "anchor.lat", "anchor.lon"}}}, Select: []string{"o1.id", "o3.candidate.id", "o3.distance_m"}, Roles: []RoleBinding{{Role: "facility", Observation: "o1"}, {Role: "transit", Observation: "o3"}}, Outputs: []OutputBinding{{Output: "distance", Field: "o3.distance_m"}}, Assumptions: []string{"conditional degrees, datum/route/operation unverified"}}
	steps := []Decision{{Action: "define", Contract: &c}, {Action: "search", Query: "시설", Role: "facility"}, {Action: "inspect", PK: "111"}, {Action: "sample", Sample: &SampleRequest{PK: "111", Delivery: "file", Asset: "source.csv"}}, {Action: "search", Query: "정류장", Role: "transit"}, {Action: "inspect", PK: "222"}, {Action: "sample", Sample: &SampleRequest{PK: "222", Delivery: "file", Asset: "source.csv", ScanCSV: true}}, {Action: "sample", Sample: &nearest}, {Action: "compose", Composition: &p}, {Action: "execute", CompositionID: "near"}}
	n := 0
	v, err := Run(context.Background(), e, func(_ context.Context, v View) (Decision, error) {
		if len(v.Gaps) > 0 || n >= len(steps) {
			return Decision{}, fmt.Errorf("fixture failed: %+v", v.Gaps)
		}
		d := steps[n]
		n++
		return d, nil
	}, nil)
	if err != nil || v.Status != "review_required" || v.Artifact == nil || len(v.Artifact.Rows) != 2 {
		t.Fatalf("HTTP nearest failed: %+v %v", v.Gaps, err)
	}
	if v.Artifact.Rows[0]["o3.candidate.id"] != "ICB001" || v.Artifact.Rows[1]["o3.candidate.id"] != "GGB001" || v.Observations[2].Spatial.Scan.MatchedRows != 1003 || v.Observations[2].Spatial.Comparisons != 2006 {
		t.Fatal("HTTP path ranked prefix or collapsed source identifiers")
	}
}
