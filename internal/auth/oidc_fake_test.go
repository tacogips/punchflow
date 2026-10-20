package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

const oidcTestClientID = "punchflow-test-client"

// pf17FakeIssuer implements only the OIDC discovery, JWKS, and code exchange used by tests.
type pf17FakeIssuer struct {
	t         *testing.T
	server    *httptest.Server
	key       *rsa.PrivateKey
	mu        sync.Mutex
	challenge string
	claims    pf17IDClaims
}

type pf17IDClaims struct {
	Email string
	Nonce string
	Aud   string
	Exp   time.Time
}

func pf17NewFakeIssuer(t *testing.T) *pf17FakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &pf17FakeIssuer{t: t, key: key, claims: pf17IDClaims{Email: "one@example.test", Aud: oidcTestClientID, Exp: time.Now().Add(time.Hour)}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("/jwks", f.jwks)
	mux.HandleFunc("/token", f.token)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *pf17FakeIssuer) discovery(w http.ResponseWriter, _ *http.Request) {
	f.writeJSON(w, map[string]any{
		"issuer": f.server.URL, "authorization_endpoint": f.server.URL + "/authorize",
		"token_endpoint": f.server.URL + "/token", "jwks_uri": f.server.URL + "/jwks",
		"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (f *pf17FakeIssuer) jwks(w http.ResponseWriter, _ *http.Request) {
	e := big.NewInt(int64(f.key.PublicKey.E)).Bytes()
	f.writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "pf17-test-key",
		"n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(e),
	}}})
}

func (f *pf17FakeIssuer) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	challenge, claims := f.challenge, f.claims
	f.mu.Unlock()
	digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if r.Form.Get("code") != "valid-code" || r.Form.Get("code_verifier") == "" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
		http.Error(w, "invalid code or PKCE verifier", http.StatusBadRequest)
		return
	}
	idToken, err := f.signIDToken(claims)
	if err != nil {
		http.Error(w, "unable to sign test token", http.StatusInternalServerError)
		return
	}
	f.writeJSON(w, map[string]any{"access_token": "test-access", "token_type": "Bearer", "id_token": idToken, "expires_in": 3600})
}

func (f *pf17FakeIssuer) signIDToken(claims pf17IDClaims) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "pf17-test-key", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"iss": f.server.URL, "sub": "test-subject", "aud": claims.Aud,
		"iat": time.Now().Unix(), "exp": claims.Exp.Unix(), "nonce": claims.Nonce, "email": claims.Email,
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (f *pf17FakeIssuer) setChallenge(value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.challenge = value
}

func (f *pf17FakeIssuer) setClaims(update func(*pf17IDClaims)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(&f.claims)
}

func (f *pf17FakeIssuer) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		f.t.Errorf("encode fake OIDC response: %v", err)
	}
}

func pf17ChallengeFromURL(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method=%q, want S256", got)
	}
	if u.Query().Has("code_verifier") {
		t.Fatal("PKCE verifier leaked into authorization URL")
	}
	challenge := u.Query().Get("code_challenge")
	if challenge == "" {
		t.Fatal("authorization URL has no code challenge")
	}
	return challenge
}

func pf17OIDCConfig(issuer string) OIDCConfig {
	return OIDCConfig{Issuer: issuer, ClientID: oidcTestClientID, RedirectURL: "http://punchflow.test/auth/oidc/callback"}
}
