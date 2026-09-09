package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

func TestInspectCommandSelectsHistoricalFileWithoutLatestFallback(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("HOME", configRoot)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(configRoot, ".config"))
	t.Setenv("APPDATA", filepath.Join(configRoot, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "snapshot", SvcType: catalog.SvcFILE}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := map[string]string{"/catalog/123/fileData.json": "current.json", "/data/123/fileData.do": "current.html", "/tcs/dss/selectHistAndCsvData.do": "list.html", "/tcs/dss/selectDpkDetailInfo.do": "past.html", "/tcs/dss/selectFileDataDownload.do": "resolved.json", "/cmm/cmm/fileDownload.do": "past.csv"}[r.URL.Path]
		b, err := os.ReadFile("../../internal/dataset/testdata/file-history/" + name)
		if err != nil || name == "" {
			t.Errorf("unexpected fixture request %s: %v", r.URL.Path, err)
			http.Error(w, "missing fixture", 500)
			return
		}
		fmt.Fprint(w, string(b))
	}))
	defer srv.Close()
	oldBase, oldFormat := flagBaseURL, flagFormat
	flagBaseURL, flagFormat = srv.URL, "json"
	t.Cleanup(func() { flagBaseURL, flagFormat = oldBase, oldFormat })
	run := func(args ...string) (*dataset.InspectionResult, error) {
		cmd := inspectCmd()
		cmd.SetArgs(append([]string{"123"}, args...))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			return nil, err
		}
		var result dataset.InspectionResult
		return &result, json.Unmarshal(out.Bytes(), &result)
	}
	listed, err := run("--file-history")
	if err != nil || listed.File == nil || len(listed.File.FileVersions) != 2 || len(listed.File.Assets) != 0 {
		t.Fatalf("CLI failed to expose historical choices: %+v %v", listed, err)
	}
	version := listed.File.FileVersions[0].ID
	selected, err := run("--file-version", version, "--observe", "--asset", "snapshot.csv")
	if err != nil || selected.File.SelectedFileVersion.ID != version || selected.Observation == nil || selected.Observation.Files[0].SampleRows != 2 {
		t.Fatalf("CLI did not observe selected history: %+v %v", selected, err)
	}
	for _, args := range [][]string{{"--file-history", "--observe"}, {"--file-history", "--delivery", "api"}, {"--file-version", "unlisted"}} {
		if _, err := run(args...); err == nil {
			t.Fatalf("CLI accepted invalid historical selection: %v", args)
		}
	}
}
