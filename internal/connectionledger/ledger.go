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
)

var relationPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)

type FieldEvidence struct {
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
	dir, err := portal.ConfigDir()
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
	line, _ := json.Marshal(record)
	line = append(line, '\n')
	_, writeErr := f.Write(line)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return Record{}, fmt.Errorf("연결 근거 기록 실패: %w", writeErr)
	}
	if closeErr != nil {
		return Record{}, closeErr
	}
	return record, nil
}

func (s *Store) List(ctx context.Context, filter Filter) ([]Record, error) {
	if filter.PK != "" {
		if err := portal.ValidatePublicDataPK(filter.PK); err != nil {
			return nil, err
		}
	}
	if filter.Status != "" && !validStatus(filter.Status) {
		return nil, errors.New("알 수 없는 connection assessment status입니다")
	}
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
			if strings.TrimSpace(field.Selector) == "" || strings.TrimSpace(field.Namespace) == "" || strings.TrimSpace(field.DataType) == "" || strings.TrimSpace(field.Grain) == "" {
				return errors.New("verified field에는 selector, namespace, dataType, grain이 모두 필요합니다")
			}
		}
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
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "auth") || strings.Contains(lower, "secret") {
			return fmt.Errorf("%s source.url에는 credential query를 넣을 수 없습니다", side)
		}
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
		if len([]rune(field.Selector)) > 80 || len([]rune(field.Namespace)) > 200 || len([]rune(field.DataType)) > 80 || len([]rune(field.Grain)) > 200 {
			return fmt.Errorf("%s field evidence가 너무 깁니다", side)
		}
		if field.Count < 0 || field.NullCount < 0 || field.DistinctCount < 0 || field.DuplicateCount < 0 {
			return fmt.Errorf("%s field 집계값은 음수일 수 없습니다", side)
		}
	}
	if d.Record.EvidenceHash != "" && !validSHA256(d.Record.EvidenceHash) {
		return fmt.Errorf("%s evidenceHash는 SHA-256 hex여야 합니다", side)
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

func validStatus(status string) bool {
	return status == StatusStructurallyVerified || status == StatusSampleVerified || status == StatusBlocked || status == StatusRejected
}

func samePair(a, b Assessment) bool {
	return (a.Left.PK == b.Left.PK && a.Right.PK == b.Right.PK) || (a.Left.PK == b.Right.PK && a.Right.PK == b.Left.PK)
}

func normalize(a Assessment) Assessment {
	a.Relation = strings.TrimSpace(a.Relation)
	a.MatchMethod = strings.TrimSpace(a.MatchMethod)
	a.Status = strings.TrimSpace(a.Status)
	a.IncrementalValue = strings.TrimSpace(a.IncrementalValue)
	a.Reason = strings.TrimSpace(a.Reason)
	a.Transform = strings.TrimSpace(a.Transform)
	a.Left.Delivery = strings.ToUpper(strings.TrimSpace(a.Left.Delivery))
	a.Right.Delivery = strings.ToUpper(strings.TrimSpace(a.Right.Delivery))
	a.Left.Source.URL = strings.TrimSpace(a.Left.Source.URL)
	a.Right.Source.URL = strings.TrimSpace(a.Right.Source.URL)
	a.Left.Record.Kind = strings.TrimSpace(a.Left.Record.Kind)
	a.Right.Record.Kind = strings.TrimSpace(a.Right.Record.Kind)
	a.EdgeKinds = append([]string(nil), a.EdgeKinds...)
	a.ExpectedKeys = append([]string(nil), a.ExpectedKeys...)
	sort.Strings(a.EdgeKinds)
	sort.Strings(a.ExpectedKeys)
	return a
}
