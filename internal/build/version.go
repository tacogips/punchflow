// Package build provides build metadata.
package build

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version returns the trimmed version from VERSION.
func Version() string { return strings.TrimSpace(version) }
