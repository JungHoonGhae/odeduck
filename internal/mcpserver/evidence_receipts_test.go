package mcpserver

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/apicall"
	"github.com/JungHoonGhae/odeduck/internal/connectionledger"
)

func TestEvidenceReceiptCacheIsBounded(t *testing.T) {
	receipts := newEvidenceReceipts()
	for i := 0; i < maxProfileReceipts+5; i++ {
		profile, err := apicall.ProfileBody(map[string]any{"id": fmt.Sprintf("%03d", i)}, []string{"id"})
		if err != nil {
			t.Fatal(err)
		}
		receipts.registerAPI("15000001", "REST", "getRows", map[string]string{"page": fmt.Sprint(i)}, profile)
	}
	if got := len(receipts.profiles); got != maxProfileReceipts {
		t.Fatalf("receipt cache size=%d, want %d", got, maxProfileReceipts)
	}
}

func TestInspectionReceiptsExpireAndStayBounded(t *testing.T) {
	receipts := newEvidenceReceipts()
	for i := 0; i < maxProfileReceipts+5; i++ {
		receipts.registerInspection(fmt.Sprintf("15%06d", i), "REST")
	}
	if got := len(receipts.inspected); got != maxProfileReceipts {
		t.Fatalf("inspection cache size=%d, want %d", got, maxProfileReceipts)
	}
	receipts.inspected[inspectionKey("15000001", "REST")] = time.Now().Add(-profileReceiptTTL - time.Minute)
	receipts.inspected[inspectionKey("15000002", "REST")] = time.Now().Add(-profileReceiptTTL - time.Minute)
	err := receipts.verify(connectionledger.Assessment{
		Status: connectionledger.StatusStructurallyVerified,
		Left:   connectionledger.DatasetEvidence{PK: "15000001", Delivery: "REST"}, Right: connectionledger.DatasetEvidence{PK: "15000002", Delivery: "REST"},
	})
	if err == nil || !strings.Contains(err.Error(), "inspect_dataset") || len(receipts.inspected) > maxProfileReceipts {
		t.Fatalf("expired inspection err=%v cache=%d", err, len(receipts.inspected))
	}
}

func TestStructuralReceiptIsDeliveryScoped(t *testing.T) {
	receipts := newEvidenceReceipts()
	receipts.registerInspection("15000001", "FILE")
	receipts.registerInspection("15000002", "REST")
	err := receipts.verify(connectionledger.Assessment{
		Status: connectionledger.StatusStructurallyVerified,
		Left:   connectionledger.DatasetEvidence{PK: "15000001", Delivery: "REST"}, Right: connectionledger.DatasetEvidence{PK: "15000002", Delivery: "REST"},
	})
	if err == nil {
		t.Fatal("FILE inspection authorized a REST structural claim")
	}
}

func TestOversizedProfileDoesNotCreateReceipt(t *testing.T) {
	receipts := newEvidenceReceipts()
	profile, err := apicall.ProfileBody(map[string]any{"id": strings.Repeat("x", maxReceiptBytes+1)}, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	receipts.registerAPI("15000001", "REST", "getRows", nil, profile)
	if profile.ReceiptStatus != "not_issued_profile_too_large" || len(receipts.profiles) != 0 {
		t.Fatalf("status=%q receipts=%d", profile.ReceiptStatus, len(receipts.profiles))
	}
}
