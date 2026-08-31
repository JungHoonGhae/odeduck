package portal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const sessionLockFile = "datagokr-session.lock"

// acquireSessionFileLock prevents separate gongctl processes from consuming the
// same rotating cookie concurrently. The lock file contains no credentials and
// intentionally remains after release; removing a lock path while another
// process is waiting on its inode can split future callers across two locks.
func acquireSessionFileLock(ctx context.Context) (func(), error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, sessionLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("세션 잠금 파일 열기 실패: %w", err)
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		locked, lockErr := trySessionFileLock(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("세션 잠금 실패: %w", lockErr)
		}
		if locked {
			return func() {
				_ = unlockSessionFile(file)
				_ = file.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
