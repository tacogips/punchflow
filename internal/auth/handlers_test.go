package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"punchflow/internal/store"
)

func TestHandlersLocalLoginAndOIDCDisabled(t *testing.T) {
	db, clock := pf05TestDB(t)
	a := NewAuthenticator(db, clock)
	mux := http.NewServeMux()
	a.Register(mux, RoutesOptions{})
	disabled := httptest.NewRecorder()
	mux.ServeHTTP(disabled, httptest.NewRequest("POST", "/auth/local/login", strings.NewReader(`{"email":"one@example.test"}`)))
	if disabled.Code != 404 {
		t.Fatalf("disabled local login status=%d", disabled.Code)
	}
	oidc := httptest.NewRecorder()
	mux.ServeHTTP(oidc, httptest.NewRequest("GET", "/auth/oidc/login", nil))
	if oidc.Code != 404 {
		t.Fatalf("disabled OIDC status=%d", oidc.Code)
	}
	mux = http.NewServeMux()
	a.Register(mux, RoutesOptions{EnableLocalLogin: true, SecureCookie: true})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("POST", "/auth/local/login", strings.NewReader(`{"email":"one@example.test"}`)))
	if response.Code != 200 {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.HasSuffix(response.Body.String(), "\n") {
		t.Fatal("session response must not have a trailing newline")
	}
	cookie := response.Result().Cookies()[0]
	if cookie.Name != SessionCookie || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("cookie=%+v", cookie)
	}
	var body struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
		AuthMethods struct {
			Local bool `json:"local"`
		} `json:"authMethods"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.User.Email != "one@example.test" || !body.AuthMethods.Local {
		t.Fatalf("body=%+v", body)
	}
	logout := httptest.NewRecorder()
	mux.ServeHTTP(logout, httptest.NewRequest("POST", "/auth/logout", nil))
	if logout.Code != 200 || len(logout.Result().Cookies()) == 0 || logout.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout=%d cookie=%+v", logout.Code, logout.Result().Cookies())
	}
}

func TestHandlersLocalLoginActivatesInvitedUser(t *testing.T) {
	db, clock := pf05TestDB(t)
	now := clock.Now()
	_, err := db.Q().ExecContext(context.Background(), `INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES('i','inv','inv@example.test','ja',?,?,?)`, store.Millis(now.Add(time.Hour)), store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuthenticator(db, clock)
	mux := http.NewServeMux()
	a.Register(mux, RoutesOptions{EnableLocalLogin: true})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/auth/local/login", strings.NewReader(`{"email":"inv@example.test"}`)))
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var status string
	if err := db.Q().QueryRowContext(context.Background(), "SELECT status FROM users WHERE id='inv'").Scan(&status); err != nil || status != "activated" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestHandlersEmptyOIDCConfig(t *testing.T) {
	oidc, err := NewOIDC(context.Background(), OIDCConfig{}, nil, nil, nil, false)
	if err != nil || oidc != nil {
		t.Fatalf("NewOIDC() = (%v, %v), want (nil, nil)", oidc, err)
	}
}
