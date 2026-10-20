package engine

import (
	"context"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf14WorkflowViewer(ctx context.Context, q store.Querier, a Actor, ticket model.Ticket, def model.WorkflowDef) (bool, error) {
	if ticket.AuthorTeamID != nil && def.VisibleToTeamMembers {
		ok, err := store.EngUserIsActiveMember(ctx, q, a.UserID, *ticket.AuthorTeamID)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	for _, viewer := range def.TicketViewers {
		if viewer.UserCode != "" {
			id, err := store.UserIDByCode(ctx, q, viewer.UserCode)
			if err == nil && id == a.UserID {
				return true, nil
			}
		}
		if viewer.TeamCode != "" {
			chart, err := store.ActiveOrgChartID(ctx, q)
			if err != nil {
				return false, err
			}
			team, err := store.TeamIDByCode(ctx, q, chart, viewer.TeamCode)
			if err == nil {
				ids, err := store.EngUsersByTeam(ctx, q, team, viewer.Descendants)
				if err != nil {
					return false, err
				}
				if pf14Contains(ids, a.UserID) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
