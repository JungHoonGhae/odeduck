package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// DocumentReference is an inspected, registered supporting source, not a data
// download or a declaration that its contents apply to any particular record.
type DocumentReference struct {
	ID               string `json:"id"`
	URL              string `json:"url"`
	Title            string `json:"title"`
	Basis            string `json:"basis"`
	AdvertisedURL    string `json:"advertisedUrl"`
	DiscoveryURL     string `json:"discoveryUrl"`
	DiscoveryLocator string `json:"discoveryLocator"`
	AdapterID        string `json:"adapterId"`
	AdapterRevision  int    `json:"adapterRevision"`
	VerifiedAt       string `json:"verifiedAt"`
}

const monthlyDocumentURL = "https://jumin.mois.go.kr/ageStatMonth.do"
const answerDocumentURL = "https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124"

// Supporting-document producers and consumers share this bounded extraction
// contract; consumers still validate provenance and selected text independently.
const (
	DocumentExtractorRevision = "html-block-space-v1"
	DocumentMaxBytes          = 1 << 20
	DocumentMaxSections       = 20
	DocumentMaxTextJSONBytes  = 2048
)

func supportingDocuments(source, discovery string) []DocumentReference {
	if source != monthlyDocumentURL {
		return nil
	}
	return []DocumentReference{
		{ID: "mois-monthly-help", URL: monthlyDocumentURL, Title: "행정동별 연령별 인구현황 도움말", Basis: "portal_link", AdvertisedURL: source, DiscoveryURL: discovery, DiscoveryLocator: "URL", AdapterID: "mois-monthly-documents", AdapterRevision: 1, VerifiedAt: "2026-09-08"},
		{ID: "kosis-answer-22124", URL: answerDocumentURL, Title: "KOSIS 공식 답변 22124", Basis: "adapter_reference", AdvertisedURL: source, DiscoveryURL: discovery, DiscoveryLocator: "adapter:mois-monthly-documents@1", AdapterID: "mois-monthly-documents", AdapterRevision: 1, VerifiedAt: "2026-09-08"},
	}
}

type DocumentSelection struct {
	ReferenceID string `json:"referenceId"`
	Contains    string `json:"contains,omitempty"`
}

type DocumentBlock struct {
	Ordinal    int    `json:"ordinal"`
	Locator    string `json:"locator"`
	TextSHA256 string `json:"textSha256"`
}

// DocumentProvenance describes selected registered sections, never all records
// or the population to which a document's wording may apply. It contains no text.
type DocumentProvenance struct {
	Reference         DocumentReference `json:"reference"`
	SourceSHA256      string            `json:"sourceSha256"`
	Bytes             int64             `json:"bytes"`
	ExtractorRevision string            `json:"extractorRevision"`
	Sections          int               `json:"sections"`
	MatchedSections   int               `json:"matchedSections"`
	Blocks            []DocumentBlock   `json:"blocks"`
}

func ValidateDocumentSelection(s DocumentSelection) error {
	if strings.TrimSpace(s.ReferenceID) == "" || len(s.ReferenceID) > 128 || !utf8.ValidString(s.ReferenceID) || len(s.Contains) > 128 || !utf8.ValidString(s.Contains) {
		return fmt.Errorf("document selection needs an inspected reference ID and a literal contains of at most 128 UTF-8 bytes")
	}
	return nil
}

// SampleDocument reads only a reference issued by Inspect. Reconstructed public
// contracts cannot authorize a URL; the inspection's private references pin it.
func (i *Inspector) SampleDocument(ctx context.Context, c *Contract, s DocumentSelection) (TableSample, error) {
	if err := ValidateDocumentSelection(s); err != nil {
		return TableSample{}, err
	}
	var ref DocumentReference
	if c != nil {
		for _, r := range c.documentReferences {
			if r.ID == s.ReferenceID {
				ref = r
				break
			}
		}
	}
	if ref.ID == "" {
		return TableSample{}, fmt.Errorf("document reference was not issued by inspection")
	}
	transport, ok := i.http.(interface {
		OpenPublicGETNoRedirect(context.Context, string) (*fetch.StreamResponse, error)
	})
	if !ok {
		return TableSample{}, fmt.Errorf("document acquisition requires credentialless no-redirect transport")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, err := transport.OpenPublicGETNoRedirect(ctx, ref.URL)
	if err != nil {
		return TableSample{}, err
	}
	defer res.Body.Close()
	if res.Status != http.StatusOK {
		return TableSample{}, fmt.Errorf("document returned HTTP %d; redirects are not followed", res.Status)
	}
	media, params, err := mime.ParseMediaType(res.ContentType)
	if err != nil || media != "text/html" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		return TableSample{}, fmt.Errorf("document requires UTF-8 text/html")
	}
	if res.ContentLength > DocumentMaxBytes {
		return TableSample{}, fmt.Errorf("document exceeds 1 MiB")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, DocumentMaxBytes+1))
	if err != nil {
		return TableSample{}, err
	}
	if len(body) > DocumentMaxBytes {
		return TableSample{}, fmt.Errorf("document exceeds 1 MiB")
	}
	if !utf8.Valid(body) {
		return TableSample{}, fmt.Errorf("document is not UTF-8")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return TableSample{}, fmt.Errorf("document HTML cannot be parsed")
	}
	var sections *goquery.Selection
	expected := 0
	switch ref.URL {
	case monthlyDocumentURL:
		if doc.Find(`form[name="search"][action="ageStatMonth.do"]`).Length() != 1 {
			return TableSample{}, fmt.Errorf("document identity drift")
		}
		sections = doc.Find(`form[name="search"] .popoverBox .pContent`)
		expected = 3
	case answerDocumentURL:
		identity := doc.Find(`input#boardIdx[name="boardIdx"]`)
		value, _ := identity.Attr("value")
		if identity.Length() != 1 || value != "22124" {
			return TableSample{}, fmt.Errorf("document identity drift")
		}
		for _, selector := range []string{`.answers > .tbx`, `.answers > .an_txt`} {
			if count := doc.Find(selector).Length(); count != 1 {
				return TableSample{}, fmt.Errorf("document section count drift: %s expected 1, observed %d", selector, count)
			}
		}
		sections = doc.Find(`.answers > .tbx, .answers > .an_txt`)
		expected = 2
	default:
		return TableSample{}, fmt.Errorf("unregistered document contract")
	}
	if sections.Length() != expected {
		return TableSample{}, fmt.Errorf("document section count drift: expected %d, observed %d", expected, sections.Length())
	}
	sha := fmt.Sprintf("%x", sha256.Sum256(body))
	p := &DocumentProvenance{Reference: ref, SourceSHA256: sha, Bytes: int64(len(body)), ExtractorRevision: DocumentExtractorRevision, Sections: expected}
	result := TableSample{SHA256: sha, Bytes: int64(len(body)), Document: p}
	for n, node := range sections.Nodes {
		if err := ctx.Err(); err != nil {
			return TableSample{}, err
		}
		text := documentText(node)
		if text == "" {
			return TableSample{}, fmt.Errorf("document contains an empty registered section")
		}
		if !strings.Contains(text, s.Contains) {
			continue
		}
		cell, _ := json.Marshal(text)
		if len(cell) > DocumentMaxTextJSONBytes {
			return TableSample{}, fmt.Errorf("document section exceeds %d JSON bytes; selection is not truncated", DocumentMaxTextJSONBytes)
		}
		p.MatchedSections++
		if p.MatchedSections > DocumentMaxSections {
			return TableSample{}, fmt.Errorf("document selection exceeds %d sections", DocumentMaxSections)
		}
		p.Blocks = append(p.Blocks, DocumentBlock{Ordinal: n + 1, Locator: documentPath(node), TextSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(text)))})
		result.Rows = append(result.Rows, map[string]any{"text": text})
	}
	if len(result.Rows) == 0 {
		return TableSample{}, fmt.Errorf("document selection matched no registered section")
	}
	return result, nil
}

// Block and table-cell boundaries become spaces, while inline text is kept in
// order. Whitespace is collapsed only after entity-decoded HTML text extraction.
func documentText(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "script", "style", "template", "noscript", "iframe":
			return
		}
		block := false
		switch n.Data {
		case "br", "p", "div", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "ul", "ol", "li", "dl", "dt", "dd", "h1", "h2", "h3", "h4", "h5", "h6", "section", "article", "blockquote", "pre", "hr":
			block = true
		}
		if block {
			b.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if block {
			b.WriteByte(' ')
		}
	}
	walk(node)
	return strings.Join(strings.Fields(b.String()), " ")
}

func documentPath(n *html.Node) string {
	var parts []string
	for ; n != nil && n.Type != html.DocumentNode; n = n.Parent {
		ordinal := 1
		for prev := n.PrevSibling; prev != nil; prev = prev.PrevSibling {
			if prev.Type == n.Type && prev.Data == n.Data {
				ordinal++
			}
		}
		parts = append(parts, fmt.Sprintf("%s[%d]", n.Data, ordinal))
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return "/" + strings.Join(parts, "/")
}
