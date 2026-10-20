package auth

import (
	"context"
	"fmt"
	"net/http"

	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

// OIDCConfig configures the identity provider used by the browser login flow.
type OIDCConfig struct{ Issuer, ClientID, ClientSecret, RedirectURL string }

// OIDC is replaced with the provider-backed implementation by PF-17.
type OIDC struct{}

// NewOIDC returns nil when OIDC is disabled; PF-17 supplies the enabled implementation.
func NewOIDC(_ context.Context, cfg OIDCConfig, _ *Authenticator, _ *store.DB, _ kftime.Clock, _ bool) (*OIDC, error) {
	if cfg.Issuer == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("auth: OIDC implementation is provided by PF-17")
}

// Login is the PF-17 OIDC login seam.
func (o *OIDC) Login(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }

// Callback is the PF-17 OIDC callback seam.
func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
