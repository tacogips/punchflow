package engine

import (
	"context"

	"punchflow/internal/auth"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// ActorFromPrincipal preserves resource scopes when converting an authenticated
// principal for ticket engine operations.
func ActorFromPrincipal(p *auth.Principal) Actor {
	actor := Actor{UserID: p.User.ID, Permissions: map[string]bool{}}
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
		for id := range p.Grants[model.PermissionTicketRead].Targets {
			actor.ReadFolderIDs[id] = true
		}
	}
	return actor
}

// ActorForUser loads a user's effective permissions and folder scopes for
// background visibility checks such as notification delivery.
func ActorForUser(ctx context.Context, q store.Querier, userID string) (Actor, error) {
	permissions, err := store.UserPermissions(ctx, q, userID)
	if err != nil {
		return Actor{}, err
	}
	grants, err := store.UserGrants(ctx, q, userID)
	if err != nil {
		return Actor{}, err
	}
	return ActorFromPrincipal(&auth.Principal{User: model.User{ID: userID}, Permissions: permissions, Grants: grants}), nil
}

// ticketReadAllowed reports whether the actor's ticket_read permission covers
// tickets of the workflow: an unrestricted grant covers every ticket, while a
// restricted grant covers workflows whose folder is within the granted folders.
func ticketReadAllowed(ctx context.Context, q store.Querier, a Actor, workflowID string) (bool, error) {
	if a.Permissions[model.PermissionTicketRead] {
		return true, nil
	}
	if len(a.ReadFolderIDs) == 0 {
		return false, nil
	}
	wf, err := store.GetWorkflow(ctx, q, workflowID)
	if err != nil {
		return false, err
	}
	return a.ReadFolderIDs[wf.FolderID], nil
}
