package goalwork_test

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These checks validate retained reference data, NOT production execution or
// autonomous goal completion. References were calculated before implementation
// with independent CSV/OOXML parsing; expected literals must not follow solve.
type referenceSlice struct {
	Case    string `json:"case"`
	Status  string `json:"status"`
	Sources []struct {
		PK            string `json:"pk"`
		URL           string `json:"url"`
		ObservedAt    string `json:"observedAt"`
		ContentSHA256 string `json:"contentSha256"`
		License       string `json:"license"`
		Records       []struct {
			CSVRecord int               `json:"csvRecord"`
			Sheet     string            `json:"sheet"`
			Row       int               `json:"row"`
			Values    map[string]string `json:"values"`
			Cells     map[string]string `json:"cells"`
		} `json:"records"`
	} `json:"sources"`
}

func readReference(t *testing.T, filename string, into any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "goalbench-v1", filename))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, into); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentReferenceSlicesRetainSourceProvenance(t *testing.T) {
	for _, name := range []string{"mobility-reference.json", "mobility-comparative-reference.json", "environment-reference.json", "education-reference.json", "education-citywide-reference.json", "product-reference.json"} {
		t.Run(name, func(t *testing.T) {
			var ref referenceSlice
			readReference(t, name, &ref)
			if ref.Status != "partial_oracle_not_goal_completion" || len(ref.Sources) < 2 {
				t.Fatal("reference slice is not a completed goal or a complete oracle")
			}
			for _, source := range ref.Sources {
				u, err := url.Parse(source.URL)
				if err != nil || u.Scheme != "https" || u.Host != "www.data.go.kr" || u.Path != "/cmm/cmm/fileDownload.do" || u.User != nil {
					t.Fatalf("invalid official download provenance for %s", source.PK)
				}
				if source.License == "" || len(source.Records) == 0 {
					t.Fatalf("source %s lacks licensed retained records", source.PK)
				}
				if _, err := time.Parse(time.RFC3339Nano, source.ObservedAt); err != nil {
					t.Fatal(err)
				}
				hash, err := hex.DecodeString(source.ContentSHA256)
				if err != nil || len(hash) != 32 {
					t.Fatal("source revision hash missing; hash is identification, not an archived original")
				}
				for _, record := range source.Records {
					if record.CSVRecord < 2 && (record.Sheet == "" || record.Row < 1) {
						t.Fatal("retained record lacks original source position")
					}
				}
			}
		})
	}
}

func TestCitywideEducationReferenceMatchesPublisherAgeTotals(t *testing.T) {
	var ref referenceSlice
	readReference(t, "education-citywide-reference.json", &ref)
	if len(ref.Sources) != 2 || ref.Sources[0].PK != "15097972" || len(ref.Sources[0].Records) != 162 {
		t.Fatal("expected all 162 Incheon population records, including branch offices")
	}
	counts := map[string]int{}
	ageCounts := map[string]int{}
	codes := map[string]bool{}
	for _, record := range ref.Sources[0].Records {
		v := record.Values
		code := v["행정기관코드"]
		if codes[code] || len(code) != 10 || v["시도명"] != "인천광역시" || v["기준연월"] != "2026-07-31" {
			t.Fatal("population selection, identifier uniqueness or snapshot changed")
		}
		codes[code] = true
		for age := 6; age <= 17; age++ {
			for _, sex := range []string{"남자", "여자"} {
				n, err := strconv.Atoi(v[strconv.Itoa(age)+"세"+sex])
				if err != nil || n < 0 {
					t.Fatal("exact age/sex count missing or invalid")
				}
				counts[v["시군구명"]] += n
				ageCounts[v["시군구명"]+"/"+strconv.Itoa(age)] += n
			}
		}
	}
	var oracle struct {
		PopulationCrossCheck struct {
			URL     string `json:"url"`
			Records [][]struct {
				Text  string            `json:"text"`
				Attrs map[string]string `json:"attrs"`
			} `json:"records"`
		} `json:"populationCrossCheck"`
		Expected struct {
			SchoolAgePopulation int `json:"schoolAgePopulation"`
			Rows                []struct {
				District           string `json:"district"`
				ResidentPopulation int    `json:"residentPopulation"`
			} `json:"rows"`
		} `json:"expected"`
	}
	readReference(t, "education-citywide-reference.json", &oracle)
	if len(counts) != 11 || len(oracle.Expected.Rows) != 11 || oracle.Expected.SchoolAgePopulation != 304280 {
		t.Fatal("citywide independent expectation changed")
	}
	for _, row := range oracle.Expected.Rows {
		if counts[row.District] != row.ResidentPopulation {
			t.Fatalf("%s serialized expectation disagrees with actual retained counts", row.District)
		}
	}
	check := oracle.PopulationCrossCheck
	if check.URL != "https://jumin.mois.go.kr/ageStatMonth.do" || len(check.Records) != 12 {
		t.Fatal("publisher city + 11 district cross-check missing")
	}
	seen := map[string]bool{}
	for _, row := range check.Records {
		if len(row) != 44 || row[3].Attrs["title"] != "2026년 07월 / 계" {
			t.Fatal("publisher table layout or date changed")
		}
		district := row[1].Text
		if seen[district] {
			t.Fatal("duplicate publisher comparison region")
		}
		seen[district] = true
		total := counts[district]
		if district == "인천광역시" {
			for _, n := range counts {
				total += n
			}
		}
		published, err := strconv.Atoi(strings.ReplaceAll(row[3].Text, ",", ""))
		if err != nil || published != total {
			t.Fatalf("%s record sum differs from publisher aggregate", district)
		}
		if district == "인천광역시" {
			continue
		}
		for age := 6; age <= 17; age++ {
			n, err := strconv.Atoi(strings.ReplaceAll(row[age-6+4].Text, ",", ""))
			if err != nil || ageCounts[district+"/"+strconv.Itoa(age)] != n {
				t.Fatalf("%s age %d differs from publisher aggregate", district, age)
			}
		}
	}
}

func TestCitywideEducationReferenceRetainsReorganisationAndCensusSeparately(t *testing.T) {
	var ref referenceSlice
	readReference(t, "education-citywide-reference.json", &ref)
	var details struct {
		AreaMapping struct {
			SourceRow                    int    `json:"sourceRow"`
			SourceLabel                  string `json:"sourceLabel"`
			TargetLabel                  string `json:"targetLabel"`
			SourceGeographyEffectiveDate string `json:"sourceGeographyEffectiveDate"`
			OfficialURL                  string `json:"officialURL"`
		} `json:"areaMapping"`
		Sources []struct {
			ReferenceDate               string `json:"referenceDate"`
			GeographyEffectiveDate      string `json:"geographyEffectiveDate"`
			ExcludedOldGeographyRecords []struct {
				Row   int               `json:"row"`
				Cells map[string]string `json:"cells"`
			} `json:"excludedOldGeographyRecords"`
		} `json:"sources"`
	}
	readReference(t, "education-citywide-reference.json", &details)
	m := details.AreaMapping
	if m.SourceRow != 35 || m.SourceLabel != "서구" || m.TargetLabel != "서해구" || m.SourceGeographyEffectiveDate != "2026-07-01" || !strings.Contains(m.OfficialURL, "lsiSeq=286453") {
		t.Fatal("mapping must be bound to the post-split table and official renaming evidence")
	}
	s := details.Sources[1]
	if s.ReferenceDate != "2026-04-01" || s.GeographyEffectiveDate != "2026-07-01" || len(ref.Sources[1].Records) != 11 {
		t.Fatal("school census and geography revision are different dates")
	}
	totals := map[string]int{}
	for _, record := range ref.Sources[1].Records {
		if record.Row < 27 || record.Row > 37 || record.Sheet != "구·군별" {
			t.Fatal("old table, total or individual-school record entered the selected table")
		}
		for _, column := range []string{"AG", "AH", "AK", "AL"} {
			n, err := strconv.Atoi(record.Cells[column])
			if err != nil {
				t.Fatal(err)
			}
			totals[column] += n
		}
	}
	if totals["AG"] != 960 || totals["AH"] != 7 || totals["AK"] != 335562 || totals["AL"] != 63 {
		t.Fatal("published school total and branch counts must remain separate")
	}
	oldSeo := ""
	for _, r := range s.ExcludedOldGeographyRecords {
		if r.Row == 14 {
			oldSeo = r.Cells["AG"]
		}
	}
	if oldSeo != "191" || ref.Sources[1].Records[8].Cells["AG"] != "111" || ref.Sources[1].Records[9].Cells["AG"] != "80" {
		t.Fatal("old Seo-gu is not the post-split Seohae-gu: 191 = 111 + 80")
	}
}

func TestEducationReferenceArithmeticMatchesIndependentOracle(t *testing.T) {
	var ref referenceSlice
	readReference(t, "education-reference.json", &ref)
	if ref.Case != "G4" || len(ref.Sources) != 2 || ref.Sources[0].PK != "15085585" || ref.Sources[1].PK != "15004947" {
		t.Fatal("reference source order changed; review the independent oracle")
	}
	population := map[string]int{}
	ageSex := map[string]bool{}
	for _, record := range ref.Sources[0].Records {
		v := record.Values
		age, err := strconv.Atoi(v["연령(세)"])
		key := v["연령(세)"] + "/" + v["성별"]
		if err != nil || age < 6 || age > 17 || ageSex[key] || (v["성별"] != "남" && v["성별"] != "여") {
			t.Fatal("age bounds, sex or uniqueness differ from oracle")
		}
		ageSex[key] = true
		for field, value := range v {
			if strings.HasSuffix(field, "구성비") {
				t.Fatal("ratios must not enter the population sum")
			}
			if strings.HasSuffix(field, "인구수") {
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					t.Fatal("invalid reference count")
				}
				population[field] += n
			}
		}
	}
	total := 0
	for _, count := range population {
		total += count
	}
	if len(ageSex) != 24 || len(population) != 22 || total != 40981 || population["부평1동인구수"] != 2332 {
		t.Fatalf("independent 6–17 population oracle changed: combinations=%d dongs=%d total=%d", len(ageSex), len(population), total)
	}
	var oracle struct {
		Expected struct {
			PopulationByDong    map[string]int `json:"populationByDong"`
			SchoolAgePopulation int            `json:"schoolAgePopulation"`
			SchoolRows          int            `json:"schoolRows"`
			EnrolledStudents    int            `json:"enrolledStudents"`
		} `json:"expected"`
	}
	readReference(t, "education-reference.json", &oracle)
	if oracle.Expected.SchoolAgePopulation != total || len(oracle.Expected.PopulationByDong) != len(population) {
		t.Fatal("serialized expected population disagrees with retained reference records")
	}
	for field, count := range population {
		if oracle.Expected.PopulationByDong[strings.TrimSuffix(field, "인구수")] != count {
			t.Fatal("serialized dong expectation disagrees with retained reference records")
		}
	}
	counts, students := map[string]int{}, map[string]int{}
	for _, record := range ref.Sources[1].Records {
		column := map[string]string{"초등학교": "AH", "중학교": "Z", "고등학교": "Z"}[record.Sheet]
		if column == "" || record.Cells["B"] != "부평구" {
			t.Fatal("unexpected sheet or pre-reorganisation district")
		}
		n, err := strconv.Atoi(record.Cells[column])
		if err != nil {
			t.Fatal(err)
		}
		counts[record.Sheet]++
		students[record.Sheet] += n
	}
	for _, want := range []struct {
		sheet string
		count int
		total int
	}{{"초등학교", 42, 19189}, {"중학교", 21, 11120}, {"고등학교", 19, 9934}} {
		if counts[want.sheet] != want.count || students[want.sheet] != want.total {
			t.Fatalf("%s disagrees with independently read district summary cells", want.sheet)
		}
	}
	if oracle.Expected.SchoolRows != 82 || oracle.Expected.EnrolledStudents != 40243 {
		t.Fatal("serialized school expectation disagrees with independent summary")
	}
}

func TestMobilityReferenceDoesNotSilentlySwapAxes(t *testing.T) {
	var ref referenceSlice
	readReference(t, "mobility-reference.json", &ref)
	if ref.Sources[0].Records[0].CSVRecord != 149 || ref.Sources[0].Records[1].CSVRecord != 937 {
		t.Fatal("same facility has different row ordinals; positional joins are invalid")
	}
	stop := ref.Sources[1].Records[0].Values
	latitude, err := strconv.ParseFloat(stop["위도"], 64)
	if err != nil || latitude <= 90 {
		t.Fatal("real malformed latitude counterexample changed")
	}
}

func TestEnvironmentReferencePreservesStatisticalGrainAndCoverage(t *testing.T) {
	var ref referenceSlice
	readReference(t, "environment-reference.json", &ref)
	locations := map[string]int{}
	for _, record := range ref.Sources[0].Records {
		if record.Values["측정소구분"] != "도시대기" {
			t.Fatal("reference location network changed")
		}
		locations[record.Values["시도"]]++
	}
	if locations["서울"] != 25 || locations["인천"] != 26 {
		t.Fatal("December 31 location snapshot changed")
	}
	if len(ref.Sources[1].Records) != 2 {
		t.Fatal("city reference slice changed")
	}
	for _, record := range ref.Sources[1].Records {
		v := record.Values
		if v["구분"] != "도시별" || v["통계월"] != "2024-12" || v["오염물질명"] != "PM25" || v["1시간 월평균"] != "19" || v["측정소수"] != "25" {
			t.Fatal("city measurement oracle changed; province duplicates cannot substitute")
		}
	}
	// Incheon's 26 location records must not be rewritten to equal the 25
	// measurement contributors. Agreement in group names is not membership proof.
}

func TestProductReferenceDistinguishesCertificateFromModelLabel(t *testing.T) {
	var ref referenceSlice
	readReference(t, "product-reference.json", &ref)
	exact, nameOnly := 0, 0
	for _, recall := range ref.Sources[0].Records {
		for _, certificate := range ref.Sources[1].Records {
			if recall.Values["모델명"] != certificate.Values["모델명"] {
				continue
			}
			if recall.Values["인증번호"] == certificate.Values["인증번호"] {
				exact++
			} else {
				nameOnly++
			}
		}
	}
	if exact != 3 || nameOnly != 1 {
		t.Fatalf("expected 3 exact tuple pairs and 1 name-only hard negative, got %d and %d", exact, nameOnly)
	}
}
