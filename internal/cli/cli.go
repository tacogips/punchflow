// Package cli implements the punchflow command line interface.
package cli

import (
	"context"
	"fmt"
	"io"

	"punchflow/internal/build"
)

const usage = "usage: punchflow <serve|fixtures|token|version>\n"

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }

// Run executes a punchflow command and returns its process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "version":
		if len(args) != 1 {
			_, _ = io.WriteString(stderr, usage)
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "punchflow %s\n", build.Version())
		return 0
	case "serve":
		err = runServe(ctx, args[1:], stderr)
	case "fixtures":
		err = runFixtures(ctx, args[1:], stdout, stderr)
	case "token":
		err = runToken(ctx, args[1:], stdout, stderr)
	default:
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, err)
	var usageErr usageError
	if AsUsageError(err, &usageErr) {
		return 2
	}
	return 1
}
