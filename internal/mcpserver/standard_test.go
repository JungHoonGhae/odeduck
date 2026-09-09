package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInspectDatasetStandardDoesNotInventFileOrAPIContract(t *testing.T) {
	isolateConfigHome(t)
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "300", Title: "표준시설", SvcType: "STD"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download/columList.json":
			fmt.Fprint(w, `{"fileName":"표준시설","columList":[{"columCode":"CODE","columNm":"코드"},{"columCode":"MISSING","columNm":"미관측"}],"tableVO":{"publicDataPk":"300","svcTableNm":"tn_fixture_svc","colNmList":["CODE","MISSING"]},"totalCount":10}`)
		case "/download/standard.json":
			fmt.Fprint(w, `[{"CODE":"001"}]`)
		default:
			t.Errorf("unexpected path: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client := connectTestClient(t, New(Deps{Fetch: fetch.New(fetch.WithDelay(0)), BaseURL: srv.URL}))
	res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "inspect_dataset", Arguments: map[string]any{"pk": "300", "delivery": "standard", "observe": true}})
	if err != nil || res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var got dataset.InspectionResult
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Delivery != "STD" || got.Standard == nil || got.API != nil || got.File != nil || got.Observation == nil || got.Observation.Bytes == 0 || strings.Join(got.Observation.Files[0].Columns, ",") != "CODE" {
		t.Fatalf("inspection: %s", b)
	}
	if strings.Contains(string(b), "tn_fixture_svc") || strings.Contains(string(b), `"001"`) {
		t.Fatalf("metadata leaked selectors or raw rows: %s", b)
	}
}
