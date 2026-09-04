package connectionledger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func acquireFileLock(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return acquireFileLockWithFlags(ctx, path, os.O_CREATE|os.O_RDWR)
}

func acquireExistingFileLock(ctx context.Context, path string) (func(), error) {
	return acquireFileLockWithFlags(ctx, path, os.O_RDWR)
}

func acquireFileLockWithFlags(ctx context.Context, path string, flags int) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return nil, fmt.Errorf("연결 근거 잠금 파일 열기 실패: %w", err)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		locked, lockErr := tryFileLock(f)
		if lockErr != nil {
			_ = f.Close()
			return nil, lockErr
		}
		if locked {
			if err := ctx.Err(); err != nil {
				_ = unlockFile(f)
				_ = f.Close()
				return nil, err
			}
			return func() { _ = unlockFile(f); _ = f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
