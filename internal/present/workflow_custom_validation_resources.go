package present

import (
	"context"

	"punchflow/internal/store"
)

func pf08CustomValidationResources(ctx context.Context, q store.Querier, masterCode, itemCode, gradeCode, teamCode string) (any, any, any) {
	var masterItem, grade, team any
	if itemCode != "" && masterCode != "" {
		masterID, err := store.ResolveID(ctx, q, "general_masters", masterCode)
		if err == nil {
			if itemID, itemErr := store.MasterItemIDByCode(ctx, q, masterID, itemCode); itemErr == nil {
				if item, itemErr := store.GetGeneralMasterItem(ctx, q, itemID); itemErr == nil {
					masterItem = map[string]any{"id": item.ID, "code": item.Code, "name": item.Name, "description": item.Description, "createdAt": pf08Time(item.CreatedAt), "updatedAt": pf08Time(item.UpdatedAt), "startsOn": item.StartsOn, "endsOn": item.EndsOn}
				}
			}
		}
	}
	if gradeCode != "" {
		if gradeID, err := store.GradeIDByCode(ctx, q, gradeCode); err == nil {
			if value, err := store.GetGrade(ctx, q, gradeID); err == nil {
				grade = Grade(value)
			}
		}
	}
	if teamCode != "" {
		if chartID, err := store.ActiveOrgChartID(ctx, q); err == nil {
			if teamID, err := store.TeamIDByCode(ctx, q, chartID, teamCode); err == nil {
				team, _ = Team(ctx, q, teamID)
			}
		}
	}
	return masterItem, grade, team
}
