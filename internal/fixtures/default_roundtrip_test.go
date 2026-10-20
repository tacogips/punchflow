package fixtures_test

import (
	"bytes"
	"context"
	"io/fs"
	"testing"

	punchflow "punchflow"
	"punchflow/internal/fixtures"
	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

func TestDefaultRoundTrip(t *testing.T) {
	ctx := context.Background()
	defaults, err := fixtures.DefaultFS(punchflow.DefaultFixtures)
	if err != nil {
		t.Fatal(err)
	}
	set, err := fixtures.Parse(defaults)
	if err != nil {
		t.Fatalf("parse default fixtures: %v", err)
	}
	if err := fixtures.Validate(set); err != nil {
		t.Fatalf("validate default fixtures: %v", err)
	}
	db, err := store.Open(ctx, t.TempDir()+"/roundtrip.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := fixtures.Load(ctx, db, defaults, kftime.SystemClock{}); err != nil {
		t.Fatalf("load default fixtures: %v", err)
	}
	exported, err := fixtures.Export(ctx, db.Q())
	if err != nil {
		t.Fatalf("export loaded fixtures: %v", err)
	}
	entries, err := fs.ReadDir(defaults, ".")
	if err != nil {
		t.Fatal(err)
	}
	fixtureCount := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == ".gitkeep" {
			continue
		}
		fixtureCount++
		want, err := fs.ReadFile(defaults, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		got, ok := exported[entry.Name()]
		if !ok {
			t.Errorf("export is missing %s", entry.Name())
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("exported %s is not byte-identical", entry.Name())
		}
	}
	if len(exported) != fixtureCount {
		t.Fatalf("exported %d fixture files, source has %d", len(exported), fixtureCount)
	}
}
