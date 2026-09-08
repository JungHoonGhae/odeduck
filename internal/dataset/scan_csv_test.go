package dataset

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"golang.org/x/text/encoding/korean"
)

func scanFixture(body string) (*Inspector, Asset) {
	u := "https://data.test/large.csv"
	return NewInspector(fixtureTransport{gets: map[string]*fetch.Response{u: {Status: 200, Body: []byte(body)}}}, ""), Asset{Name: "large.csv", Format: "CSV", Request: Request{Method: http.MethodGet, URL: u}}
}

func TestCSVFullScanSelectsExactValueSetsBeforeRetention(t *testing.T) {
	i, a := scanFixture("city,kind,id\nA,urban,001\nB,urban,002\nB,rural,003\n C,urban,004\nC,urban,005\nA,urban,006\n")
	s, err := i.SampleCSVScanned(context.Background(), a, 2, CSVSelection{Equals: map[string]string{"kind": "urban"}, In: map[string][]string{"city": {"A", "C"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 2 || s.Rows[0]["id"] != "001" || s.Rows[1]["id"] != "005" || s.CSV.DataRecords[1] != 5 || !s.Prefix || !s.Selection.Exhausted || s.Selection.ScannedRows != 6 || s.Selection.MatchedRows != 3 || s.Selection.ReturnedRows != 2 {
		t.Fatalf("set selection lost exact matching or scan/retention distinction: %+v", s)
	}
}

func TestCSVFullScanRejectsInvalidValueSetsAndBadExcludedTails(t *testing.T) {
	for _, selection := range []CSVSelection{
		{In: map[string][]string{"id": {}}},
		{In: map[string][]string{"id": {"001", "001"}}},
		{In: map[string][]string{"id": {" "}}},
		{In: map[string][]string{"id": {strings.Repeat("x", 257)}}},
		{In: map[string][]string{"id": strings.Split(strings.Repeat("x,", 33), ",")}},
		{In: map[string][]string{"missing": {"001"}}},
		{Equals: map[string]string{"id": "001"}, In: map[string][]string{"id": {"001"}}},
		{Equals: map[string]string{"a": "1", "b": "1", "c": "1", "d": "1", "e": "1", "f": "1", "g": "1", "h": "1"}, In: map[string][]string{"id": {"001"}}},
	} {
		i, a := scanFixture("id\n001\n")
		if s, err := i.SampleCSVScanned(context.Background(), a, 1, selection); err == nil || len(s.Rows) > 0 || s.SHA256 != "" {
			t.Fatalf("invalid selection accepted: %+v", selection)
		}
	}
	for _, tail := range []string{"other,extra\n", "\xff\n", "\"unterminated\n"} {
		i, a := scanFixture("id\n001\n" + tail)
		if s, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{In: map[string][]string{"id": {"001", "002"}}}); err == nil || len(s.Rows) > 0 || s.SHA256 != "" {
			t.Fatal("valid selected prefix hid an invalid excluded tail")
		}
	}
}

func TestCSVFullScanContinuesAfterRetainedPrefixAndPreservesHash(t *testing.T) {
	body := "scope,id\n" + strings.Repeat("other,999\n", 1001) + "target,001\ntarget,002\ntarget,003\n"
	i, a := scanFixture(body)
	s, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{Equals: map[string]string{"scope": "target"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 1 || s.Rows[0]["id"] != "001" || !s.Prefix || s.Selection == nil || s.Selection.ScannedRows != 1004 || s.Selection.MatchedRows != 3 || !s.Selection.Exhausted || s.CSV.DataRecords[0] != 1002 || s.CSV.StartLines[0] != 1003 || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
		t.Fatalf("full scan confused with retained prefix: %+v", s)
	}
}

func TestCSVFullScanRetainsEmptyStringsLikeStreamedRecords(t *testing.T) {
	i, a := scanFixture("id,empty,space,literal\n001,, ,null\n")
	s, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{})
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{"id": "001", "empty": "", "space": " ", "literal": "null"} {
		if got, exists := s.Rows[0][field]; !exists || got != want {
			t.Fatalf("CSV retained value changed: %s=%#v, want %q", field, got, want)
		}
	}
}

func TestCSVFullScanRejectsBadTailAfterUsablePrefix(t *testing.T) {
	for _, tail := range []string{"other,invalid,extra\n", "other,\xff\n", "other,\"unterminated\n"} {
		i, a := scanFixture("scope,id\ntarget,001\n" + tail)
		if s, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{Equals: map[string]string{"scope": "target"}}); err == nil || len(s.Rows) > 0 || s.SHA256 != "" {
			t.Fatal("invalid excluded tail yielded verified data")
		}
	}
}

func TestCSVFullScanStreamsBeyondOldDownloadCap(t *testing.T) {
	body := "scope,id\n" + strings.Repeat("other,"+strings.Repeat("x", 1024)+"\n", 9000) + "target,001\n"
	i, a := scanFixture(body)
	seen := 0
	report, err := i.ScanCSV(context.Background(), a, CSVSelection{Equals: map[string]string{"scope": "target"}}, func(r CSVScanRecord) error {
		seen++
		if r.Values["id"] != "001" || r.DataRecord != 9001 {
			return fmt.Errorf("wrong streamed row")
		}
		return nil
	})
	if err != nil || seen != 1 || report.ScannedRows != 9001 || report.MatchedRows != 1 || report.Bytes <= 8<<20 || !report.Exhausted {
		t.Fatalf("large source failed: %+v %v", report, err)
	}
}

func TestCSVFullScanDoesNotPromoteConsumerFailureOrCancelledContext(t *testing.T) {
	i, a := scanFixture("id\n001\n002\n")
	report, err := i.ScanCSV(context.Background(), a, CSVSelection{}, func(CSVScanRecord) error { return fmt.Errorf("consumer rejected record") })
	if err == nil || report.Exhausted || report.SHA256 != "" {
		t.Fatal("failed stream consumer certified a full scan")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := i.ScanCSV(ctx, a, CSVSelection{}, nil); err == nil {
		t.Fatal("cancelled scan proceeded")
	}
}

func TestCSVFullScanStrictEUCKRAndChunkBoundaries(t *testing.T) {
	// Odd and even padding place two-byte code units on both sides of reader
	// buffer boundaries. Hashes describe original encoded bytes, not UTF-8 output.
	for _, padding := range []int{4091, 4092, 8191, 8192} {
		body, err := korean.EUCKR.NewEncoder().String("도시명,id,설명\n인천광역시,001," + strings.Repeat("x", padding) + strings.Repeat("한", 5000) + "\n")
		if err != nil {
			t.Fatal(err)
		}
		i, a := scanFixture(body)
		s, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{Equals: map[string]string{"도시명": "인천광역시"}})
		if err != nil {
			t.Fatal(err)
		}
		if s.CSV.Encoding != "euc-kr" || s.Rows[0]["id"] != "001" || s.Rows[0]["설명"] != strings.Repeat("x", padding)+strings.Repeat("한", 5000) || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(body))) {
			t.Fatal("stream decoding lost source bytes or values")
		}
	}
	header, err := korean.EUCKR.NewEncoder().String("도시명,id\n인천광역시,001\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{"other,\xff\xff\n", "other,\xb0"} {
		i, a := scanFixture(header + tail)
		if s, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{}); err == nil || s.SHA256 != "" || len(s.Rows) != 0 {
			t.Fatal("invalid EUC-KR excluded tail certified a source")
		}
	}
	// An ASCII header is deliberately not sufficient evidence to switch codecs.
	i, a := scanFixture("city,id\n\xb0\xa1,001\n")
	if _, err := i.SampleCSVScanned(context.Background(), a, 1, CSVSelection{}); err == nil {
		t.Fatal("silently switched encoding after an ASCII header")
	}
}

func TestCSVFullScanBoundsMalformedRecordsAndRetainedMemory(t *testing.T) {
	for _, body := range []string{
		"id\n" + strings.Repeat("x", (64<<10)+1) + "\n",
		"id\n\"" + strings.Repeat("x", 3<<20),
		"id\n" + strings.Repeat(strings.Repeat("x", 60<<10)+"\n", 40),
	} {
		i, a := scanFixture(body)
		if s, err := i.SampleCSVScanned(context.Background(), a, 1000, CSVSelection{}); err == nil || len(s.Rows) != 0 || s.SHA256 != "" {
			t.Fatal("oversized field, parser window or retained sample was accepted")
		}
	}
}
