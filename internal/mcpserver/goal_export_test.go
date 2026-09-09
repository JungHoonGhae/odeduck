package mcpserver

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
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPMonthlyExportUsesInspectedContractAndSelectedEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "3033304", Title: "export fixture", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	f := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: goalDocumentHTTP(func(r *http.Request) (*http.Response, error) {
		body, contentType := "", "text/html; charset=UTF-8"
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = `{}`
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = `<li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li>`
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
	client := connectTestClient(t, New(Deps{Fetch: f, ShareGoalEvidence: true}))
	call := func(args map[string]any) goalOut {
		t.Helper()
		res, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("MCP: %+v %v", res, err)
		}
		var out goalOut
		if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.State.Gaps) != 0 {
			t.Fatalf("MCP gaps: %+v", out.State.Gaps)
		}
		return out
	}
	v := call(map[string]any{"goal": "report selected monthly source records", "requireSemantic": false})
	for _, wire := range []string{
		`{"action":"define","contract":{"outcome":"source records","region":"source","period":"source","coverage":"sample","roles":[{"id":"r","description":"records"}],"outputs":[{"id":"n","role":"r","type":"string","description":"count"}]}}`,
		`{"action":"search","query":"export fixture","role":"r"}`,
		`{"action":"inspect","pk":"3033304"}`,
		`{"action":"sample","sample":{"pk":"3033304","delivery":"file","operation":"mois-monthly-age-csv","params":{"month":"2026-07","registration":"all","provinceCode":"2800000000","ageFrom":"6","ageTo":"7"}}}`,
	} {
		var d map[string]any
		if err := json.Unmarshal([]byte(wire), &d); err != nil {
			t.Fatal(err)
		}
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Observations) != 1 || v.State.Observations[0].CSV == nil || v.State.Observations[0].CSV.Export == nil {
		t.Fatal("missing export observation")
	}
	body, _ := json.Marshal(v)
	if strings.Contains(string(body), "Province (") || strings.Contains(string(body), "Child (") {
		t.Fatal("MCP sample exposed unselected records")
	}
	o := v.State.Observations[0]
	v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": map[string]any{"action": "read_evidence", "evidence": map[string]any{"observation": o.ID, "rowsSha256": o.RowsSHA256, "rows": []int{2}, "fields": []string{"행정구역"}}}})
	if len(v.State.Evidence) != 1 || v.State.Evidence[0].Records[0].Values["행정구역"] != "Child (2811051000)" || v.State.Evidence[0].Records[0].Origins["행정구역"].Ordinal != 3 || o.CSV.Export.Choices.AgeTo != 7 || o.CSV.Export.Request.Form.Get("sltArgTypeB") != "7" || v.State.Status == "output_ready" {
		t.Fatal("MCP lost typed choices, original positions or completion distinction")
	}
}
