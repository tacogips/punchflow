package model

import "time"

type APIToken struct {
	ID               string     `json:"id"`
	Kind             string     `json:"kind"`
	Name             string     `json:"name"`
	Description      *string    `json:"description"`
	TokenHash        string     `json:"-"`
	UserID           *string    `json:"userId"`
	FixturePlaintext *string    `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	LastUsedAt       *time.Time `json:"lastUsedAt"`
}
type Session struct {
	IDHash     string    `json:"-"`
	UserID     string    `json:"userId"`
	CSRFToken  string    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}
type OIDCState struct {
	State        string    `json:"-"`
	Nonce        string    `json:"-"`
	CodeVerifier string    `json:"-"`
	ReturnTo     string    `json:"-"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
