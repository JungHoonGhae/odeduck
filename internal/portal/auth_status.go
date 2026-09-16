package portal

import (
	"context"
	"fmt"
)

// CheckSession verifies the saved session over HTTP without opening a browser
// or exposing credentials. Only a missing or expired session requires login;
// transport errors and unexpected portal pages remain distinct failures.
func CheckSession(ctx context.Context) error {
	release, err := acquireSessionOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	_, err = verifiedSavedPage(ctx)
	return err
}

// Caller holds the session operation lock so a rotated cookie cannot race a
// login, application, or another authenticated request.
func verifiedSavedPage(ctx context.Context) (*sessionPage, error) {
	session, err := loadSession()
	if err != nil {
		return nil, err
	}
	page, err := getAuthed(ctx, session, AccountListPath)
	if err != nil {
		return nil, err
	}
	if !isAuthed(page.HTML, page.Location) {
		return nil, fmt.Errorf("로그인 상태를 확인할 수 없습니다: 활용신청 현황 페이지 형식을 확인하세요")
	}
	if err := persistSessionRefresh(page); err != nil {
		return nil, fmt.Errorf("세션 자동 갱신 저장 실패: %w", err)
	}
	return page, nil
}
