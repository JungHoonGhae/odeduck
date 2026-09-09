// goal-audit is a read-only developer audit, not a public product command or an
// independent correctness grader. Run it with one local manifest path.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JungHoonGhae/odeduck/internal/goalaudit"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, diagnostic io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(diagnostic, "usage: go run ./scripts/goal-audit <manifest.json>")
		return 2
	}
	root, err := os.OpenRoot(filepath.Dir(args[0]))
	if err != nil {
		fmt.Fprintln(diagnostic, "cannot open manifest directory")
		return 2
	}
	defer root.Close()
	input, err := root.Open(filepath.Base(args[0]))
	if err != nil {
		fmt.Fprintln(diagnostic, "cannot open manifest")
		return 2
	}
	defer input.Close()
	r, err := goalaudit.Audit(input, root.FS())
	if err != nil {
		fmt.Fprintln(diagnostic, err)
		return 2
	}
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	if err := e.Encode(r); err != nil {
		fmt.Fprintln(diagnostic, "cannot write audit report")
		return 2
	}
	if !r.ReadinessFloorMet {
		return 1
	}
	return 0
}
