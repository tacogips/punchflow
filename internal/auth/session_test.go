package auth

import (
	"context"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestSessionExpiryIdleAndAbsolute(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, kftime.JST)
	s := model.Session{CreatedAt: now, LastSeenAt: now}
	if pf05IsSessionExpired(s, now.Add(13*time.Hour)) != true {
		t.Fatal("session beyond idle limit remained valid")
	}
	s.LastSeenAt = now.Add(23 * time.Hour)
	if pf05IsSessionExpired(s, now.Add(23*time.Hour)) {
		t.Fatal("session expired before absolute limit")
	}
	if !pf05IsSessionExpired(s, now.Add(25*time.Hour)) {
		t.Fatal("session past absolute limit remained valid")
	}
}

func TestSessionActivationRequiresValidInvitation(t *testing.T) {
	db, clock := pf05TestDB(t)
	now := clock.Now()
	_, err := db.Q().ExecContext(context.Background(), `INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES('i','inv','inv@example.test','ja',?,?,?)`, store.Millis(now.Add(time.Hour)), store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuthenticator(db, clock)
	user, err := store.UserByID(context.Background(), db.Q(), "inv")
	if err != nil {
		t.Fatal(err)
	}
	user, err = a.ActivateOnLogin(context.Background(), db.Q(), user)
	if err != nil || user.Status != model.UserStatusActive {
		t.Fatalf("activation user=%+v err=%v", user, err)
	}
	user, err = store.UserByID(context.Background(), db.Q(), "sus")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ActivateOnLogin(context.Background(), db.Q(), user); err == nil {
		t.Fatal("suspended user login accepted")
	}
}

func TestSessionActivationRejectsExpiredInvitation(t *testing.T) {
	db, clock := pf05TestDB(t)
	now := clock.Now()
	_, err := db.Q().ExecContext(context.Background(), `INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES('i','inv','inv@example.test','ja',?,?,?)`, store.Millis(now.Add(-time.Second)), store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.UserByID(context.Background(), db.Q(), "inv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewAuthenticator(db, clock).ActivateOnLogin(context.Background(), db.Q(), user); err == nil {
		t.Fatal("expired invitation accepted")
	}
}
