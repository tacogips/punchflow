package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"punchflow/internal/store"
)

func pf17NewOIDCTestFlow(t *testing.T, email string) (*pf17FakeIssuer, *store.DB, *http.ServeMux, *httptest.ResponseRecorder, string, string) {
	t.Helper()
	db, clock := pf05TestDB(t)
	issuer := pf17NewFakeIssuer(t)
	a := NewAuthenticator(db, clock)
	oidc, err := NewOIDC(context.Background(), pf17OIDCConfig(issuer.server.URL), a, db, clock, true)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	a.Register(mux, RoutesOptions{OIDC: oidc, SecureCookie: true})
	login := httptest.NewRecorder()
	mux.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?returnTo=%2Fdashboard", nil))
	if login.Code != http.StatusFound {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	state := login.Result().Header.Get("Location")
	challenge := pf17ChallengeFromURL(t, state)
	loginURL, err := url.Parse(state)
	if err != nil {
		t.Fatal(err)
	}
	stateValue := loginURL.Query().Get("state")
	if stateValue == "" {
		t.Fatal("authorization URL has no state")
	}
	var nonce string
	if err := db.Q().QueryRowContext(context.Background(), "SELECT nonce FROM oidc_states WHERE state=?", stateValue).Scan(&nonce); err != nil {
		t.Fatal(err)
	}
	issuer.setChallenge(challenge)
	issuer.setClaims(func(claims *pf17IDClaims) {
		claims.Email = email
		claims.Nonce = nonce
	})
	return issuer, db, mux, login, stateValue, nonce
}

func pf17Callback(t *testing.T, mux *http.ServeMux, state string) *httptest.ResponseRecorder {
	t.Helper()
	callback := httptest.NewRecorder()
	mux.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=valid-code&state="+url.QueryEscape(state), nil))
	return callback
}

func TestOIDCLoginCallbackSessionSuccess(t *testing.T) {
	_, _, mux, _, state, _ := pf17NewOIDCTestFlow(t, "ONE@example.test")
	callback := pf17Callback(t, mux, state)
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/dashboard" {
		t.Fatalf("callback status=%d location=%q body=%s", callback.Code, callback.Header().Get("Location"), callback.Body.String())
	}
	cookies := callback.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v, want one session cookie", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookie || cookie.Value == "" || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie=%+v", cookie)
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	sessionRequest.AddCookie(cookie)
	session := httptest.NewRecorder()
	mux.ServeHTTP(session, sessionRequest)
	if session.Code != http.StatusOK || !strings.Contains(session.Body.String(), `"email":"one@example.test"`) {
		t.Fatalf("session status=%d body=%s", session.Code, session.Body.String())
	}
}

func TestOIDCUnknownExpiredAndReplayedState(t *testing.T) {
	unknown := httptest.NewRecorder()
	unknownMux := http.NewServeMux()
	db, clock := pf05TestDB(t)
	issuer := pf17NewFakeIssuer(t)
	a := NewAuthenticator(db, clock)
	oidc, err := NewOIDC(context.Background(), pf17OIDCConfig(issuer.server.URL), a, db, clock, false)
	if err != nil {
		t.Fatal(err)
	}
	unknownMux.HandleFunc("GET /auth/oidc/callback", oidc.Callback)
	unknownMux.HandleFunc("GET /auth/oidc/login", oidc.Login)
	unknownMux.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=unknown", nil))
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown state status=%d", unknown.Code)
	}

	login := httptest.NewRecorder()
	unknownMux.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil))
	if login.Code != http.StatusFound {
		t.Fatalf("expired-state setup login status=%d", login.Code)
	}
	loginURL, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	expiredState := loginURL.Query().Get("state")
	clock.Advance(11 * time.Minute)
	expired := pf17Callback(t, unknownMux, expiredState)
	if expired.Code != http.StatusBadRequest {
		t.Fatalf("expired state status=%d", expired.Code)
	}

	_, _, replayMux, _, replayState, _ := pf17NewOIDCTestFlow(t, "one@example.test")
	first := pf17Callback(t, replayMux, replayState)
	second := pf17Callback(t, replayMux, replayState)
	if first.Code != http.StatusFound || second.Code != http.StatusBadRequest {
		t.Fatalf("replay statuses first=%d second=%d", first.Code, second.Code)
	}
}

func TestOIDCRejectsInvalidTokenClaimsAndUsers(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(*pf17FakeIssuer)
		wantBody string
	}{
		{name: "nonce", setup: func(f *pf17FakeIssuer) { f.setClaims(func(c *pf17IDClaims) { c.Nonce = "wrong" }) }},
		{name: "audience", setup: func(f *pf17FakeIssuer) { f.setClaims(func(c *pf17IDClaims) { c.Aud = "another-client" }) }},
		{name: "expired token", setup: func(f *pf17FakeIssuer) { f.setClaims(func(c *pf17IDClaims) { c.Exp = time.Now().Add(-time.Minute) }) }},
		{name: "unknown email", setup: func(f *pf17FakeIssuer) { f.setClaims(func(c *pf17IDClaims) { c.Email = "missing@example.test" }) }, wantBody: "no punchflow user for missing@example.test"},
		{name: "suspended user", setup: func(f *pf17FakeIssuer) { f.setClaims(func(c *pf17IDClaims) { c.Email = "sus@example.test" }) }, wantBody: "user is not eligible to log in"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issuer, _, mux, _, state, _ := pf17NewOIDCTestFlow(t, "one@example.test")
			tc.setup(issuer)
			callback := pf17Callback(t, mux, state)
			if callback.Code != http.StatusForbidden {
				t.Fatalf("callback status=%d body=%s", callback.Code, callback.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(callback.Body.String(), tc.wantBody) {
				t.Fatalf("callback body=%q, want substring %q", callback.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestOIDCInvitedUserActivatesWithValidInvitation(t *testing.T) {
	issuer, db, mux, _, state, _ := pf17NewOIDCTestFlow(t, "inv@example.test")
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	// The fake clock used by pf05TestDB is the same fixed instant; invitation remains valid for one hour.
	_, err := db.Q().ExecContext(context.Background(), `INSERT INTO user_invitations(id,user_id,user_email,user_locale,expires_at,created_at,updated_at) VALUES('pf17-inv','inv','inv@example.test','ja',?,?,?)`, store.Millis(now.Add(time.Hour)), store.Millis(now), store.Millis(now))
	if err != nil {
		t.Fatal(err)
	}
	issuer.setClaims(func(c *pf17IDClaims) { c.Email = "inv@example.test" })
	callback := pf17Callback(t, mux, state)
	if callback.Code != http.StatusFound {
		t.Fatalf("callback status=%d body=%s", callback.Code, callback.Body.String())
	}
	var status string
	if err := db.Q().QueryRowContext(context.Background(), "SELECT status FROM users WHERE id='inv'").Scan(&status); err != nil || status != "activated" {
		t.Fatalf("user status=%q err=%v", status, err)
	}
}

func TestOIDCOpenRedirectTargetFallsBackToRoot(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantPath string
	}{
		{name: "protocol-relative", query: "%2F%2Fevil.example", wantPath: "/"},
		{name: "backslash-host", query: "%2F%5Cevil.example", wantPath: "/"},
		{name: "backslash-separator", query: "%2F%5C%2Fevil.example", wantPath: "/"},
		{name: "tab-control", query: "%2F%09%2Fevil.example", wantPath: "/"},
		{name: "absolute-url", query: "https%3A%2F%2Fevil.example", wantPath: "/"},
		{name: "legitimate-path-with-query", query: "%2Fdashboard%3Fx%3D1", wantPath: "/dashboard?x=1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issuer, db, mux, _, _, _ := pf17NewOIDCTestFlow(t, "one@example.test")
			login := httptest.NewRecorder()
			mux.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?returnTo="+tc.query, nil))
			if login.Code != http.StatusFound {
				t.Fatalf("login status=%d", login.Code)
			}
			location := login.Header().Get("Location")
			challenge := pf17ChallengeFromURL(t, location)
			parsed, err := url.Parse(location)
			if err != nil {
				t.Fatal(err)
			}
			state := parsed.Query().Get("state")
			issuer.setChallenge(challenge)
			var nonce string
			var returnTo string
			if err := db.Q().QueryRowContext(context.Background(), "SELECT nonce,return_to FROM oidc_states WHERE state=?", state).Scan(&nonce, &returnTo); err != nil {
				t.Fatal(err)
			}
			if returnTo != tc.wantPath {
				t.Fatalf("returnTo=%q, want %q", returnTo, tc.wantPath)
			}
			issuer.setClaims(func(claims *pf17IDClaims) { claims.Nonce = nonce })
			callback := pf17Callback(t, mux, state)
			if callback.Code != http.StatusFound || callback.Header().Get("Location") != tc.wantPath {
				t.Fatalf("callback status=%d location=%q, want %q; body=%s", callback.Code, callback.Header().Get("Location"), tc.wantPath, callback.Body.String())
			}
		})
	}
}

func TestOIDCEmptyIssuerRoutesNotFound(t *testing.T) {
	db, clock := pf05TestDB(t)
	a := NewAuthenticator(db, clock)
	oidc, err := NewOIDC(context.Background(), OIDCConfig{}, a, db, clock, false)
	if err != nil || oidc != nil {
		t.Fatalf("NewOIDC empty issuer=(%v,%v), want (nil,nil)", oidc, err)
	}
	mux := http.NewServeMux()
	a.Register(mux, RoutesOptions{OIDC: oidc})
	for _, path := range []string{"/auth/oidc/login", "/auth/oidc/callback"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "endpoint_not_found") {
			t.Errorf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
