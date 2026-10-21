package auth

import (
	"context"
	"net/http"
	"sort"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
)

type pf05PrincipalContextKey struct{}

// Principal is the authenticated identity and effective authorization context.
type Principal struct {
	User model.User
	// Permissions lists every permission held through any role, restricted or
	// not. Use Grants (or the helper methods) to honor restricted scopes.
	Permissions map[string]bool
	// Grants carries the merged scope of each permission. A nil map means every
	// held permission is unrestricted (used by tests that build principals by hand).
	Grants    map[string]model.PermissionGrant
	Via       string
	TokenID   string
	CSRFToken string
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

// Unrestricted reports whether permission is held without a target restriction.
func (p *Principal) Unrestricted(permission string) bool {
	if p == nil || !p.Permissions[permission] {
		return false
	}
	if p.Grants == nil {
		return true
	}
	return p.Grants[permission].Unrestricted
}

// Allows reports whether the permission covers the target (a folder, general
// master or team ID, depending on the permission). Unrestricted holders cover
// every target.
func (p *Principal) Allows(permission, targetID string) bool {
	if p == nil || !p.Permissions[permission] {
		return false
	}
	if p.Grants == nil {
		return true
	}
	return p.Grants[permission].Allows(targetID)
}

// AllowsAny reports whether any of the permissions covers the target.
func (p *Principal) AllowsAny(targetID string, permissions ...string) bool {
	for _, permission := range permissions {
		if p.Allows(permission, targetID) {
			return true
		}
	}
	return false
}

// Targets returns the explicit target IDs of a restricted permission in stable
// order. It is meaningful only when Unrestricted(permission) is false.
func (p *Principal) Targets(permission string) []string {
	if p == nil || p.Grants == nil {
		return nil
	}
	out := make([]string, 0, len(p.Grants[permission].Targets))
	for id := range p.Grants[permission].Targets {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
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
