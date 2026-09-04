// Package connectionledger persists evidence-backed assessments of proposed
// cross-domain dataset connections. It stores provenance and aggregate evidence,
// never API response rows or credentials.
package connectionledger

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/portal"
)

const (
	StatusStructurallyVerified = "structurally_verified"
	StatusSampleVerified       = "sample_verified"
	StatusBlocked              = "blocked"
	StatusRejected             = "rejected"
	ledgerFile                 = "connection-evidence.jsonl"
	maxRecords                 = 10000
	maxEvidenceCount           = 10_000_000
	maxSampleAggregate         = 1_000_000_000
)

var relationPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
var credentialMaterialPattern = regexp.MustCompile(`(?i)(service[_-]?key|api[_-]?key|access[_-]?token|authorization|secret|bearer|인증키|일반인증키|인코딩키|디코딩키)\s*[:=]?\s*[a-z0-9%._~+/=-]{12,}`)

type FieldEvidence struct {
	Key            string `json:"key" jsonschema:"expectedKeys member represented by this field"`
	Selector       string `json:"selector" jsonschema:"exact field name or dotted response path"`
	Namespace      string `json:"namespace" jsonschema:"identifier namespace and version, for example 법정동코드 10자리"`
	DataType       string `json:"dataType" jsonschema:"observed or officially specified field type"`
	Grain          string `json:"grain" jsonschema:"what one value or record represents"`
	Count          int    `json:"count,omitempty"`
	NullCount      int    `json:"nullCount,omitempty"`
	DistinctCount  int    `json:"distinctCount,omitempty"`
	DuplicateCount int    `json:"duplicateCount,omitempty"`
}

type SourceEvidence struct {
	URL string `json:"url" jsonschema:"official specification or dataset URL; credentials and raw API query parameters are forbidden"`
}

type RecordEvidence struct {
	Kind         string `json:"kind" jsonschema:"official_spec, api_profile, or file_observation"`
	Operation    string `json:"operation,omitempty" jsonschema:"inspected operation name for API evidence"`
	RequestHash  string `json:"requestHash,omitempty" jsonschema:"SHA-256 of the non-secret call parameters for API profile evidence"`
	Asset        string `json:"asset,omitempty" jsonschema:"observed asset name for FILE evidence"`
	EvidenceHash string `json:"evidenceHash,omitempty" jsonschema:"SHA-256 from call profile or observed file; raw rows are not stored"`
}

type DatasetEvidence struct {
	PK       string          `json:"pk" jsonschema:"data.go.kr publicDataPk inspected before recording"`
	Delivery string          `json:"delivery" jsonschema:"REST, LINK, or FILE"`
	Source   SourceEvidence  `json:"source"`
	Record   RecordEvidence  `json:"record"`
	Fields   []FieldEvidence `json:"fields,omitempty" jsonschema:"fields used by the proposed connection"`
}

type SampleEvidence struct {
	LeftDistinct       int `json:"leftDistinct"`
	RightDistinct      int `json:"rightDistinct"`
	OverlapDistinct    int `json:"overlapDistinct"`
	JoinedRows         int `json:"joinedRows"`
	LeftMaxRowsPerKey  int `json:"leftMaxRowsPerKey"`
	RightMaxRowsPerKey int `json:"rightMaxRowsPerKey"`
}

type Assessment struct {
	Left             DatasetEvidence `json:"left"`
	Right            DatasetEvidence `json:"right"`
	Relation         string          `json:"relation" jsonschema:"specific uppercase predicate; generic RELATED_TO or HAS is rejected"`
	EdgeKinds        []string        `json:"edgeKinds" jsonschema:"one or more of entity, spatial, temporal, proxy"`
	ExpectedKeys     []string        `json:"expectedKeys" jsonschema:"connection keys verified on both sides"`
	Transform        string          `json:"transform,omitempty" jsonschema:"required for proxy edges; describe normalization or crosswalk"`
	MatchMethod      string          `json:"matchMethod" jsonschema:"exact, deterministic, probabilistic, or unresolved"`
	Status           string          `json:"status" jsonschema:"structurally_verified, sample_verified, blocked, or rejected; candidate is not recordable"`
	IncrementalValue string          `json:"incrementalValue" jsonschema:"new decision enabled only by combining both datasets"`
	Reason           string          `json:"reason" jsonschema:"evidence-based conclusion; required for blocked and rejected"`
	ObservedAt       string          `json:"observedAt" jsonschema:"RFC3339 time when the evidence was observed"`
	ValidFrom        string          `json:"validFrom,omitempty" jsonschema:"optional RFC3339 start of real-world validity"`
	ValidTo          string          `json:"validTo,omitempty" jsonschema:"optional RFC3339 end of real-world validity"`
	Supersedes       string          `json:"supersedes,omitempty" jsonschema:"record ID replaced by this assessment"`
	Sample           *SampleEvidence `json:"sample,omitempty"`
}

type Record struct {
	ID         string `json:"id"`
	RecordedAt string `json:"recordedAt"`
	Assessment
}

type Filter struct {
	PK     string
	Status string
}

// Store is one local, append-only evidence ledger. The lock serializes CLI and
// MCP processes; JSONL keeps previous records intact when a later write fails.
type Store struct{ path string }

func New(path string) *Store { return &Store{path: path} }

func Default() (*Store, error) {
	dir, err := portal.ConfigDirPath()
	if err != nil {
		return nil, err
	}
	return New(filepath.Join(dir, ledgerFile)), nil
}

func (s *Store) Record(ctx context.Context, assessment Assessment) (Record, error) {
	assessment = normalize(assessment)
	if err := validate(assessment); err != nil {
		return Record{}, err
	}
	release, err := acquireFileLock(ctx, s.path+".lock")
	if err != nil {
		return Record{}, err
	}
	defer release()
	if err := repairIncompleteTail(s.path); err != nil {
		return Record{}, err
	}

	records, err := s.listUnlocked(Filter{})
	if err != nil {
		return Record{}, err
	}
	if assessment.Supersedes != "" {
		var prior *Record
		for i := range records {
			if records[i].ID == assessment.Supersedes {
				prior = &records[i]
				break
			}
		}
		if prior == nil {
			return Record{}, fmt.Errorf("supersedes 대상 %q을 장부에서 찾을 수 없습니다", assessment.Supersedes)
		}
		if !samePair(prior.Assessment, assessment) {
			return Record{}, errors.New("다른 데이터셋 pair의 기록을 supersede할 수 없습니다")
		}
	}

	canonical, _ := json.Marshal(assessment)
	sum := sha256.Sum256(append([]byte("odeduck-connection-evidence-v1\x00"), canonical...))
	id := "ce_" + hex.EncodeToString(sum[:12])
	for _, existing := range records {
		if existing.ID == id {
			return existing, nil
		}
	}
	if len(records) >= maxRecords {
		return Record{}, fmt.Errorf("연결 근거 장부는 최대 %d건입니다", maxRecords)
	}
	record := Record{ID: id, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), Assessment: assessment}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return Record{}, err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Record{}, fmt.Errorf("연결 근거 장부 열기 실패: %w", err)
	}
	before, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		_ = f.Close()
		return Record{}, fmt.Errorf("연결 근거 장부 위치 확인 실패: %w", err)
	}
	line, _ := json.Marshal(record)
	line = append(line, '\n')
	_, writeErr := f.Write(line)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Truncate(s.path, before)
		return Record{}, fmt.Errorf("연결 근거 기록 실패: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Truncate(s.path, before)
		return Record{}, closeErr
	}
	return record, nil
}

func (s *Store) List(ctx context.Context, filter Filter) ([]Record, error) {
	filter.PK = strings.TrimSpace(filter.PK)
	filter.Status = strings.TrimSpace(filter.Status)
	if filter.PK != "" {
		if err := portal.ValidatePublicDataPK(filter.PK); err != nil {
			return nil, err
		}
	}
	if filter.Status != "" && !validStatus(filter.Status) {
		return nil, errors.New("알 수 없는 connection assessment status입니다")
	}
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return []Record{}, nil
	} else if err != nil {
		return nil, err
	}
	// The empty-ledger return above keeps a read-only query side-effect free.
	// Once a ledger exists, create/acquire the sidecar so a legacy/manual file
	// cannot race a writer that is joining the locking protocol for the first time.
	release, err := acquireFileLock(ctx, s.path+".lock")
	if err != nil {
		return nil, err
	}
	defer release()
	return s.listUnlocked(filter)
}

func (s *Store) listUnlocked(filter Filter) ([]Record, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("연결 근거 장부 읽기 실패: %w", err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	seen := map[string]bool{}
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("연결 근거 장부 손상: %w", err)
		}
		if record.ID == "" || seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		if filter.PK != "" && record.Left.PK != filter.PK && record.Right.PK != filter.PK {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		records = append(records, record)
		if len(records) > maxRecords {
			return nil, fmt.Errorf("연결 근거 장부가 최대 %d건을 초과했습니다", maxRecords)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].RecordedAt > records[j].RecordedAt })
	return records, nil
}

func validate(a Assessment) error {
	if err := validateDataset("left", a.Left); err != nil {
		return err
	}
	if err := validateDataset("right", a.Right); err != nil {
		return err
	}
	if containsCredentialMaterial(persistedStrings(a)...) {
		return errors.New("connection assessment에는 credential 값을 넣을 수 없습니다")
	}
	if a.Left.PK == a.Right.PK {
		return errors.New("서로 다른 두 데이터셋 PK가 필요합니다")
	}
	if !relationPattern.MatchString(a.Relation) || a.Relation == "RELATED_TO" || a.Relation == "HAS" || a.Relation == "CONNECTED_TO" {
		return errors.New("relation은 RELATED_TO/HAS가 아닌 구체적인 대문자 predicate여야 합니다")
	}
	if len(a.EdgeKinds) == 0 || len(a.EdgeKinds) > 4 {
		return errors.New("edgeKinds는 1~4개가 필요합니다")
	}
	allowedKinds := map[string]bool{"entity": true, "spatial": true, "temporal": true, "proxy": true}
	for _, kind := range a.EdgeKinds {
		if !allowedKinds[kind] {
			return fmt.Errorf("지원하지 않는 edge kind %q", kind)
		}
		if kind == "proxy" && strings.TrimSpace(a.Transform) == "" {
			return errors.New("proxy edge에는 transform이 필요합니다")
		}
	}
	if len(a.ExpectedKeys) == 0 || len(a.ExpectedKeys) > 8 {
		return errors.New("expectedKeys는 1~8개가 필요합니다")
	}
	for _, key := range a.ExpectedKeys {
		if strings.TrimSpace(key) == "" || len([]rune(key)) > 80 {
			return errors.New("expectedKeys의 각 값은 1~80자여야 합니다")
		}
	}
	if strings.TrimSpace(a.IncrementalValue) == "" {
		return errors.New("incrementalValue가 필요합니다")
	}
	if len([]rune(a.IncrementalValue)) > 1000 || len([]rune(a.Reason)) > 2000 || len([]rune(a.Transform)) > 1000 {
		return errors.New("incrementalValue/reason/transform이 너무 깁니다")
	}
	if containsCredentialMaterial(a.IncrementalValue, a.Reason, a.Transform) {
		return errors.New("incrementalValue/reason/transform에는 credential 값을 넣을 수 없습니다")
	}
	if a.MatchMethod != "exact" && a.MatchMethod != "deterministic" && a.MatchMethod != "probabilistic" && a.MatchMethod != "unresolved" {
		return errors.New("matchMethod는 exact, deterministic, probabilistic, unresolved 중 하나여야 합니다")
	}
	if !validStatus(a.Status) {
		return errors.New("status는 structurally_verified, sample_verified, blocked, rejected 중 하나여야 하며 candidate는 기록할 수 없습니다")
	}
	observed, err := time.Parse(time.RFC3339, a.ObservedAt)
	if err != nil {
		return errors.New("observedAt은 RFC3339이어야 합니다")
	}
	if observed.After(time.Now().Add(5 * time.Minute)) {
		return errors.New("observedAt은 미래일 수 없습니다")
	}
	if err := validateValidity(a.ValidFrom, a.ValidTo); err != nil {
		return err
	}
	if a.Status == StatusBlocked || a.Status == StatusRejected {
		if strings.TrimSpace(a.Reason) == "" {
			return errors.New("blocked/rejected에는 reason이 필요합니다")
		}
		return nil
	}
	if a.MatchMethod != "exact" && a.MatchMethod != "deterministic" {
		return errors.New("verified 상태에는 exact 또는 deterministic matchMethod가 필요합니다")
	}
	if len(a.Left.Fields) == 0 || len(a.Right.Fields) == 0 {
		return errors.New("verified 상태에는 양쪽 field evidence가 필요합니다")
	}
	for _, side := range []DatasetEvidence{a.Left, a.Right} {
		for _, field := range side.Fields {
			if strings.TrimSpace(field.Key) == "" || strings.TrimSpace(field.Selector) == "" || strings.TrimSpace(field.Namespace) == "" || strings.TrimSpace(field.DataType) == "" || strings.TrimSpace(field.Grain) == "" {
				return errors.New("verified field에는 key, selector, namespace, dataType, grain이 모두 필요합니다")
			}
		}
	}
	if err := validateFieldMapping("left", a.ExpectedKeys, a.Left.Fields); err != nil {
		return err
	}
	if err := validateFieldMapping("right", a.ExpectedKeys, a.Right.Fields); err != nil {
		return err
	}
	if a.Status == StatusSampleVerified {
		if a.Sample == nil || a.Sample.OverlapDistinct <= 0 || a.Sample.JoinedRows <= 0 {
			return errors.New("sample_verified에는 양수 overlapDistinct와 joinedRows가 필요합니다")
		}
		if !validSHA256(a.Left.Record.EvidenceHash) || !validSHA256(a.Right.Record.EvidenceHash) {
			return errors.New("sample_verified에는 양쪽 SHA-256 evidenceHash가 필요합니다")
		}
		for _, side := range []DatasetEvidence{a.Left, a.Right} {
			wantKind := "api_profile"
			if side.Delivery == "FILE" {
				wantKind = "file_observation"
			}
			if side.Record.Kind != wantKind {
				return fmt.Errorf("sample_verified의 %s evidence에는 record.kind=%s가 필요합니다", side.Delivery, wantKind)
			}
		}
		if a.Sample.OverlapDistinct > a.Sample.LeftDistinct || a.Sample.OverlapDistinct > a.Sample.RightDistinct {
			return errors.New("overlapDistinct는 양쪽 distinct 수를 넘을 수 없습니다")
		}
		if a.Sample.LeftDistinct < 0 || a.Sample.RightDistinct < 0 || a.Sample.JoinedRows < 0 || a.Sample.LeftMaxRowsPerKey < 0 || a.Sample.RightMaxRowsPerKey < 0 {
			return errors.New("sample 집계값은 음수일 수 없습니다")
		}
		if a.Sample.LeftMaxRowsPerKey < 1 || a.Sample.RightMaxRowsPerKey < 1 {
			return errors.New("sample_verified에는 양쪽 max rows per key가 필요합니다")
		}
		if err := validateSampleConsistency(a); err != nil {
			return err
		}
	}
	return nil
}

func validateDataset(side string, d DatasetEvidence) error {
	if err := portal.ValidatePublicDataPK(strings.TrimSpace(d.PK)); err != nil {
		return fmt.Errorf("%s: %w", side, err)
	}
	delivery := strings.ToUpper(strings.TrimSpace(d.Delivery))
	if delivery != "REST" && delivery != "LINK" && delivery != "FILE" {
		return fmt.Errorf("%s delivery는 REST, LINK, FILE 중 하나여야 합니다", side)
	}
	if strings.TrimSpace(d.Source.URL) == "" {
		return fmt.Errorf("%s source.url이 필요합니다", side)
	}
	parsed, err := url.Parse(d.Source.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%s source.url은 credential 없는 공식 HTTPS URL이어야 합니다", side)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s source.url에는 query나 fragment를 넣을 수 없습니다", side)
	}
	wantPrefix := "/data/" + strings.TrimSpace(d.PK) + "/"
	if parsed.Port() != "" || (parsed.Hostname() != "www.data.go.kr" && parsed.Hostname() != "data.go.kr") || parsed.RawPath != "" || !strings.HasPrefix(parsed.Path, wantPrefix) {
		return fmt.Errorf("%s source.url은 해당 PK의 data.go.kr 공식 상세페이지여야 합니다", side)
	}
	page := strings.TrimPrefix(parsed.Path, wantPrefix)
	if page != "openapi.do" && page != "fileData.do" && page != "standard.do" {
		return fmt.Errorf("%s source.url은 지원하는 data.go.kr 상세페이지가 아닙니다", side)
	}
	if (delivery == "FILE" && page == "openapi.do") || (delivery != "FILE" && page != "openapi.do") {
		return fmt.Errorf("%s source.url 상세페이지 유형이 delivery=%s와 일치하지 않습니다", side, delivery)
	}
	if len([]rune(d.Source.URL)) > 2000 || len([]rune(d.Record.Operation)) > 200 || len([]rune(d.Record.Asset)) > 500 {
		return fmt.Errorf("%s source/record evidence가 너무 깁니다", side)
	}
	if d.Record.Kind != "official_spec" && d.Record.Kind != "api_profile" && d.Record.Kind != "file_observation" {
		return fmt.Errorf("%s record.kind는 official_spec, api_profile, file_observation 중 하나여야 합니다", side)
	}
	if len(d.Fields) > 8 {
		return fmt.Errorf("%s fields는 최대 8개입니다", side)
	}
	for _, field := range d.Fields {
		if len([]rune(field.Key)) > 80 || len([]rune(field.Selector)) > 80 || len([]rune(field.Namespace)) > 200 || len([]rune(field.DataType)) > 80 || len([]rune(field.Grain)) > 200 {
			return fmt.Errorf("%s field evidence가 너무 깁니다", side)
		}
		if field.Count < 0 || field.NullCount < 0 || field.DistinctCount < 0 || field.DuplicateCount < 0 {
			return fmt.Errorf("%s field 집계값은 음수일 수 없습니다", side)
		}
		if field.Count > maxEvidenceCount || field.NullCount > maxEvidenceCount || field.DistinctCount > maxEvidenceCount || field.DuplicateCount > maxEvidenceCount {
			return fmt.Errorf("%s field 집계값이 최대 %d를 넘습니다", side, maxEvidenceCount)
		}
	}
	if d.Record.EvidenceHash != "" && !validSHA256(d.Record.EvidenceHash) {
		return fmt.Errorf("%s evidenceHash는 SHA-256 hex여야 합니다", side)
	}
	if d.Record.Kind == "api_profile" && (delivery == "FILE" || !validSHA256(d.Record.EvidenceHash)) {
		return fmt.Errorf("%s api_profile은 REST/LINK delivery와 SHA-256 evidenceHash가 필요합니다", side)
	}
	if d.Record.Kind == "api_profile" && !validSHA256(d.Record.RequestHash) {
		return fmt.Errorf("%s api_profile에는 SHA-256 requestHash가 필요합니다", side)
	}
	if d.Record.Kind == "file_observation" && (delivery != "FILE" || !validSHA256(d.Record.EvidenceHash)) {
		return fmt.Errorf("%s file_observation은 FILE delivery와 SHA-256 evidenceHash가 필요합니다", side)
	}
	if d.Record.Kind == "file_observation" && strings.TrimSpace(d.Record.Asset) == "" {
		return fmt.Errorf("%s file_observation에는 관찰한 asset 이름이 필요합니다", side)
	}
	values := []string{d.Record.Operation, d.Record.RequestHash, d.Record.Asset}
	for _, field := range d.Fields {
		values = append(values, field.Key, field.Selector, field.Namespace, field.DataType, field.Grain)
	}
	if containsCredentialMaterial(values...) {
		return fmt.Errorf("%s record/field evidence에는 credential 값을 넣을 수 없습니다", side)
	}
	return nil
}

func validateValidity(from, to string) error {
	if from == "" && to == "" {
		return nil
	}
	var start, end time.Time
	var err error
	if from != "" {
		start, err = time.Parse(time.RFC3339, from)
		if err != nil {
			return errors.New("validFrom은 RFC3339이어야 합니다")
		}
	}
	if to != "" {
		end, err = time.Parse(time.RFC3339, to)
		if err != nil {
			return errors.New("validTo는 RFC3339이어야 합니다")
		}
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return errors.New("validTo는 validFrom보다 이를 수 없습니다")
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func containsCredentialMaterial(values ...string) bool {
	for _, value := range values {
		decoded, err := url.PathUnescape(value)
		if err == nil {
			value = decoded
		}
		if credentialMaterialPattern.MatchString(value) {
			return true
		}
	}
	return false
}

func persistedStrings(a Assessment) []string {
	values := []string{
		a.Left.Source.URL, a.Right.Source.URL,
		a.Relation, a.Transform, a.MatchMethod, a.Status,
		a.IncrementalValue, a.Reason, a.ObservedAt, a.ValidFrom, a.ValidTo, a.Supersedes,
		a.Left.Record.Kind, a.Left.Record.Operation, a.Left.Record.RequestHash, a.Left.Record.Asset, a.Left.Record.EvidenceHash,
		a.Right.Record.Kind, a.Right.Record.Operation, a.Right.Record.RequestHash, a.Right.Record.Asset, a.Right.Record.EvidenceHash,
	}
	values = append(values, a.EdgeKinds...)
	values = append(values, a.ExpectedKeys...)
	for _, dataset := range []DatasetEvidence{a.Left, a.Right} {
		values = append(values, dataset.PK, dataset.Delivery)
		for _, field := range dataset.Fields {
			values = append(values, field.Key, field.Selector, field.Namespace, field.DataType, field.Grain)
		}
	}
	return values
}

func validStatus(status string) bool {
	return status == StatusStructurallyVerified || status == StatusSampleVerified || status == StatusBlocked || status == StatusRejected
}

func samePair(a, b Assessment) bool {
	return (a.Left.PK == b.Left.PK && a.Right.PK == b.Right.PK) || (a.Left.PK == b.Right.PK && a.Right.PK == b.Left.PK)
}

func validateSampleConsistency(a Assessment) error {
	if a.Sample.LeftDistinct > maxSampleAggregate || a.Sample.RightDistinct > maxSampleAggregate || a.Sample.OverlapDistinct > maxSampleAggregate ||
		a.Sample.JoinedRows > maxSampleAggregate || a.Sample.LeftMaxRowsPerKey > maxSampleAggregate || a.Sample.RightMaxRowsPerKey > maxSampleAggregate {
		return fmt.Errorf("sample 집계값이 최대 %d를 넘습니다", maxSampleAggregate)
	}
	leftRows, err := validateSampleSide("left", a.Left.Fields, a.Sample.LeftDistinct, a.Sample.LeftMaxRowsPerKey)
	if err != nil {
		return err
	}
	rightRows, err := validateSampleSide("right", a.Right.Fields, a.Sample.RightDistinct, a.Sample.RightMaxRowsPerKey)
	if err != nil {
		return err
	}
	if a.Sample.JoinedRows < a.Sample.OverlapDistinct {
		return errors.New("joinedRows는 overlapDistinct보다 작을 수 없습니다")
	}
	maxJoined, ok := checkedProduct(int64(a.Sample.OverlapDistinct), int64(a.Sample.LeftMaxRowsPerKey), int64(a.Sample.RightMaxRowsPerKey))
	if !ok {
		return errors.New("sample join expansion 계산이 범위를 넘습니다")
	}
	if int64(a.Sample.JoinedRows) > maxJoined {
		return errors.New("joinedRows가 overlap과 좌우 max rows per key가 허용하는 범위를 넘습니다")
	}
	if a.Sample.LeftMaxRowsPerKey > leftRows || a.Sample.RightMaxRowsPerKey > rightRows {
		return errors.New("max rows per key는 관측 row 수를 넘을 수 없습니다")
	}
	if len(a.ExpectedKeys) == 1 && (a.Sample.LeftDistinct != a.Left.Fields[0].DistinctCount || a.Sample.RightDistinct != a.Right.Fields[0].DistinctCount) {
		return errors.New("단일 expected key의 sample distinct는 양쪽 field distinctCount와 일치해야 합니다")
	}
	return nil
}

func checkedProduct(values ...int64) (int64, bool) {
	product := int64(1)
	for _, value := range values {
		if value != 0 && product > math.MaxInt64/value {
			return 0, false
		}
		product *= value
	}
	return product, true
}

func validateSampleSide(side string, fields []FieldEvidence, distinct, maxRowsPerKey int) (int, error) {
	rows := -1
	nonNullRows := -1
	maxDistinct := int64(1)
	for _, field := range fields {
		if field.DistinctCount > field.Count || field.DuplicateCount != field.Count-field.DistinctCount {
			return 0, fmt.Errorf("%s field %q의 count/distinct/duplicate가 모순됩니다", side, field.Selector)
		}
		fieldRows := field.Count + field.NullCount
		if rows == -1 {
			rows = fieldRows
		} else if rows != fieldRows {
			return 0, fmt.Errorf("%s key field들의 관측 row 수가 다릅니다", side)
		}
		if nonNullRows == -1 || field.Count < nonNullRows {
			nonNullRows = field.Count
		}
		if field.DistinctCount == 0 {
			maxDistinct = 0
		} else if maxDistinct > int64(^uint(0)>>1)/int64(field.DistinctCount) {
			maxDistinct = int64(^uint(0) >> 1)
		} else {
			maxDistinct *= int64(field.DistinctCount)
		}
	}
	if rows <= 0 || nonNullRows <= 0 || distinct > nonNullRows || int64(distinct) > maxDistinct || maxRowsPerKey > nonNullRows {
		return 0, fmt.Errorf("%s sample distinct/max rows가 관측 row 수와 모순됩니다", side)
	}
	return nonNullRows, nil
}

func validateFieldMapping(side string, expectedKeys []string, fields []FieldEvidence) error {
	expected := make(map[string]bool, len(expectedKeys))
	for _, key := range expectedKeys {
		expected[key] = true
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if !expected[field.Key] {
			return fmt.Errorf("%s field key %q가 expectedKeys에 없습니다", side, field.Key)
		}
		if seen[field.Key] {
			return fmt.Errorf("%s expected key %q의 field evidence가 중복됩니다", side, field.Key)
		}
		seen[field.Key] = true
	}
	for _, key := range expectedKeys {
		if !seen[key] {
			return fmt.Errorf("%s에는 expected key %q의 field evidence가 없습니다", side, key)
		}
	}
	return nil
}

func repairIncompleteTail(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("연결 근거 장부 복구 확인 실패: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	last := []byte{0}
	if _, err := f.ReadAt(last, info.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	const blockSize = int64(4096)
	for end := info.Size(); end > 0; {
		start := end - blockSize
		if start < 0 {
			start = 0
		}
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if idx := strings.LastIndexByte(string(buf), '\n'); idx >= 0 {
			frameStart := start + int64(idx) + 1
			return repairFinalFrame(f, frameStart, info.Size())
		}
		end = start
	}
	return repairFinalFrame(f, 0, info.Size())
}

func repairFinalFrame(f *os.File, start, end int64) error {
	frame := make([]byte, end-start)
	if _, err := f.ReadAt(frame, start); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	var record Record
	err := json.Unmarshal(frame, &record)
	if err == nil {
		if record.ID == "" {
			return errors.New("연결 근거 장부의 마지막 기록에 ID가 없어 자동 복구하지 않았습니다")
		}
		_, writeErr := f.WriteAt([]byte{'\n'}, end)
		return writeErr
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Errorf("연결 근거 장부의 마지막 기록이 불완전하거나 손상되어 자동 복구하지 않았습니다: %w", err)
	}
	return fmt.Errorf("연결 근거 장부의 마지막 기록을 해석할 수 없어 자동 복구하지 않았습니다: %w", err)
}

func normalize(a Assessment) Assessment {
	a.Relation = strings.TrimSpace(a.Relation)
	a.MatchMethod = strings.TrimSpace(a.MatchMethod)
	a.Status = strings.TrimSpace(a.Status)
	a.IncrementalValue = strings.TrimSpace(a.IncrementalValue)
	a.Reason = strings.TrimSpace(a.Reason)
	a.Transform = strings.TrimSpace(a.Transform)
	a.ObservedAt = strings.TrimSpace(a.ObservedAt)
	a.ValidFrom = strings.TrimSpace(a.ValidFrom)
	a.ValidTo = strings.TrimSpace(a.ValidTo)
	a.Supersedes = strings.TrimSpace(a.Supersedes)
	a.Left = normalizeDataset(a.Left)
	a.Right = normalizeDataset(a.Right)
	a.EdgeKinds = canonicalStrings(a.EdgeKinds, true)
	a.ExpectedKeys = canonicalStrings(a.ExpectedKeys, false)
	return a
}

func normalizeDataset(d DatasetEvidence) DatasetEvidence {
	d.PK = strings.TrimSpace(d.PK)
	d.Delivery = strings.ToUpper(strings.TrimSpace(d.Delivery))
	d.Source.URL = strings.TrimSpace(d.Source.URL)
	d.Record.Kind = strings.TrimSpace(d.Record.Kind)
	d.Record.Operation = strings.TrimSpace(d.Record.Operation)
	d.Record.RequestHash = strings.ToLower(strings.TrimSpace(d.Record.RequestHash))
	d.Record.Asset = strings.TrimSpace(d.Record.Asset)
	d.Record.EvidenceHash = strings.ToLower(strings.TrimSpace(d.Record.EvidenceHash))
	d.Fields = append([]FieldEvidence(nil), d.Fields...)
	for i := range d.Fields {
		d.Fields[i].Key = strings.TrimSpace(d.Fields[i].Key)
		d.Fields[i].Selector = strings.TrimSpace(d.Fields[i].Selector)
		d.Fields[i].Namespace = strings.TrimSpace(d.Fields[i].Namespace)
		d.Fields[i].DataType = strings.TrimSpace(d.Fields[i].DataType)
		d.Fields[i].Grain = strings.TrimSpace(d.Fields[i].Grain)
	}
	sort.Slice(d.Fields, func(i, j int) bool {
		left, _ := json.Marshal(d.Fields[i])
		right, _ := json.Marshal(d.Fields[j])
		return string(left) < string(right)
	})
	return d
}

func canonicalStrings(values []string, lower bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
