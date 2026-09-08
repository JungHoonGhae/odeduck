package dataset

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/JungHoonGhae/odeduck/internal/fetch"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

func sampleZIPFixture(t *testing.T, members [][2]string) (*Inspector, Asset, []byte) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, m := range members {
		w, err := z.CreateHeader(&zip.FileHeader{Name: m[0], Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(m[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	body := b.Bytes()
	u := "https://data.test/source.zip"
	return NewInspector(fixtureTransport{gets: map[string]*fetch.Response{u: {Status: 200, Body: body}}}, ""), Asset{Name: "source.zip", Format: "ZIP", Request: Request{Method: http.MethodGet, URL: u}}, body
}

func TestZIPCSVExactMemberSelectionPreservesRecordsAndBothHashes(t *testing.T) {
	content := "id,name\n999,other\n001,\" first\nsecond \"\n001,same\n"
	i, a, body := sampleZIPFixture(t, [][2]string{{"folder/source.csv", content}, {"other/source.csv", "id,name\n002,private\n"}})
	l, err := i.LayoutFile(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	if l.Format != "ZIP" || len(l.Members) != 2 || l.Members[0].Name != "folder/source.csv" {
		t.Fatalf("archive discovery missing: %+v", l)
	}
	s, err := i.SampleZIPCSV(context.Background(), a, l.Members[0].Name, 1, map[string]string{"id": "001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 1 || s.Rows[0]["id"] != "001" || s.Rows[0]["name"] != " first\nsecond " || !s.Prefix {
		t.Fatalf("values or selection changed: %+v", s)
	}
	if s.Archive == nil || s.Archive.Member != "folder/source.csv" || s.Archive.MemberSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(content))) || s.SHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) || s.SHA256 != l.SHA256 {
		t.Fatal("archive/member provenance missing")
	}
	if s.CSV == nil || !reflect.DeepEqual(s.CSV.DataRecords, []int{2}) || !reflect.DeepEqual(s.CSV.StartLines, []int{3}) || s.Selection.ScannedRows != 3 || s.Selection.MatchedRows != 2 || s.Selection.Exhausted {
		t.Fatalf("source record position lost: %+v %+v", s.CSV, s.Selection)
	}
	if _, err := i.SampleZIPCSV(context.Background(), a, "source.csv", 1, nil); err == nil {
		t.Fatal("basename fallback accepted")
	}
}

func TestZIPCSVRejectsAmbiguousUnsafeAndNestedMembers(t *testing.T) {
	for _, members := range [][][2]string{
		{{"a.csv", "id\n1\n"}, {"a.csv", "id\n2\n"}},
		{{"a.csv", "id\n1\n"}, {"../outside.csv", "id\n2\n"}},
		{{"a.csv", "id\n1\n"}, {"C:/outside.csv", "id\n2\n"}},
		{{"a.csv", "id\n1\n"}, {"dir\\outside.csv", "id\n2\n"}},
	} {
		i, a, _ := sampleZIPFixture(t, members)
		if _, err := i.LayoutFile(context.Background(), a, ""); err == nil {
			t.Fatal("unsafe archive accepted")
		}
		if _, err := i.SampleZIPCSV(context.Background(), a, "a.csv", 1, nil); err == nil {
			t.Fatal("unsafe archive sampled")
		}
	}
	i, a, _ := sampleZIPFixture(t, [][2]string{{"nested.zip", "id\n1\n"}})
	if _, err := i.SampleZIPCSV(context.Background(), a, "nested.zip", 1, nil); err == nil {
		t.Fatal("nested archive treated as CSV")
	}
}

func TestZIPCSVChecksWholeMemberCRCBeforeReturningPrefix(t *testing.T) {
	i, a, body := sampleZIPFixture(t, [][2]string{{"a.csv", "id\n1\n2\n3\n"}})
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := z.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	body[int(offset)+8] = '4'
	if _, err := i.SampleZIPCSV(context.Background(), a, "a.csv", 1, nil); err == nil {
		t.Fatal("corrupt tail accepted because prefix parsed")
	}
}

func TestZIPCSVLosslessKoreanMemberNamesAndDecodedCollisions(t *testing.T) {
	name := "자료/시설.csv"
	legacy, _, err := transform.String(korean.EUCKR.NewEncoder(), name)
	if err != nil {
		t.Fatal(err)
	}
	i, a, _ := sampleZIPFixture(t, [][2]string{{legacy, "id\n001\n"}})
	l, err := i.LayoutFile(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Members) != 1 || l.Members[0].Name != name {
		t.Fatal("legacy name not discoverable")
	}
	s, err := i.SampleZIPCSV(context.Background(), a, name, 1, nil)
	if err != nil || s.Rows[0]["id"] != "001" {
		t.Fatalf("decoded exact name did not select member: %+v %v", s, err)
	}
	i, a, _ = sampleZIPFixture(t, [][2]string{{legacy, "id\n001\n"}, {name, "id\n002\n"}})
	if _, err := i.LayoutFile(context.Background(), a, ""); err == nil {
		t.Fatal("two raw names decoded to one selector")
	}
}

func TestZIPCSVRejectsLossyTextDecoding(t *testing.T) {
	i, a, _ := sampleZIPFixture(t, [][2]string{{"data.csv", "id,name\n001,\xff\n"}})
	if _, err := i.SampleZIPCSV(context.Background(), a, "data.csv", 1, nil); err == nil {
		t.Fatal("invalid encoding silently replaced source bytes")
	}
}
