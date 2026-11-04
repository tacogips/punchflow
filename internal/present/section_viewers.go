package present

import (
	"context"
	"errors"
	"fmt"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// sectionViewerID returns the fixture-provided viewer ID or a stable UUID
// derived from the section and the viewer's target.
func sectionViewerID(s model.WorkflowSection, v model.SectionViewer, index int) string {
	if v.ID != "" {
		return v.ID
	}
	return stableUUID(s.ID, fmt.Sprint(index), v.UserCode, v.TeamCode, v.GradeCode)
}

// sectionViewersJSON presents the viewers of a restricted form section. When
// stamps is non-nil the ticket variant (with createdAt/updatedAt) is produced.
func sectionViewersJSON(ctx context.Context, q store.Querier, s model.WorkflowSection, stamps *time.Time) ([]any, error) {
	out := make([]any, 0, len(s.SectionViewers))
	for i, v := range s.SectionViewers {
		var user, team, grade any
		if v.UserCode != "" {
			if id, err := store.UserIDByCode(ctx, q, v.UserCode); err == nil {
				if user, err = User(ctx, q, id, UserOpts{}); err != nil {
					return nil, err
				}
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		if v.TeamCode != "" {
			chart, err := store.ActiveOrgChartID(ctx, q)
			if err != nil {
				return nil, err
			}
			if id, err := store.TeamIDByCode(ctx, q, chart, v.TeamCode); err == nil {
				if team, err = Team(ctx, q, id); err != nil {
					return nil, err
				}
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		if v.GradeCode != "" {
			if id, err := store.GradeIDByCode(ctx, q, v.GradeCode); err == nil {
				m, err := store.GetGrade(ctx, q, id)
				if err != nil {
					return nil, err
				}
				grade = Grade(m)
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		item := map[string]any{"id": sectionViewerID(s, v, i), "descendants": v.Descendants, "user": user, "team": team, "grade": grade}
		if stamps != nil {
			item["createdAt"] = pf08Time(*stamps)
			item["updatedAt"] = pf08Time(*stamps)
		}
		out = append(out, item)
	}
	return out, nil
}
