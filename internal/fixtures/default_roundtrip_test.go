package fixtures_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

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
	defer func() { _ = db.Close() }()
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

	secondFS := fstest.MapFS{}
	for name, data := range exported {
		secondFS[name] = &fstest.MapFile{Data: append([]byte(nil), data...)}
	}
	secondDB, err := store.Open(ctx, t.TempDir()+"/roundtrip-second.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = secondDB.Close() }()
	if _, err := fixtures.Load(ctx, secondDB, secondFS, kftime.SystemClock{}); err != nil {
		t.Fatalf("load first export: %v", err)
	}
	secondExport, err := fixtures.Export(ctx, secondDB.Q())
	if err != nil {
		t.Fatalf("export second round: %v", err)
	}
	if len(secondExport) != len(exported) {
		t.Fatalf("second export has %d files, first export has %d", len(secondExport), len(exported))
	}
	for name, want := range exported {
		if got, ok := secondExport[name]; !ok || !bytes.Equal(got, want) {
			t.Errorf("second export %s is not a fixed point", name)
		}
	}
}

func TestTenantTimestampFallback(t *testing.T) {
	ctx := context.Background()
	defaults, err := fixtures.DefaultFS(punchflow.DefaultFixtures)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(defaults, ".")
	if err != nil {
		t.Fatal(err)
	}
	fixtureFS := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == ".gitkeep" {
			continue
		}
		data, readErr := fs.ReadFile(defaults, entry.Name())
		if readErr != nil {
			t.Fatal(readErr)
		}
		fixtureFS[entry.Name()] = &fstest.MapFile{Data: data}
	}
	var tenant fixtures.TenantFile
	if err := json.Unmarshal(fixtureFS[fixtures.FileTenant].Data, &tenant); err != nil {
		t.Fatal(err)
	}
	tenant.Tenant.CreatedAt = time.Time{}
	tenant.Tenant.UpdatedAt = time.Time{}
	fixtureFS[fixtures.FileTenant].Data, err = json.Marshal(tenant)
	if err != nil {
		t.Fatal(err)
	}

	fakeNow := time.Date(2026, time.January, 2, 3, 4, 5, 678000000, time.UTC)
	clock := kftime.NewFakeClock(fakeNow)
	db, err := store.Open(ctx, t.TempDir()+"/timestamp-fallback.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	assertTenantTimestamps := func(label string) {
		t.Helper()
		var created, updated int64
		if err := db.Q().QueryRowContext(ctx, "SELECT created_at,updated_at FROM tenant").Scan(&created, &updated); err != nil {
			t.Fatalf("read tenant timestamps after %s: %v", label, err)
		}
		want := store.Millis(fakeNow)
		if created != want || updated != want {
			t.Errorf("tenant timestamps after %s = (%d,%d), want (%d,%d)", label, created, updated, want, want)
		}
	}
	if _, err := fixtures.Load(ctx, db, fixtureFS, clock); err != nil {
		t.Fatalf("load without tenant timestamps: %v", err)
	}
	assertTenantTimestamps("Load")
	if _, err := fixtures.Reset(ctx, db, fixtureFS, "", clock); err != nil {
		t.Fatalf("reset without tenant timestamps: %v", err)
	}
	assertTenantTimestamps("Reset")
}
