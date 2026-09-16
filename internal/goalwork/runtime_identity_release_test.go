package goalwork_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/JungHoonGhae/odeduck/internal/goalwork"
)

func TestRuntimeIdentitySurvivesUpgradeBeforeFirstGoal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit replacing an open executable")
	}
	// Run a disposable copy, never rename the user's installed executable.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "runtime-helper")
	if err := os.WriteFile(path, original, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-test.run=^TestRuntimeIdentityReleaseHelper$")
	cmd.Env = append(os.Environ(), "ODEDUCK_RUNTIME_IDENTITY_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("helper startup: %q, %v", line, err)
	}
	replacement := path + ".new"
	if err := os.WriteFile(replacement, []byte("new executable bytes after process startup"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(stdin, "start first goal"); err != nil {
		t.Fatal(err)
	}
	var got goalwork.Runtime
	if err := json.NewDecoder(reader).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(original)
	if got.IdentityError != "" || got.BinarySHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("runtime fingerprint followed replacement instead of running process: %+v", got)
	}
}

func TestRuntimeIdentityReleaseHelper(t *testing.T) {
	if os.Getenv("ODEDUCK_RUNTIME_IDENTITY_HELPER") != "1" {
		return
	}
	fmt.Println("ready")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	e, err := goalwork.Start("first goal after upgrade", goalwork.Policy{}, goalwork.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(e.View().Runtime); err != nil {
		t.Fatal(err)
	}
}
