package portal

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Dataset is one data.go.kr dataset from a search result. Beyond the identity
// fields, the list carries per-dataset metadata (publisher, last update, and how
// much the dataset is actually looked at and applied for) which is what makes it
// possible to judge a dataset without opening its page.
type Dataset struct {
	PublicDataPk string   `json:"publicDataPk"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Formats      []string `json:"formats,omitempty"`
	HasOpenAPI   bool     `json:"hasOpenApi"`           // true when the result links to /data/{pk}/openapi.do
	Org          string   `json:"org,omitempty"`        // 제공기관
	ModifiedAt   string   `json:"modifiedAt,omitempty"` // 수정일
	ViewCount    int      `json:"viewCount,omitempty"`  // 조회수
	ApplyCount   int      `json:"applyCount,omitempty"` // 활용신청 건수
	Category     string   `json:"category,omitempty"`   // 분류체계
	OrgType      string   `json:"orgType,omitempty"`    // 기관유형 (공공기관/지자체 등)
}

// SearchOptions filters a dataset search. Blank Org = all publishers; blank
// Type = all dataset types.
type SearchOptions struct {
	Keyword string
	Org     string
	Type    string // "FILE" | "API" | "" (all)
	// SvcType narrows OpenAPI datasets by service type as the portal's own filter
	// does: "REST" (spec published on the portal) or "LINK" (only a pointer to the
	// publisher's site). Empty means every type.
	SvcType string
	Page    int
	// PerPage is how many results one page returns (default 10, the portal's own
	// default). Raise it to sweep a large result set in fewer requests.
	PerPage int
}

var (
	rePkFile = regexp.MustCompile(`/data/(\d+)/fileData\.do`)
	rePkAPI  = regexp.MustCompile(`/data/(\d+)/openapi\.do`)
	reDigits = regexp.MustCompile(`\d[\d,]*`)
)

// SearchDatasets scrapes /tcs/dss/selectDataSetList.do. A blank keyword lists
// the first page.
func (c *Client) SearchDatasets(ctx context.Context, opts SearchOptions) ([]Dataset, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	q := url.Values{}
	if opts.Type != "" {
		q.Set("dType", opts.Type)
	}
	if opts.SvcType != "" {
		q.Set("svcType", opts.SvcType)
	}
	if opts.Org != "" {
		q.Set("org", opts.Org)
	}
	q.Set("keyword", opts.Keyword)
	q.Set("currentPage", strconv.Itoa(page))
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 10
	}
	q.Set("perPage", strconv.Itoa(perPage))

	doc, err := c.getDoc(ctx, "/tcs/dss/selectDataSetList.do", q)
	if err != nil {
		return nil, err
	}

	var out []Dataset
	seen := map[string]bool{}
	// One .apply-result-item per dataset, as of the 2026-08 portal redesign
	// (KRDS). A keyword search without dType groups the items under 파일데이터 /
	// 오픈API / 연계데이터 headings; the items themselves are identical either way,
	// so a flat sweep reads both layouts.
	doc.Find(".apply-result-item").Each(func(_ int, item *goquery.Selection) {
		link := item.Find(`.apply-result-link a[href*="/data/"]`).First()
		href, _ := link.Attr("href")
		var pk string
		hasAPI := false
		if m := rePkAPI.FindStringSubmatch(href); m != nil {
			pk, hasAPI = m[1], true
		} else if m := rePkFile.FindStringSubmatch(href); m != nil {
			pk = m[1]
		}
		if pk == "" || seen[pk] {
			return
		}
		seen[pk] = true
		d := Dataset{
			PublicDataPk: pk,
			Title:        cleanText(link.Text()),
			Description:  cleanText(item.Find(".apply-result-summary").First().Text()),
			HasOpenAPI:   hasAPI,
		}
		// Formats are their own badges now (data-ext="CSV"), no longer a prefix
		// glued onto the title. FILE results with an automatically converted API
		// also carry a separate "JSON + XML" badge without data-ext.
		item.Find(".apply-result-link .krds-badge").Each(func(_ int, b *goquery.Selection) {
			if ext, ok := b.Attr("data-ext"); ok {
				d.Formats = appendFormat(d.Formats, ext)
				return
			}
			badge := strings.ToUpper(cleanText(b.Text()))
			for _, format := range []string{"JSON", "XML"} {
				if strings.Contains(badge, format) {
					d.Formats = appendFormat(d.Formats, format)
				}
			}
			if strings.Contains(badge, "JSON") && strings.Contains(badge, "XML") {
				d.HasOpenAPI = true
			}
		})
		badges := item.Find(".apply-result-category .krds-badge")
		d.Category = cleanText(badges.Eq(0).Text())
		d.OrgType = cleanText(badges.Eq(1).Text())
		// Metadata is a label/value list: <li><strong>제공기관</strong>기상청</li>
		item.Find(".in-result-item ul li").Each(func(_ int, li *goquery.Selection) {
			label := cleanText(li.Find("strong").First().Text())
			value := strings.TrimSpace(strings.TrimPrefix(cleanText(li.Text()), label))
			switch label {
			case "제공기관":
				d.Org = value
			case "수정일":
				d.ModifiedAt = value
			case "조회수":
				d.ViewCount = atoiLoose(value)
			case "활용신청":
				d.ApplyCount = atoiLoose(value)
			}
		})
		out = append(out, d)
	})
	return out, nil
}

func appendFormat(formats []string, value string) []string {
	value = strings.ToUpper(cleanText(value))
	if value == "" {
		return formats
	}
	for _, existing := range formats {
		if existing == value {
			return formats
		}
	}
	return append(formats, value)
}

// atoiLoose parses a count out of the portal's rendering of it — "132,828회",
// "5,491건" — and returns 0 when there is no number to read (never a fabricated
// one).
func atoiLoose(s string) int {
	m := reDigits.FindString(s)
	if m == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
	if err != nil {
		return 0
	}
	return n
}

// cleanText collapses runs of whitespace to single spaces.
func cleanText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
