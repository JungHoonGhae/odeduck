//go:build !windows

package providerauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearAllDoesNotFollowInterruptedWriteSymlink(t *testing.T) {
	isolateConfigHome(t)
	if err := Set("safetykorea", Credential{Key: "SAFETY-SECRET-123"}); err != nil {
		t.Fatal(err)
	}
	dir, err := credentialDir()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".provider-key-link")); err != nil {
		t.Fatal(err)
	}

	if err := ClearAll(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "unrelated" {
		t.Fatalf("cleanup followed temp symlink: body=%q error=%v", got, err)
	}
}
