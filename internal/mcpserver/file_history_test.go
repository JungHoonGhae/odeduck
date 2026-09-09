package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInspectDatasetSelectsHistoricalFileWithoutLatestFallback(t *testing.T) {
	isolateConfigHome(t)
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "123", Title: "snapshot", SvcType: catalog.SvcFILE}, {PK: "124", Title: "second snapshot", SvcType: catalog.SvcFILE}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := map[string]string{"/catalog/123/fileData.json": "current.json", "/data/123/fileData.do": "current.html", "/tcs/dss/selectHistAndCsvData.do": "list.html", "/tcs/dss/selectDpkDetailInfo.do": "past.html", "/tcs/dss/selectFileDataDownload.do": "resolved.json", "/cmm/cmm/fileDownload.do": "past.csv"}[strings.ReplaceAll(r.URL.Path, "/124/", "/123/")]
		b, err := os.ReadFile("../dataset/testdata/file-history/" + name)
		if err != nil || name == "" {
			t.Errorf("unexpected fixture request %s: %v", r.URL.Path, err)
			http.Error(w, "missing fixture", 500)
			return
		}
		body := string(b)
		if strings.Contains(r.URL.Path, "/124/") {
			body = strings.ReplaceAll(body, "'123'", "'124'")
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	sess := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), BaseURL: srv.URL}))
	call := func(args map[string]any) (*dataset.InspectionResult, bool) {
		t.Helper()
		args["pk"] = "123"
		res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "inspect_dataset", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			return nil, true
		}
		raw, err := json.Marshal(res.StructuredContent)
		var result dataset.InspectionResult
		if err != nil || json.Unmarshal(raw, &result) != nil {
			t.Fatal("invalid MCP inspection content")
		}
		return &result, false
	}
	listed, failed := call(map[string]any{"fileHistory": true})
	if failed || listed.File == nil || len(listed.File.FileVersions) != 2 || len(listed.File.Assets) != 0 {
		t.Fatalf("MCP failed to expose historical choices: %+v", listed)
	}
	other, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "inspect_dataset", Arguments: map[string]any{"pk": "124", "fileHistory": true}})
	if err != nil || other.IsError {
		t.Fatal("second history listing failed")
	}
	assessment := validConnectionAssessmentArguments()
	assessment["status"], assessment["sample"] = "structurally_verified", nil
	for n, side := range []string{"left", "right"} {
		d := assessment[side].(map[string]any)
		pk := fmt.Sprint(123 + n)
		d["pk"], d["delivery"] = pk, "FILE"
		d["source"] = map[string]any{"url": "https://www.data.go.kr/data/" + pk + "/fileData.do"}
		d["record"] = map[string]any{"kind": "official_spec"}
	}
	uninspected, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "record_connection_assessment", Arguments: assessment})
	if err != nil || !uninspected.IsError {
		t.Fatal("version names alone granted a structural inspection receipt")
	}
	version := listed.File.FileVersions[0].ID
	selected, failed := call(map[string]any{"fileVersion": version, "observe": true, "asset": "snapshot.csv"})
	if failed || selected.File.SelectedFileVersion.ID != version || selected.Observation == nil || selected.Observation.Files[0].SampleRows != 2 {
		t.Fatalf("MCP did not observe selected history: %+v", selected)
	}
	for _, args := range []map[string]any{{"fileHistory": true, "observe": true}, {"fileHistory": true, "delivery": "api"}, {"fileVersion": "unlisted"}} {
		if _, failed := call(args); !failed {
			t.Fatalf("MCP accepted invalid historical selection: %v", args)
		}
	}
}
