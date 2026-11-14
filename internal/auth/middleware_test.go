package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

func pf05TestDB(t *testing.T) (*store.DB, *kftime.FakeClock) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, kftime.JST)
	_, err = db.Q().ExecContext(ctx, `INSERT INTO tenant(id,public_id,name,plan,mode,ip_allowlist,created_at,updated_at) VALUES('t','t','Tenant','enterprise','normal','[]',?,?)`, store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []struct{ id, email, status string }{{"u1", "one@example.test", "activated"}, {"u2", "two@example.test", "activated"}, {"00000000-0000-4000-8000-000000000001", "caller@example.test", "activated"}, {"inv", "inv@example.test", "invited"}, {"sus", "sus@example.test", "suspended"}} {
		_, err = db.Q().ExecContext(ctx, `INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES(?,?,?,'A','B',?,'ja','normal','{}',?,?)`, u.id, u.id, u.email, u.status, store.Millis(now), store.Millis(now))
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, kftime.NewFakeClock(now)
}

func pf05InsertToken(t *testing.T, db *store.DB, kind, user string) string {
	t.Helper()
	raw := NewTokenValue(kind)
	now := time.Now()
	var uid any
	if user != "" {
		uid = user
	}
	_, err := db.Q().ExecContext(context.Background(), `INSERT INTO api_tokens(id,kind,name,token_hash,user_id,created_at) VALUES(?,?,?,?,?,?)`, store.NewID(), kind, "test", HashToken(raw), uid, store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMiddlewareBearerResolutionAndErrors(t *testing.T) {
	db, clock := pf05TestDB(t)
	a := NewAuthenticator(db, clock)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		if !ok {
			t.Error("principal missing")
		}
		if p.User.ID != "u1" || p.Via != "pat" {
			t.Errorf("principal=%+v", p)
		}
		w.WriteHeader(204)
	})
	for _, tc := range []struct {
		name, authorization, caller string
		want                        int
		code                        string
	}{{"missing", "", "", 401, "invalid_access_token"}, {"bad", "Bearer invalid", "", 401, "invalid_access_token"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/users", nil)
			r.Header.Set("Authorization", tc.authorization)
			w := httptest.NewRecorder()
			a.Middleware(next).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Code != tc.code {
				t.Errorf("error code=%q want %q", body.Code, tc.code)
			}
		})
	}
	pat := pf05InsertToken(t, db, "pat", "u1")
	r := httptest.NewRequest("GET", "/v1/users", nil)
	r.Header.Set("Authorization", "Bearer "+pat)
	r.Header.Set("X-Caller-Id", "u2")
	w := httptest.NewRecorder()
	a.Middleware(next).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("PAT status=%d body=%s", w.Code, w.Body.String())
	}
	sat := pf05InsertToken(t, db, "sat", "")
	valid := httptest.NewRequest("GET", "/v1", nil)
	valid.Header.Set("Authorization", "Bearer "+sat)
	valid.Header.Set("X-Caller-Id", "00000000-0000-4000-8000-000000000001")
	validNext := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := FromContext(r.Context())
		if p.User.ID != "00000000-0000-4000-8000-000000000001" || p.Via != "sat" {
			t.Errorf("SAT principal=%+v", p)
		}
		w.WriteHeader(204)
	})
	validResponse := httptest.NewRecorder()
	a.Middleware(validNext).ServeHTTP(validResponse, valid)
	if validResponse.Code != 204 {
		t.Fatalf("valid SAT status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	for _, tc := range []struct {
		caller string
		want   int
		code   string
	}{{"", 401, "invalid_caller_id"}, {"abc", 400, "invalid_header"}, {"00000000-0000-4000-8000-000000000002", 401, "invalid_caller_id"}} {
		r := httptest.NewRequest("GET", "/v1", nil)
		r.Header.Set("Authorization", "Bearer "+sat)
		if tc.caller != "" {
			r.Header.Set("X-Caller-Id", tc.caller)
		}
		w := httptest.NewRecorder()
		a.Middleware(next).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("caller %q status=%d want %d", tc.caller, w.Code, tc.want)
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Errorf("caller %q decode error response: %v", tc.caller, err)
			continue
		}
		if body.Code != tc.code {
			t.Errorf("caller %q error code=%q want %q", tc.caller, body.Code, tc.code)
		}
	}
}

func TestMiddlewareTenantAndIPAllowlist(t *testing.T) {
	db, clock := pf05TestDB(t)
	a := NewAuthenticator(db, clock)
	raw := pf05InsertToken(t, db, "pat", "u1")
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, tc := range []struct {
		mode, allow, remote, code string
		status                    int
	}{{"suspended", "[]", "192.0.2.1:9", "tenant_suspended", 403}, {"maintenance", "[]", "192.0.2.1:9", "tenant_maintenance", 403}, {"normal", `["10.0.0.0/8"]`, "192.0.2.1:9", "ip_address_denied", 403}} {
		_, err := db.Q().ExecContext(context.Background(), "UPDATE tenant SET mode=?,ip_allowlist=?", tc.mode, tc.allow)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		a.Middleware(next).ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Errorf("got %d %s", w.Code, w.Body.String())
		}
	}
}

func TestMiddlewareSessionCSRF(t *testing.T) {
	db, clock := pf05TestDB(t)
	a := NewAuthenticator(db, clock)
	cookie, csrf, err := a.CreateSession(context.Background(), db.Q(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := FromContext(r.Context())
		if p.Via != "session" {
			t.Errorf("via=%q", p.Via)
		}
		w.WriteHeader(204)
	})
	for _, tc := range []struct {
		method string
		header string
		want   int
	}{{"GET", "", 204}, {"POST", "", 403}, {"POST", csrf, 204}} {
		r := httptest.NewRequest(tc.method, "/", nil)
		r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		if tc.header != "" {
			r.Header.Set(CSRFHeader, tc.header)
		}
		w := httptest.NewRecorder()
		a.Middleware(next).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s csrf=%q got=%d", tc.method, tc.header, w.Code)
		}
	}
}

func TestMiddlewareUserStatusAndLastUsedDate(t *testing.T) {
	for _, tc := range []struct {
		name, user, invitation string
		status                 int
		code                   string
	}{
		{"invited valid", "inv", "future", 401, "user_not_registered"},
		{"invited expired", "inv", "past", 401, "invitation_expired"},
		{"invited without invitation", "inv", "none", 401, "invitation_expired"},
		{"suspended", "sus", "none", 401, "user_inactive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, clock := pf05TestDB(t)
			if tc.invitation != "none" {
				expires := clock.Now().Add(time.Hour)
				if tc.invitation == "past" {
					expires = clock.Now().Add(-time.Hour)
				}
				_, err := db.Q().ExecContext(context.Background(), `INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES('i','inv','inv@example.test','ja',?,?,?)`, store.Millis(expires), store.Millis(clock.Now()), store.Millis(clock.Now()))
				if err != nil {
					t.Fatal(err)
				}
			}
			token := pf05InsertToken(t, db, "pat", tc.user)
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			NewAuthenticator(db, clock).Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}

	db, clock := pf05TestDB(t)
	_, err := db.Q().ExecContext(context.Background(), `CREATE TABLE pf05_last_used_updates(n INTEGER NOT NULL); INSERT INTO pf05_last_used_updates VALUES(0); CREATE TRIGGER pf05_count_last_used AFTER UPDATE OF last_used_on ON users BEGIN UPDATE pf05_last_used_updates SET n=n+1; END;`)
	if err != nil {
		t.Fatal(err)
	}
	token := pf05InsertToken(t, db, "pat", "u1")
	a := NewAuthenticator(db, clock)
	for range 2 {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
	var updates int
	if err := db.Q().QueryRowContext(context.Background(), "SELECT n FROM pf05_last_used_updates").Scan(&updates); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatalf("last_used_on updates=%d, want 1", updates)
	}
}

func TestMiddlewareDeletesIdleExpiredSession(t *testing.T) {
	db, clock := pf05TestDB(t)
	a := NewAuthenticator(db, clock)
	cookie, _, err := a.CreateSession(context.Background(), db.Q(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(13 * time.Hour)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
	if w.Code != 401 || !strings.Contains(w.Body.String(), "invalid_access_token") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var count int
	if err := db.Q().QueryRowContext(context.Background(), "SELECT count(*) FROM sessions").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired sessions remaining=%d", count)
	}
}
