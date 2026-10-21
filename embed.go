// Package punchflow embeds the frontend and default fixtures.
package punchflow

import "embed"

// WebDist contains the built web assets.
//
//go:embed all:web/dist
var WebDist embed.FS

// DefaultFixtures contains the built-in fixture set.
//
//go:embed all:fixtures/default
var DefaultFixtures embed.FS
