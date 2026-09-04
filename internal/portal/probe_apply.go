package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
)

// applyFormJS builds the script that reads the 활용신청 form. It is the single
// definition of which elements the form is made of, used both to fill the form for
// a real application and to check that those elements still exist.
//
// Keeping one definition is the point. Two copies would drift, and a probe that
// checks selectors apply no longer uses would report health while apply is broken —
// worse than no probe, because it would be believed.
//
// fill=false reports what it found and changes nothing on the page, so it can run
// against a live account without creating an application.
func applyFormJS(purpose, category string, fill bool) string {
	return `(function(purpose, cat, fill){
		var out = {form:false, purposeRadio:false, purposeText:false, ops:0, agreement:false, name:''};
		var f = document.getElementById('reqForm');
		out.form = !!f;
		if(!f) return JSON.stringify(out);
		var r = document.querySelector("input[name='prcusePrpos'][value='"+cat+"']");
		// Filling needs the one radio for this category; checking structure asks the
		// broader question, because apply accepts any category and a probe that only
		// ever looks at one value would pass while the others vanished.
		out.purposeRadio = fill ? !!r : !!document.querySelector("input[name='prcusePrpos']");
		if(r && fill){ r.checked = true; }
		var ta = document.getElementById('prcusePurps');
		out.purposeText = !!ta;
		if(ta && fill){ ta.value = purpose; }
		var n = 0;
		document.querySelectorAll('.col-table input[type=checkbox]').forEach(function(o){
			if(!o.classList.contains('all-chk')){ if(fill){ o.checked = true; } n++; }
		});
		// ODCloud's current multiCloud form represents one selected operation with
		// a hidden publicDataDetailPk instead of a checkbox table.
		var detailPk = document.getElementById('publicDataDetailPk');
		if(n === 0 && f.action.indexOf(` + strconv.Quote(multiCloudApplicationPath) + `) >= 0 && detailPk && String(detailPk.value || '').trim() !== ''){ n = 1; }
		out.ops = n;
		var ag = document.getElementById('useScopeAgreAt');
		out.agreement = !!ag;
		if(ag && fill){ ag.checked = true; }
		var tag = document.querySelector('.tagset');
		if(tag){ var box = tag.closest('div'); var t = box && box.querySelector('.tit'); if(t){ out.name = t.textContent.replace(/\s+/g,' ').trim(); } }
		if(!out.name){
			document.querySelectorAll('#reqForm .key').forEach(function(key){
				if(!out.name && key.textContent.replace(/\s+/g,' ').trim() === '데이터명'){
					var value = key.parentElement && key.parentElement.querySelector('.value');
					if(value){ out.name = value.textContent.replace(/\s+/g,' ').trim(); }
				}
			});
		}
		return JSON.stringify(out);
	})(` + strconv.Quote(purpose) + `,` + strconv.Quote(category) + `,` + strconv.FormatBool(fill) + `)`
}

// FormProbe is what the 활용신청 form looked like, element by element.
type FormProbe struct {
	DataName     string `json:"name,omitempty"`
	Form         bool   `json:"form"`         // #reqForm
	PurposeRadio bool   `json:"purposeRadio"` // input[name=prcusePrpos]
	PurposeText  bool   `json:"purposeText"`  // #prcusePurps
	Operations   int    `json:"ops"`          // .col-table checkboxes, excluding all-chk
	Agreement    bool   `json:"agreement"`    // #useScopeAgreAt
}

// Missing names the pieces apply needs and did not find. Operations being zero
// counts: apply submits a set of 상세기능 checkboxes, and submitting none is not an
// application, so an empty set means the table moved rather than that the dataset
// has no operations.
func (p *FormProbe) Missing() []string {
	var missing []string
	if !p.Form {
		return []string{"#reqForm"}
	}
	if !p.PurposeRadio {
		missing = append(missing, "목적분류 라디오(input[name=prcusePrpos])")
	}
	if !p.PurposeText {
		missing = append(missing, "활용목적 입력(#prcusePurps)")
	}
	if p.Operations == 0 {
		missing = append(missing, "상세기능 체크박스(.col-table input[type=checkbox])")
	}
	if !p.Agreement {
		missing = append(missing, "이용동의(#useScopeAgreAt)")
	}
	return missing
}

// OK reports whether apply could fill and submit this form.
func (p *FormProbe) OK() bool { return len(p.Missing()) == 0 }

// ProbeApplyForm opens the real 활용신청 form for pk and reports which of the elements
// apply drives are still present, without filling or submitting anything.
//
// This is the only automated check that covers 활용신청 — the one thing odeduck does
// that nothing else does, and the seam most likely to break silently, since a
// changed form id degrades into an error only a human running a real application
// would ever see. It runs the same navigation and reads the same selectors as apply.
//
// It needs a pk the account has NOT applied for: the portal serves the form only
// once. Because the probe never submits, that stays true forever, so a chosen pk
// keeps working as a canary. When the form is unreachable the caller gets
// ErrFormUnreachable and must not read it as breakage — see doctor.
func ProbeApplyForm(ctx context.Context, pk string) (*FormProbe, error) {
	var probe *FormProbe
	err := onApplyForm(ctx, pk, func(tctx context.Context, _ func() string) error {
		var raw string
		if err := chromedp.Run(tctx, chromedp.Evaluate(applyFormJS("", PurposeResearch, false), &raw)); err != nil {
			return fmt.Errorf("폼 점검 실패: %w", err)
		}
		var p FormProbe
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &p); err != nil {
			return fmt.Errorf("폼 점검 결과를 읽지 못했습니다: %w", err)
		}
		probe = &p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return probe, nil
}
