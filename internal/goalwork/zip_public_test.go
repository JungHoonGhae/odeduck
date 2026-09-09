package goalwork

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/dataset"
	"github.com/JungHoonGhae/odeduck/internal/fetch"
)

// Scripted production-reader replay against independently retained G2 records.
// No keywords/PK-free discovery or geographic/accessibility approval is claimed.
func TestLiveZIPCSVReaderMatchesIndependentMobilityRecords(t *testing.T) {
	if os.Getenv("ODEDUCK_LIVE_ZIP") != "1" {
		t.Skip("opt-in public ZIP reader replay")
	}
	b, err := os.ReadFile("testdata/goalbench-v1/mobility-comparative-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Sources []struct {
			URL     string `json:"url"`
			SHA     string `json:"contentSha256"`
			Records []struct {
				Member string            `json:"member"`
				SHA    string            `json:"memberSha256"`
				Record int               `json:"csvRecord"`
				Values map[string]string `json:"values"`
			} `json:"records"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(b, &reference); err != nil {
		t.Fatal(err)
	}
	source := reference.Sources[0]
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	i := dataset.NewInspector(fetch.New(), "")
	a := dataset.Asset{Name: "accessibility.zip", Format: "ZIP", Request: dataset.Request{Method: http.MethodGet, URL: source.URL}}
	l, err := i.LayoutFile(ctx, a, "")
	if err != nil {
		t.Fatal(err)
	}
	if l.SHA256 != source.SHA || l.Format != "ZIP" {
		t.Fatal("ZIP changed or was misidentified; do not rewrite oracle")
	}
	samples := map[string]dataset.TableSample{}
	for _, record := range source.Records {
		s, ok := samples[record.Member]
		if !ok {
			found := false
			for _, m := range l.Members {
				found = found || m.Name == record.Member
			}
			if !found {
				t.Fatalf("reference member not discoverable: %s", record.Member)
			}
			s, err = i.SampleZIPCSV(ctx, a, record.Member, 1000, nil)
			if err != nil {
				t.Fatal(err)
			}
			samples[record.Member] = s
		}
		if s.SHA256 != source.SHA || s.Archive == nil || s.Archive.MemberSHA256 != record.SHA || s.CSV == nil {
			t.Fatal("source/member provenance differs from frozen evidence")
		}
		rowIndex := -1
		for n, pos := range s.CSV.DataRecords {
			// Frozen csvRecord includes the header as record 1. The production
			// provenance explicitly numbers data records AFTER the header.
			if pos+1 == record.Record {
				rowIndex = n
				break
			}
		}
		if rowIndex < 0 {
			t.Fatalf("reference record %d missing", record.Record)
		}
		for field, want := range record.Values {
			got := s.Rows[rowIndex][field]
			if want == "" && got == nil {
				continue
			}
			if got != want {
				t.Fatalf("member %s record %d field %s: got %#v want %#v", record.Member, record.Record, field, got, want)
			}
		}
	}
	t.Logf("matched %d independent facility/accessibility source records across %d discovered CSV members; ZIP SHA256 %s", len(source.Records), len(samples), l.SHA256)
}
