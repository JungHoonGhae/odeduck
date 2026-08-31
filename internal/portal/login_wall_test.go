package portal

import "testing"

// The portal's authenticated pages carry the string "통합 로그인" inside a script
// comment. Matching it anywhere in the document made every authenticated read
// look like the login wall, so `gongctl login` polled for its full 5 minutes and
// exited 1 while the browser was in fact logged in.
func TestIsLoginWallIgnoresTheWordInScriptComments(t *testing.T) {
	authed := `<html><head><title>활용신청 현황 | 공공데이터포털</title></head><body>
	<script>
	//    - 회원 없음  : 통합 로그인 페이지 + linkToken 으로 이동
	</script>
	<div class="mypage-dataset-list">활용신청 현황</div></body></html>`

	if isLoginWall(authed, "https://www.data.go.kr/iim/api/selectAcountList.do") {
		t.Error("authenticated page treated as the login wall")
	}
	if !isAuthed(authed, "https://www.data.go.kr/iim/api/selectAcountList.do") {
		t.Error("authenticated page not recognised as authed")
	}

	wall := `<html><head><title>통합 로그인 | 공공데이터포털</title></head><body></body></html>`
	if !isLoginWall(wall, "https://www.data.go.kr/iim/api/selectAcountList.do") {
		t.Error("login page not recognised as the login wall")
	}
	// The URL alone is enough, whatever the body says.
	if !isLoginWall("<html></html>", "https://auth.data.go.kr/common-login") {
		t.Error("login URL not recognised as the login wall")
	}
}
