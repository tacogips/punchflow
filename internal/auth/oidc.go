package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// OIDCConfig configures the identity provider used by the browser login flow.
type OIDCConfig struct{ Issuer, ClientID, ClientSecret, RedirectURL string }

// OIDC implements the browser-facing authorization-code flow as a BFF.
type OIDC struct {
	authenticator *Authenticator
	db            *store.DB
	clock         kftime.Clock
	secureCookie  bool
	provider      *oidc.Provider
	config        oauth2.Config
	verifier      *oidc.IDTokenVerifier
}

// NewOIDC returns nil when OIDC is disabled; otherwise it discovers and configures the issuer.
func NewOIDC(ctx context.Context, cfg OIDCConfig, authenticator *Authenticator, db *store.DB, clock kftime.Clock, secureCookie bool) (*OIDC, error) {
	if cfg.Issuer == "" {
		return nil, nil
	}
	if authenticator == nil || db == nil {
		return nil, fmt.Errorf("auth: OIDC requires authenticator and database")
	}
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: discover OIDC issuer: %w", err)
	}
	return &OIDC{
		authenticator: authenticator,
		db:            db,
		clock:         clock,
		secureCookie:  secureCookie,
		provider:      provider,
		config: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

// Login stores a single-use state and redirects to the issuer using PKCE S256.
func (o *OIDC) Login(w http.ResponseWriter, r *http.Request) {
	state, err := oidcRandomValue(32)
	if err != nil {
		o.oidcError(w, http.StatusInternalServerError, "unable to start login")
		return
	}
	nonce, err := oidcRandomValue(32)
	if err != nil {
		o.oidcError(w, http.StatusInternalServerError, "unable to start login")
		return
	}
	verifier := oauth2.GenerateVerifier()
	returnTo := oidcSafeReturnTo(r.URL.Query().Get("returnTo"))
	if err := store.InsertOIDCState(r.Context(), o.db.Q(), model.OIDCState{
		State: state, Nonce: nonce, CodeVerifier: verifier, ReturnTo: returnTo,
		ExpiresAt: o.clock.Now().Add(10 * time.Minute),
	}); err != nil {
		o.oidcError(w, http.StatusInternalServerError, "unable to start login")
		return
	}
	authURL := o.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func oidcSafeReturnTo(raw string) string {
	if len(raw) == 0 || raw[0] != '/' {
		return "/"
	}
	if len(raw) > 1 && (raw[1] == '/' || raw[1] == '\\') || strings.ContainsRune(raw, '\\') {
		return "/"
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f {
			return "/"
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "/"
	}
	return raw
}

// Callback consumes state, validates the issuer response, then creates a browser session.
func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		o.oidcError(w, http.StatusBadRequest, "identity provider rejected login")
		return
	}
	state, err := store.TakeOIDCState(r.Context(), o.db.Q(), r.URL.Query().Get("state"))
	if err != nil || !o.clock.Now().Before(state.ExpiresAt) {
		o.oidcError(w, http.StatusBadRequest, "invalid or expired login state")
		return
	}
	token, err := o.config.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(state.CodeVerifier))
	if err != nil {
		o.oidcError(w, http.StatusForbidden, "identity token validation failed")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		o.oidcError(w, http.StatusForbidden, "identity token validation failed")
		return
	}
	idToken, err := o.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		o.oidcError(w, http.StatusForbidden, "identity token validation failed")
		return
	}
	var claims struct {
		Email string `json:"email"`
		Nonce string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Email == "" || claims.Nonce != state.Nonce {
		o.oidcError(w, http.StatusForbidden, "identity token claims are invalid")
		return
	}
	user, err := store.UserByEmail(r.Context(), o.db.Q(), claims.Email)
	if err != nil {
		if err == store.ErrNotFound {
			o.oidcError(w, http.StatusForbidden, "no punchflow user for "+claims.Email)
		} else {
			o.oidcError(w, http.StatusInternalServerError, "unable to resolve user")
		}
		return
	}
	user, err = o.authenticator.ActivateOnLogin(r.Context(), o.db.Q(), user)
	if err != nil {
		o.oidcError(w, http.StatusForbidden, "user is not eligible to log in")
		return
	}
	cookie, _, err := o.authenticator.CreateSession(r.Context(), o.db.Q(), user.ID)
	if err != nil {
		o.oidcError(w, http.StatusInternalServerError, "unable to create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: o.secureCookie})
	http.Redirect(w, r, state.ReturnTo, http.StatusFound)
}

func oidcRandomValue(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("auth: generate OIDC random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (o *OIDC) oidcError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><html><body><p>%s</p></body></html>", htmlEscape(message))
}

func htmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&#34;", "'", "&#39;").Replace(value)
}
