package portal

import (
	"context"
	"strings"
	"time"
)

// Keep the confirmed form and its controls in the page. Hidden values (including
// CSRF fields) participate in freshness checks but never leave the browser.
func guardedApplyFillJS(purpose, category string) string {
	return `(function(){
 const result = ` + applyFormJS(purpose, category, true) + `;
 const form=document.getElementById('reqForm');
 const controls=()=>Array.from(new Set([...(form ? form.elements : []),
 ...document.querySelectorAll("input[name='prcusePrpos'],#prcusePurps,.col-table input[type=checkbox],#useScopeAgreAt,#publicDataDetailPk")]));
 const state=()=>JSON.stringify([location.href,form.action,form.method,
 controls().map(e=>[e.id,e.name,e.type,e.value,e.checked,e.disabled,e.readOnly,
 e.tagName==='SELECT' ? Array.from(e.options).map(o=>[o.value,o.selected,o.disabled]) : null]),
 JSON.parse(` + applyFormJS(purpose, category, false) + `)]);
 const nodes=controls(), save=window.fn_save;
 const before=form ? state() : null;
 window.__odeduckSubmitOnce=function(){
  delete window.__odeduckSubmitOnce;
  const current=controls();
  if(!form || document.getElementById('reqForm')!==form || !form.isConnected ||
    typeof save!=='function' || window.fn_save!==save || current.length!==nodes.length ||
    current.some((e,i)=>e!==nodes[i]) || state()!==before) return 'changed';
  try { save.call(window); return 'submitted'; } catch(e) { return 'uncertain'; }
 };
 return result;
 })()`
}

const guardedApplySubmitJS = `(function(){
 if(typeof window.__odeduckSubmitOnce!=='function') return 'changed';
 return window.__odeduckSubmitOnce();
})()`

// Do not navigate away from a potentially in-flight POST merely because a
// fixed sleep elapsed. A terminal portal dialog or the deadline ends the wait.
func waitApplyDialog(ctx context.Context, dialog func() string, timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		message := dialog()
		if message != "" && !isApplyConfirmationDialog(message) {
			return message, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return "", context.DeadlineExceeded
		case <-ticker.C:
		}
	}
}

func isApplyConfirmationDialog(message string) bool {
	return strings.Contains(message, "신청하시겠습니까")
}
