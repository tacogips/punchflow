package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedTicketAndUsers(t *testing.T, ctx context.Context, db *DB) (ticketID, userA, userB string) {
	t.Helper()
	statements := []string{
		`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('f1','f','F',1,1)`,
		`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('w1','w','visible','f1','{}',1,1)`,
		`INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,created_at,updated_at) VALUES('ua','a','a@example.test','A','A','activated','ja','member',1,1)`,
		`INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,created_at,updated_at) VALUES('ub','b','b@example.test','B','B','activated','ja','member',1,1)`,
		`INSERT INTO tickets(id,workflow_id,status,created_at,updated_at) VALUES('t1','w1','draft',1,1)`,
	}
	for _, statement := range statements {
		if _, err := db.Q().ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}
	return "t1", "ua", "ub"
}

func openMemory(t *testing.T) (context.Context, *DB) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

func TestTicketWatchState(t *testing.T) {
	ctx, db := openMemory(t)
	ticket, userA, userB := seedTicketAndUsers(t, ctx, db)
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	if state, err := TicketWatchState(ctx, db.Q(), ticket, userA); err != nil || state != "" {
		t.Fatalf("default state = %q, %v", state, err)
	}
	if err := SetTicketWatchState(ctx, db.Q(), ticket, userA, WatchStateWatching, now); err != nil {
		t.Fatal(err)
	}
	if err := SetTicketWatchState(ctx, db.Q(), ticket, userB, WatchStateUnwatched, now); err != nil {
		t.Fatal(err)
	}
	if err := SetTicketWatchState(ctx, db.Q(), ticket, userB, WatchStateWatching, now.Add(time.Hour)); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	states, err := TicketWatchStates(ctx, db.Q(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[userA] != WatchStateWatching || states[userB] != WatchStateWatching {
		t.Fatalf("states = %v", states)
	}
	if err = SetTicketWatchState(ctx, db.Q(), ticket, userA, "bogus", now); err == nil {
		t.Fatalf("invalid state accepted")
	}
}

func TestNotificationsLifecycle(t *testing.T) {
	ctx, db := openMemory(t)
	ticket, userA, userB := seedTicketAndUsers(t, ctx, db)
	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	dedupe := "approval:x:1"

	insert := func(id, user, kind string, at time.Time, key *string) bool {
		created, err := InsertNotification(ctx, db.Q(), Notification{ID: id, UserID: user, Kind: kind, TicketID: ticket, DedupeKey: key, CreatedAt: at})
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
		return created
	}
	insert("n1", userA, "ticket_applied", base, nil)
	insert("n2", userA, "ticket_approved", base.Add(time.Minute), nil)
	insert("n3", userB, "ticket_approved", base, nil)
	if !insert("n4", userA, "approval_requested", base.Add(2*time.Minute), &dedupe) {
		t.Fatalf("first dedupe insert reported no row")
	}
	if insert("n5", userA, "approval_requested", base.Add(3*time.Minute), &dedupe) {
		t.Fatalf("duplicate dedupe key inserted a second row")
	}
	if !insert("n6", userB, "approval_requested", base, &dedupe) {
		t.Fatalf("dedupe key must be scoped per user")
	}

	items, total, err := ListNotifications(ctx, db.Q(), userA, false, 0, 10)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("list = %d items, total %d, err %v", len(items), total, err)
	}
	if items[0].ID != "n4" || items[1].ID != "n2" || items[2].ID != "n1" {
		t.Fatalf("order = %s %s %s, want newest first", items[0].ID, items[1].ID, items[2].ID)
	}
	if page, total, _ := ListNotifications(ctx, db.Q(), userA, false, 1, 1); total != 3 || len(page) != 1 || page[0].ID != "n2" {
		t.Fatalf("page = %+v total %d", page, total)
	}

	if err = MarkNotificationRead(ctx, db.Q(), userA, "n4", base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = MarkNotificationRead(ctx, db.Q(), userA, "n3", base); !errors.Is(err, ErrNotFound) {
		t.Fatalf("marking another user's notification: %v, want ErrNotFound", err)
	}
	if err = MarkNotificationRead(ctx, db.Q(), userA, "missing", base); !errors.Is(err, ErrNotFound) {
		t.Fatalf("marking a missing notification: %v", err)
	}
	if n, _ := CountUnreadNotifications(ctx, db.Q(), userA); n != 2 {
		t.Fatalf("unread = %d, want 2", n)
	}
	unread, total, _ := ListNotifications(ctx, db.Q(), userA, true, 0, 10)
	if total != 2 || len(unread) != 2 {
		t.Fatalf("unread list = %d / %d", len(unread), total)
	}
	changed, err := MarkAllNotificationsRead(ctx, db.Q(), userA, base.Add(2*time.Hour))
	if err != nil || changed != 2 {
		t.Fatalf("mark all = %d, %v", changed, err)
	}
	if n, _ := CountUnreadNotifications(ctx, db.Q(), userB); n != 2 {
		t.Fatalf("other user's unread = %d, want untouched 2", n)
	}
}

func TestSchemaCascadesAndTruncate(t *testing.T) {
	ctx, db := openMemory(t)
	ticket, userA, _ := seedTicketAndUsers(t, ctx, db)
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := SetTicketWatchState(ctx, db.Q(), ticket, userA, WatchStateWatching, now); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertNotification(ctx, db.Q(), Notification{ID: "n1", UserID: userA, Kind: "ticket_applied", TicketID: ticket, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := TruncateAll(ctx, db.Q()); err != nil {
		t.Fatalf("TruncateAll with watch and notification rows: %v", err)
	}
	for _, table := range []string{"ticket_watches", "notifications"} {
		var n int
		if err := db.Q().QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s rows after truncate = %d, %v", table, n, err)
		}
	}
}

func TestTicketKeywordCandidates(t *testing.T) {
	ctx, db := openMemory(t)
	_, userA, _ := seedTicketAndUsers(t, ctx, db)
	statements := []string{
		`UPDATE tickets SET title='Quarterly offsite', ticket_number='EXP-0007' WHERE id='t1'`,
		`INSERT INTO tickets(id,workflow_id,status,title,created_at,updated_at) VALUES('t2','w1','draft','Taxi',2,2)`,
		`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,created_at,updated_at) VALUES('i1','t1','f1','"team lunch"',1,1)`,
		`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,created_at,updated_at) VALUES('i2','t2','f1','12500',1,1)`,
		`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,created_at,updated_at) VALUES('i3','t2','f2','["alpha","beta"]',1,1)`,
		`INSERT INTO ticket_slip_items(id,ticket_id,slip_section_id,position,inputs,created_at,updated_at) VALUES('s1','t2','sec',0,'{"fieldkey-uuid":"Notebook","qty":3}',1,1)`,
		`INSERT INTO comments(id,ticket_id,user_id,body,created_at,updated_at) VALUES('c1','t1','` + userA + `','Looks fine',1,1)`,
		`INSERT INTO comments(id,ticket_id,user_id,body,deleted_at,created_at,updated_at) VALUES('c2','t2','` + userA + `','removed words',5,1,1)`,
	}
	for _, statement := range statements {
		if _, err := db.Q().ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}
	tests := []struct {
		name  string
		terms []string
		want  []string
	}{
		{"title", []string{"offsite"}, []string{"t1"}},
		{"ticket number", []string{"EXP-0007"}, []string{"t1"}},
		{"string input", []string{"lunch"}, []string{"t1"}},
		{"number input", []string{"12500"}, []string{"t2"}},
		{"array input element", []string{"beta"}, []string{"t2"}},
		{"detail cell value", []string{"notebook"}, []string{"t2"}},
		{"detail cell key is not searchable", []string{"fieldkey"}, []string{}},
		{"detail cell number", []string{"3"}, []string{"t2"}},
		{"live comment", []string{"fine"}, []string{"t1"}},
		{"deleted comment", []string{"removed"}, []string{}},
		{"all terms", []string{"offsite", "lunch"}, []string{"t1"}},
		{"terms across tickets", []string{"offsite", "taxi"}, []string{}},
		{"percent literal", []string{"%"}, []string{}},
		{"backslash literal", []string{`\`}, []string{}},
		{"no terms", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TicketKeywordCandidates(ctx, db.Q(), tt.terms)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("candidates = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("candidates = %v, want %v", got, tt.want)
				}
			}
		})
	}
	bodies, err := TicketLiveCommentBodies(ctx, db.Q(), "t2")
	if err != nil || len(bodies) != 0 {
		t.Fatalf("live bodies for t2 = %v, %v", bodies, err)
	}
}
