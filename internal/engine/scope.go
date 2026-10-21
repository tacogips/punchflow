package engine

import (
	"context"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

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
