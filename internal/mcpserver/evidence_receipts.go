package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/connectionledger"
)

// evidenceReceipts binds sample_verified claims to profiles produced by this
// MCP server process. Raw values live only in memory for the session; the
// durable ledger receives aggregates and the deterministic evidence hash.
type evidenceReceipts struct {
	mu        sync.RWMutex
	profiles  map[string]profileReceipt
	order     []string
	inspected map[string]time.Time
}

type profileReceipt struct {
	fields     map[string]apicall.ProfileEvidence
	observedAt time.Time
}

const (
	maxProfileReceipts = 64
	profileReceiptTTL  = 15 * time.Minute
	maxReceiptValues   = 10_000
	maxReceiptBytes    = 1 << 20
)

func newEvidenceReceipts() *evidenceReceipts {
	return &evidenceReceipts{profiles: map[string]profileReceipt{}, inspected: map[string]time.Time{}}
}

func (r *evidenceReceipts) registerInspection(pk string, deliveries ...string) {
	now := time.Now().UTC()
	r.mu.Lock()
	r.pruneLocked(now)
	for _, delivery := range deliveries {
		r.setInspectionLocked(inspectionKey(pk, delivery), now)
	}
	r.mu.Unlock()
}

func (r *evidenceReceipts) setInspectionLocked(key string, observedAt time.Time) {
	if _, exists := r.inspected[key]; !exists && len(r.inspected) >= maxProfileReceipts {
		var oldestPK string
		var oldest time.Time
		for candidate, observedAt := range r.inspected {
			if oldestPK == "" || observedAt.Before(oldest) {
				oldestPK, oldest = candidate, observedAt
			}
		}
		delete(r.inspected, oldestPK)
	}
	r.inspected[key] = observedAt
}

func (r *evidenceReceipts) registerAPI(pk, delivery, operation string, params map[string]string, profile *apicall.SampleProfile) {
	if profile == nil || profile.EvidenceHash == "" {
		return
	}
	observedAt := time.Now().UTC()
	profile.Operation = strings.TrimSpace(operation)
	profile.RequestHash = requestHash(params)
	profile.ObservedAt = observedAt.Format(time.RFC3339Nano)
	fields := map[string]apicall.ProfileEvidence{}
	valueCount := 0
	valueBytes := 0
	for _, field := range profile.Evidence() {
		fields[strings.TrimSpace(field.Field)] = field
		valueCount += len(field.ValueCounts)
		for value := range field.ValueCounts {
			valueBytes += len(value)
		}
	}
	if valueCount > maxReceiptValues || valueBytes > maxReceiptBytes {
		profile.ReceiptStatus = "not_issued_profile_too_large"
		return
	}
	profile.ReceiptStatus = "session_bound"
	r.mu.Lock()
	r.pruneLocked(observedAt)
	delivery = strings.ToUpper(strings.TrimSpace(delivery))
	key := receiptKey(pk, delivery, profile.EvidenceHash, profile.Operation, profile.RequestHash)
	if _, exists := r.profiles[key]; !exists {
		r.order = append(r.order, key)
	}
	r.profiles[key] = profileReceipt{fields: fields, observedAt: observedAt}
	r.setInspectionLocked(inspectionKey(pk, delivery), observedAt)
	for len(r.order) > maxProfileReceipts {
		delete(r.profiles, r.order[0])
		r.order = r.order[1:]
	}
	r.mu.Unlock()
}

func (r *evidenceReceipts) verify(a connectionledger.Assessment) error {
	if strings.TrimSpace(a.Status) != connectionledger.StatusSampleVerified {
		if strings.TrimSpace(a.Status) != connectionledger.StatusStructurallyVerified {
			return nil
		}
		r.mu.Lock()
		r.pruneLocked(time.Now().UTC())
		leftOK := !r.inspected[inspectionKey(a.Left.PK, a.Left.Delivery)].IsZero()
		rightOK := !r.inspected[inspectionKey(a.Right.PK, a.Right.Delivery)].IsZero()
		r.mu.Unlock()
		if !leftOK || !rightOK {
			return errors.New("structurally_verified는 이 MCP 세션에서 양쪽 inspect_dataset을 먼저 실행해야 합니다")
		}
		return nil
	}
	if len(a.ExpectedKeys) != 1 {
		return errors.New("sample_verified는 현재 한 개의 실제 profile key만 지원합니다")
	}
	if a.Left.Record.Kind != "api_profile" || a.Right.Record.Kind != "api_profile" {
		return errors.New("sample_verified는 현재 이 MCP 세션에서 call_api로 관찰한 두 API profile만 기록할 수 있습니다")
	}
	left, leftObservedAt, err := r.profile(a.Left)
	if err != nil {
		return fmt.Errorf("left: %w", err)
	}
	right, rightObservedAt, err := r.profile(a.Right)
	if err != nil {
		return fmt.Errorf("right: %w", err)
	}
	if a.Sample == nil {
		return errors.New("sample_verified에는 sample 집계가 필요합니다")
	}
	assessmentTime, err := time.Parse(time.RFC3339, strings.TrimSpace(a.ObservedAt))
	if err != nil {
		return errors.New("observedAt은 RFC3339이어야 합니다")
	}
	latest := leftObservedAt
	if rightObservedAt.After(latest) {
		latest = rightObservedAt
	}
	if assessmentTime.Before(latest) || assessmentTime.After(latest.Add(profileReceiptTTL)) {
		return errors.New("observedAt은 두 call_api profile 발급 이후 15분 안이어야 합니다")
	}
	overlap := 0
	joined := int64(0)
	for value, leftCount := range left.ValueCounts {
		if rightCount := right.ValueCounts[value]; rightCount > 0 {
			overlap++
			product := int64(leftCount) * int64(rightCount)
			if joined > math.MaxInt64-product {
				return errors.New("profile join 집계가 범위를 넘습니다")
			}
			joined += product
		}
	}
	if joined > int64(^uint(0)>>1) {
		return errors.New("profile join 집계가 현재 플랫폼의 정수 범위를 넘습니다")
	}
	want := connectionledger.SampleEvidence{
		LeftDistinct: left.DistinctCount, RightDistinct: right.DistinctCount,
		OverlapDistinct: overlap, JoinedRows: int(joined),
		LeftMaxRowsPerKey: maxValueCount(left.ValueCounts), RightMaxRowsPerKey: maxValueCount(right.ValueCounts),
	}
	if *a.Sample != want {
		return fmt.Errorf("sample 집계가 이 MCP 세션의 call_api profile과 일치하지 않습니다: expected %+v", want)
	}
	return nil
}

func (r *evidenceReceipts) profile(dataset connectionledger.DatasetEvidence) (apicall.ProfileEvidence, time.Time, error) {
	if len(dataset.Fields) != 1 {
		return apicall.ProfileEvidence{}, time.Time{}, errors.New("한 개의 profile field evidence가 필요합니다")
	}
	r.mu.RLock()
	receipt, ok := r.profiles[receiptKey(dataset.PK, dataset.Delivery, dataset.Record.EvidenceHash, dataset.Record.Operation, dataset.Record.RequestHash)]
	r.mu.RUnlock()
	if !ok || time.Since(receipt.observedAt) > profileReceiptTTL {
		return apicall.ProfileEvidence{}, time.Time{}, errors.New("operation/requestHash/evidenceHash 조합이 이 MCP 세션의 call_api에서 발급되지 않았거나 만료됐습니다")
	}
	field, ok := receipt.fields[strings.TrimSpace(dataset.Fields[0].Selector)]
	if !ok {
		return apicall.ProfileEvidence{}, time.Time{}, errors.New("selector가 evidenceHash의 profile field와 일치하지 않습니다")
	}
	claimed := dataset.Fields[0]
	if claimed.Count != field.Count || claimed.NullCount != field.NullCount || claimed.DistinctCount != field.DistinctCount || claimed.DuplicateCount != field.DuplicateCount {
		return apicall.ProfileEvidence{}, time.Time{}, errors.New("field 집계가 evidenceHash의 profile과 일치하지 않습니다")
	}
	return field, receipt.observedAt, nil
}

func receiptKey(pk, delivery, hash, operation, requestHash string) string {
	return inspectionKey(pk, delivery) + "\x00" + strings.ToLower(strings.TrimSpace(hash)) + "\x00" + strings.TrimSpace(operation) + "\x00" + strings.ToLower(strings.TrimSpace(requestHash))
}

func inspectionKey(pk, delivery string) string {
	return strings.TrimSpace(pk) + "\x00" + strings.ToUpper(strings.TrimSpace(delivery))
}

func requestHash(params map[string]string) string {
	canonical, _ := json.Marshal(params)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func (r *evidenceReceipts) pruneLocked(now time.Time) {
	kept := r.order[:0]
	for _, key := range r.order {
		if receipt, ok := r.profiles[key]; ok && now.Sub(receipt.observedAt) <= profileReceiptTTL {
			kept = append(kept, key)
		} else {
			delete(r.profiles, key)
		}
	}
	r.order = kept
	for pk, observedAt := range r.inspected {
		if now.Sub(observedAt) > profileReceiptTTL {
			delete(r.inspected, pk)
		}
	}
}

func maxValueCount(counts map[string]int) int {
	max := 0
	for _, count := range counts {
		if count > max {
			max = count
		}
	}
	return max
}
