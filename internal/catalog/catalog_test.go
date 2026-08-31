package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/opendatactl/internal/portal"
)

func sample() *Catalog {
	return &Catalog{
		SyncedAt: time.Now(),
		Type:     "API",
		Entries: []Entry{
			{PK: "1", Title: "행정안전부_무더위쉼터", Org: "행정안전부", ApplyCount: 2796,
				Desc: "폭염 대비 무더위쉼터 위치 정보"},
			{PK: "2", Title: "기상청_단기예보 조회서비스", Org: "기상청", ApplyCount: 63350,
				Desc: "동네예보 기온 강수 정보"},
			{PK: "3", Title: "행정안전부_폭염 인명피해", Org: "행정안전부", ApplyCount: 508,
				Desc: "온열질환자 지역별 현황"},
		},
	}
}

func isolateConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
}

func TestCatalogSaveAtomicallyReplacesSnapshot(t *testing.T) {
	isolateConfigHome(t)
	first := &Catalog{SyncedAt: time.Now(), Type: "API", Entries: []Entry{{PK: "1", Title: "first"}}}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	second := &Catalog{SyncedAt: time.Now(), Type: "API", Entries: []Entry{{PK: "2", Title: "second"}}}
	if err := second.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].PK != "2" {
		t.Fatalf("loaded snapshot = %+v", loaded.Entries)
	}
	dir, err := portal.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, "catalog-*.tmp"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary files after save = %v, err=%v", temps, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "catalog.json")); err != nil {
		t.Fatalf("catalog stat: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("catalog mode = %v, want 0644", info.Mode().Perm())
	}
}

// Every term must match, and matches rank by demand — the agent should see the
// heavily used dataset first rather than whatever happened to be stored first.
func TestSearchRanksByDemand(t *testing.T) {
	c := sample()
	r := c.Search("기온", 10, false)
	hits, total := r.Hits, r.Total
	if total != 1 || len(hits) != 1 || hits[0].PK != "2" {
		t.Fatalf("기온 → %d hits (total %d), first %+v", len(hits), total, hits)
	}

	r = c.Search("행정안전부", 10, false)
	hits, total = r.Hits, r.Total
	if total != 2 {
		t.Fatalf("행정안전부 → total %d, want 2", total)
	}
	if hits[0].ApplyCount < hits[1].ApplyCount {
		t.Errorf("not ranked by applyCount: %d then %d", hits[0].ApplyCount, hits[1].ApplyCount)
	}
}

// Precision first: when entries match every term, only those are returned and
// the result is not marked relaxed.
func TestSearchPrefersEveryTermMatching(t *testing.T) {
	r := sample().Search("행정안전부 온열질환자", 10, false)
	if r.Relaxed {
		t.Error("both terms matched, should not relax")
	}
	if r.Total != 1 {
		t.Errorf("both-terms search → total %d, want 1", r.Total)
	}
}

// Recall as a fallback: a query no entry fully satisfies must still answer, and
// must say it loosened the query rather than pretending the result is exact.
func TestSearchRelaxesWhenNothingMatchesEveryTerm(t *testing.T) {
	r := sample().Search("행정안전부 존재하지않는말", 10, false)
	if !r.Relaxed {
		t.Fatal("no entry has both terms — expected a relaxed result, not silence")
	}
	if r.Total == 0 {
		t.Fatal("relaxed search returned nothing")
	}
	if r.Hits[0].Matched != 1 {
		t.Errorf("relaxed hit should report how many terms it matched, got %d", r.Hits[0].Matched)
	}
}

// A single unmatchable word is a genuine miss, not something to widen.
func TestSearchSingleTermMissStaysEmpty(t *testing.T) {
	if r := sample().Search("존재하지않는말", 10, false); r.Total != 0 || r.Relaxed {
		t.Errorf("single unmatchable term → total %d relaxed %v, want 0/false", r.Total, r.Relaxed)
	}
}

// The query is written by a person or an agent, not filled into a search form:
// particles and filler words must not decide whether it finds anything.
func TestSearchHandlesNaturalLanguage(t *testing.T) {
	plain := sample().Search("폭염 인명피해", 10, false)
	natural := sample().Search("폭염으로 인한 인명피해 데이터 알려줘", 10, false)
	if natural.Total != plain.Total {
		t.Errorf("natural phrasing → %d hits, keyword phrasing → %d; should agree", natural.Total, plain.Total)
	}
	if natural.Relaxed {
		t.Errorf("natural phrasing should not need relaxing, terms=%v", natural.Terms)
	}
}

// Trimming must not eat a word: 고가 is not 고 + the particle 가.
func TestQueryTermsKeepsShortWordsIntact(t *testing.T) {
	for q, want := range map[string]string{
		"폭염에":   "폭염",
		"광진구에서": "광진구",
		"고가":    "고가",
		"실거래가":  "실거래",
	} {
		if got := queryTerms(q); len(got) != 1 || got[0] != want {
			t.Errorf("queryTerms(%q) = %v, want [%s]", q, got, want)
		}
	}
}

func TestQueryTermsSplitsAgentPunctuation(t *testing.T) {
	got := queryTerms("공매·압류재산/매각-현황")
	want := []string{"공매", "압류재산", "매각", "현황"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("queryTerms punctuation = %v, want %v", got, want)
	}
}

func TestQueryTermsDropsFillersAfterParticleStripping(t *testing.T) {
	got := queryTerms("폭염 데이터를 자료를 정보를 찾아줘")
	if len(got) != 1 || got[0] != "폭염" {
		t.Fatalf("queryTerms inflected fillers = %v, want [폭염]", got)
	}
}

func TestSearchPlanRecentInspectsBeyondExternalResultCap(t *testing.T) {
	c := &Catalog{}
	for i := 0; i < 150; i++ {
		c.Entries = append(c.Entries, Entry{
			PK: fmt.Sprintf("%d", i), Title: "상권 매출", SvcType: SvcREST,
			ApplyCount: 1000 - i, ModifiedAt: fmt.Sprintf("2025-01-%02d", i%28+1),
		})
	}
	c.Entries[149].ModifiedAt = "2026-08-31"
	res := c.SearchPlan(QueryPlan{Concepts: []string{"상권"}, Limit: 1, RESTOnly: true, Ranking: RankRecent})
	if len(res.Hits) != 1 || res.Hits[0].PK != "149" {
		t.Fatalf("recent result = %+v, want low-demand newest entry beyond top 100", res.Hits)
	}
}

// The description is what makes matching work, and is exactly what must not be
// handed back — ten descriptions is thousands of characters of an agent's context.
func TestSearchDoesNotReturnDescriptions(t *testing.T) {
	hits := sample().Search("폭염", 10, false).Hits
	if len(hits) == 0 {
		t.Fatal("expected description-matched hits")
	}
	for _, h := range hits {
		if strings.Contains(h.Title, "온열질환자 지역별") {
			t.Error("description leaked into the title field")
		}
	}
	// Hit has no Desc field at all; assert the type stays that way.
	var h any = hits[0]
	if _, bad := h.(interface{ GetDesc() string }); bad {
		t.Error("Hit must not expose a description")
	}
}

// A capped result set still reports how many matched, so a caller knows to narrow.
func TestSearchCapsButReportsTotal(t *testing.T) {
	r := sample().Search("", 2, false)
	hits, total := r.Hits, r.Total
	if len(hits) != 2 || total != 3 {
		t.Errorf("limit 2 over 3 entries → %d hits, total %d", len(hits), total)
	}
}

func TestStale(t *testing.T) {
	c := sample()
	if c.Stale() {
		t.Error("just-synced catalogue must not be stale")
	}
	c.SyncedAt = time.Now().Add(-StaleAfter - time.Hour)
	if !c.Stale() {
		t.Error("old catalogue must be stale")
	}
}

// A word in the dataset's name is a stronger signal than the same word buried in
// its description — otherwise a relaxed search recommends whatever popular
// dataset happens to mention the word.
func TestSearchPrefersNameMatchesOverDescriptionMatches(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "1", Title: "인기 있는 교통 통계", ApplyCount: 9000, Desc: "지역 상권 변화도 참고할 수 있습니다"},
		{PK: "2", Title: "소상공인시장진흥공단_상권정보", ApplyCount: 10},
	}}
	r := c.Search("상권", 10, false)
	if r.Hits[0].PK != "2" {
		t.Errorf("name match should outrank a description mention; got pk=%s first", r.Hits[0].PK)
	}
}

// A LINK dataset has no spec on the portal, so an agent heading for describe →
// call must be able to exclude them: applying for one spends a real application
// on something it cannot call. Unlabelled entries are excluded too — restOnly
// promises "the portal says REST", and an unverified guess is not that.
func TestSearchRESTOnlyExcludesLinkAndUnknown(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "rest", Title: "폭염 정보", SvcType: SvcREST, ApplyCount: 1},
		{PK: "link", Title: "폭염 연계", SvcType: SvcLINK, ApplyCount: 900},
		{PK: "unknown", Title: "폭염 기타", ApplyCount: 500},
	}}
	if all := c.Search("폭염", 10, false); all.Total != 3 {
		t.Errorf("unfiltered search → %d, want 3", all.Total)
	}
	r := c.Search("폭염", 10, true)
	if r.Total != 1 || r.Hits[0].PK != "rest" {
		t.Fatalf("restOnly → total %d first %v, want 1 rest", r.Total, r.Hits)
	}
	if r.Hits[0].SvcType != SvcREST {
		t.Errorf("hit should carry its service type, got %q", r.Hits[0].SvcType)
	}
}

// Semantic interpretation belongs to the MCP host model, not a growing list of
// hard-coded synonyms in the catalogue. SearchPlan executes those inferred data
// axes independently and interleaves them, so one prolific publisher cannot
// bury every other way of satisfying a broad user goal.
func TestSearchPlanDiversifiesSemanticQueries(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "onbid-list", Title: "온비드 공매 부동산 물건", SvcType: SvcREST, ApplyCount: 1200},
		{PK: "onbid-detail", Title: "온비드 공매 부동산 상세", SvcType: SvcREST, ApplyCount: 1100},
		{PK: "bid", Title: "나라장터 입찰 공고", SvcType: SvcREST, ApplyCount: 9000},
		{PK: "auction", Title: "도매시장 실시간 경매 가격", SvcType: SvcREST, ApplyCount: 800},
		{PK: "noise", Title: "인기 관광 정보", SvcType: SvcREST, ApplyCount: 50000},
	}}

	r := c.SearchPlan(QueryPlan{
		Intent: "돈이 될 만한 데이터를 찾아줘",
		Concepts: []string{
			"온비드 공매", "나라장터 입찰", "도매시장 경매",
		},
		Limit: 3, RESTOnly: true, IncludePreviews: true,
	})
	if r.Mode != SearchModePlanned {
		t.Fatalf("mode = %q, want %q", r.Mode, SearchModePlanned)
	}
	if len(r.Hits) != 3 {
		t.Fatalf("hits = %+v, want one candidate from each semantic query", r.Hits)
	}
	want := []string{"onbid-list", "bid", "auction"}
	for i, pk := range want {
		if r.Hits[i].PK != pk {
			t.Errorf("hit[%d] = %s, want %s; hits=%+v", i, r.Hits[i].PK, pk, r.Hits)
		}
		if r.Hits[i].MatchedQuery == "" {
			t.Errorf("hit[%d] does not explain which semantic query found it", i)
		}
	}
	if r.Total != 4 { // both Onbid rows plus one row from each other axis
		t.Errorf("total = %d, want 4 distinct candidates", r.Total)
	}
}

func TestSearchPlanFallsBackToOrdinarySearch(t *testing.T) {
	c := sample()
	plain := c.Search("폭염", 10, false)
	planned := c.SearchPlan(QueryPlan{Intent: "폭염", Limit: 10})
	if planned.Mode != SearchModeLexical || len(planned.Hits) != len(plain.Hits) || planned.Hits[0].PK != plain.Hits[0].PK {
		t.Fatalf("unplanned search changed behavior: plain=%+v planned=%+v", plain, planned)
	}
}

func TestSearchPlanRelaxationRejectsSingleGenericWord(t *testing.T) {
	c := &Catalog{SyncedAt: time.Now(), Entries: []Entry{
		{PK: "relevant", Title: "국유재산 매각 공고", SvcType: SvcREST, ApplyCount: 10},
		{PK: "noise", Title: "인기 대기오염 현황", SvcType: SvcREST, ApplyCount: 9000},
	}}
	r := c.SearchPlan(QueryPlan{Intent: "저가 자산", Concepts: []string{"국유재산 매각 현황"}, Limit: 10, RESTOnly: true})
	if len(r.Hits) != 1 || r.Hits[0].PK != "relevant" {
		t.Fatalf("planned relaxed hits = %+v, want only the two-term match", r.Hits)
	}
}
