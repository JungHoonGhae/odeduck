package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type goalDocumentHTTP func(*http.Request) (*http.Response, error)

func (f goalDocumentHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMCPDocumentSelectionUsesActualInspectionAndEvidenceContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "3033304", Title: "documentation", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	f := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: goalDocumentHTTP(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = `{}`
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = `<li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li>`
		case "https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124":
			body = `<div class="answers"><div class="tbx">UNSELECTED_HEADER</div><div class="an_txt">Original reply text</div></div><input id="boardIdx" name="boardIdx" value="22124">`
		default:
			return nil, fmt.Errorf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=UTF-8"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	client := connectTestClient(t, New(Deps{Fetch: f, ShareGoalEvidence: true}))
	call := func(args map[string]any) goalOut {
		t.Helper()
		r, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "advance_goal", Arguments: args})
		if err != nil || r.IsError {
			t.Fatalf("MCP: %+v %v", r, err)
		}
		body := r.Content[0].(*mcp.TextContent).Text
		if strings.Contains(body, "UNSELECTED_") {
			t.Fatal("MCP exposed unselected document")
		}
		var out goalOut
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.State.Gaps) != 0 {
			t.Fatalf("MCP gaps: %+v", out.State.Gaps)
		}
		return out
	}
	v := call(map[string]any{"goal": "report source value with definition", "requireSemantic": false})
	for _, wire := range []string{
		`{"action":"define","contract":{"outcome":"source value","region":"source","period":"source","coverage":"sample","roles":[{"id":"r","description":"records"}],"outputs":[{"id":"n","role":"r","type":"string","description":"value"}]}}`,
		`{"action":"search","query":"documentation","role":"r"}`,
		`{"action":"inspect","pk":"3033304"}`,
		`{"action":"sample","sample":{"pk":"3033304","delivery":"document","document":{"referenceId":"kosis-answer-22124","contains":"Original"}}}`,
	} {
		var d map[string]any
		if err := json.Unmarshal([]byte(wire), &d); err != nil {
			t.Fatal(err)
		}
		v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": d})
	}
	if len(v.State.Observations) != 1 || v.State.Observations[0].Document == nil {
		t.Fatal("missing document observation")
	}
	body, _ := json.Marshal(v)
	if strings.Contains(string(body), "Original reply text") {
		t.Fatal("sample exposed document text")
	}
	o := v.State.Observations[0]
	v = call(map[string]any{"sessionId": v.SessionID, "revision": v.State.Revision, "decision": map[string]any{"action": "read_evidence", "evidence": map[string]any{"observation": o.ID, "rowsSha256": o.RowsSHA256, "rows": []int{1}, "fields": []string{"text"}}}})
	if len(v.State.Evidence) != 1 || v.State.Evidence[0].Records[0].Values["text"] != "Original reply text" || v.State.Evidence[0].Records[0].Origins["text"].Kind != "document_block" || v.State.Evidence[0].Records[0].Origins["text"].Ordinal != 2 || o.Document.Reference.Basis != "adapter_reference" || v.State.Status == "output_ready" {
		t.Fatal("MCP lost provenance, selection or support-only boundary")
	}
}
