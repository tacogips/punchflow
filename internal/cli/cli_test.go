package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
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
	for _, args := range [][]string{nil, {"nope"}} {
		var stdout, stderr bytes.Buffer
		if got := Run(context.Background(), args, &stdout, &stderr); got != 2 {
			t.Fatalf("Run(%v) = %d, want 2", args, got)
		}
		if stderr.Len() == 0 {
			t.Fatalf("Run(%v) did not print usage", args)
		}
	}
	for _, args := range [][]string{{"fixtures"}, {"fixtures", "export"}, {"token"}, {"token", "issue"}} {
		var stdout, stderr bytes.Buffer
		if got := Run(context.Background(), args, &stdout, &stderr); got != 2 {
			t.Fatalf("Run(%v) = %d, want usage exit 2 (stderr %q)", args, got, stderr.String())
		}
	}
}

func TestRunFixturesAndToken(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	call := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	if code, _, stderr := call("fixtures", "load", "--db", dbPath); code != 0 {
		t.Fatalf("fixtures load exit=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := call("fixtures", "load", "--db", dbPath); code != 1 {
		t.Fatalf("second fixtures load exit=%d want 1 stderr=%q", code, stderr)
	}
	out := filepath.Join(dir, "export")
	if code, _, stderr := call("fixtures", "export", "--db", dbPath, "--out", out); code != 0 {
		t.Fatalf("fixtures export exit=%d stderr=%q", code, stderr)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 15 {
		t.Fatalf("export entries=%d err=%v, want 15", len(entries), err)
	}
	if code, token, stderr := call("token", "issue", "--db", dbPath, "--user", "admin"); code != 0 || !regexp.MustCompile(`^pf_pat_[0-9A-Za-z]{32}\n$`).MatchString(token) {
		t.Fatalf("token issue exit=%d token=%q stderr=%q", code, token, stderr)
	}
	if code, _, stderr := call("token", "issue", "--db", dbPath); code != 2 {
		t.Fatalf("token missing target exit=%d want 2 stderr=%q", code, stderr)
	}
}

func TestRunEnvironmentAndFlagPrecedence(t *testing.T) {
	dir := t.TempDir()
	fromEnv, fromFlag := filepath.Join(dir, "env.sqlite"), filepath.Join(dir, "flag.sqlite")
	t.Setenv("PUNCHFLOW_DB", fromEnv)
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"fixtures", "load"}, &stdout, &stderr); code != 0 {
		t.Fatalf("env fixtures load exit=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"fixtures", "load", "--db", fromFlag}, &stdout, &stderr); code != 0 {
		t.Fatalf("flag override fixtures load exit=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(fromFlag); err != nil {
		t.Fatalf("flag-selected DB missing: %v", err)
	}
}

func TestRunServeInvalidListenIsRuntimeError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"serve", "--listen", "bad", "--db", filepath.Join(t.TempDir(), "server.sqlite")}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("serve with invalid listen address exit=%d, want runtime exit 1 (stderr %q)", code, stderr.String())
	}
}

func TestRunExplicitBoolFlagOverridesInvalidEnvironment(t *testing.T) {
	t.Setenv("PUNCHFLOW_SEED", "invalid")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"serve", "--seed=true", "--listen", "bad", "--db", filepath.Join(t.TempDir(), "server.sqlite")}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("explicit seed flag exit=%d, want runtime exit 1 after flag override (stderr %q)", code, stderr.String())
	}
}
