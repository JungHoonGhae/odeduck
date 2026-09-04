package connectionledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const testHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validAssessment() Assessment {
	leftField := FieldEvidence{Key: "lawdCd", Selector: "lawdCd", Namespace: "법정동코드 10자리", DataType: "string", Grain: "행정구역", Count: 12, DistinctCount: 10, DuplicateCount: 2}
	rightField := FieldEvidence{Key: "lawdCd", Selector: "lawdCd", Namespace: "법정동코드 10자리", DataType: "string", Grain: "행정구역", Count: 8, DistinctCount: 8}
	return Assessment{
		Left:     DatasetEvidence{PK: "15000001", Delivery: "REST", Source: SourceEvidence{URL: "https://www.data.go.kr/data/15000001/openapi.do"}, Record: RecordEvidence{Kind: "api_profile", RequestHash: testHash, EvidenceHash: testHash}, Fields: []FieldEvidence{leftField}},
		Right:    DatasetEvidence{PK: "15000002", Delivery: "FILE", Source: SourceEvidence{URL: "https://www.data.go.kr/data/15000002/fileData.do"}, Record: RecordEvidence{Kind: "file_observation", Asset: "sample.csv", EvidenceHash: testHash}, Fields: []FieldEvidence{rightField}},
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

func TestRecordCanonicalizesWhitespaceAndOrder(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	first := validAssessment()
	first.EdgeKinds = []string{"temporal", "spatial"}
	first.ExpectedKeys = []string{"lawdCd", "lawdCd"}
	recorded, err := store.Record(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	variant := validAssessment()
	variant.Left.PK = " 15000001 "
	variant.Left.Delivery = " rest "
	variant.Left.Fields[0].Namespace = " 법정동코드 10자리 "
	variant.Left.Record.EvidenceHash = strings.ToUpper(testHash)
	variant.EdgeKinds = []string{" spatial ", "temporal", "spatial"}
	variant.ExpectedKeys = []string{" lawdCd "}
	again, err := store.Record(context.Background(), variant)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.ID != again.ID {
		t.Fatalf("canonical variants differ: %q %q", recorded.ID, again.ID)
	}
	records, err := store.List(context.Background(), Filter{PK: "15000001"})
	if err != nil || len(records) != 1 {
		t.Fatalf("canonical record not filterable: records=%+v err=%v", records, err)
	}
}

func TestWhitespaceCannotBypassSameDatasetGuard(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	a := validAssessment()
	a.Right.PK = " 15000001 "
	a.Right.Source.URL = "https://www.data.go.kr/data/15000001/fileData.do"
	if _, err := store.Record(context.Background(), a); err == nil || !strings.Contains(err.Error(), "서로 다른") {
		t.Fatalf("same-PK err=%v", err)
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
	next.Right.Source.URL = "https://www.data.go.kr/data/15000003/fileData.do"
	next.Supersedes = first.ID
	if _, err := store.Record(context.Background(), next); err == nil || !strings.Contains(err.Error(), "다른 데이터셋 pair") {
		t.Fatalf("supersede err=%v", err)
	}
}

func TestStructurallyVerifiedAndReversedSupersession(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	structural := validAssessment()
	structural.Status = StatusStructurallyVerified
	structural.Sample = nil
	structural.Left.Record = RecordEvidence{Kind: "official_spec"}
	structural.Right.Record = RecordEvidence{Kind: "official_spec"}
	first, err := store.Record(context.Background(), structural)
	if err != nil {
		t.Fatal(err)
	}

	verified := validAssessment()
	verified.Left, verified.Right = verified.Right, verified.Left
	verified.Sample.LeftDistinct, verified.Sample.RightDistinct = verified.Sample.RightDistinct, verified.Sample.LeftDistinct
	verified.Sample.LeftMaxRowsPerKey, verified.Sample.RightMaxRowsPerKey = verified.Sample.RightMaxRowsPerKey, verified.Sample.LeftMaxRowsPerKey
	verified.Supersedes = first.ID
	second, err := store.Record(context.Background(), verified)
	if err != nil {
		t.Fatal(err)
	}
	if second.Supersedes != first.ID {
		t.Fatalf("supersedes = %q, want %q", second.Supersedes, first.ID)
	}
	records, err := store.List(context.Background(), Filter{})
	if err != nil || len(records) != 2 || records[0].ID != second.ID {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}

func TestAssessmentValidationRejectsUnsafeOrUnprovenEvidence(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Assessment)
		want string
	}{
		{"same dataset", func(a *Assessment) {
			a.Right.PK = a.Left.PK
			a.Right.Source.URL = "https://www.data.go.kr/data/15000001/fileData.do"
		}, "서로 다른"},
		{"generic relation", func(a *Assessment) { a.Relation = "RELATED_TO" }, "predicate"},
		{"empty edge kinds", func(a *Assessment) { a.EdgeKinds = nil }, "edgeKinds"},
		{"unknown edge kind", func(a *Assessment) { a.EdgeKinds = []string{"semantic"} }, "edge kind"},
		{"proxy without transform", func(a *Assessment) { a.EdgeKinds = []string{"proxy"} }, "transform"},
		{"empty expected keys", func(a *Assessment) { a.ExpectedKeys = nil }, "expectedKeys"},
		{"missing incremental value", func(a *Assessment) { a.IncrementalValue = " " }, "incrementalValue"},
		{"credential in reason", func(a *Assessment) { a.Reason = "authorization: very-secret-value" }, "credential"},
		{"Korean credential marker", func(a *Assessment) { a.Reason = "인증키 abcdefghijklmnop" }, "credential"},
		{"credential in expected keys", func(a *Assessment) {
			a.Status, a.MatchMethod, a.Sample = StatusBlocked, "unresolved", nil
			a.Left.Fields, a.Right.Fields = nil, nil
			a.Left.Record, a.Right.Record = RecordEvidence{Kind: "official_spec"}, RecordEvidence{Kind: "official_spec"}
			a.ExpectedKeys = []string{"serviceKey abcdefghijklmnop"}
		}, "credential"},
		{"unknown match method", func(a *Assessment) { a.MatchMethod = "fuzzy" }, "matchMethod"},
		{"verified probabilistic match", func(a *Assessment) { a.MatchMethod = "probabilistic" }, "verified"},
		{"verified without fields", func(a *Assessment) { a.Left.Fields = nil }, "field evidence"},
		{"incomplete field", func(a *Assessment) { a.Left.Fields[0].Namespace = "" }, "selector"},
		{"duplicate expected key field", func(a *Assessment) { a.Left.Fields = append(a.Left.Fields, a.Left.Fields[0]) }, "중복"},
		{"missing sample", func(a *Assessment) { a.Sample = nil }, "overlapDistinct"},
		{"left missing expected key", func(a *Assessment) { a.Left.Fields[0].Key = "other" }, "expectedKeys"},
		{"right missing expected key", func(a *Assessment) { a.Right.Fields[0].Key = "other" }, "expectedKeys"},
		{"distinct exceeds count", func(a *Assessment) { a.Left.Fields[0].DistinctCount = 13 }, "count/distinct/duplicate"},
		{"duplicate count mismatch", func(a *Assessment) { a.Left.Fields[0].DuplicateCount = 1 }, "count/distinct/duplicate"},
		{"sample distinct exceeds rows", func(a *Assessment) { a.Sample.LeftDistinct = 13 }, "관측 row 수"},
		{"single key distinct mismatch", func(a *Assessment) { a.Sample.LeftDistinct = 9 }, "field distinctCount"},
		{"composite distinct exceeds component product", func(a *Assessment) {
			a.ExpectedKeys = []string{"lawdCd", "month"}
			a.Left.Fields = append(a.Left.Fields, FieldEvidence{Key: "month", Selector: "month", Namespace: "YYYYMM", DataType: "string", Grain: "기준월", Count: 12, DistinctCount: 1, DuplicateCount: 11})
			a.Right.Fields = append(a.Right.Fields, FieldEvidence{Key: "month", Selector: "month", Namespace: "YYYYMM", DataType: "string", Grain: "기준월", Count: 8, DistinctCount: 1, DuplicateCount: 7})
			a.Sample.LeftDistinct = 11
		}, "관측 row 수"},
		{"overlap exceeds distinct", func(a *Assessment) { a.Sample.OverlapDistinct = 11 }, "넘을 수"},
		{"joined rows below overlap", func(a *Assessment) { a.Sample.JoinedRows = 6 }, "overlapDistinct보다 작을"},
		{"joined rows exceeds expansion bound", func(a *Assessment) { a.Sample.JoinedRows = 15 }, "허용하는 범위"},
		{"negative sample aggregate", func(a *Assessment) { a.Sample.LeftMaxRowsPerKey = -1 }, "음수"},
		{"zero expansion", func(a *Assessment) { a.Sample.LeftMaxRowsPerKey = 0 }, "max rows"},
		{"wrong file evidence kind", func(a *Assessment) { a.Right.Record.Kind = "api_profile" }, "REST/LINK"},
		{"file observation without asset", func(a *Assessment) { a.Right.Record.Asset = "" }, "asset"},
		{"observational kind without hash", func(a *Assessment) { a.Left.Record.EvidenceHash = "" }, "evidenceHash"},
		{"non https source", func(a *Assessment) { a.Left.Source.URL = "http://www.data.go.kr/data/15000001/openapi.do" }, "HTTPS"},
		{"REST with file page", func(a *Assessment) { a.Left.Source.URL = "https://www.data.go.kr/data/15000001/fileData.do" }, "delivery"},
		{"FILE with API page", func(a *Assessment) { a.Right.Source.URL = "https://www.data.go.kr/data/15000002/openapi.do" }, "delivery"},
		{"query forbidden", func(a *Assessment) { a.Left.Source.URL += "?serviceKey=secret" }, "query나 fragment"},
		{"unknown record kind", func(a *Assessment) { a.Left.Record.Kind = "raw_rows" }, "record.kind"},
		{"invalid hash", func(a *Assessment) { a.Left.Record.EvidenceHash = "not-a-hash" }, "SHA-256"},
		{"future observation", func(a *Assessment) { a.ObservedAt = time.Now().Add(10 * time.Minute).Format(time.RFC3339) }, "미래"},
		{"reversed validity", func(a *Assessment) { a.ValidFrom, a.ValidTo = "2026-09-05T00:00:00Z", "2026-09-04T00:00:00Z" }, "validTo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := validAssessment()
			tt.edit(&a)
			_, err := New(filepath.Join(t.TempDir(), "evidence.jsonl")).Record(context.Background(), a)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSourceURLRejectsCredentialBearingVariants(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "plain HTTP", url: "http://www.data.go.kr/data/15000001/openapi.do", want: "HTTPS"},
		{name: "untrusted host", url: "https://attacker.example/data/15000001/openapi.do", want: "공식 상세페이지"},
		{name: "wrong dataset PK", url: "https://www.data.go.kr/data/15099999/openapi.do", want: "해당 PK"},
		{name: "credential in path", url: "https://www.data.go.kr/serviceKey/secret", want: "해당 PK"},
		{name: "userinfo username", url: "https://serviceKey@www.data.go.kr/data/15000001/openapi.do", want: "credential"},
		{name: "userinfo password", url: "https://user:secret@www.data.go.kr/data/15000001/openapi.do", want: "credential"},
		{name: "mixed case service key", url: "https://www.data.go.kr/data/15000001/openapi.do?SeRvIcEKeY=secret", want: "query나 fragment"},
		{name: "mixed case authorization", url: "https://www.data.go.kr/data/15000001/openapi.do?AuThOrIzAtIoN=secret", want: "query나 fragment"},
		{name: "encoded token key", url: "https://www.data.go.kr/data/15000001/openapi.do?access%54oken=secret", want: "query나 fragment"},
		{name: "benign query key", url: "https://www.data.go.kr/data/15000001/openapi.do?view=secret", want: "query나 fragment"},
		{name: "fragment", url: "https://www.data.go.kr/data/15000001/openapi.do#serviceKey=secret", want: "query나 fragment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := validAssessment()
			a.Left.Source.URL = tt.url
			_, err := New(filepath.Join(t.TempDir(), "evidence.jsonl")).Record(context.Background(), a)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestAssessmentValidationAcceptsSupportedStates(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Assessment)
	}{
		{name: "sample verified REST and FILE", edit: func(*Assessment) {}},
		{name: "sample verified REST and LINK profiles", edit: func(a *Assessment) {
			a.Right.Delivery = "LINK"
			a.Right.Source.URL = "https://www.data.go.kr/data/15000002/openapi.do"
			a.Right.Record.Kind = "api_profile"
			a.Right.Record.RequestHash = testHash
		}},
		{name: "structurally verified official specs", edit: func(a *Assessment) {
			a.Status = StatusStructurallyVerified
			a.MatchMethod = "exact"
			a.Sample = nil
			a.Left.Record = RecordEvidence{Kind: "official_spec"}
			a.Right.Record = RecordEvidence{Kind: "official_spec"}
		}},
		{name: "blocked probabilistic assessment", edit: func(a *Assessment) {
			a.Status = StatusBlocked
			a.MatchMethod = "probabilistic"
			a.Sample = nil
			a.Left.Fields, a.Right.Fields = nil, nil
			a.Left.Record = RecordEvidence{Kind: "official_spec"}
			a.Right.Record = RecordEvidence{Kind: "official_spec"}
			a.Reason = "승인 전이라 표본 검증을 진행할 수 없음"
		}},
		{name: "rejected unresolved assessment", edit: func(a *Assessment) {
			a.Status = StatusRejected
			a.MatchMethod = "unresolved"
			a.Sample = nil
			a.Reason = "기관별 식별자 체계가 달라 연결을 기각"
		}},
		{name: "bounded real world validity", edit: func(a *Assessment) {
			a.ValidFrom = "2026-01-01T00:00:00Z"
			a.ValidTo = "2026-12-31T23:59:59Z"
		}},
		{name: "proxy with explicit crosswalk", edit: func(a *Assessment) {
			a.EdgeKinds = []string{"proxy"}
			a.Transform = "법정동코드 앞 5자리를 행정구역 코드로 변환"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := validAssessment()
			tt.edit(&a)
			if _, err := New(filepath.Join(t.TempDir(), "evidence.jsonl")).Record(context.Background(), a); err != nil {
				t.Fatalf("Record() error = %v", err)
			}
		})
	}
}

func TestBlockedAndRejectedRequireReasonButNotJoinProof(t *testing.T) {
	for _, status := range []string{StatusBlocked, StatusRejected} {
		t.Run(status, func(t *testing.T) {
			store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
			a := validAssessment()
			a.Status, a.MatchMethod, a.Sample = status, "unresolved", nil
			a.Left.Fields, a.Right.Fields = nil, nil
			a.Left.Record, a.Right.Record = RecordEvidence{Kind: "official_spec"}, RecordEvidence{Kind: "official_spec"}
			a.Reason = ""
			if _, err := store.Record(context.Background(), a); err == nil || !strings.Contains(err.Error(), "reason") {
				t.Fatalf("missing reason err=%v", err)
			}
			a.Reason = "승인 대기 또는 식별자 불일치"
			if _, err := store.Record(context.Background(), a); err != nil {
				t.Fatalf("record %s: %v", status, err)
			}
		})
	}
}

func TestListRejectsInvalidFilterAndCorruptLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	store := New(path)
	if _, err := store.List(context.Background(), Filter{PK: "not-a-pk"}); err == nil {
		t.Fatal("invalid PK filter should fail")
	}
	if _, err := store.List(context.Background(), Filter{Status: "candidate"}); err == nil {
		t.Fatal("invalid status filter should fail")
	}
	if err := os.WriteFile(path, []byte("{broken json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background(), Filter{}); err == nil || !strings.Contains(err.Error(), "손상") {
		t.Fatalf("corrupt ledger err=%v", err)
	}
}

func TestListDeduplicatesIDsAndRecordCreatesPrivateFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "evidence.jsonl")
	store := New(path)
	record, err := store.Record(context.Background(), validAssessment())
	if err != nil {
		t.Fatal(err)
	}
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	records, err := store.List(context.Background(), Filter{})
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("directory mode=%v", dirInfo.Mode().Perm())
	}
}

func TestWaitingForLedgerLockHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	if _, err := New(path).Record(context.Background(), validAssessment()); err != nil {
		t.Fatal(err)
	}
	release, err := acquireFileLock(context.Background(), path+".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = New(path).List(ctx, Filter{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestCanceledContextDoesNotAcquireFreeLedgerLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "evidence.jsonl.lock")
	if _, err := acquireFileLock(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestWaitingForLedgerLockHonorsCancellationAfterWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	if _, err := New(path).Record(context.Background(), validAssessment()); err != nil {
		t.Fatal(err)
	}
	release, err := acquireFileLock(context.Background(), path+".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 125*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = New(path).List(ctx, Filter{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed < 75*time.Millisecond {
		t.Fatalf("lock wait returned too early after %s", elapsed)
	}
}

func TestRecordPreservesIncompleteFinalFrameAndFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	store := New(path)
	first, err := store.Record(context.Background(), validAssessment())
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"id":"truncated`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next := validAssessment()
	next.Reason = "불완전 tail 뒤 기록 시도"
	if _, err := store.Record(context.Background(), next); err == nil || !strings.Contains(err.Error(), "자동 복구하지 않았습니다") {
		t.Fatalf("err=%v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || first.ID == "" {
		t.Fatal("incomplete tail was modified")
	}
}

func TestRecordPreservesCompleteFinalFrameWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	store := New(path)
	first, err := store.Record(context.Background(), validAssessment())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data[:len(data)-1], 0o600); err != nil {
		t.Fatal(err)
	}
	next := validAssessment()
	next.Reason = "newline 복구 뒤 새 표본"
	second, err := store.Record(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != second.ID || records[1].ID != first.ID {
		t.Fatalf("records=%+v", records)
	}
}

func TestRecordDoesNotSilentlyDeleteAmbiguouslyCorruptTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	store := New(path)
	if _, err := store.Record(context.Background(), validAssessment()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[1] = '!'
	data = data[:len(data)-1]
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), data...)
	next := validAssessment()
	next.Reason = "손상 뒤 기록 시도"
	if _, err := store.Record(context.Background(), next); err == nil || !strings.Contains(err.Error(), "자동 복구하지 않았습니다") {
		t.Fatalf("err=%v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("ambiguous corruption was modified")
	}
}

func TestDefaultListDoesNotCreateConfigFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	store, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List(context.Background(), Filter{})
	if err != nil || len(records) != 0 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	if _, err := os.Stat(filepath.Dir(store.path)); !os.IsNotExist(err) {
		t.Fatalf("read-only list created config dir or unexpected stat error: %v", err)
	}
}

func TestConcurrentRecordAppendsPreserveEveryAssessment(t *testing.T) {
	const count = 24
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := validAssessment()
			a.Reason = fmt.Sprintf("동시 검증 표본 %02d", i)
			if _, err := store.Record(context.Background(), a); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Record() error = %v", err)
	}
	if t.Failed() {
		return
	}
	records, err := store.List(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != count {
		t.Fatalf("records=%d, want %d", len(records), count)
	}
	seen := make(map[string]bool, count)
	for _, record := range records {
		if seen[record.ID] {
			t.Fatalf("duplicate record ID %q", record.ID)
		}
		seen[record.ID] = true
	}
}

func TestNormalizationMakesEquivalentAssessmentsIdempotent(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "evidence.jsonl"))
	first := validAssessment()
	first.EdgeKinds = []string{"temporal", "spatial"}
	first.ExpectedKeys = []string{"lawdCd", "lawdCd"}
	one, err := store.Record(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := validAssessment()
	second.EdgeKinds = []string{"spatial", "temporal"}
	second.ExpectedKeys = []string{"lawdCd"}
	second.Relation = "  " + second.Relation + "  "
	two, err := store.Record(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID != two.ID {
		t.Fatalf("normalized IDs differ: %s != %s", one.ID, two.ID)
	}
}
