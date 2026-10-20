package cli

import (
	"bytes"
	"context"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := Run(context.Background(), []string{"version"}, &stdout, &stderr); got != 0 {
		t.Fatalf("Run(version) = %d, want 0", got)
	}
	if got, want := stdout.String(), "punchflow 0.1.0\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunPlaceholdersAndUsage(t *testing.T) {
	for _, command := range []string{"serve", "fixtures", "token"} {
		var stdout, stderr bytes.Buffer
		if got := Run(context.Background(), []string{command}, &stdout, &stderr); got != 1 {
			t.Fatalf("Run(%s) = %d, want 1", command, got)
		}
		if got := stderr.String(); got == "" {
			t.Fatalf("Run(%s) did not report placeholder status", command)
		}
	}
	for _, args := range [][]string{nil, {"nope"}} {
		var stdout, stderr bytes.Buffer
		if got := Run(context.Background(), args, &stdout, &stderr); got != 2 {
			t.Fatalf("Run(%v) = %d, want 2", args, got)
		}
		if stderr.Len() == 0 {
			t.Fatalf("Run(%v) did not print usage", args)
		}
	}
}
