package punchflow

import (
	"io/fs"
	"testing"
)

func TestEmbeddedDirectoriesContainAnchors(t *testing.T) {
	if _, err := fs.Stat(WebDist, "web/dist/.gitkeep"); err != nil {
		t.Fatalf("embedded web anchor missing: %v", err)
	}
	if _, err := fs.Stat(DefaultFixtures, "fixtures/default/.gitkeep"); err != nil {
		t.Fatalf("embedded fixture anchor missing: %v", err)
	}
}
