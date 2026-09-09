package goalwork_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/catalog"
	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestAcquisitionReturningAfterExpiryDiscardsRetainedEvidence(t *testing.T) {
	for _, action := range []string{"search", "inspect", "sample"} {
		for _, outcome := range []string{"success", "failure", "cancelled"} {
			t.Run(action+"/"+outcome, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					delayed := false
					acquire := func() error {
						if !delayed {
							return nil
						}
						time.Sleep(time.Hour + time.Second) // virtual time inside synctest
						if outcome == "cancelled" {
							cancel()
							return ctx.Err()
						}
						if outcome == "failure" {
							return errors.New("source unavailable")
						}
						return nil
					}
					e, err := goalwork.Start("Report source labels", goalwork.Policy{EvidenceRecipient: "claude"}, goalwork.Dependencies{
						Search: func(context.Context, string) (catalog.Result, error) {
							return catalog.Result{Hits: []catalog.Hit{{PK: "records"}, {PK: "other"}}}, acquire()
						},
						Inspect: func(_ context.Context, pk string) (goalwork.Inspection, error) {
							return goalwork.Inspection{PK: pk}, acquire()
						},
						Sample: func(context.Context, goalwork.SampleRequest, goalwork.Inspection) (goalwork.Acquired, error) {
							return goalwork.Acquired{Delivery: "REST", Rows: []goalwork.Row{{"record": "expired-private-A", "year": "2025"}, {"record": "expired-private-B", "year": "2025"}}}, acquire()
						},
					})
					if err != nil {
						t.Fatal(err)
					}
					contract := resultContract()
					for _, d := range []goalwork.Decision{
						{Action: "define", Contract: &contract},
						{Action: "search", Query: "records", Role: "records"},
						{Action: "inspect", PK: "records"},
						{Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api"}},
					} {
						v, err := e.Advance(ctx, e.View().Revision, d)
						if err != nil || len(v.Gaps) != 0 {
							t.Fatalf("setup failed: %+v %v", v.Gaps, err)
						}
					}
					readSourceReviewEvidence(t, e)
					before := executeResult(t, e, resultRecipe())
					if before.Artifact == nil || len(before.Evidence) == 0 || before.Status != "review_required" {
						t.Fatal("test must begin with retained result and disclosed evidence")
					}
					decisions := map[string]goalwork.Decision{
						"search":  {Action: "search", Query: "additional records", Role: "records"},
						"inspect": {Action: "inspect", PK: "other"},
						"sample":  {Action: "sample", Sample: &goalwork.SampleRequest{PK: "records", Delivery: "api", Params: map[string]string{"page": "2"}}},
					}
					delayed = true
					v, err := e.Advance(ctx, before.Revision, decisions[action])
					if (outcome == "cancelled" && !errors.Is(err, context.Canceled)) || (outcome != "cancelled" && err != nil) {
						t.Fatalf("unexpected acquisition error: %v", err)
					}
					// Inspect Advance's direct response: a subsequent View call already
					// expires memory and would mask this regression.
					wire, marshalErr := json.Marshal(v)
					if marshalErr != nil || v.Status != "expired" || v.Artifact != nil || len(v.Evidence) != 0 || strings.Contains(string(wire), "expired-private-") {
						t.Fatalf("in-flight acquisition returned expired evidence: status=%s artifact=%t packets=%d", v.Status, v.Artifact != nil, len(v.Evidence))
					}
				})
			})
		}
	}
}
