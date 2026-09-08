package goalwork_test

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Fixture-integrity checks only. The source scan and haversine oracle were
// calculated independently before production geographic execution exists.
func TestMobilityComparativeReferenceRetainsIdentityAndDistances(t *testing.T) {
	var ref referenceSlice
	readReference(t, "mobility-comparative-reference.json", &ref)
	if len(ref.Sources) != 2 || ref.Sources[0].PK != "15109171" || ref.Sources[1].PK != "15067528" || len(ref.Sources[0].Records) != 8 {
		t.Fatal("four facility/accessibility pairs and alternative stop source required")
	}
	var oracle struct {
		Expected struct {
			Rows []struct {
				FacilityCSVRecord      int    `json:"facilityCSVRecord"`
				AccessibilityCSVRecord int    `json:"accessibilityCSVRecord"`
				RouteAccessibility     string `json:"routeAccessibility"`
				CurrentOperation       string `json:"currentOperation"`
				Nearby                 []struct {
					StopCSVRecord int     `json:"stopCSVRecord"`
					StopID        string  `json:"stopID"`
					Manager       string  `json:"manager"`
					Meters        float64 `json:"sphericalDistanceMeters"`
				} `json:"nearbyTransitRecords"`
			} `json:"rows"`
			Distance struct {
				Radius float64 `json:"sphereRadiusMeters"`
				Status string  `json:"status"`
			} `json:"distanceDefinition"`
		} `json:"expected"`
	}
	readReference(t, "mobility-comparative-reference.json", &oracle)
	if len(oracle.Expected.Rows) != 4 || oracle.Expected.Distance.Radius != 6371008.8 || oracle.Expected.Distance.Status != "calculation_verified_datum_policy_pending" {
		t.Fatal("numeric calculation must not silently assert verified common datum or route")
	}
	stops := map[int]map[string]string{}
	for _, r := range ref.Sources[1].Records {
		stops[r.CSVRecord] = r.Values
	}
	wantFirst := []string{"GGB168000447", "ICB164000151", "ICB164000397", "ICB161000126"}
	for i, row := range oracle.Expected.Rows {
		f, a := ref.Sources[0].Records[2*i], ref.Sources[0].Records[2*i+1]
		if f.CSVRecord != row.FacilityCSVRecord || a.CSVRecord != row.AccessibilityCSVRecord || f.CSVRecord == a.CSVRecord {
			t.Fatal("ZIP member row ordinals are not identity")
		}
		for _, k := range []string{"업체명", "시도명", "시군구명", "도로명", "건물번호", "위도", "경도"} {
			if f.Values[k] == "" || f.Values[k] != a.Values[k] {
				t.Fatalf("full observed identity mismatch: %s", k)
			}
		}
		if row.RouteAccessibility != "unknown" || row.CurrentOperation != "unknown" || len(row.Nearby) != 3 || row.Nearby[0].StopID != wantFirst[i] {
			t.Fatal("published nearest source-record reference or unknown route/operation changed")
		}
		for _, nearby := range row.Nearby {
			s := stops[nearby.StopCSVRecord]
			if s["정류장번호"] != nearby.StopID || s["관리도시명"] != nearby.Manager || s["정보수집일"] != "2025-10-31" {
				t.Fatal("stop source namespace, record or collection date lost")
			}
			// Independent check uses unit-vector cross/dot angle, not the
			// haversine expression that generated the oracle.
			u, v := referenceUnitVector(t, f.Values), referenceUnitVector(t, s)
			cross := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
			sine := math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
			cosine := u[0]*v[0] + u[1]*v[1] + u[2]*v[2]
			distance := 6371008.8 * math.Atan2(sine, cosine)
			if math.Abs(distance-nearby.Meters) > 0.000001 {
				t.Fatalf("distance mismatch: %.9f vs %.9f", distance, nearby.Meters)
			}
		}
	}
	city := oracle.Expected.Rows[2].Nearby
	if !strings.HasPrefix(city[0].StopID, "ICB") || !strings.HasPrefix(city[1].StopID, "GGB") || city[0].StopID[3:] != city[1].StopID[3:] || city[0].Manager == city[1].Manager {
		t.Fatal("actual same-suffix cross-BIS records must remain distinct source identifiers")
	}
}

func referenceUnitVector(t *testing.T, fields map[string]string) [3]float64 {
	t.Helper()
	lat, err1 := strconv.ParseFloat(fields["위도"], 64)
	lon, err2 := strconv.ParseFloat(fields["경도"], 64)
	if err1 != nil || err2 != nil || math.IsNaN(lat) || math.IsNaN(lon) || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		t.Fatal("invalid source geometry")
	}
	lat, lon = lat*math.Pi/180, lon*math.Pi/180
	return [3]float64{math.Cos(lat) * math.Cos(lon), math.Cos(lat) * math.Sin(lon), math.Sin(lat)}
}

func TestMobilityEventsDoNotCloseTheParentMuseumOrUsePublicationAsEventDate(t *testing.T) {
	var ref struct {
		OperationEvents []struct {
			Subject      string `json:"subject"`
			PublishedAt  string `json:"publishedAt"`
			ValidFrom    string `json:"validFrom"`
			ValidThrough string `json:"validThrough"`
			EventDate    string `json:"eventDate"`
			URL          string `json:"url"`
		} `json:"operationEvents"`
	}
	readReference(t, "mobility-comparative-reference.json", &ref)
	if len(ref.OperationEvents) != 2 {
		t.Fatal("actual bounded opening and closure events required")
	}
	garden, closed := ref.OperationEvents[0], ref.OperationEvents[1]
	if garden.Subject != "검단선사박물관 하늘정원" || garden.PublishedAt != "2026-04-07" || garden.ValidFrom != "2026-09-01" || garden.ValidThrough != "2026-10-31" {
		t.Fatal("one subfacility's operation interval differs from publication date and whole-museum availability")
	}
	if closed.Subject != "인천시청역 열린박물관" || closed.PublishedAt != "2026-06-04" || closed.EventDate != "2026-06-30" || closed.URL != "https://www.incheon.go.kr/museum/MU060102/3075576" {
		t.Fatal("named station venue closure is not city museum closure")
	}
}

func TestMobilityReferenceDoesNotMergeMuseumAndDirectorOffice(t *testing.T) {
	var ref referenceSlice
	readReference(t, "mobility-comparative-reference.json", &ref)
	var exclusions struct {
		Sources []struct {
			ExcludedRecords []struct {
				CSVRecord int               `json:"csvRecord"`
				Values    map[string]string `json:"values"`
			} `json:"excludedRecords"`
		} `json:"sources"`
	}
	readReference(t, "mobility-comparative-reference.json", &exclusions)
	if len(exclusions.Sources[0].ExcludedRecords) != 1 {
		t.Fatal("real part/whole negative missing")
	}
	office := exclusions.Sources[0].ExcludedRecords[0]
	museum := ref.Sources[0].Records[2]
	if office.CSVRecord != 158 || museum.CSVRecord != 143 || office.Values["업체명"] != "인천시립박물관 관장실" || museum.Values["업체명"] != "인천광역시립박물관" {
		t.Fatal("part and whole identity evidence changed")
	}
	for _, key := range []string{"시도명", "시군구명", "도로명", "건물번호", "위도", "경도"} {
		if office.Values[key] != museum.Values[key] {
			t.Fatal("counterexample must retain identical location despite distinct subjects")
		}
	}
}
