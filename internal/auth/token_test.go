package auth

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestTokenFormatAndDigest(t *testing.T) {
	value := NewTokenValue("pat")
	if !regexp.MustCompile(`^pf_pat_[0-9A-Za-z]{32}$`).MatchString(value) {
		t.Fatalf("unexpected token format %q", value)
	}
	if HashToken(value) == value || len(HashToken(value)) != 64 {
		t.Fatal("token digest must be a distinct SHA-256 hex value")
	}
}

func TestPrincipalRequire(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	if err := Require(request, "user"); err == nil {
		t.Fatal("request without principal accepted")
	}
	principal := &Principal{Permissions: map[string]bool{"other": true}}
	request = request.WithContext(WithPrincipal(request.Context(), principal))
	if err := Require(request, "user"); err == nil {
		t.Fatal("missing permission accepted")
	}
	if err := Require(request, "other", "user"); err != nil {
		t.Fatalf("held alternative permission rejected: %v", err)
	}
}

func TestTokenIssuePersistsDigestOnly(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.Q().ExecContext(ctx, `INSERT INTO tenant(id,public_id,name,plan,mode,created_at,updated_at) VALUES('t','t','Tenant','enterprise','normal',1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Q().ExecContext(ctx, `INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('u','U','u@example.test','A','B','activated','ja','normal','{}',1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	clock := kftime.NewFakeClock(time.Date(2026, 10, 20, 12, 0, 0, 0, kftime.JST))
	a := NewAuthenticator(db, clock)
	plain, id, err := a.IssueToken(ctx, db.Q(), "pat", "u", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	var fixture sql.NullString
	if err := db.Q().QueryRowContext(ctx, "SELECT token_hash,fixture_plaintext FROM api_tokens WHERE id=?", id).Scan(&digest, &fixture); err != nil {
		t.Fatal(err)
	}
	if digest != HashToken(plain) || fixture.Valid {
		t.Fatal("runtime token must persist only its digest")
	}
}

func TestTokenUserPermissionsUnion(t *testing.T) {
	db, _ := pf05TestDB(t)
	for _, role := range []struct{ id, permissions string }{
		{"r1", `[{"permission":"user"},{"permission":"integration"}]`},
		{"r2", `[{"permission":"ticket_write"},{"permission":"integration"}]`},
	} {
		if _, err := db.Q().ExecContext(context.Background(), "INSERT INTO roles(id,name,permission_list,created_at,updated_at) VALUES(?,?,?,1,1)", role.id, role.id, role.permissions); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Q().ExecContext(context.Background(), "INSERT INTO role_memberships(role_id,user_id,created_at) VALUES(?,'u1',1)", role.id); err != nil {
			t.Fatal(err)
		}
	}
	permissions, err := store.UserPermissions(context.Background(), db.Q(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"user", "ticket_write", "integration"} {
		if !permissions[permission] {
			t.Errorf("missing permission %q in %v", permission, permissions)
		}
	}
	if len(permissions) != 3 {
		t.Fatalf("permissions=%v", permissions)
	}
}

func TestTokenOIDCStateConsumedOnce(t *testing.T) {
	db, clock := pf05TestDB(t)
	state := model.OIDCState{State: "state-value", Nonce: "nonce-value", CodeVerifier: "verifier-value", ReturnTo: "/home", ExpiresAt: clock.Now().Add(time.Minute)}
	ctx := context.Background()
	if err := store.InsertOIDCState(ctx, db.Q(), state); err != nil {
		t.Fatal(err)
	}
	got, err := store.TakeOIDCState(ctx, db.Q(), state.State)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != state.State || got.Nonce != state.Nonce || got.CodeVerifier != state.CodeVerifier || got.ReturnTo != state.ReturnTo || !got.ExpiresAt.Equal(state.ExpiresAt) {
		t.Fatalf("state=%+v want %+v", got, state)
	}
	if _, err := store.TakeOIDCState(ctx, db.Q(), state.State); err != store.ErrNotFound {
		t.Fatalf("second take error=%v, want ErrNotFound", err)
	}
}
