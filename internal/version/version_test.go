package version

import (
	"strings"
	"testing"
)

func TestStringUsesBuildCommandName(t *testing.T) {
	originalName := CommandName
	t.Cleanup(func() { CommandName = originalName })

	CommandName = "gongctl"
	if got := String(); !strings.HasPrefix(got, "gongctl ") {
		t.Fatalf("compatibility version = %q, want gongctl prefix", got)
	}
}
