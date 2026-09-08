package dataset_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

const monthlyDocumentURL = "https://jumin.mois.go.kr/ageStatMonth.do"
const answerDocumentURL = "https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124"

type documentTransport func(*http.Request) (*http.Response, error)

func (f documentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func documentResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html; charset=UTF-8"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func documentInspector(t *testing.T, advertised string, read documentTransport) *dataset.Inspector {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	for _, raw := range []string{monthlyDocumentURL, answerDocumentURL} {
		u, _ := url.Parse(raw)
		jar.SetCookies(u, []*http.Cookie{{Name: "private_session", Value: "not-document-input"}})
	}
	client := fetch.New(fetch.WithDelay(0), fetch.WithHTTPClient(&http.Client{Jar: jar, Transport: documentTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case "https://www.data.go.kr/catalog/12345678/fileData.json":
			return documentResponse(`{}`), nil
		case "https://www.data.go.kr/data/12345678/fileData.do":
			return documentResponse(`<ul class="info-ul"><li><strong class="key">URL</strong><div class="value"><a href="` + advertised + `">official</a></div></li></ul>`), nil
		default:
			if read != nil {
				return read(r)
			}
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
	})}))
	return dataset.NewInspector(client, "")
}

// Reduced HTML fixtures preserve the observed selectors, not a population answer.
const monthlyHTML = `<!doctype html><html><body><aside>unselected menu</aside><form name="search" action="ageStatMonth.do">
<div class="popoverBox"><div class="pContent"><p>지역 범위</p></div></div>
<div class="popoverBox"><div class="pContent"><table><tr><th>전체</th><td>주민 <strong>등록</strong> &amp; 범위</td></tr></table><script>not source text</script></div></div>
<div class="popoverBox"><div class="pContent">기준 <em>시점</em><br>매월 말일</div></div>
</form></body></html>`

func TestReadSupportingDocumentPreservesSelectedOriginalLocation(t *testing.T) {
	i := documentInspector(t, monthlyDocumentURL, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != monthlyDocumentURL || r.Method != "GET" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("document request leaked credentials or changed exact contract")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 15*time.Second {
			t.Fatal("missing document deadline")
		}
		return documentResponse(monthlyHTML), nil
	})
	c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := i.SampleDocument(context.Background(), c, dataset.DocumentSelection{ReferenceID: "mois-monthly-help", Contains: "전체"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 1 || len(s.Rows[0]) != 1 || s.Rows[0]["text"] != "전체 주민 등록 & 범위" {
		t.Fatalf("not literal selected section: %+v", s.Rows)
	}
	p := s.Document
	if p == nil || p.Reference.URL != monthlyDocumentURL || p.Sections != 3 || p.MatchedSections != 1 || len(p.Blocks) != 1 || p.Blocks[0].Ordinal != 2 || p.Blocks[0].Locator != "/html[1]/body[1]/form[1]/div[2]/div[1]" {
		t.Fatalf("lost original section address: %+v", p)
	}
	if p.SourceSHA256 != s.SHA256 || len(p.SourceSHA256) != 64 || p.Bytes != int64(len(monthlyHTML)) || p.Blocks[0].TextSHA256 == "" || p.ExtractorRevision == "" || s.Prefix {
		t.Fatal("unbound document revision")
	}
}

const answerHTML = `<html><body><p>questioner text excluded</p><div class="answers"><div class="tbx">Official reply 2024-06-28</div><div class="an_txt">Source &lsquo;definition&rsquo;<br>applies with limits</div></div><input id="boardIdx" name="boardIdx" value="22124"></body></html>`

func TestReadRegisteredOfficialAnswerDistinguishesItFromPortalLink(t *testing.T) {
	i := documentInspector(t, monthlyDocumentURL, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != answerDocumentURL || r.Header.Get("Cookie") != "" {
			t.Fatal("wrong official answer request")
		}
		return documentResponse(answerHTML), nil
	})
	c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := i.SampleDocument(context.Background(), c, dataset.DocumentSelection{ReferenceID: "kosis-answer-22124"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 2 || s.Rows[1]["text"] != "Source ‘definition’ applies with limits" || s.Document.Reference.Basis != "adapter_reference" || s.Document.Reference.DiscoveryURL != "https://www.data.go.kr/data/12345678/fileData.do" || s.Document.Reference.AdvertisedURL != monthlyDocumentURL {
		t.Fatalf("answer lost scope/basis: %+v", s)
	}
}

func TestSupportingDocumentRejectsDriftAndBoundsWithoutPartialObservations(t *testing.T) {
	for _, scenario := range []string{"redirect", "status", "media", "charset", "encoding", "oversize", "cell", "identity", "sections", "empty section", "empty selection", "long selector", "unknown reference", "reconstructed contract", "answer identity"} {
		t.Run(scenario, func(t *testing.T) {
			i := documentInspector(t, monthlyDocumentURL, func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != monthlyDocumentURL && r.URL.String() != answerDocumentURL {
					t.Fatal("followed unregistered destination")
				}
				body := monthlyHTML
				switch scenario {
				case "encoding":
					body = string([]byte{0xff})
				case "oversize":
					body = strings.Repeat("a", (1<<20)+1)
				case "cell":
					body = strings.Replace(body, "지역 범위", strings.Repeat("<", 400), 1)
				case "identity":
					body = strings.Replace(body, `action="ageStatMonth.do"`, `action="other.do"`, 1)
				case "sections":
					body = strings.Replace(body, `class="pContent"`, `class="changed"`, 1)
				case "empty section":
					body = strings.Replace(body, "지역 범위", " ", 1)
				case "answer identity":
					body = strings.Replace(answerHTML, `value="22124"`, `value="other"`, 1)
				}
				res := documentResponse(body)
				switch scenario {
				case "redirect":
					res.StatusCode = 302
					res.Header.Set("Location", "https://example.com/forbidden")
				case "status":
					res.StatusCode = 503
				case "media":
					res.Header.Set("Content-Type", "application/json")
				case "charset":
					res.Header.Set("Content-Type", "text/html; charset=euc-kr")
				}
				return res, nil
			})
			c, err := i.Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
			if err != nil {
				t.Fatal(err)
			}
			s := dataset.DocumentSelection{ReferenceID: "mois-monthly-help"}
			switch scenario {
			case "empty selection":
				s.Contains = "absent literal"
			case "long selector":
				s.Contains = strings.Repeat("가", 43)
			case "unknown reference":
				s.ReferenceID = "https://example.com/forbidden"
			case "reconstructed contract":
				b, _ := json.Marshal(c)
				c = &dataset.Contract{}
				if err := json.Unmarshal(b, c); err != nil {
					t.Fatal(err)
				}
			case "answer identity":
				s.ReferenceID = "kosis-answer-22124"
			}
			got, err := i.SampleDocument(context.Background(), c, s)
			if err == nil || len(got.Rows) != 0 || got.Document != nil {
				t.Fatalf("invalid document returned partial success: %+v %v", got, err)
			}
		})
	}
}

func TestSupportingDocumentLiveContracts(t *testing.T) {
	if os.Getenv("ODEDUCK_DOCUMENT_LIVE") != "1" {
		t.Skip("opt-in official public document contract check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	i := dataset.NewInspector(fetch.New(), "")
	c, err := i.Inspect(ctx, dataset.Ref{PK: "3033304", Delivery: "FILE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Documents) != 2 {
		t.Fatalf("live document discovery drift: %+v", c.Documents)
	}
	for _, ref := range c.Documents {
		s, err := i.SampleDocument(ctx, c, dataset.DocumentSelection{ReferenceID: ref.ID})
		if err != nil {
			t.Fatalf("%s: %v", ref.ID, err)
		}
		t.Logf("%s source=%s bytes=%d extractor=%s sections=%d retained=%d", ref.ID, s.SHA256, s.Bytes, s.Document.ExtractorRevision, s.Document.Sections, len(s.Rows))
		for n, b := range s.Document.Blocks {
			cell, _ := json.Marshal(s.Rows[n]["text"])
			t.Logf("block=%d locator=%s text=%s jsonBytes=%d", b.Ordinal, b.Locator, b.TextSHA256, len(cell))
		}
	}
}

func TestInspectionDiscoversOnlyRegisteredSupportingDocuments(t *testing.T) {
	for _, source := range []string{monthlyDocumentURL, monthlyDocumentURL + "?x=1", monthlyDocumentURL + "#x", "https://jumin.mois.go.kr.evil/ageStatMonth.do", "https://example.com/ageStatMonth.do"} {
		t.Run(source, func(t *testing.T) {
			contract, err := documentInspector(t, source, nil).Inspect(context.Background(), dataset.Ref{PK: "12345678", Delivery: "FILE"})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(contract)
			var view struct {
				Documents []struct{ ID, URL, Basis string }
			}
			if err := json.Unmarshal(body, &view); err != nil {
				t.Fatal(err)
			}
			if source != monthlyDocumentURL {
				if len(view.Documents) != 0 {
					t.Fatal("unregistered URL gained document references")
				}
				return
			}
			if len(view.Documents) != 2 || view.Documents[0].URL != monthlyDocumentURL || view.Documents[0].Basis != "portal_link" || view.Documents[1].URL != answerDocumentURL || view.Documents[1].Basis != "adapter_reference" {
				t.Fatalf("missing or misattributed document references: %s", body)
			}
			if view.Documents[0].ID == "" || len(contract.Assets) != 0 || contract.Capability != dataset.CapabilityInspectable {
				t.Fatal("documentation was promoted to a data download")
			}
		})
	}
}
