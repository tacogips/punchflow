package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type commonConfig struct{ dataDir, dbPath, fixtures *string }

func AsUsageError(err error, target *usageError) bool { return errors.As(err, target) }

func cliFlagSet(name string, stderr interface{ Write([]byte) (int, error) }) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return usageError{err: err}
	}
	if fs.NArg() != 0 {
		return usageError{err: fmt.Errorf("unexpected argument %q", fs.Arg(0))}
	}
	return nil
}

func envString(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return fallback
}

func envBool(name, flagName string, args []string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		if cliFlagSetInArgs(args, flagName) {
			return fallback, nil
		}
		return false, usageError{err: fmt.Errorf("%s must be true, false, 1, or 0", name)}
	}
}

func cliFlagSetInArgs(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--"+name || arg == "-"+name || strings.HasPrefix(arg, "--"+name+"=") || strings.HasPrefix(arg, "-"+name+"=") {
			return true
		}
	}
	return false
}

func commonFlags(fs *flag.FlagSet) *commonConfig {
	dataDir := fs.String("data-dir", envString("PUNCHFLOW_DATA_DIR", ".punchflow-data"), "data directory")
	dbPath := fs.String("db", envString("PUNCHFLOW_DB", ""), "SQLite database path")
	fixtures := fs.String("fixtures", envString("PUNCHFLOW_FIXTURES", ""), "fixture directory")
	return &commonConfig{dataDir: dataDir, dbPath: dbPath, fixtures: fixtures}
}

func (c *commonConfig) resolve() {
	if *c.dbPath == "" {
		*c.dbPath = *c.dataDir + string(os.PathSeparator) + "punchflow.sqlite"
	}
}
