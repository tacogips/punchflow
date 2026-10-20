// Package cli implements the punchflow command line interface.
package cli

import (
	"context"
	"fmt"
	"io"

	"punchflow/internal/build"
)

const usage = "usage: punchflow <serve|fixtures|token|version>\n"

// Run executes a punchflow command and returns its process exit code.
func Run(_ context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		_, _ = fmt.Fprintf(stdout, "punchflow %s\n", build.Version())
		return 0
	case "serve", "fixtures", "token":
		_, _ = fmt.Fprintf(stderr, "%s not implemented\n", args[0])
		return 1
	default:
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
}
