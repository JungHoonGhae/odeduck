package main

import (
	"bytes"
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
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

type solveDocumentHTTP func(*http.Request) (*http.Response, error)

func (f solveDocumentHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSolveAcquiresSelectedDocumentWithoutTreatingItAsGoalCompletion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "3033304", Title: "documentation", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: solveDocumentHTTP(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = `{}`
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = `<li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li>`
		case "https://jumin.mois.go.kr/ageStatMonth.do":
			body = `<form name="search" action="ageStatMonth.do"><div class="popoverBox"><div class="pContent">PRIVATE_REGION</div><div class="pContent">Observed definition</div><div class="pContent">PRIVATE_DATE</div></div></form>`
		default:
			return nil, fmt.Errorf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=UTF-8"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.LiveDependencies(client, "", nil, catalog.Searcher{}, policy))
		if err != nil {
			return goalwork.View{}, err
		}
		return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
			switch v.Revision {
			case 0:
				return goalwork.Decision{Action: "define", Contract: &goalwork.GoalContract{Outcome: goal, Region: "source", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "value"}}}}, nil
			case 1:
				return goalwork.Decision{Action: "search", Query: "documentation", Role: "r"}, nil
			case 2:
				return goalwork.Decision{Action: "inspect", PK: v.Nodes[0].Hit.PK}, nil
			case 3:
				return goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: v.Nodes[0].Hit.PK, Delivery: "document", Document: &dataset.DocumentSelection{ReferenceID: v.Nodes[0].Inspection.Documents[0].ID, Contains: "Observed"}}}, nil
			case 4:
				if len(v.Observations) != 1 {
					return goalwork.Decision{}, fmt.Errorf("document missing: %+v", v.Gaps)
				}
				o := v.Observations[0]
				return goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{1}, Fields: []string{"text"}}}, nil
			default:
				return goalwork.Decision{Action: "abstain", Reason: "document acquired; data and applicability still required"}, nil
			}
		}, progress)
	})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"report source value and definition", "--require-semantic=false", "--agent=claude", "--share-evidence"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("support document was goal completion")
	}
	var v goalwork.View
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Status != "abstained" || len(v.Gaps) != 1 || v.Gaps[0].Action != "abstain" || len(v.Evidence) != 1 || v.Observations[0].Document == nil || v.Evidence[0].Records[0].Origins["text"].Kind != "document_block" || v.Evidence[0].Records[0].Origins["text"].Ordinal != 2 || v.Evidence[0].Records[0].Values["text"] != "Observed definition" || strings.Contains(out.String(), "PRIVATE_") {
		t.Fatalf("CLI document evidence contract: %+v", v)
	}
}
