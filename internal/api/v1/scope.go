package v1

import (
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// actorFor converts the authenticated principal into an engine actor. A
// restricted ticket_read grant is not exposed as a global permission; the
// engine receives its folder scope instead.
func actorFor(p *auth.Principal) engine.Actor {
	actor := engine.Actor{UserID: p.User.ID, Permissions: make(map[string]bool, len(p.Permissions))}
	for permission, held := range p.Permissions {
		if !held {
			continue
		}
		if permission == model.PermissionTicketRead && !p.Unrestricted(permission) {
			continue
		}
		actor.Permissions[permission] = true
	}
	if p.Permissions[model.PermissionTicketRead] && !p.Unrestricted(model.PermissionTicketRead) {
		actor.ReadFolderIDs = make(map[string]bool)
		for _, id := range p.Targets(model.PermissionTicketRead) {
			actor.ReadFolderIDs[id] = true
		}
	}
	return actor
}

// errOutOfScope is returned when a restricted permission does not cover the
// resource being accessed.
func errOutOfScope() error { return kferr.MissingPermission("Missing permission") }

// requireTarget returns errOutOfScope unless one of the permissions covers target.
func requireTarget(p *auth.Principal, target string, permissions ...string) error {
	if !p.AllowsAny(target, permissions...) {
		return errOutOfScope()
	}
	return nil
}

// requireUnrestricted returns errOutOfScope unless the principal holds one of
// the permissions without a target restriction.
func requireUnrestricted(p *auth.Principal, permissions ...string) error {
	for _, permission := range permissions {
		if p.Unrestricted(permission) {
			return nil
		}
	}
	return errOutOfScope()
}

// principalOf returns the authenticated principal of the request; handlers call
// it only after auth.Require succeeded.
func principalOf(r *http.Request) *auth.Principal {
	p, _ := auth.FromContext(r.Context())
	return p
}

// guardMaster writes a 403 and returns false when the principal's master
// permissions do not cover the general master. Write operations need the master
// permission; reads accept master_read as well.
func (a *API) guardMaster(w http.ResponseWriter, r *http.Request, masterID string, write bool) bool {
	permissions := []string{model.PermissionMasterRead, model.PermissionMaster}
	if write {
		permissions = []string{model.PermissionMaster}
	}
	if err := requireTarget(principalOf(r), masterID, permissions...); err != nil {
		kferr.Write(w, err)
		return false
	}
	return true
}

// masterListFilter limits general master lists to the masters the principal may read.
func masterListFilter(p *auth.Principal) store.IDFilter {
	if p.Unrestricted(model.PermissionMasterRead) || p.Unrestricted(model.PermissionMaster) {
		return store.IDFilter{}
	}
	ids := append(p.Targets(model.PermissionMasterRead), p.Targets(model.PermissionMaster)...)
	return store.RestrictedTo(ids)
}

// guardTeam writes a 403 and returns false when the principal's team
// permission does not cover the team.
func (a *API) guardTeam(w http.ResponseWriter, r *http.Request, teamID string) bool {
	if err := requireTarget(principalOf(r), teamID, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return false
	}
	return true
}

// requireTeamParent checks that a team may be placed under parent. Creating or
// moving a team to the root requires an unrestricted team permission.
func requireTeamParent(p *auth.Principal, parent *string) error {
	if parent == nil {
		return requireUnrestricted(p, model.PermissionTeam)
	}
	return requireTarget(p, *parent, model.PermissionTeam)
}

// teamListFilter limits team lists to the teams the principal may manage.
func teamListFilter(p *auth.Principal) store.IDFilter {
	if p.Unrestricted(model.PermissionTeam) {
		return store.IDFilter{}
	}
	return store.RestrictedTo(p.Targets(model.PermissionTeam))
}
