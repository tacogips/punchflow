package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"punchflow/internal/fixtures"
	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

func runFixtures(ctx context.Context, args []string, stdout, stderr interface{ Write([]byte) (int, error) }) error {
	if len(args) == 0 {
		return usageError{err: fmt.Errorf("fixtures requires load, reset, or export")}
	}
	command, rest := args[0], args[1:]
	fs := cliFlagSet("fixtures "+command, stderr)
	common := commonFlags(fs)
	out := fs.String("out", "", "export directory")
	if err := parseFlags(fs, rest); err != nil {
		return err
	}
	if command != "load" && command != "reset" && command != "export" {
		return usageError{err: fmt.Errorf("unknown fixtures command %q", command)}
	}
	if command == "export" && *out == "" {
		return usageError{err: fmt.Errorf("fixtures export requires --out DIR")}
	}
	common.resolve()
	fixtureFS, err := cliFixturesFS(*common.fixtures)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, *common.dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	switch command {
	case "load":
		if _, err := fixtures.Load(ctx, db, fixtureFS, kftime.SystemClock{}); err != nil {
			return fmt.Errorf("load fixtures: %w", err)
		}
	case "reset":
		if _, err := fixtures.Reset(ctx, db, fixtureFS, filepath.Join(*common.dataDir, "files"), kftime.SystemClock{}); err != nil {
			return fmt.Errorf("reset fixtures: %w", err)
		}
	case "export":
		files, err := fixtures.Export(ctx, db.Q())
		if err != nil {
			return fmt.Errorf("export fixtures: %w", err)
		}
		if err := os.MkdirAll(*out, 0700); err != nil {
			return fmt.Errorf("create export directory: %w", err)
		}
		for name, contents := range files {
			if err := os.WriteFile(filepath.Join(*out, name), contents, 0600); err != nil {
				return fmt.Errorf("write exported fixture %s: %w", name, err)
			}
		}
	}
	_, _ = fmt.Fprintf(stdout, "fixtures %s complete\n", command)
	return nil
}
