package portal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const sessionLockFile = "datagokr-session.lock"

// acquireSessionFileLock prevents separate odeduck processes from consuming the
// same rotating cookie concurrently. The lock file contains no credentials and
// intentionally remains after release; removing a lock path while another
// process is waiting on its inode can split future callers across two locks.
func acquireSessionFileLock(ctx context.Context) (func(), error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	return acquireSessionFileLockIn(ctx, dir)
}

func acquireSessionFileLockIn(ctx context.Context, dir string) (func(), error) {
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

// acquireAllSessionOperations serializes logout against both v0.9 processes
// and a still-running v0.8 process that only knows the legacy config root.
func acquireAllSessionOperations(ctx context.Context) (func(), error) {
	select {
	case sessionOperationSlot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Ensure the selected root exists before enumerating every known root.
	if _, err := configDir(); err != nil {
		<-sessionOperationSlot
		return nil, err
	}
	dirs, err := configDirsForCleanup()
	if err != nil {
		<-sessionOperationSlot
		return nil, err
	}
	releases := make([]func(), 0, len(dirs))
	for _, dir := range dirs {
		release, lockErr := acquireSessionFileLockIn(ctx, dir)
		if lockErr != nil {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
			<-sessionOperationSlot
			return nil, lockErr
		}
		releases = append(releases, release)
	}
	return func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
		<-sessionOperationSlot
	}, nil
}
