package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
)

func TestInspectCommandObservesLegacyUnknownStandard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "300", Title: "legacy unknown delivery"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download/columList.json":
			fmt.Fprint(w, `{"fileName":"표준시설","columList":[{"columCode":"CODE","columNm":"코드"}],"tableVO":{"publicDataPk":"300","svcTableNm":"tn_fixture_svc","colNmList":["CODE"]},"totalCount":10}`)
		case "/download/standard.json":
			fmt.Fprint(w, `[{"CODE":"001"}]`)
		default:
			t.Errorf("unexpected path: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldBase, oldDelay := flagBaseURL, flagDelay
	flagBaseURL, flagDelay = srv.URL, 0
	t.Cleanup(func() { flagBaseURL, flagDelay = oldBase, oldDelay })
	cmd := inspectCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"300", "--observe"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got dataset.InspectionResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Delivery != "STD" || got.Standard == nil || got.Observation == nil || got.Observation.Files[0].SampleRows != 1 {
		t.Fatalf("%s", out.String())
	}
}
