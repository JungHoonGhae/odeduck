package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/oddsock/internal/agentplan"
	"github.com/JungHoonGhae/oddsock/internal/catalog"
	"github.com/JungHoonGhae/oddsock/internal/portal"
)

type syncSourceFunc func(context.Context, string, int, func(int)) (*catalog.Catalog, error)

func (f syncSourceFunc) Sync(ctx context.Context, scope string, perPage int, progress func(int)) (*catalog.Catalog, error) {
	return f(ctx, scope, perPage, progress)
}

func TestRootCommandPresentsOddsockBrand(t *testing.T) {
	if rootCmd.Use != "oddsock" {
		t.Fatalf("root command use = %q", rootCmd.Use)
	}
	if !strings.Contains(rootCmd.Long, "oddsock") {
		t.Fatalf("root command does not present the product brand: %q", rootCmd.Long)
	}
}

func TestCatalogSearchKeepsLinkDatasetsDiscoverableByDefault(t *testing.T) {
	cmd := catalogQueryCmd(false)
	flag := cmd.Flags().Lookup("rest-only")
	if flag == nil {
		t.Fatal("catalog search is missing --rest-only")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--rest-only default = %q, want false so LINK datasets are not buried", flag.DefValue)
	}
}

func TestCatalogSyncDefaultsToAPIAndFileDiscovery(t *testing.T) {
	cmd := catalogSyncCmd()
	flag := cmd.Flags().Lookup("type")
	if flag == nil {
		t.Fatal("catalog sync is missing --type")
	}
	if flag.DefValue != "ALL" {
		t.Fatalf("--type default = %q, want ALL so file datasets join the discovery catalogue", flag.DefValue)
	}
	source := cmd.Flags().Lookup("source")
	if source == nil || source.DefValue != "auto" {
		t.Fatalf("--source = %+v, want auto so documented API is preferred with web fallback", source)
	}
}

func TestCatalogInfoReportsCollectionSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	if err := (&catalog.Catalog{
		SyncedAt: time.Now(), Type: "ALL", Source: catalog.SourceOfficial,
		Entries: []catalog.Entry{{PK: "100", Title: "공식 데이터", SvcType: catalog.SvcREST}},
	}).Save(); err != nil {
		t.Fatal(err)
	}
	oldFormat := flagFormat
	flagFormat = "json"
	t.Cleanup(func() { flagFormat = oldFormat })

	cmd := catalogInfoCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var info struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Source != catalog.SourceOfficial {
		t.Fatalf("catalog source = %q, want official", info.Source)
	}
}

func TestInspectCommandObservesFileSchemaThroughUnifiedDatasetPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	const pk = "15151047"
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{
		PK: pk, Title: "성장상권", SvcType: catalog.SvcFILE, DataTypes: []string{"FILE"},
	}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog/" + pk + "/fileData.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"alternateName":"성장상권","creator":{"name":"소상공인시장진흥공단"},"encodingFormat":"CSV","@type":"Dataset"}`))
		case "/data/" + pk + "/fileData.do":
			_, _ = w.Write([]byte(`<button onclick="fileDetailObj.fn_fileDataDown('15151047','uddi:growth','','1','3')">다운로드</button>
				<li><strong class="key">파일데이터명</strong><div class="value">성장상권</div></li>
				<li><strong class="key">확장자</strong><div class="value">CSV</div></li>`))
		case "/tcs/dss/selectFileDataDownload.do":
			_, _ = w.Write([]byte(`{"status":true,"atchFileId":"FILE_123","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"성장상권","orginlFileNm":"growth.csv","atchFileExtsn":"csv"}}`))
		case "/cmm/cmm/fileDownload.do":
			_, _ = w.Write([]byte("CRTR_YM,MJR_BZZNNO\n202401,100\n"))
		default:
			t.Fatalf("unexpected request: %s", r.URL)
		}
	}))
	defer srv.Close()
	oldBase, oldFormat := flagBaseURL, flagFormat
	flagBaseURL, flagFormat = srv.URL, "json"
	t.Cleanup(func() { flagBaseURL, flagFormat = oldBase, oldFormat })

	cmd := inspectCmd()
	cmd.SetArgs([]string{pk, "--observe"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"delivery": "FILE"`) || !strings.Contains(stdout.String(), `"MJR_BZZNNO"`) {
		t.Fatalf("inspect output = %s", stdout.String())
	}
}

func TestCatalogSyncIfStaleUpgradesFreshAPISnapshotToAll(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "API"}).Save(); err != nil {
		t.Fatal(err)
	}
	fileRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dType") == "FILE" {
			fileRequests++
		}
		_, _ = w.Write([]byte(`<html></html>`))
	}))
	defer srv.Close()
	oldBase := flagBaseURL
	flagBaseURL = srv.URL
	t.Cleanup(func() { flagBaseURL = oldBase })
	cmd := catalogSyncCmd()
	cmd.SetArgs([]string{"--if-stale"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	if fileRequests == 0 || got.Type != "ALL" || got.Source != catalog.SourceWeb {
		t.Fatalf("file requests=%d catalog type/source=%q/%q", fileRequests, got.Type, got.Source)
	}
}

func TestCatalogSyncOfficialUsesApprovedHeaderSourceWithoutWebScraping(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	configDir, err := portal.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "datagokr-apikey"), []byte("approved-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	var portalRequests int
	portalServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		portalRequests++
	}))
	defer portalServer.Close()

	oldBase, oldOfficialSource := flagBaseURL, newOfficialSource
	flagBaseURL = portalServer.URL
	newOfficialSource = func(key string) catalog.SyncSource {
		if key != "approved-key" {
			t.Fatalf("official source key = %q", key)
		}
		return syncSourceFunc(func(_ context.Context, scope string, _ int, progress func(int)) (*catalog.Catalog, error) {
			if scope != "ALL" {
				t.Fatalf("official source scope = %q", scope)
			}
			if progress != nil {
				progress(1)
			}
			return &catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Source: catalog.SourceOfficial, Entries: []catalog.Entry{{
				PK: "300", Title: "상권 매출 파일", SvcType: catalog.SvcFILE, DataTypes: []string{"FILE"},
			}}}, nil
		})
	}
	t.Cleanup(func() { flagBaseURL, newOfficialSource = oldBase, oldOfficialSource })
	cmd := catalogSyncCmd()
	cmd.SetArgs([]string{"--source", "official", "--per-page", "10"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != catalog.SourceOfficial || len(got.Entries) != 1 || got.Entries[0].PK != "300" {
		t.Fatalf("official snapshot = %+v", got)
	}
	if portalRequests != 0 {
		t.Fatalf("official credentials were routed through --base-url (%d requests)", portalRequests)
	}
}

func TestCatalogInstallSnapshotCommandInstallsValidatedPrebuilt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	snapshotPath := filepath.Join(t.TempDir(), "oddsock-catalog.json.gz")
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if err := json.NewEncoder(zw).Encode(&catalog.Catalog{
		SyncedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Type: "ALL", Source: catalog.SourceOfficial,
		Entries: []catalog.Entry{{PK: "15000001", Title: "공식 사전 구축 데이터", SvcType: catalog.SvcFILE, DataTypes: []string{"FILE"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	oldFormat := flagFormat
	flagFormat = "json"
	t.Cleanup(func() { flagFormat = oldFormat })
	checkCmd := catalogInstallSnapshotCmd()
	checkCmd.SetArgs([]string{snapshotPath, "--check-only"})
	var checkOutput bytes.Buffer
	checkCmd.SetOut(&checkOutput)
	checkCmd.SetErr(&bytes.Buffer{})
	if err := checkCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(checkOutput.String(), `"reason": "validated"`) {
		t.Fatalf("validation output = %s", checkOutput.String())
	}
	if _, err := catalog.Load(); err == nil {
		t.Fatal("--check-only must not install the snapshot")
	}

	cmd := catalogInstallSnapshotCmd()
	cmd.SetArgs([]string{snapshotPath})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"installed": true`) {
		t.Fatalf("install output = %s", stdout.String())
	}
	loaded, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source != catalog.SourceOfficial || len(loaded.Entries) != 1 || loaded.Entries[0].PK != "15000001" {
		t.Fatalf("installed snapshot = %+v", loaded)
	}
}

func TestCatalogValidateReleaseCommandGatesGoldenSearchQuality(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	golden, err := catalog.DefaultReleaseGoldenSet()
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]catalog.Entry, 0, len(golden.Cases))
	for _, item := range golden.Cases {
		entries = append(entries, catalog.Entry{PK: item.ExpectedPK, Title: item.Query, SvcType: item.ExpectedSvcType})
	}
	save := func(candidate []catalog.Entry) {
		t.Helper()
		if err := (&catalog.Catalog{
			SyncedAt: time.Now(), Type: "ALL", Source: catalog.SourceCombined, Entries: candidate,
		}).Save(); err != nil {
			t.Fatal(err)
		}
	}
	run := func() (string, error) {
		t.Helper()
		cmd := catalogValidateReleaseCmd()
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		return stdout.String(), err
	}

	save(entries)
	passed, err := run()
	if err != nil {
		t.Fatalf("passing release gate: %v", err)
	}
	if !strings.Contains(passed, `"passed": true`) {
		t.Fatalf("passing report = %s", passed)
	}

	save(entries[:len(entries)-1])
	failed, err := run()
	if err == nil || !strings.Contains(err.Error(), "검색 품질 회귀") {
		t.Fatalf("failing release gate error = %v", err)
	}
	if !strings.Contains(failed, `"passed": false`) {
		t.Fatalf("failing report = %s", failed)
	}
}

func TestApplyCommandRejectsLinkAndSurfacesProviderApplication(t *testing.T) {
	const pk = "15116894"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/" + pk + "/openapi.do":
			_, _ = w.Write([]byte(`<ul><li><strong class="key">API 유형</strong><div class="value">LINK</div></li></ul>`))
		case "/tcs/dss/selectApiLinkUrl.do":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"linkUrl":"https://www.safetykorea.kr/release/openapi","status":true}`))
		default:
			t.Fatalf("unexpected request before LINK apply rejection: %s", r.URL)
		}
	}))
	defer srv.Close()

	oldBase := flagBaseURL
	flagBaseURL = srv.URL
	t.Cleanup(func() { flagBaseURL = oldBase })
	cmd := applyCmd()
	cmd.SetArgs([]string{pk, "--purpose", "제품 안전 분석", "--category", "research", "--yes"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "LINK") || !strings.Contains(err.Error(), "https://www.safetykorea.kr/release/openapi2") {
		t.Fatalf("LINK apply error = %v", err)
	}
}

func TestMergeBridgeAxesPrioritizesPostRetrievalAndDeduplicatesRoles(t *testing.T) {
	initial := []catalog.DiscoveryAxis{
		{Role: "수요", Query: "인구 이동"},
		{Role: "위험", Query: "침수 위험"},
	}
	expanded := []catalog.DiscoveryAxis{
		{Role: "상권", Query: "점포 개폐업"},
		{Role: "수요", Query: "전월세 거래"},
	}
	got := mergeBridgeAxes(expanded, initial, 3)
	if len(got) != 3 {
		t.Fatalf("axes = %+v", got)
	}
	if got[0].Role != "상권" || got[1].Query != "전월세 거래" || got[2].Role != "위험" {
		t.Fatalf("post-retrieval priority/dedup = %+v", got)
	}
}

func TestSelectAnchorHitsDoesNotGuessFromUnstructuredResults(t *testing.T) {
	hits := []catalog.Hit{{PK: "popular"}, {PK: "anchor", Role: "anchor"}}
	got := selectAnchorHits(hits, 1)
	if len(got) != 1 || got[0].PK != "anchor" {
		t.Fatalf("anchors = %+v", got)
	}
}

func TestCatalogDiscoverRunsProgressiveThreeStageFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	cat := &catalog.Catalog{SyncedAt: time.Now(), Type: "API", Entries: []catalog.Entry{
		{PK: "anchor", Title: "온비드 공매 물건", SvcType: catalog.SvcREST},
		{PK: "demand", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
		{PK: "risk", Title: "침수 위험 지도", SvcType: catalog.SvcREST},
		{PK: "risk-history", Title: "침수 위험 이력", SvcType: catalog.SvcREST},
	}}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}

	oldGenerate, oldExpand, oldCompose := generateDiscoveryPlan, expandDiscoveryPlan, composeDiscoveryPlan
	oldFormat := flagFormat
	t.Cleanup(func() {
		generateDiscoveryPlan, expandDiscoveryPlan, composeDiscoveryPlan = oldGenerate, oldExpand, oldCompose
		flagFormat = oldFormat
	})
	flagFormat = "json"
	spatial := catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}
	generateDiscoveryPlan = func(context.Context, string, string) (agentplan.Plan, error) {
		return agentplan.Plan{Provider: agentplan.ProviderCodex, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
			{Role: "anchor", Query: "온비드 공매 물건"},
			{Role: "상권 수요", Query: "상권 점포 개폐업", Contribution: "수요를 비교", Edge: spatial},
		}}, nil
	}
	expandDiscoveryPlan = func(_ context.Context, _ string, provider string, _ agentplan.Plan, observed []catalog.Hit) (agentplan.Plan, error) {
		if provider != agentplan.ProviderCodex || len(observed) == 0 {
			t.Fatalf("expand provider=%q observed=%+v", provider, observed)
		}
		return agentplan.Plan{Provider: provider, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{{
			Role: "재난 위험", Query: "침수 위험", Contribution: "위험조정 판단", Edge: spatial,
		}}}, nil
	}
	composeDiscoveryPlan = func(_ context.Context, _ string, provider string, anchors, candidates []catalog.Hit) (agentplan.SelectionPlan, error) {
		if provider != agentplan.ProviderCodex || len(anchors) != 1 || anchors[0].PK != "anchor" {
			t.Fatalf("compose provider=%q anchors=%+v", provider, anchors)
		}
		for _, hit := range candidates {
			if hit.PK == "risk-history" {
				return agentplan.SelectionPlan{Provider: provider, Status: agentplan.StatusUsed, Selections: []catalog.BridgeSelection{{
					PK: hit.PK, WhyCandidate: "침수 위험 이력이라는 제목이 위험 역할을 뒷받침",
				}}}, nil
			}
		}
		t.Fatalf("risk candidate missing: %+v", candidates)
		return agentplan.SelectionPlan{}, nil
	}

	cmd := catalogQueryCmd(true)
	cmd.SetArgs([]string{"공매 투자 판단", "--semantic=false", "--limit=2"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("discover: %v\nstderr: %s", err, stderr.String())
	}
	var result struct {
		Connections []catalog.ConnectionCandidate `json:"connections"`
		Planner     agentplan.Plan                `json:"planner"`
		Bridge      agentplan.Plan                `json:"bridgePlanner"`
		Selection   agentplan.SelectionPlan       `json:"selectionPlanner"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.Planner.Status != agentplan.StatusUsed || result.Bridge.Status != agentplan.StatusUsed ||
		result.Selection.Status != agentplan.StatusUsed || len(result.Connections) != 1 {
		t.Fatalf("progressive result = %+v\nstderr: %s", result, stderr.String())
	}
	connection := result.Connections[0]
	if connection.Anchor.PK != "anchor" || connection.Bridge.PK != "risk-history" || connection.BridgeRole != "재난 위험" {
		t.Fatalf("connection = %+v", connection)
	}
}

func TestCatalogDiscoverAbstainsWhenInitialSearchHasNoAnchor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	cat := &catalog.Catalog{SyncedAt: time.Now(), Type: "API", Entries: []catalog.Entry{
		{PK: "bridge", Title: "상권 점포 개폐업", SvcType: catalog.SvcREST},
	}}
	if err := cat.Save(); err != nil {
		t.Fatal(err)
	}

	oldGenerate, oldFormat := generateDiscoveryPlan, flagFormat
	t.Cleanup(func() {
		generateDiscoveryPlan = oldGenerate
		flagFormat = oldFormat
	})
	flagFormat = "json"
	generateDiscoveryPlan = func(context.Context, string, string) (agentplan.Plan, error) {
		return agentplan.Plan{Provider: agentplan.ProviderCodex, Status: agentplan.StatusUsed, Axes: []catalog.DiscoveryAxis{
			{Role: "anchor", Query: "존재하지 않는 직접 대상"},
			{Role: "수요", Query: "상권 점포", Contribution: "수요 비교", Edge: catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
			{Role: "위험", Query: "침수 위험", Contribution: "위험 비교", Edge: catalog.EdgeHypothesis{Kinds: []string{"spatial"}, ExpectedKeys: []string{"법정동코드"}}},
		}}, nil
	}

	cmd := catalogQueryCmd(true)
	cmd.SetArgs([]string{"사업 기회", "--semantic=false"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Connections []catalog.ConnectionCandidate `json:"connections"`
		Abstention  *catalog.Abstention           `json:"abstention"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Connections) != 0 || result.Abstention == nil {
		t.Fatalf("missing anchor must abstain: %+v", result)
	}
}
