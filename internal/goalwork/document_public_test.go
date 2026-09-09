package goalwork_test

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
	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

type documentHTTP func(*http.Request) (*http.Response, error)

func (f documentHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Only the external HTTP boundary is substituted. Catalogue, inspection,
// selection, evidence and review-input construction use production modules.
func documentEngine(t *testing.T, disclosure bool, review func(context.Context, goalwork.ReviewInput) (goalwork.ReviewAssessment, error)) *goalwork.Engine {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	if err := (&catalog.Catalog{SyncedAt: time.Now(), Type: "ALL", Entries: []catalog.Entry{{PK: "12345678", Title: "counts", SvcType: "FILE"}, {PK: "3033304", Title: "documentation", SvcType: "FILE"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Transport: documentHTTP(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/12345678/fileData.json", "https://www.data.go.kr/catalog/3033304/fileData.json":
			body = `{}`
		case "https://www.data.go.kr/data/3033304/fileData.do":
			body = `<ul><li><strong class="key">URL</strong><div class="value"><a href="https://jumin.mois.go.kr/ageStatMonth.do">official</a></div></li></ul>`
		case "https://www.data.go.kr/data/12345678/fileData.do":
			body = `<script>fileDetailObj.fn_fileDataDown('12345678','detail','FILE_fixture','1','fixture')</script>`
		case "https://www.data.go.kr/tcs/dss/selectFileDataDownload.do":
			body = `{"status":true,"atchFileId":"FILE_fixture","fileDetailSn":"1","fileDataRegistVO":{"dataNm":"counts","orginlFileNm":"counts.csv","atchFileExtsn":"CSV"}}`
		case "https://www.data.go.kr/cmm/cmm/fileDownload.do?atchFileId=FILE_fixture&dataNm=counts&fileDetailSn=1":
			body = "count\n12\n"
		case "https://jumin.mois.go.kr/ageStatMonth.do":
			body = `<html><body><form name="search" action="ageStatMonth.do"><div class="popoverBox"><div class="pContent">UNSELECTED_REGION</div></div><div class="popoverBox"><div class="pContent"><p>Definition of count</p><p>Published records, not current capacity</p></div></div><div class="popoverBox"><div class="pContent">UNSELECTED_DATE</div></div></form></body></html>`
		default:
			return nil, fmt.Errorf("unexpected HTTP: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=UTF-8"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}))
	policy := goalwork.Policy{}
	if disclosure {
		policy.EvidenceRecipient = "claude"
		if review != nil {
			policy.ReviewRecipient, policy.ReviewAnalyses = "claude", true
		}
	}
	deps := goalwork.LiveDependencies(client, "", nil, catalog.Searcher{}, policy)
	deps.Review = review
	e, err := goalwork.Start("Report published counts with their definition and limitations", policy, deps)
	if err != nil {
		t.Fatal(err)
	}
	c := goalwork.GoalContract{Outcome: "published counts", Region: "observed source", Period: "source snapshot", Coverage: "sample", Roles: []goalwork.RoleRequirement{{ID: "r", Description: "count records"}}, Outputs: []goalwork.OutputRequirement{{ID: "n", Role: "r", Type: "string", Description: "published count"}}}
	advanceUnmatched(t, e, goalwork.Decision{Action: "define", Contract: &c})
	for _, x := range []struct{ pk, query, wire string }{
		{"12345678", "counts", `{"pk":"12345678","delivery":"file","asset":"counts.csv"}`},
		{"3033304", "documentation", `{"pk":"3033304","delivery":"document","document":{"referenceId":"mois-monthly-help"}}`},
	} {
		advanceUnmatched(t, e, goalwork.Decision{Action: "search", Query: x.query, Role: "r"})
		advanceUnmatched(t, e, goalwork.Decision{Action: "inspect", PK: x.pk})
		var s goalwork.SampleRequest
		if err := json.Unmarshal([]byte(x.wire), &s); err != nil {
			t.Fatal(err)
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &s})
	}
	return e
}

func TestOfficialDocumentReachesReviewOnlyAsSelectedExplicitSupport(t *testing.T) {
	calls := 0
	e := documentEngine(t, true, func(_ context.Context, in goalwork.ReviewInput) (goalwork.ReviewAssessment, error) {
		calls++
		if len(in.Artifact.Sources) != 1 || in.Artifact.Rows[0]["o1.count"] != "12" {
			t.Fatal("document changed calculation")
		}
		if calls == 1 {
			if in.Analysis != nil {
				t.Fatal("document attached without explicit support")
			}
		} else {
			if in.Analysis == nil || len(in.Analysis.SourceContext) != 1 {
				t.Fatal("missing supporting document")
			}
			c := in.Analysis.SourceContext[0]
			if c.Source.Delivery != "DOCUMENT" || !c.Proposed || c.Targets[0] != "o1" || reviewPacket(t, in, c.PacketID).Records[0].Values["text"] != "Definition of count Published records, not current capacity" {
				t.Fatalf("wrong document context: %+v", c)
			}
			origin := reviewPacket(t, in, c.PacketID).Records[0].Origins["text"]
			if origin.Kind != "document_block" || origin.Ordinal != 2 {
				t.Fatalf("not original section address: %+v", origin)
			}
			encoded, _ := json.Marshal(in)
			if strings.Contains(string(encoded), "UNSELECTED_") || !strings.Contains(string(encoded), `"referenceId":"mois-monthly-help"`) {
				t.Fatal("document disclosure or request provenance lost")
			}
		}
		a := supportedSourceReview(in)
		if in.Analysis != nil {
			a = supportedAnalysisReview(in)
		}
		a.Outputs[0].Output = "n"
		a.GoalFit.Verdict = "insufficient" // A fixture verdict is not applicability proof.
		return a, nil
	})
	encoded, _ := json.Marshal(e.PlanningView())
	if strings.Contains(string(encoded), "Definition of count") || strings.Contains(string(encoded), "UNSELECTED_") {
		t.Fatal("sampling exposed original text")
	}
	for _, x := range []struct {
		index, row int
		field      string
	}{{0, 1, "count"}, {1, 2, "text"}} {
		o := e.View().Observations[x.index]
		advanceUnmatched(t, e, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{x.row}, Fields: []string{x.field}}})
	}
	p := goalwork.Composition{ID: "counts", Base: "o1", Purpose: "published count report", Select: []string{"o1.count"}, Roles: []goalwork.RoleBinding{{Role: "r", Observation: "o1"}}, Outputs: []goalwork.OutputBinding{{Output: "n", Field: "o1.count"}}, Assumptions: []string{"source report, not capacity approval"}}
	for n := 0; n < 2; n++ {
		if n == 1 {
			p.ID = "counts-supported"
			p.Support = []goalwork.SupportBinding{{PacketID: e.View().Evidence[1].ID, Targets: []string{"o1"}, Purpose: "Check definition applicability"}}
		}
		advanceUnmatched(t, e, goalwork.Decision{Action: "compose", Composition: &p})
		advanceUnmatched(t, e, goalwork.Decision{Action: "execute", CompositionID: p.ID})
		advanceUnmatched(t, e, goalwork.Decision{Action: "review_result", CompositionID: p.ID})
	}
	if calls != 2 || e.View().Status != "review_required" {
		t.Fatal("document acquisition became goal approval")
	}
}

func TestDocumentCannotBecomeComputationalInput(t *testing.T) {
	for _, scenario := range []string{"base", "join"} {
		t.Run(scenario, func(t *testing.T) {
			e := documentEngine(t, false, nil)
			p := goalwork.Composition{ID: "invalid", Base: "o2", Purpose: "treat documentation as records", Select: []string{"o2.text"}, Assumptions: []string{"this must not authorize document computation"}}
			if scenario == "join" {
				p.Base = "o1"
				p.Joins = []goalwork.Join{{Right: "o2", LeftKeys: []string{"o1.count"}, RightKeys: []string{"text"}}}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "compose", Composition: &p})
			if err != nil || len(v.Gaps) != 1 || len(v.Compositions) != 0 || !strings.Contains(v.Gaps[0].Detail, "support-only") {
				t.Fatalf("document accepted as computation: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestDocumentDisclosureRemainsDisabledByDefault(t *testing.T) {
	e := documentEngine(t, false, nil)
	o := e.View().Observations[1]
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "read_evidence", Evidence: &goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: o.RowsSHA256, Rows: []int{2}, Fields: []string{"text"}}})
	if err != nil || len(v.Gaps) != 1 || len(v.Evidence) != 0 {
		t.Fatal("public document bypassed disclosure authority")
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "Definition of count") || strings.Contains(string(encoded), "UNSELECTED_") {
		t.Fatal("document text escaped through metadata")
	}
}

func TestDocumentSelectorsCannotMixWithDataAcquisitionOrReduction(t *testing.T) {
	for _, variant := range []string{"wrong delivery", "no selection", "asset", "params", "where", "whereIn", "fileVersion", "xlsx", "member", "layout", "operation", "rowPath", "scan", "reduce", "nearest"} {
		t.Run(variant, func(t *testing.T) {
			e := documentEngine(t, false, nil)
			s := goalwork.SampleRequest{PK: "3033304", Delivery: "document", Document: &dataset.DocumentSelection{ReferenceID: "mois-monthly-help", Contains: "Definition"}}
			switch variant {
			case "wrong delivery":
				s.Delivery = "file"
			case "no selection":
				s.Document = nil
			case "asset":
				s.Asset = "invented"
			case "params":
				s.Params = map[string]string{"page": "1"}
			case "where":
				s.Where = map[string]string{"text": "Definition"}
			case "whereIn":
				s.WhereIn = map[string][]string{"text": {"Definition"}}
			case "fileVersion":
				s.FileVersion = "other"
			case "xlsx":
				s.XLSX = &dataset.XLSXSelection{Sheet: "sheet", Range: "A1:A1"}
			case "member":
				s.Member = "file.csv"
			case "layout":
				s.LayoutID = "layout"
			case "operation":
				s.Operation = "read"
			case "rowPath":
				s.RowPath = "/rows"
			case "scan":
				s.ScanCSV = true
			case "reduce":
				s.Reduce = &goalwork.SourceReduction{Observation: "o2", RowsSHA256: e.View().Observations[1].RowsSHA256, Aggregates: []goalwork.Aggregate{{As: "n", Op: "count"}}}
			case "nearest":
				s.Nearest = &goalwork.NearestSelection{}
			}
			v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &s})
			if err != nil || len(v.Gaps) != 1 || len(v.Observations) != 2 || len(v.SampleAttempts) != 2 {
				t.Fatalf("mixed document selector crossed acquisition boundary: %+v %v", v.Gaps, err)
			}
		})
	}
}

func TestDocumentRevisionAndReturnedMetadataCannotRewriteEvidence(t *testing.T) {
	e := documentEngine(t, true, nil)
	v := e.View()
	o := v.Observations[1]
	o.Document.Reference.URL = "https://example.com/forged"
	o.Document.Blocks[1].Ordinal = 99
	o.Document.Blocks[1].Locator = "/forged"
	r := goalwork.EvidenceRequest{Observation: o.ID, RowsSHA256: strings.Repeat("0", 64), Rows: []int{2}, Fields: []string{"text"}}
	v, err := e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &r})
	if err != nil || len(v.Gaps) != 1 || len(v.Evidence) != 0 {
		t.Fatal("wrong document revision disclosed evidence")
	}
	r.RowsSHA256 = o.RowsSHA256
	v, err = e.Advance(context.Background(), v.Revision, goalwork.Decision{Action: "read_evidence", Evidence: &r})
	if err != nil || len(v.Gaps) != 1 || len(v.Evidence) != 1 || v.Observations[1].Document.Reference.URL != "https://jumin.mois.go.kr/ageStatMonth.do" || v.Evidence[0].Records[0].Origins["text"].Ordinal != 2 || v.Evidence[0].Records[0].Origins["text"].Locator != "/html[1]/body[1]/form[1]/div[2]/div[1]" {
		t.Fatal("caller rewrote retained document provenance")
	}
}

func TestDocumentSelectionsShareOriginalAcquisitionBudget(t *testing.T) {
	e := documentEngine(t, false, nil)
	for _, literal := range []string{"Definition", "efinition", "finition", "inition", "nition", "ition"} {
		advanceUnmatched(t, e, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "3033304", Delivery: "document", Document: &dataset.DocumentSelection{ReferenceID: "mois-monthly-help", Contains: literal}}})
	}
	v, err := e.Advance(context.Background(), e.View().Revision, goalwork.Decision{Action: "sample", Sample: &goalwork.SampleRequest{PK: "3033304", Delivery: "document", Document: &dataset.DocumentSelection{ReferenceID: "mois-monthly-help", Contains: "tion"}}})
	if err != nil || len(v.Gaps) != 1 || v.Gaps[0].Detail != "sample budget exhausted" || len(v.SampleAttempts) != 8 || len(v.Observations) != 8 || v.Budget.SamplesRemaining != 0 {
		t.Fatal("document created a second acquisition budget")
	}
}
