package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Exercise the real fill/confirm/submit seam against an isolated local page.
// No portal session, authentication command, or real application is used.
func TestApplyConfirmedState(t *testing.T) {
	if testing.Short() {
		t.Skip("local Chrome fixture")
	}
	chrome, err := findChrome()
	if err != nil {
		t.Skip(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<form id="reqForm" action="/submit"><input id="pk" name="publicDataPk" value="123"><input type="radio" name="prcusePrpos" value="PROS05"><textarea id="prcusePurps"></textarea><div class="col-table"><input id="op" type="checkbox" name="op" value="one"></div><input id="useScopeAgreAt" type="checkbox"><div class="key">데이터명</div><div class="value">fixture</div></form><script>window.submits=0;function fn_save(){window.submits++;setTimeout(()=>window.notice='활용신청이 완료되었습니다.',30)}</script>`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	alloc, closeAlloc := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome))...)
	defer closeAlloc()
	tab, closeTab := chromedp.NewContext(alloc)
	defer closeTab()
	for _, tc := range []struct {
		name, mutation string
		accepted       bool
	}{
		{"unchanged", "", true},
		{"ODCloud", "", true},
		{"ODCloud changed", `document.getElementById('publicDataDetailPk').value='changed'`, false},
		{"URL", `history.pushState({},'', '/changed')`, false},
		{"disabled", `document.getElementById('op').disabled=true`, false},
		{"new field", `document.getElementById('reqForm').insertAdjacentHTML('beforeend','<input name="extra" value="new">')`, false},
		{"purpose", `document.getElementById('prcusePurps').value='changed'`, false},
		{"category", `document.querySelector('[name=prcusePrpos]').checked=false`, false},
		{"operation", `document.getElementById('op').checked=false`, false},
		{"operation identity", `document.getElementById('op').value='other'`, false},
		{"dataset", `document.getElementById('pk').value='456'`, false},
		{"agreement", `document.getElementById('useScopeAgreAt').checked=false`, false},
		{"target", `document.getElementById('reqForm').action='/other'`, false},
		{"replacement", `var f=document.getElementById('reqForm');f.replaceWith(f.cloneNode(true))`, false},
		{"handler", `window.fn_save=function(){window.submits+=10}`, false},
		{"canceled", "", false},
		{"throw after submission", "", false},
		{"validation rejection", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := chromedp.Run(tab, chromedp.Navigate(srv.URL)); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(tc.name, "ODCloud") {
				if e := chromedp.Run(tab, chromedp.Evaluate(`document.getElementById('reqForm').action='/iim/api/multiCloudApiRequestForm.do';document.querySelector('.col-table').innerHTML='<input id="publicDataDetailPk" type="hidden" value="detail-1">'`, nil)); e != nil {
					t.Fatal(e)
				}
			}
			if tc.name == "throw after submission" {
				if e := chromedp.Run(tab, chromedp.Evaluate(`window.fn_save=function(){window.submits++;throw new Error('response lost')}`, nil)); e != nil {
					t.Fatal(e)
				}
			}
			if tc.name == "validation rejection" {
				if e := chromedp.Run(tab, chromedp.Evaluate(`window.fn_save=function(){window.submits++;window.notice='활용목적을 입력해주세요.'}`, nil)); e != nil {
					t.Fatal(e)
				}
			}
			started := time.Now()
			confirmed := false
			result, err := fillAndSubmit(tab, "123", "research", PurposeResearch, func(ApplySummary) bool {
				confirmed = true
				if tc.mutation != "" {
					if e := chromedp.Run(tab, chromedp.Evaluate(tc.mutation, nil)); e != nil {
						t.Fatal(e)
					}
				}
				return tc.name != "canceled"
			}, func() string {
				var notice string
				if e := chromedp.Run(tab, chromedp.Evaluate("window.notice || ''", &notice)); e != nil {
					t.Fatal(e)
				}
				return notice
			})
			if !confirmed {
				t.Fatalf("confirmation was not reached: %v", err)
			}
			var submits int
			if e := chromedp.Run(tab, chromedp.Evaluate("window.submits", &submits)); e != nil {
				t.Fatal(e)
			}
			if tc.name == "throw after submission" {
				if err == nil || submits != 1 {
					t.Fatalf("uncertain mutation: submits=%d err=%v", submits, err)
				}
				var replay string
				if e := chromedp.Run(tab, chromedp.Evaluate(guardedApplySubmitJS, &replay)); e != nil {
					t.Fatal(e)
				}
				if replay != "changed" {
					t.Fatalf("uncertain submission was reusable: %s", replay)
				}
			} else if tc.name == "validation rejection" {
				if err != nil || result == nil || result.Submitted || submits != 1 || !strings.Contains(result.Message, "활용목적") {
					t.Fatalf("rejection: result=%+v err=%v submits=%d", result, err, submits)
				}
			} else if tc.accepted {
				if err != nil || result == nil || !result.Submitted || submits != 1 {
					t.Fatalf("result=%+v err=%v submits=%d", result, err, submits)
				}
				var replay string
				if e := chromedp.Run(tab, chromedp.Evaluate(guardedApplySubmitJS, &replay)); e != nil {
					t.Fatal(e)
				}
				if replay != "changed" {
					t.Fatalf("submission was reusable: %s", replay)
				}
				if time.Since(started) > 3500*time.Millisecond {
					t.Fatal("already completed submission still waits")
				}
			} else if submits != 0 {
				t.Fatalf("changed/canceled form submitted %d times", submits)
			} else if tc.name != "canceled" && err == nil {
				t.Fatal("changed form must return an error")
			}
		})
	}
}

func TestWaitApplyDialog(t *testing.T) {
	t.Run("delayed success", func(t *testing.T) {
		start := time.Now()
		got, err := waitApplyDialog(context.Background(), func() string {
			if time.Since(start) > 30*time.Millisecond {
				return "활용신청이 완료되었습니다."
			}
			return "신청하시겠습니까?"
		}, time.Second)
		if err != nil || !isApplySuccessDialog(got) {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("validation", func(t *testing.T) {
		got, err := waitApplyDialog(context.Background(), func() string { return "활용목적을 입력해주세요." }, time.Second)
		if err != nil || !strings.Contains(got, "활용목적") {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		_, err := waitApplyDialog(context.Background(), func() string { return "신청하시겠습니까?" }, 20*time.Millisecond)
		if err != context.DeadlineExceeded {
			t.Fatalf("%v", err)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := waitApplyDialog(ctx, func() string { return "" }, time.Second)
		if err != context.Canceled {
			t.Fatalf("%v", err)
		}
	})
}
