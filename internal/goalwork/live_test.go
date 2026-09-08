package goalwork

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

type goalRoundTripper func(*http.Request) (*http.Response, error)

func (f goalRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type goalFixtureCredentials struct{ reads int }

func (c *goalFixtureCredentials) DataGoKR(context.Context) (string, error) {
	c.reads++
	return "FIXTURE_KEY", nil
}
func (*goalFixtureCredentials) InvalidateDataGoKR() {}
func (*goalFixtureCredentials) External(context.Context, string, string) (string, string, error) {
	return "", "", fmt.Errorf("unexpected external credential access")
}

func TestLiveInspectionOperationCanBePassedDirectlyToDatasetCaller(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "fixture", SvcType: "REST"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"getStanReginCdList", "uddi:084e3ab6-e936-42a6-9358-ba584a543b4f"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: goalRoundTripper(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch r.URL.Host {
				case "www.data.go.kr":
					body = `<table><tr><th class="th">API 유형</th><td class="td">REST</td></tr></table><script>var swaggerJson = ` + "`" + `{"swagger":"2.0","host":"apis.data.go.kr","schemes":["https"],"paths":{"/fixture/` + name + `":{"get":{"summary":"법정동코드 조회","parameters":[{"name":"serviceKey","in":"query","required":true},{"name":"pageNo","in":"query","required":true}]}}}}` + "`" + `;</script>`
				case "apis.data.go.kr":
					calls++
					if r.URL.Path != "/fixture/"+name || r.URL.Query().Get("pageNo") != "1" || r.URL.Query().Get("serviceKey") != "FIXTURE_KEY" {
						t.Error("inspected operation/parameters did not reach exact caller route")
					}
					body = `{"data":[{"code":"001"}]}`
				default:
					t.Errorf("unexpected origin: %s", r.URL.Host)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}))
			credentials := &goalFixtureCredentials{}
			deps := LiveDependencies(client, "", apicall.NewDatasetCaller(client, "", credentials), catalog.Searcher{}, Policy{})
			i, err := deps.Inspect(context.Background(), "123")
			if err != nil || len(i.Operations) != 1 {
				t.Fatalf("inspection failed: %+v %v", i, err)
			}
			got, err := deps.Sample(context.Background(), SampleRequest{PK: "123", Delivery: "api", Operation: i.Operations[0].Name, Params: map[string]string{"pageNo": "1"}, RowPath: "/data"}, i)
			if err != nil || calls != 1 || credentials.reads != 1 || len(got.Rows) != 1 || got.Rows[0]["code"] != "001" {
				t.Fatalf("inspection-to-call contract mismatch: %+v %v calls=%d credentialReads=%d", got, err, calls, credentials.reads)
			}
			b, _ := json.Marshal(i)
			if !strings.Contains(string(b), `"title":"법정동코드 조회"`) || strings.Contains(string(b), "serviceKey") || strings.Contains(string(b), "https://apis.data.go.kr") {
				t.Fatalf("operation meaning or private contract isolation lost: %s", b)
			}
		})
	}
}

func TestLiveLINKInspectionKeepsTypedRegistryNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "fixture", SvcType: "LINK"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tcs/dss/selectApiLinkUrl.do" {
			fmt.Fprint(w, `{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`)
			return
		}
		fmt.Fprint(w, `<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`)
	}))
	defer srv.Close()
	i, err := LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{}).Inspect(context.Background(), "123")
	if err != nil || len(i.Operations) != 5 {
		t.Fatalf("LINK registry lost: %+v %v", i, err)
	}
	found := false
	for _, op := range i.Operations {
		if op.Name == "certificationDetail" {
			found = op.Title == "KC 인증정보 상세"
		}
		if strings.HasSuffix(op.Name, ".json") {
			t.Fatal("REST endpoint-segment naming leaked into typed LINK operation")
		}
	}
	if !found {
		t.Fatal("typed registry name/description lost")
	}
}

func TestLiveInspectionRetainsConflictingNoticesAndMarksTruncation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "fixture", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/123/fileData.json":
			b, _ := json.Marshal(map[string]any{"name": "fixture", "encodingFormat": "CSV", "description": strings.Repeat("한글", 1000), "spatialCoverage": "official region", "temporalCoverage": "2025", "dateModified": "2026-01-01", "serviceKey": "SECRET_NOT_ALLOWED"})
			_, _ = w.Write(b)
		case "/data/123/fileData.do":
			fmt.Fprint(w, `<ul class="info-ul"><li><strong class="key">공간범위</strong><div class="value">different HTML region</div></li><li><strong class="key">시간범위</strong><div class="value">2024</div></li><li><strong class="key">기타 유의사항</strong><div class="value">Current processed codes may differ from another namespace.</div></li></ul>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	deps := LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{})
	i, err := deps.Inspect(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	d := i.Declarations["file"]
	if d.Status != "publisher_declared" || !d.Truncated || len(d.Description) > 2400 || !utf8.ValidString(d.Description) || d.SpatialCoverage != "official region" || d.ModifiedAt != "2026-01-01" || len(d.Notices) != 3 {
		t.Fatalf("incomplete declaration: %+v", d)
	}
	b, _ := json.Marshal(i)
	if strings.Contains(string(b), "SECRET_NOT_ALLOWED") || !strings.Contains(string(b), "different HTML region") || !strings.Contains(string(b), "another namespace") {
		t.Fatal("source notices or metadata allowlist lost")
	}
}

func TestLiveStandardSamplingRequiresInspectedSelectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "300", Title: "무더위쉼터", SvcType: "STD"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download/columList.json" {
			fmt.Fprint(w, `{"fileName":"쉼터","columList":[{"columCode":"CODE","columNm":"코드"}],"tableVO":{"publicDataPk":"300","svcTableNm":"tn_fixture_svc","colNmList":["CODE"]},"totalCount":2000}`)
			return
		}
		if r.URL.Path != "/download/standard.json" || r.URL.Query().Get("perPage") != "1000" {
			t.Errorf("unexpected standard request: %s", r.URL)
		}
		calls++
		fmt.Fprint(w, `[{"CODE":"001"}]`)
	}))
	defer srv.Close()
	deps := LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{})
	inspection, err := deps.Inspect(context.Background(), "300")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(inspection)
	if !strings.Contains(string(encoded), "CODE") || strings.Contains(string(encoded), "tn_fixture_svc") {
		t.Fatalf("planner schema/selector isolation: %s", encoded)
	}
	for _, request := range []SampleRequest{{PK: "300", Delivery: "standard", Params: map[string]string{"svcTableNm": "evil"}}, {PK: "300", Delivery: "standard", Operation: "custom"}, {PK: "300", Delivery: "standard", Asset: "invented.csv"}, {PK: "300", Delivery: "standard", Where: map[string]string{"CODE": "001"}}} {
		if _, err := deps.Sample(context.Background(), request, inspection); err == nil {
			t.Fatal("accepted untrusted standard selector")
		}
	}
	if calls != 0 {
		t.Fatal("invalid requests reached network")
	}
	got, err := deps.Sample(context.Background(), SampleRequest{PK: "300", Delivery: "standard"}, inspection)
	if err != nil || got.Delivery != "STD" || len(got.Rows) != 1 || got.Rows[0]["CODE"] != "001" || got.ContractSHA256 == "" || got.ContentSHA256 == "" || len(got.Warnings) == 0 {
		t.Fatalf("%+v %v", got, err)
	}
	e, err := Start("쉼터 표본", Policy{}, deps)
	if err != nil {
		t.Fatal(err)
	}
	contract := GoalContract{Outcome: "쉼터 코드 표본", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []RoleRequirement{{ID: "shelters", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "code", Description: "쉼터 코드", Role: "shelters", Type: "string"}}}
	for _, d := range []Decision{{Action: "define", Contract: &contract}, {Action: "search", Query: "무더위쉼터", Role: "shelters"}, {Action: "inspect", PK: "300"}, {Action: "sample", Sample: &SampleRequest{PK: "300", Delivery: "standard"}}} {
		v, err := e.Advance(context.Background(), e.View().Revision, d)
		if err != nil || len(v.Gaps) != 0 {
			t.Fatalf("engine rejected STD: %+v %v", v, err)
		}
	}
	if len(e.View().Observations) != 1 || e.View().Observations[0].Delivery != "STD" {
		t.Fatalf("STD observation lost: %+v", e.View())
	}
}

func TestLiveAdaptersSearchInspectDownloadAndJoinThreeCSVFixtures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	entries := []catalog.Entry{{PK: "111", Title: "고령인구", SvcType: "FILE"}, {PK: "222", Title: "쉼터현황", SvcType: "FILE"}, {PK: "333", Title: "구역대응", SvcType: "FILE"}}
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: entries}).Save(); err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{"111": "legal,n\n001,100\n", "222": "admin,name\nA,쉼터\n", "333": "legal,admin,scope\n" + strings.Repeat("999,Z,other\n", 1001) + "001,A,target\n"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/catalog/"):
			fmt.Fprint(w, `{"name":"fixture","encodingFormat":"CSV","description":"Current codes do not prove historical identity","spatialCoverage":"fixture region","temporalCoverage":"2025","creator":{"name":"fixture provider"},"serviceKey":"DO_NOT_EXPOSE"}`)
		case strings.HasPrefix(r.URL.Path, "/data/"):
			pk := strings.Split(r.URL.Path, "/")[2]
			fmt.Fprintf(w, `<button onclick="fileDetailObj.fn_fileDataDown('%s','uddi:x','','1','3')">download</button>`, pk)
		case r.URL.Path == "/tcs/dss/selectFileDataDownload.do":
			_ = r.ParseForm()
			pk := r.Form.Get("publicDataPk")
			fmt.Fprintf(w, `{"status":true,"atchFileId":"%s","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"fixture","orginlFileNm":"%s.csv","atchFileExtsn":"csv"}}`, pk, pk)
		case r.URL.Path == "/cmm/cmm/fileDownload.do":
			fmt.Fprint(w, bodies[r.URL.Query().Get("atchFileId")])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e, err := Start("지역이 다른 원천을 공식 구역 대응으로 연결", Policy{}, LiveDependencies(fetch.New(fetch.WithDelay(0)), srv.URL, nil, catalog.Searcher{}, Policy{}))
	if err != nil {
		t.Fatal(err)
	}
	contract := GoalContract{Outcome: "표본 비교", Region: "fixture", Period: "fixture", Coverage: "sample", Roles: []RoleRequirement{{ID: "고령인구", Description: "인구"}, {ID: "쉼터현황", Description: "쉼터"}}, Outputs: []OutputRequirement{{ID: "population", Description: "인구", Role: "고령인구", Type: "number"}, {ID: "shelter", Description: "쉼터", Role: "쉼터현황", Type: "string"}}}
	decisions := []Decision{{Action: "define", Contract: &contract}}
	for _, entry := range entries {
		request := SampleRequest{PK: entry.PK, Delivery: "file", Asset: entry.PK + ".csv"}
		if entry.PK == "333" {
			request.ScanCSV = true
			request.Where = map[string]string{"scope": "target"}
		}
		decisions = append(decisions, Decision{Action: "search", Query: entry.Title, Role: entry.Title}, Decision{Action: "inspect", PK: entry.PK}, Decision{Action: "sample", Sample: &request})
	}
	p := Composition{ID: "crosswalk", Purpose: "표본 비교", Base: "o1", Joins: []Join{{Right: "o3", LeftKeys: []string{"o1.legal"}, RightKeys: []string{"legal"}}, {Right: "o2", LeftKeys: []string{"o3.admin"}, RightKeys: []string{"admin"}}}, Select: []string{"o1.n", "o2.name"}, Assumptions: []string{"fixture code namespaces and period are declared compatible"}}
	p.Roles = []RoleBinding{{Role: "고령인구", Observation: "o1"}, {Role: "쉼터현황", Observation: "o2"}}
	p.Measures = []Measure{{As: "population_number", Field: "o1.n", Format: "decimal_v1", Unit: "persons"}}
	p.Select = []string{"population_number", "o2.name"}
	p.Outputs = []OutputBinding{{Output: "population", Field: "population_number"}, {Output: "shelter", Field: "o2.name"}}
	decisions = append(decisions, Decision{Action: "compose", Composition: &p}, Decision{Action: "execute", CompositionID: p.ID}, Decision{Action: "abstain", Reason: "fixture mapping does not verify real-world meaning"})
	i := 0
	v, err := Run(context.Background(), e, func(context.Context, View) (Decision, error) {
		if i >= len(decisions) {
			return Decision{}, fmt.Errorf("fixture exhausted: %+v", e.View().Gaps)
		}
		d := decisions[i]
		i++
		return d, nil
	}, nil)
	if err != nil || v.Status != "abstained" || len(v.Artifact.Rows) != 1 || !v.Evaluation.NeedsSemanticReview {
		t.Fatalf("%+v %v", v, err)
	}
	for _, o := range v.Artifact.Sources {
		if o.ContractSHA256 == "" || o.ContentSHA256 == "" || o.Delivery != "FILE" {
			t.Fatalf("missing provenance: %+v", o)
		}
	}
	if len(v.Artifact.Requests) != 3 {
		t.Fatal("missing reproducible requests")
	}
	selection := v.Artifact.Sources[2].Selection
	if selection == nil || selection.Mode != "exact_strings_full_scan_v1" || selection.ScannedRows != 1002 || selection.MatchedRows != 1 || !selection.Exhausted || !v.Artifact.Requests[2].ScanCSV || v.Artifact.Requests[2].Where["scope"] != "target" {
		t.Fatal("selected crosswalk lost its scan provenance or request")
	}
	declaration := v.Artifact.Sources[2].Declaration
	if declaration == nil || declaration.SpatialCoverage != "fixture region" || declaration.TemporalCoverage != "2025" || declaration.Description != "Current codes do not prove historical identity" || declaration.Provider != "fixture provider" || declaration.Status != "publisher_declared" {
		t.Fatalf("source interpretation context lost: %+v", declaration)
	}
	encoded, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(encoded), "DO_NOT_EXPOSE") || !strings.Contains(string(encoded), "Current codes do not prove historical identity") {
		t.Fatal("planning context must keep allowed source declarations, not unrelated metadata")
	}
	if v.Artifact.Rows[0]["population_number"] != json.Number("100") {
		t.Fatal("CSV measure conversion lost")
	}
}
