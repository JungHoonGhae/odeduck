package main

import (
	"bytes"
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
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestSolveMonthlyExportUsesInspectedOperationAndRetainsProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "3033304", Title: "export fixture", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: solveDocumentHTTP(func(r *http.Request) (*http.Response, error) {
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
			b, err := os.ReadFile(filepath.Join("..", "..", "internal", "dataset", "testdata", name))
			if err != nil {
				return nil, err
			}
			body = string(b)
		default:
			return nil, fmt.Errorf("unexpected HTTP: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	cmd := solveCommand(func(ctx context.Context, goal string, policy goalwork.Policy, _ string, progress func(goalwork.View)) (goalwork.View, error) {
		e, err := goalwork.Start(goal, policy, goalwork.LiveDependencies(client, "", nil, catalog.Searcher{}, policy))
		if err != nil {
			return goalwork.View{}, err
		}
		return goalwork.Run(ctx, e, func(_ context.Context, v goalwork.View) (goalwork.Decision, error) {
			switch v.Revision {
			case 0:
				return goalwork.Decision{Action: "define", Contract: &goalwork.GoalContract{Outcome: goal, Region: "source", Period: "source", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "count"}}}}, nil
			case 1:
				return goalwork.Decision{Action: "search", Query: "export fixture", Role: "r"}, nil
			case 2:
				return goalwork.Decision{Action: "inspect", PK: v.Nodes[0].Hit.PK}, nil
			case 3:
				return goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: v.Nodes[0].Hit.PK, Delivery: "file", Operation: v.Nodes[0].Inspection.Exports[0].ID, Params: map[string]string{"month": "2026-07", "registration": "all", "provinceCode": "2800000000", "ageFrom": "6", "ageTo": "7"}}}, nil
			default:
				return goalwork.Decision{Action: "abstain", Reason: "export acquired; independent source applicability comparison remains"}, nil
			}
		}, progress)
	})
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"report original selected records", "--require-semantic=false", "--agent=claude"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("acquisition became goal completion")
	}
	var v goalwork.View
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Status != "abstained" || len(v.Observations) != 1 || len(v.SampleAttempts) != 1 || v.SampleAttempts[0].Request.Params["ageTo"] != "7" || v.Observations[0].CSV.Export.Choices.Month != "2026-07" || v.Observations[0].CSV.DataRecords[0] != 2 || strings.Contains(out.String(), "Province (") {
		t.Fatalf("CLI lost inspected export / private original rows: %+v", v)
	}
}
