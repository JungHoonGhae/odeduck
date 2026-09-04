package version

import (
	"strings"
	"testing"
)

func TestStringUsesBuildCommandName(t *testing.T) {
	originalName := CommandName
	t.Cleanup(func() { CommandName = originalName })

	CommandName = "odeduck-test"
	if got := String(); !strings.HasPrefix(got, "odeduck-test ") {
		t.Fatalf("version = %q, want injected command prefix", got)
	}
}
