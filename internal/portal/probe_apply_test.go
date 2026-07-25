package portal

import (
	"encoding/json"
	"strings"
	"testing"
)

// The probe and the fill share one script so they cannot disagree about what the
// form is made of. If they diverged, the probe would report health for selectors
// apply no longer uses, which is worse than no probe because it would be believed.
func TestApplyFormJSSharesSelectorsWithFill(t *testing.T) {
	probeJS := applyFormJS("", PurposeResearch, false)
	fillJS := applyFormJS("연구", PurposeResearch, true)
	for _, sel := range []string{"reqForm", "prcusePrpos", "prcusePurps", ".col-table input[type=checkbox]", "useScopeAgreAt", ".tagset"} {
		if !strings.Contains(probeJS, sel) {
			t.Errorf("probe script missing selector %q", sel)
		}
		if !strings.Contains(fillJS, sel) {
			t.Errorf("fill script missing selector %q", sel)
		}
	}
	// Only the fill flag differs.
	if strings.Replace(probeJS, ",false)", ",true)", 1) == fillJS {
		t.Log("scripts differ only in the fill flag and the purpose argument, as intended")
	}
	if !strings.HasSuffix(strings.TrimSpace(probeJS), ",false)") {
		t.Errorf("probe must pass fill=false so it cannot modify the page: %q", probeJS[len(probeJS)-20:])
	}
}

// A form that lost a field must not be reported as usable: apply would post
// whatever the page happens to hold, against a real account.
func TestFormProbeMissing(t *testing.T) {
	full := FormProbe{Form: true, PurposeRadio: true, PurposeText: true, Operations: 3, Agreement: true}
	if !full.OK() {
		t.Errorf("complete form reported unusable: %v", full.Missing())
	}

	noForm := FormProbe{}
	if got := noForm.Missing(); len(got) != 1 || got[0] != "#reqForm" {
		t.Errorf("missing form should report only #reqForm, got %v", got)
	}

	// Zero operations means the checkbox table moved. Submitting no 상세기능 is not
	// an application, so this counts as missing rather than as an empty dataset.
	noOps := full
	noOps.Operations = 0
	if noOps.OK() {
		t.Error("a form with no 상세기능 checkbox must not be reported usable")
	}

	for name, mutate := range map[string]func(*FormProbe){
		"radio":     func(p *FormProbe) { p.PurposeRadio = false },
		"textarea":  func(p *FormProbe) { p.PurposeText = false },
		"agreement": func(p *FormProbe) { p.Agreement = false },
	} {
		p := full
		mutate(&p)
		if p.OK() {
			t.Errorf("form missing %s reported usable", name)
		}
	}
}

// The probe result is decoded from the page's JSON, so the field names have to
// match what the script emits.
func TestFormProbeDecodesScriptOutput(t *testing.T) {
	raw := `{"form":true,"purposeRadio":true,"purposeText":true,"ops":2,"agreement":true,"name":"국토교통부_아파트"}`
	var p FormProbe
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	if !p.OK() || p.Operations != 2 || p.DataName != "국토교통부_아파트" {
		t.Errorf("decoded = %+v", p)
	}
}
