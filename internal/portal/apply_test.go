package portal

import "testing"

func TestApplySuccessDialogIsAcceptedWhenListPropagationLags(t *testing.T) {
	for _, message := range []string{
		"활용신청이 완료되었습니다.",
		"  활용신청이\n완료되었습니다.  ",
		"신청이 완료되었습니다. 신청된 API는 1~2시간 후 호출 가능합니다. 일부 API에서 24시간 후 인증이 가능합니다.",
	} {
		if !isApplySuccessDialog(message) {
			t.Errorf("isApplySuccessDialog(%q) = false, want true", message)
		}
	}

	for _, message := range []string{
		"신청하시겠습니까?",
		"활용목적을 입력해주세요.",
		"뉴스레터 신청이 완료되었습니다.",
	} {
		if isApplySuccessDialog(message) {
			t.Errorf("isApplySuccessDialog(%q) = true, want false", message)
		}
	}
}

func TestApplyFormLocationAcceptsLegacyAndODCloudForms(t *testing.T) {
	for _, location := range []string{
		"https://www.data.go.kr/iim/api/selectDevAcountRequestForm.do?x=1",
		"https://www.data.go.kr/iim/api/multiCloudApiRequestForm.do?x=1",
	} {
		if !isApplyFormLocation(location) {
			t.Errorf("valid application form rejected: %s", location)
		}
	}
	if isApplyFormLocation("https://www.data.go.kr/index.do") {
		t.Fatal("index page must not be accepted as an application form")
	}
}

func TestNormalizePurposeCategoryRejectsSilentMisclassification(t *testing.T) {
	for input, want := range map[string]string{
		"web": PurposeWeb, "app": PurposeApp, "research": PurposeResearch,
		"ref": PurposeRef, "etc": PurposeEtc, PurposeWeb: PurposeWeb,
	} {
		got, err := NormalizePurposeCategory(input)
		if err != nil || got != want {
			t.Errorf("NormalizePurposeCategory(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := NormalizePurposeCategory(""); err == nil {
		t.Fatal("empty category must not silently become research")
	}
	if _, err := NormalizePurposeCategory("commercial"); err == nil {
		t.Fatal("unknown category must be rejected")
	}
}

func TestApplicationSuccessRequiresExactNormalizedTitle(t *testing.T) {
	if !sameApplicationTitle("기상청_지상 일자료 조회서비스", "  기상청_지상   일자료 조회서비스 ") {
		t.Fatal("equivalent whitespace-normalized titles should match")
	}
	if sameApplicationTitle("기상청_지상 일자료 조회서비스 상세", "기상청_지상 일자료 조회서비스") {
		t.Fatal("a pre-existing superset title must not prove that this application succeeded")
	}
}
