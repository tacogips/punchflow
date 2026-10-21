package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"punchflow/internal/model"
)

// embeddedMigrations returns the number of embedded migration files and the
// highest migration version, which every opened database must have applied.
func embeddedMigrations(t *testing.T) (count, latest int) {
	t.Helper()
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		version, convErr := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if convErr != nil {
			t.Fatalf("invalid migration name %q: %v", entry.Name(), convErr)
		}
		count++
		if version > latest {
			latest = version
		}
	}
	return count, latest
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), t.TempDir()+"/store.db")
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

func TestOpenMigratesAllTablesAndReopenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/store.db"
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{"tenant", "grades", "users", "user_invitations", "organization_charts", "teams", "team_memberships", "roles", "role_memberships", "categories", "folders", "general_masters", "general_master_fields", "general_master_items", "routes", "workflows", "proxy_applicants", "proxy_approvers", "tickets", "ticket_inputs", "ticket_slip_items", "ticket_steps", "ticket_assignees", "ticket_custom_approvers", "ticket_viewers", "ticket_links", "comments", "files", "file_attachments", "audit_logs", "api_tokens", "sessions", "oidc_states", "webhooks", "webhook_events", "webhook_attempts", "server_secrets", "schema_migrations"}
	for _, table := range tables {
		var name string
		if err := db.Q().QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
	var inputIDsDefault sql.NullString
	rows, err := db.Q().QueryContext(ctx, "PRAGMA table_info(general_master_items)")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "input_ids" {
			inputIDsDefault = defaultValue
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !inputIDsDefault.Valid || inputIDsDefault.String != "'{}'" {
		t.Fatalf("general_master_items.input_ids default=%v; want '{}'", inputIDsDefault)
	}
	var version int
	wantCount, wantLatest := embeddedMigrations(t)
	if err := db.Q().QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != wantLatest {
		t.Fatalf("migration version=%d err=%v", version, err)
	}
	var fk int
	if err := db.Q().QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys=%d err=%v", fk, err)
	}
	var journal string
	if err := db.Q().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal_mode=%q err=%v", journal, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Close() }()
	if err = again.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = again.Q().QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&version); err != nil || version != wantCount {
		t.Fatalf("migration rows=%d err=%v", version, err)
	}
}

func TestOpenRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	ctx := context.Background()
	path := "rel/sub/pf.sqlite"
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("relative database file was not created: %v", err)
	}
	check, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = check.Close() }()
	var version, fk, migrations int
	var journal string
	wantCount, wantLatest := embeddedMigrations(t)
	if err = check.Q().QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != wantLatest {
		t.Fatalf("migration version=%d err=%v", version, err)
	}
	if err = check.Q().QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrations); err != nil || migrations != wantCount {
		t.Fatalf("migration rows=%d err=%v", migrations, err)
	}
	if err = check.Q().QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys=%d err=%v", fk, err)
	}
	if err = check.Q().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal_mode=%q err=%v", journal, err)
	}
	spacePath := "dir with space/pf.sqlite"
	spaceDB, err := Open(ctx, spacePath)
	if err != nil {
		t.Fatalf("open path containing spaces: %v", err)
	}
	if err = spaceDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(spacePath); err != nil {
		t.Fatalf("spaced database file was not created: %v", err)
	}
}

func TestOpenMemoryPath(t *testing.T) {
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var version int
	_, wantLatest := embeddedMigrations(t)
	if err := db.Q().QueryRowContext(context.Background(), "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != wantLatest {
		t.Fatalf("memory migration version=%d err=%v", version, err)
	}
}

func TestWithTxRollbackAndCommit(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	id := NewID()
	insert := func(q Querier) error {
		_, err := q.ExecContext(ctx, "INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, "u1", "u@example.test", "Test", "User", model.UserStatusActive, "en", model.UserTypeNormal, "{}", 1, 1)
		return err
	}
	want := errors.New("abort")
	if err := db.WithTx(ctx, func(tx *Tx) error {
		if err := insert(tx); err != nil {
			return err
		}
		return want
	}); !errors.Is(err, want) {
		t.Fatalf("rollback error=%v", err)
	}
	var n int
	if err := db.Q().QueryRowContext(ctx, "SELECT count(*) FROM users WHERE id=?", id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollback count=%d err=%v", n, err)
	}
	if err := db.WithTx(ctx, func(tx *Tx) error { return insert(tx) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Q().QueryRowContext(ctx, "SELECT count(*) FROM users WHERE id=?", id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("commit count=%d err=%v", n, err)
	}
}

func TestNewIDAndIsUUID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if len(id) != 36 || !IsUUID(id) {
			t.Fatalf("invalid ID %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
	if IsUUID("not-a-uuid") {
		t.Fatal("accepted invalid UUID")
	}
}
func TestTimeConversions(t *testing.T) {
	tm := time.UnixMilli(123456)
	if Millis(Time(Millis(tm))) != Millis(tm) {
		t.Fatal("millisecond conversion changed value")
	}
	if NullMillis(nil) != nil {
		t.Fatal("nil time should map to SQL NULL")
	}
	if TimePtr(sql.NullInt64{}) != nil {
		t.Fatal("invalid nullable time should map to nil")
	}
}
func TestServerSecretStable(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	a, err := ServerSecret(ctx, db.Q(), "file-url")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ServerSecret(ctx, db.Q(), "file-url")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 32 || string(a) != string(b) {
		t.Fatalf("secret length=%d stable=%t", len(a), string(a) == string(b))
	}
}
func TestTruncateAllRetainsMigrationAndSecrets(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if _, err := ServerSecret(ctx, db.Q(), "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, "INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('id','code','e@x','a','b','active','en','member','{}',1,1)"); err != nil {
		t.Fatal(err)
	}
	if err := TruncateAll(ctx, db.Q()); err != nil {
		t.Fatal(err)
	}
	var users, migrations, secrets int
	for q, dst := range map[string]*int{"SELECT count(*) FROM users": &users, "SELECT count(*) FROM schema_migrations": &migrations, "SELECT count(*) FROM server_secrets": &secrets} {
		if err := db.Q().QueryRowContext(ctx, q).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	wantCount, _ := embeddedMigrations(t)
	if users != 0 || migrations != wantCount || secrets != 1 {
		t.Fatalf("users=%d migrations=%d secrets=%d", users, migrations, secrets)
	}
}
func TestResolveID(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	id := NewID()
	if _, err := db.Q().ExecContext(ctx, "INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, "alice", "a@example.test", "A", "B", model.UserStatusActive, "en", model.UserTypeNormal, "{}", 1, 1); err != nil {
		t.Fatal(err)
	}
	byID, err := ResolveID(ctx, db.Q(), "users", id)
	if err != nil {
		t.Fatal(err)
	}
	byCode, err := ResolveID(ctx, db.Q(), "users", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if byID != id || byCode != id {
		t.Fatalf("ids: %q %q", byID, byCode)
	}
	if _, err = ResolveID(ctx, db.Q(), "users", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing error=%v", err)
	}
	if _, err = ResolveID(ctx, db.Q(), "users; DROP TABLE users", "x"); err == nil {
		t.Fatal("accepted untrusted table")
	}
}
func TestDefinitionJSONTags(t *testing.T) {
	in := `{"id":"wf-1","code":"expense","ticketNumberingTiming":"submission","availableTeams":[{"teamCode":"finance"}],"publicTicket":true,"visibleToManager":"direct","titleInputMode":"input","commentingRequiredOnApproval":true,"reportFormats":["pdf"],"sectionList":[{"id":"section-1","formFields":[{"id":"field-1","code":"amount","title":"Amount","approver":true,"author":false,"size":"full","orientation":"vertical","allowedTicketStatus":["draft"]}]}],"routeConditions":[{"id":"cond-1","conditionType":"field","combinationType":"all","combinationExpression":"1","routeCode":null,"errorMessage":null,"conditionFields":[]}],"routeStep":{"id":"step-1","stepOrder":2,"fallbackType":"skip","allowSelfApproval":false}}`
	var wf model.WorkflowDef
	if err := json.Unmarshal([]byte(in), &wf); err != nil {
		t.Fatal(err)
	}
	if wf.TicketNumberingTiming != "submission" || !wf.PublicTicket || wf.VisibleToManager != "direct" || !wf.CommentingRequiredOnApproval || len(wf.ReportFormats) != 1 || len(wf.SectionList) != 1 || wf.SectionList[0].FormFields[0].Code != "amount" || !wf.SectionList[0].FormFields[0].Approver {
		t.Fatalf("workflow tags not decoded: %+v", wf)
	}
	var route struct {
		RouteStep model.RouteStep `json:"routeStep"`
	}
	if err := json.Unmarshal([]byte(in), &route); err != nil {
		t.Fatal(err)
	}
	if route.RouteStep.StepOrder != 2 || route.RouteStep.FallbackType == nil || *route.RouteStep.FallbackType != "skip" {
		t.Fatalf("route step tags not decoded: %+v", route.RouteStep)
	}
	b, err := json.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["ticketNumberingTiming"]; !ok {
		t.Fatal("ticketNumberingTiming key missing")
	}
	if got := fmt.Sprint(out["id"]); got != "wf-1" {
		t.Fatalf("id=%s", got)
	}
}
