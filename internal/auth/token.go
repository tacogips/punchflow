package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

const pf05Base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// HashToken returns the lowercase hexadecimal SHA-256 digest of token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewTokenValue creates a prefixed random API token.
func NewTokenValue(kind string) string {
	prefix := "pf_" + kind + "_"
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		panic("auth: crypto/rand unavailable: " + err.Error())
	}
	out := make([]byte, len(random))
	for i, b := range random {
		out[i] = pf05Base62[int(b)%len(pf05Base62)]
	}
	return prefix + string(out)
}

// IssueToken stores only a digest and returns its plaintext once.
func (a *Authenticator) IssueToken(ctx context.Context, q store.Querier, kind, userID, name, description string) (string, string, error) {
	if kind != model.TokenKindPAT && kind != model.TokenKindSAT {
		return "", "", fmt.Errorf("auth: unsupported token kind %q", kind)
	}
	plain := NewTokenValue(kind)
	id := store.NewID()
	created := a.clock.Now()
	t := model.APIToken{ID: id, Kind: kind, Name: name, TokenHash: HashToken(plain), CreatedAt: created}
	if description != "" {
		t.Description = &description
	}
	if kind == model.TokenKindPAT {
		t.UserID = &userID
	}
	if err := store.InsertToken(ctx, q, t); err != nil {
		return "", "", fmt.Errorf("auth: issue token: %w", err)
	}
	return plain, id, nil
}

// CreateSession returns a random cookie value and CSRF token while storing only the cookie digest.
func (a *Authenticator) CreateSession(ctx context.Context, q store.Querier, userID string) (string, string, error) {
	var cookieBytes, csrfBytes [32]byte
	if _, err := rand.Read(cookieBytes[:]); err != nil {
		return "", "", fmt.Errorf("auth: generate session cookie: %w", err)
	}
	if _, err := rand.Read(csrfBytes[:]); err != nil {
		return "", "", fmt.Errorf("auth: generate CSRF token: %w", err)
	}
	cookie := hex.EncodeToString(cookieBytes[:])
	csrf := hex.EncodeToString(csrfBytes[:])
	now := a.clock.Now()
	s := model.Session{IDHash: HashToken(cookie), UserID: userID, CSRFToken: csrf, CreatedAt: now, LastSeenAt: now}
	if err := store.InsertSession(ctx, q, s); err != nil {
		return "", "", fmt.Errorf("auth: create session: %w", err)
	}
	return cookie, csrf, nil
}
