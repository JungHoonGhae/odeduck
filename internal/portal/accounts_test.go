package portal

import (
	"fmt"
	"os"
	"testing"
)

func TestParseApplications(t *testing.T) {
	body, err := os.ReadFile("testdata/accounts.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	apps, err := parseApplications(string(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}

	a := apps[0]
	if a.Status != "승인" {
		t.Errorf("status = %q, want 승인", a.Status)
	}
	if a.Title != "중앙선거관리위원회_당선인 정보" {
		t.Errorf("title = %q (상태접두사·공백 정리 실패)", a.Title)
	}
	if a.Org != "중앙선거관리위원회" {
		t.Errorf("org = %q", a.Org)
	}
	if a.Category != "공공행정" {
		t.Errorf("category = %q", a.Category)
	}
	if a.Account != "개발" {
		t.Errorf("account = %q", a.Account)
	}
	if a.AppliedAt != "2026-06-22" {
		t.Errorf("appliedAt = %q", a.AppliedAt)
	}
	if a.ExpiresAt != "2028-06-22" {
		t.Errorf("expiresAt = %q, want 2028-06-22", a.ExpiresAt)
	}
	if a.DetailPk != "119222395" {
		t.Errorf("detailPk = %q", a.DetailPk)
	}
	if a.UDDI != "uddi:aaaa-1111" {
		t.Errorf("uddi = %q", a.UDDI)
	}

	if apps[1].Title != "중앙선거관리위원회_투·개표 정보" {
		t.Errorf("apps[1].Title = %q", apps[1].Title)
	}
}

func TestParseApplicationsKRDS(t *testing.T) {
	body := `<html><body>
<p class="total">전체 <span>26</span> 건</p>
<div class="apply-result-item">
  <div class="apply-result-category">
    <span class="krds-badge">과학기술</span>
    <span class="krds-badge">기상청</span>
    <span class="krds-badge">국가중점</span>
  </div>
  <div class="apply-result-link">
    <span class="krds-badge">활용신청</span>
    <a href="javascript:fn_detail('uddi:weather-1','123456789','0','PR0027')">[승인] 기상청_지상 일자료 조회서비스<span class="add-state"></span></a>
  </div>
  <ul>
    <li><strong>계정</strong> 개발</li>
    <li><strong>신청일</strong> 2026-08-01</li>
    <li><strong>만료예정일</strong> 2028-08-01</li>
  </ul>
</div>
</body></html>`

	apps, err := parseApplications(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps, want 1", len(apps))
	}
	want := Application{
		Title:     "기상청_지상 일자료 조회서비스",
		Status:    "승인",
		Org:       "기상청",
		Category:  "과학기술",
		Account:   "개발",
		AppliedAt: "2026-08-01",
		ExpiresAt: "2028-08-01",
		UDDI:      "uddi:weather-1",
		DetailPk:  "123456789",
	}
	if apps[0] != want {
		t.Errorf("application = %+v, want %+v", apps[0], want)
	}
	if got := parseTotalCount(body); got != 26 {
		t.Errorf("parseTotalCount(KRDS) = %d, want 26", got)
	}
}

// The list is paginated at 10; the "총 N건" header is what tells Applications
// there are more pages. If this stops parsing, applications past the tenth
// vanish silently and an agent's "did I already apply?" check goes wrong.
func TestParseTotalCount(t *testing.T) {
	if got := parseTotalCount(`<p>총 <span class="num">16</span>건</p>`); got != 16 {
		t.Errorf("parseTotalCount = %d, want 16", got)
	}
	if got := parseTotalCount(`<p>총 <b>1,234</b>건</p>`); got != 1234 {
		t.Errorf("parseTotalCount with comma = %d, want 1234", got)
	}
	if got := parseTotalCount(`<p class="total">전체 <span>1,234</span> 건</p>`); got != 1234 {
		t.Errorf("parseTotalCount KRDS = %d, want 1234", got)
	}
	// No header → 0, so the caller reads a single page instead of looping.
	if got := parseTotalCount(`<p>목록이 없습니다</p>`); got != 0 {
		t.Errorf("parseTotalCount without header = %d, want 0", got)
	}
}

func TestApplicationIdentityPrefersPortalIdentifiers(t *testing.T) {
	base := Application{Title: "same", Org: "org", Account: "개발", AppliedAt: "2026-08-01"}
	if got := applicationIdentity(Application{DetailPk: "123", UDDI: "uddi:a"}); got != "pk:123" {
		t.Fatalf("detail identity = %q", got)
	}
	if got := applicationIdentity(Application{UDDI: "uddi:a"}); got != "uddi:uddi:a" {
		t.Fatalf("UDDI identity = %q", got)
	}
	if applicationIdentity(base) != applicationIdentity(base) {
		t.Fatal("fallback identity must be stable")
	}
	if applicationIdentity(base) == applicationIdentity(Application{Title: "other", Org: base.Org, Account: base.Account, AppliedAt: base.AppliedAt}) {
		t.Fatal("fallback identity must distinguish different applications")
	}
}

func TestAppendUniqueApplicationsStopsRepeatedPage(t *testing.T) {
	first := Application{DetailPk: "1", Title: "first"}
	second := Application{DetailPk: "2", Title: "second"}
	apps := []Application{first}
	seen := map[string]bool{applicationIdentity(first): true}

	apps, fresh := appendUniqueApplications(apps, []Application{first, second, second}, seen)
	if fresh != 1 || len(apps) != 2 || apps[1].DetailPk != "2" {
		t.Fatalf("deduplicated page = %#v, fresh=%d", apps, fresh)
	}
	apps, fresh = appendUniqueApplications(apps, []Application{first, second}, seen)
	if fresh != 0 || len(apps) != 2 {
		t.Fatalf("repeated page made progress: %#v, fresh=%d", apps, fresh)
	}
}

func TestCollectApplicationPagesUsesSamePaginationForEveryTransport(t *testing.T) {
	page := func(total int, pk, title string) string {
		return fmt.Sprintf(`<p>전체 <span>%d</span> 건</p><div class="apply-result-item">
			<div class="apply-result-link"><a href="javascript:fn_detail('uddi:%s','%s')">[승인] %s</a></div>
		</div>`, total, pk, pk, title)
	}
	var requested []int
	apps, err := collectApplicationPages(page(21, "1", "첫째"), func(index int) (string, error) {
		requested = append(requested, index)
		if index == 2 {
			return page(21, "2", "둘째"), nil
		}
		return page(21, "3", "셋째"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 3 || len(requested) != 2 || requested[0] != 2 || requested[1] != 3 {
		t.Fatalf("apps=%+v requested=%v", apps, requested)
	}
}
