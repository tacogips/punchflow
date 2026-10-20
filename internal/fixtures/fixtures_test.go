package fixtures

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

const pf06TestWorkflowID = "00000000-0000-4000-8000-000000000021"

func pf06OpenDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "fixtures.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func pf06Clock() *kftime.FakeClock {
	return kftime.NewFakeClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
}

func pf06MinimalFS() fs.FS { return os.DirFS("testdata/minimal") }

func TestLoadExportRoundTripMinimal(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	if _, err := Load(ctx, db, pf06MinimalFS(), pf06Clock()); err != nil {
		t.Fatal(err)
	}
	first, err := Export(ctx, db.Q())
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range first {
		want, err := os.ReadFile(filepath.Join("testdata/minimal", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("export %s differs from canonical fixture", name)
		}
	}
	secondDB := pf06OpenDB(t)
	if _, err := Load(ctx, secondDB, pf06MapFS(first), pf06Clock()); err != nil {
		t.Fatal(err)
	}
	second, err := Export(ctx, secondDB.Q())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("second export has %d files; want %d", len(second), len(first))
	}
	for name, got := range first {
		if !bytes.Equal(got, second[name]) {
			t.Errorf("second export %s is not byte-stable", name)
		}
	}
}

func TestExportMasterInputIDs(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	if _, err := Load(ctx, db, pf06MinimalFS(), pf06Clock()); err != nil {
		t.Fatal(err)
	}
	var inputIDs string
	if err := db.Q().QueryRowContext(ctx, "SELECT input_ids FROM general_master_items WHERE code = ?", "ITEM1").Scan(&inputIDs); err != nil {
		t.Fatal(err)
	}
	wantIDs := `{"00000000-0000-4000-8000-000000000014":"00000000-0000-4000-8000-000000000016"}`
	if inputIDs != wantIDs {
		t.Fatalf("stored input_ids=%s; want %s", inputIDs, wantIDs)
	}
	files, err := Export(ctx, db.Q())
	if err != nil {
		t.Fatal(err)
	}
	masters := string(files[FileGeneralMasters])
	if !strings.Contains(masters, `"id": "00000000-0000-4000-8000-000000000016"`) {
		t.Fatalf("export does not contain fixture input ID: %s", masters)
	}
	if !strings.Contains(masters, `"code": "ITEM2",
        "name": "Two",
        "inputs": []`) {
		t.Fatalf("empty master inputs did not export as []: %s", masters)
	}
}

func TestExportMissingMasterInputID(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	if _, err := Load(ctx, db, pf06MinimalFS(), pf06Clock()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, "UPDATE general_master_items SET input_ids = '{}' WHERE code = ?", "ITEM1"); err != nil {
		t.Fatal(err)
	}
	_, err := Export(ctx, db.Q())
	if err == nil || !strings.Contains(err.Error(), "ITEM1") {
		t.Fatalf("Export error=%v; want item code ITEM1", err)
	}
}

func TestExportRouteTargetGradeSymbolNull(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	if _, err := Load(ctx, db, pf06MinimalFS(), pf06Clock()); err != nil {
		t.Fatal(err)
	}
	files, err := Export(ctx, db.Q())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files[FileRoutes]), `"gradeSymbol": null`) {
		t.Fatalf("route target gradeSymbol was not exported as null: %s", files[FileRoutes])
	}
}

func TestLoadNonEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	if _, err := Load(ctx, db, pf06MinimalFS(), pf06Clock()); err != nil {
		t.Fatal(err)
	}
	_, err := Load(ctx, db, pf06MinimalFS(), pf06Clock())
	if err == nil || err.Error() != "database is not empty" {
		t.Fatalf("second Load error=%v; want database is not empty", err)
	}
	var count int
	if err := db.Q().QueryRowContext(ctx, "SELECT count(*) FROM tenant").Scan(&count); err != nil || count != 1 {
		t.Fatalf("tenant count=%d err=%v; want unchanged count 1", count, err)
	}
}

func TestResetClearsStateAndFiles(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	clock := pf06Clock()
	if _, err := Load(ctx, db, pf06MinimalFS(), clock); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, "INSERT INTO tickets(id,workflow_id,status,current_step,created_at,updated_at) VALUES(?,?,?,?,?,?)", "00000000-0000-4000-8000-000000000030", pf06TestWorkflowID, "draft", 0, 1, 1); err != nil {
		t.Fatal(err)
	}
	filesDir := filepath.Join(t.TempDir(), "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "upload.bin"), []byte("uploaded"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Reset(ctx, db, pf06MinimalFS(), filesDir, clock); err != nil {
		t.Fatal(err)
	}
	var tickets, tenants int
	if err := db.Q().QueryRowContext(ctx, "SELECT count(*) FROM tickets").Scan(&tickets); err != nil {
		t.Fatal(err)
	}
	if err := db.Q().QueryRowContext(ctx, "SELECT count(*) FROM tenant").Scan(&tenants); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		t.Fatal(err)
	}
	if tickets != 0 || tenants != 1 || len(entries) != 0 {
		t.Fatalf("after Reset: tickets=%d tenants=%d files=%d; want 0, 1, 0", tickets, tenants, len(entries))
	}
}

func pf06MapFS(files map[string][]byte) fstest.MapFS {
	out := make(fstest.MapFS, len(files))
	for name, data := range files {
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}

func TestLoadSummaryReportsInsertedRows(t *testing.T) {
	ctx := context.Background()
	db := pf06OpenDB(t)
	summary, err := Load(ctx, db, pf06MinimalFS(), pf06Clock())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Counts["general_master_items"] != 2 {
		t.Fatalf("general_master_items count=%d; want 2", summary.Counts["general_master_items"])
	}
	if summary.Counts["tenant"] != 1 {
		t.Fatalf("tenant count=%d; want 1", summary.Counts["tenant"])
	}
}
