package connectionledger

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

const testHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validAssessment() Assessment {
	field := FieldEvidence{Selector: "lawdCd", Namespace: "법정동코드 10자리", DataType: "string", Grain: "행정구역"}
	return Assessment{
		Left:     DatasetEvidence{PK: "15000001", Delivery: "REST", Source: SourceEvidence{URL: "https://www.data.go.kr/data/15000001/openapi.do"}, Record: RecordEvidence{Kind: "api_profile", EvidenceHash: testHash}, Fields: []FieldEvidence{field}},
		Right:    DatasetEvidence{PK: "15000002", Delivery: "FILE", Source: SourceEvidence{URL: "https://www.data.go.kr/data/15000002/fileData.do"}, Record: RecordEvidence{Kind: "file_observation", EvidenceHash: testHash}, Fields: []FieldEvidence{field}},
		Relation: "SHARES_LEGAL_DISTRICT", EdgeKinds: []string{"spatial"}, ExpectedKeys: []string{"lawdCd"},
		MatchMethod: "deterministic", Status: StatusSampleVerified, IncrementalValue: "수요와 공급을 같은 행정구역에서 비교", Reason: "공통 표본 확인",
		ObservedAt: "2026-09-04T00:00:00Z", Sample: &SampleEvidence{LeftDistinct: 10, RightDistinct: 8, OverlapDistinct: 7, JoinedRows: 12, LeftMaxRowsPerKey: 2, RightMaxRowsPerKey: 1},
	}
}

func TestRecordIsIdempotentAndFilterable(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	first, err := store.Record(context.Background(), validAssessment())
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Record(context.Background(), validAssessment())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("ids differ: %q %q", first.ID, second.ID)
	}
	records, err := store.List(context.Background(), Filter{PK: "15000002", Status: StatusSampleVerified})
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}

func TestCandidateAndUnprovenSampleCannotBeRecorded(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	a := validAssessment()
	a.Status = "candidate"
	if _, err := store.Record(context.Background(), a); err == nil || !strings.Contains(err.Error(), "candidate") {
		t.Fatalf("candidate err=%v", err)
	}
	a = validAssessment()
	a.Left.Record.EvidenceHash = ""
	if _, err := store.Record(context.Background(), a); err == nil || !strings.Contains(err.Error(), "evidenceHash") {
		t.Fatalf("hash err=%v", err)
	}
}

func TestSupersedesMustReferToSamePair(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	first, err := store.Record(context.Background(), validAssessment())
	if err != nil {
		t.Fatal(err)
	}
	next := validAssessment()
	next.Status = StatusRejected
	next.MatchMethod = "unresolved"
	next.Reason = "기관 코드 체계가 달라 후속 표본에서 기각"
	next.Sample = nil
	next.Right.PK = "15000003"
	next.Supersedes = first.ID
	if _, err := store.Record(context.Background(), next); err == nil || !strings.Contains(err.Error(), "다른 데이터셋 pair") {
		t.Fatalf("supersede err=%v", err)
	}
}
