package present

import (
	"context"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"sort"
)

func Route(ctx context.Context, q store.Querier, id string) (RouteJSON, error) {
	m, e := store.GetRoute(ctx, q, id)
	if e != nil {
		return nil, e
	}
	d, e := store.RouteDefByID(ctx, q, id)
	if e != nil {
		return nil, e
	}
	folder, e := Folder(ctx, q, m.FolderID)
	if e != nil {
		return nil, e
	}
	return RouteJSON{"id": m.ID, "code": m.Code, "status": m.Status, "versionId": m.ID, "versionNumber": 1, "name": d.Name, "description": d.Description, "notes": pf08NullableString(d.Notes), "current": true, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "versionCreatedAt": pf08Time(m.CreatedAt), "folder": folder, "author": nil}, nil
}
func RouteDetail(ctx context.Context, q store.Querier, id string) (RouteDetailJSON, error) {
	base, e := Route(ctx, q, id)
	if e != nil {
		return nil, e
	}
	d, e := store.RouteDefByID(ctx, q, id)
	if e != nil {
		return nil, e
	}
	steps := append([]model.RouteStep{}, d.Steps...)
	pf08SortRouteSteps(steps)
	outSteps := []any{}
	for _, s := range steps {
		users, e := pf08UsersByCode(ctx, q, s.UserCodes)
		if e != nil {
			return nil, e
		}
		targets := []any{}
		for _, t := range s.Targets {
			var team any
			if t.TeamCode != "" {
				if tid, er := store.TeamIDByCode(ctx, q, pf08ActiveChartID(ctx, q), t.TeamCode); er == nil {
					team, er = Team(ctx, q, tid)
					if er != nil {
						return nil, er
					}
				}
			}
			grades := []any{}
			for _, code := range t.GradeCodes {
				if id, er := store.ResolveID(ctx, q, "grades", code); er == nil {
					g, er := store.GetGrade(ctx, q, id)
					if er != nil {
						return nil, er
					}
					grades = append(grades, Grade(g))
				}
			}
			targets = append(targets, map[string]any{"team": team, "descendants": t.Descendants, "gradeSymbol": t.GradeSymbol, "grades": grades, "variable": t.Variable})
		}
		var condition any
		if s.RouteStepCondition != nil {
			fields := []any{}
			for _, f := range s.RouteStepCondition.RouteStepConditionFields {
				fields = append(fields, pf08RouteStepConditionField(ctx, q, f))
			}
			condition = map[string]any{"id": s.RouteStepCondition.ID, "conditionType": s.RouteStepCondition.ConditionType, "combinationType": s.RouteStepCondition.CombinationType, "routeStepConditionFields": fields}
		}
		subStatus, err := subStatusByCode(ctx, q, s.SubStatus)
		if err != nil {
			return nil, err
		}
		outSteps = append(outSteps, map[string]any{"id": s.ID, "stepOrder": s.StepOrder, "stepType": s.StepType, "title": s.Title, "actionType": s.ActionType, "instruction": s.Instruction, "requiredApprovalsNumber": s.RequiredApprovalsNumber, "requiredApprovalsPercent": s.RequiredApprovalsPercent, "fallbackType": s.FallbackType, "allowSelfApproval": s.AllowSelfApproval, "managerSelectionType": s.ManagerSelectionType, "allowRaising": s.AllowRaising, "minCustomAssignees": s.MinCustomAssignees, "approverAssignmentInstruction": s.ApproverAssignmentInstruction, "users": users, "code": s.Code, "targets": targets, "routeStepCondition": condition, "subStatus": subStatus})
	}
	out := RouteDetailJSON(base)
	out["steps"] = outSteps
	out["versionAuthor"] = nil
	return out, nil
}
func pf08UsersByCode(ctx context.Context, q store.Querier, codes []string) ([]any, error) {
	out := []any{}
	for _, code := range codes {
		id, e := store.UserIDByCode(ctx, q, code)
		if e != nil {
			if e == store.ErrNotFound {
				continue
			}
			return nil, e
		}
		u, e := User(ctx, q, id, UserOpts{})
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, nil
}
func pf08SortRouteSteps(s []model.RouteStep) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].StepOrder < s[j].StepOrder })
}

func pf08NullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func pf08ActiveChartID(ctx context.Context, q store.Querier) string {
	id, _ := store.ActiveOrgChartID(ctx, q)
	return id
}

func pf08RouteStepConditionField(ctx context.Context, q store.Querier, f model.ConditionField) map[string]any {
	out := map[string]any{"id": f.ID, "variable": pf08NullableString(f.Variable), "fieldKey": f.FieldKey, "symbol": f.Symbol, "value": f.Value, "grade": nil, "team": nil, "generalMaster": nil, "generalMasterItem": nil}
	code, _ := f.Value.(string)
	if f.FieldKey == "general_master_variable" {
		masterCode := f.GeneralMasterCode
		if masterCode == "" {
			masterCode = code
		}
		if masterCode != "" {
			if id, e := store.ResolveID(ctx, q, "general_masters", masterCode); e == nil {
				if master, e := store.GetGeneralMaster(ctx, q, id); e == nil {
					count, countErr := store.CountMasterItems(ctx, q, master.ID)
					if countErr == nil {
						out["generalMaster"] = map[string]any{"id": master.ID, "code": master.Code, "name": master.Name, "description": master.Description, "defaultSortBy": master.DefaultSortBy, "itemsCount": count, "initialDisplayCode": master.InitialDisplayCode, "initialDisplayCreatedAt": master.InitialDisplayCreatedAt, "initialDisplayDescription": master.InitialDisplayDescription, "createdAt": pf08Time(master.CreatedAt), "updatedAt": pf08Time(master.UpdatedAt)}
					}
					if f.GeneralMasterItemCode != "" {
						var itemID string
						if e := q.QueryRowContext(ctx, `SELECT id FROM general_master_items WHERE general_master_id=? AND code=?`, master.ID, f.GeneralMasterItemCode).Scan(&itemID); e == nil {
							if item, e := GeneralMasterItem(ctx, q, itemID); e == nil {
								out["generalMasterItem"] = item
							}
						}
					}
				}
			}
		}
		out["value"] = nil
	}
	if code != "" {
		switch f.FieldKey {
		case "author_grade":
			if id, e := store.ResolveID(ctx, q, "grades", code); e == nil {
				if x, e := store.GetGrade(ctx, q, id); e == nil {
					out["grade"] = Grade(x)
				}
			}
		case "author_team":
			if chart, e := store.ActiveOrgChartID(ctx, q); e == nil {
				if id, e := store.TeamIDByCode(ctx, q, chart, code); e == nil {
					if x, e := Team(ctx, q, id); e == nil {
						out["team"] = x
					}
				}
			}
		}
	}
	return out
}
