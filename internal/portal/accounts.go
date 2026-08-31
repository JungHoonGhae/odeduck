package portal

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Application is one OpenAPI 활용신청 (dev-account) the user holds, as listed on
// the 활용신청 현황 page. ExpiresAt (만료예정일) is the field the user most cares
// about — gongctl surfaces it so renewals can be tracked.
type Application struct {
	Title     string `json:"title"`     // 데이터명 (상태 접두사 제거)
	Status    string `json:"status"`    // 승인 / 신청 등 ([..] 접두사에서)
	Org       string `json:"org"`       // 제공기관
	Category  string `json:"category"`  // 분류
	Account   string `json:"account"`   // 계정 (개발/운영)
	AppliedAt string `json:"appliedAt"` // 신청일
	ExpiresAt string `json:"expiresAt"` // 만료예정일
	UDDI      string `json:"uddi"`      // 상세조회 uddi
	DetailPk  string `json:"detailPk"`  // publicDataDetailPk
}

// applicationIdentity is stable across pages and portal layouts. DetailPk is
// the strongest identifier; older rows may only expose UDDI, and the final
// composite keeps legacy fixtures and partially migrated rows deduplicated.
func applicationIdentity(a Application) string {
	if a.DetailPk != "" {
		return "pk:" + a.DetailPk
	}
	if a.UDDI != "" {
		return "uddi:" + a.UDDI
	}
	return strings.Join([]string{a.Title, a.Org, a.Account, a.AppliedAt}, "\x00")
}

func appendUniqueApplications(apps, batch []Application, seen map[string]bool) ([]Application, int) {
	fresh := 0
	for _, app := range batch {
		key := applicationIdentity(app)
		if seen[key] {
			continue
		}
		seen[key] = true
		apps = append(apps, app)
		fresh++
	}
	return apps, fresh
}

// collectApplicationPages applies the portal's count-based pagination and
// duplicate/no-progress guard independently of transport. Both saved-cookie
// HTTP and the live-browser fallback must use it; otherwise keep-browser users
// silently see only the first ten applications.
func collectApplicationPages(firstHTML string, next func(page int) (string, error)) ([]Application, error) {
	apps, err := parseApplications(firstHTML)
	if err != nil {
		return nil, err
	}
	total := parseTotalCount(firstHTML)
	seen := make(map[string]bool, len(apps))
	for _, app := range apps {
		seen[applicationIdentity(app)] = true
	}
	maxPages := (total + accountListPageSize - 1) / accountListPageSize
	for page := 2; page <= maxPages && len(apps) < total; page++ {
		html, err := next(page)
		if err != nil {
			return nil, err
		}
		batch, err := parseApplications(html)
		if err != nil {
			return nil, err
		}
		updated, fresh := appendUniqueApplications(apps, batch, seen)
		apps = updated
		if fresh == 0 {
			break
		}
	}
	return apps, nil
}

var reFnDetail = regexp.MustCompile(`fn_detail\('([^']*)','([^']*)'`)
var reStatusPrefix = regexp.MustCompile(`^\[([^\]]+)\]\s*`)

// AccountListPath is the 활용신청 현황 page driven by the browser session.
const AccountListPath = "/iim/api/selectAcountList.do"

// accountListPageSize is how many applications the portal puts on one page. The
// list is paginated with ?pageIndex=N, so reading only the first page silently
// drops everything past the tenth application — see Applications.
const accountListPageSize = 10

var reTotalCount = regexp.MustCompile(`(?:총|전체)\s*(?:<[^>]*>\s*)*([\d,]+)`)

// parseTotalCount reads the "총 N건" header of the 활용신청 현황 list. Returns 0
// when the page does not carry it (then the caller falls back to one page).
func parseTotalCount(body string) int {
	m := reTotalCount.FindStringSubmatch(body)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0
	}
	return n
}

// parseApplications extracts the 활용신청 현황 list items from the page HTML.
// Split out as a pure function so it can be tested against a fixture without a
// live session.
func parseApplications(body string) ([]Application, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	var apps []Application
	// KRDS layout (2026-08 redesign). Keep this separate from the legacy branch:
	// mixing selectors would let a half-migrated item combine unrelated fields.
	items := doc.Find(".apply-result-item")
	if items.Length() > 0 {
		items.Each(func(_ int, item *goquery.Selection) {
			badges := item.Find(".apply-result-category .krds-badge")
			a := Application{
				Category: cleanText(badges.Eq(0).Text()),
				Org:      cleanText(badges.Eq(1).Text()),
			}

			link := item.Find(".apply-result-link a").First()
			setApplicationTitleAndDetail(&a, cleanText(link.Text()), link)

			metadata := item.ChildrenFiltered("ul").ChildrenFiltered("li")
			if metadata.Length() == 0 {
				metadata = item.Find(".in-result-item > ul > li")
			}
			metadata.Each(func(_ int, li *goquery.Selection) {
				label := cleanText(li.Find("strong").First().Text())
				value := strings.TrimSpace(strings.TrimPrefix(cleanText(li.Text()), label))
				switch label {
				case "계정":
					a.Account = value
				case "신청일":
					a.AppliedAt = value
				case "만료예정일":
					a.ExpiresAt = value
				}
			})

			if a.Title != "" {
				apps = append(apps, a)
			}
		})
		return apps, nil
	}

	// Legacy layout, retained for fixtures and during a staggered portal rollout.
	doc.Find(".mypage-dataset-list > ul > li").Each(func(_ int, li *goquery.Selection) {
		a := Application{
			Category: strings.TrimSpace(li.Find(".tag-area .labelset.brown").First().Text()),
			Org:      strings.TrimSpace(li.Find(".tag-area .labelset.red").First().Text()),
		}

		link := li.Find(".title-area a").First()
		setApplicationTitleAndDetail(&a, li.Find(".title-area .title").First().Text(), link)

		// info-data: 계정 / 신청일 / 만료예정일 — label(.tit) → value(.data) 쌍.
		li.Find(".info-data p").Each(func(_ int, p *goquery.Selection) {
			label := strings.TrimSpace(p.Find(".tit").Text())
			value := strings.TrimSpace(p.Find(".data").Text())
			switch label {
			case "계정":
				a.Account = value
			case "신청일":
				a.AppliedAt = value
			case "만료예정일":
				a.ExpiresAt = value
			}
		})

		if a.Title != "" {
			apps = append(apps, a)
		}
	})
	return apps, nil
}

func setApplicationTitleAndDetail(a *Application, title string, link *goquery.Selection) {
	title = strings.Join(strings.Fields(title), " ")
	if m := reStatusPrefix.FindStringSubmatch(title); m != nil {
		a.Status = m[1]
		title = reStatusPrefix.ReplaceAllString(title, "")
	}
	a.Title = title

	if href, ok := link.Attr("href"); ok {
		if m := reFnDetail.FindStringSubmatch(href); m != nil {
			a.UDDI = m[1]
			a.DetailPk = m[2]
		}
	}
}
