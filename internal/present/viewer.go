package present

import (
	"context"
	"fmt"

	"punchflow/internal/store"
)

type TicketViewerJSON map[string]any

func TicketViewers(ctx context.Context, q store.Querier, ids []string) ([]TicketViewerJSON, error) {
	out := []TicketViewerJSON{}
	for _, ticketID := range ids {
		rows, err := store.TicketViewers(ctx, q, ticketID)
		if err != nil {
			return nil, fmt.Errorf("present ticket viewers: %w", err)
		}
		for _, row := range rows {
			var user, team, grade any
			if row.UserID != nil {
				user, err = User(ctx, q, *row.UserID, UserOpts{})
				if err != nil {
					return nil, err
				}
			}
			if row.TeamID != nil {
				team, err = Team(ctx, q, *row.TeamID)
				if err != nil {
					return nil, err
				}
			}
			if row.GradeID != nil {
				gradeModel, e := store.GetGrade(ctx, q, *row.GradeID)
				if e != nil {
					return nil, e
				}
				grade = Grade(gradeModel)
			}
			out = append(out, TicketViewerJSON{"id": row.ID, "user": user, "team": team, "grade": grade, "descendants": row.Descendants})
		}
	}
	return out, nil
}
