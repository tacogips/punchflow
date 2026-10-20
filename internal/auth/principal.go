package auth

import (
	"context"
	"net/http"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
)

type pf05PrincipalContextKey struct{}

// Principal is the authenticated identity and effective authorization context.
type Principal struct {
	User        model.User
	Permissions map[string]bool
	Via         string
	TokenID     string
	CSRFToken   string
}

// Has reports whether any requested permission is held.
func (p *Principal) Has(perms ...string) bool {
	if p == nil {
		return false
	}
	for _, permission := range perms {
		if p.Permissions[permission] {
			return true
		}
	}
	return false
}

// FromContext returns the authenticated principal stored in ctx.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(pf05PrincipalContextKey{}).(*Principal)
	return p, ok && p != nil
}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, pf05PrincipalContextKey{}, p)
}

// Require checks that the request principal has at least one requested permission.
func Require(r *http.Request, perms ...string) error {
	p, ok := FromContext(r.Context())
	if !ok {
		return kferr.InvalidAccessToken()
	}
	if len(perms) > 0 && !p.Has(perms...) {
		return kferr.MissingPermission("Missing permission")
	}
	return nil
}
