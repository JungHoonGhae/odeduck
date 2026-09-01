//go:build !windows

package providerauth

import "os"

func replaceFile(from, to string) error { return os.Rename(from, to) }

func secureCredentialDirectory(path string) error { return os.Chmod(path, 0o700) }
func secureCredentialFile(path string) error      { return os.Chmod(path, 0o600) }

func syncParentDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
