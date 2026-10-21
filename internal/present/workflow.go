package present

import (
	"context"
	"encoding/json"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"strconv"
)

func Workflow(ctx context.Context, q store.Querier, id string) (WorkflowJSON, error) {
	m, e := store.GetWorkflow(ctx, q, id)
	if e != nil {
		return nil, e
	}
	d, e := store.WorkflowDefByID(ctx, q, id)
	if e != nil {
		return nil, e
	}
	folder, e := Folder(ctx, q, m.FolderID)
	if e != nil {
		return nil, e
	}
	var next any
	if d.TicketNumberFormat != nil {
		next = m.TicketNumberCounter + 1
	}
	out := WorkflowJSON{"id": m.ID, "code": m.Code, "versionId": m.ID, "versionNumber": 1, "name": d.Name, "description": d.Description, "status": m.Status, "publicTicket": d.PublicTicket, "visibleToManager": d.VisibleToManager, "visibleToTeamMembers": d.VisibleToTeamMembers, "titleDescription": d.TitleDescription, "ticketNumberFormat": d.TicketNumberFormat, "overwritable": d.Overwritable, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "titleInputMode": d.TitleInputMode, "titleFormula": d.TitleFormula, "availableToEveryone": d.AvailableToEveryone, "reportFormats": pf08EmptyStrings(d.ReportFormats), "hiddenOnWorkflowFilterForTicket": d.HiddenOnWorkflowFilterForTicket, "hiddenOnWorkflowSelectionScreen": d.HiddenOnWorkflowSelectionScreen, "approvalCancellable": d.ApprovalCancellable, "reportFileNameFormat": d.ReportFileNameFormat, "current": true, "notes": d.Notes, "versionCreatedAt": pf08Time(m.CreatedAt), "collectEmailOnExternalPublish": d.CollectEmailOnExternalPublish, "notifyGuestOnCompletion": d.NotifyGuestOnCompletion, "allowCustomSteps": d.AllowCustomSteps, "commentingRequiredOnApproval": d.CommentingRequiredOnApproval, "commentingRequiredOnRejection": d.CommentingRequiredOnRejection, "commentingRequiredOnDenial": d.CommentingRequiredOnDenial, "allowEditingOfViewers": d.AllowEditingOfViewers, "commentingEnabled": d.CommentingEnabled, "nextTicketNumberValue": next, "nextTicketNumberValueByKey": nil, "author": nil, "versionAuthor": nil, "externalPublish": nil, "folder": folder, "categories": pf08Categories(ctx, q, d.CategoryCodes)}
	_ = folder
	return out, nil
}
func WorkflowInTicket(ctx context.Context, q store.Querier, id string) (WorkflowInTicketJSON, error) {
	w, e := Workflow(ctx, q, id)
	if e != nil {
		return nil, e
	}
	out := WorkflowInTicketJSON(w)
	d, err := store.WorkflowDefByID(ctx, q, id)
	if err != nil {
		return nil, err
	}
	sections, err := pf08Sections(ctx, q, d.SectionList)
	if err != nil {
		return nil, err
	}
	out["sectionList"] = sections
	out["ticketViewers"] = pf08TicketViewers(ctx, q, pf08WorkflowViewers(ctx, q, id))
	out["cloudSignSetting"] = nil
	out["ticketNumberKey"] = nil
	out["excelTemplate"] = nil
	out["customNumberings"] = []any{}
	out["customNumberingSettings"] = []any{}
	return out, nil
}
func WorkflowDetail(ctx context.Context, q store.Querier, id string) (WorkflowDetailJSON, error) {
	base, e := WorkflowInTicket(ctx, q, id)
	if e != nil {
		return nil, e
	}
	d, e := store.WorkflowDefByID(ctx, q, id)
	if e != nil {
		return nil, e
	}
	sections, e := pf08Sections(ctx, q, d.SectionList)
	if e != nil {
		return nil, e
	}
	formFields := pf08FormFieldsByCode(d.SectionList)
	conditions := []any{}
	for _, c := range d.RouteConditions {
		var route any
		if c.RouteCode != nil {
			rid, er := store.RouteIDByCode(ctx, q, *c.RouteCode)
			if er == nil {
				route, er = Route(ctx, q, rid)
			}
			if er != nil && er != store.ErrNotFound {
				return nil, er
			}
		}
		fields := []any{}
		for _, f := range c.ConditionFields {
			fields = append(fields, pf08WorkflowConditionField(ctx, q, f, formFields))
		}
		conditions = append(conditions, map[string]any{"id": c.ID, "conditionType": c.ConditionType, "combinationType": c.CombinationType, "combinationExpression": c.CombinationExpression, "conditionFields": fields, "route": route, "errorMessage": c.ErrorMessage})
	}
	out := WorkflowDetailJSON(base)
	out["sectionList"] = sections
	out["routeConditions"] = conditions
	out["availableTeams"] = pf08AvailableTeams(ctx, q, d.AvailableTeams)
	out["watchers"] = pf08Watchers(ctx, q, d.Watchers)
	out["workflowFilterVisibilities"] = []any{}
	numberingSubStatus, err := subStatusByCode(ctx, q, d.TicketNumberingSubStatus)
	if err != nil {
		return nil, err
	}
	out["ticketNumberingTiming"] = map[string]any{"id": nil, "timingType": d.TicketNumberingTiming, "subStatus": numberingSubStatus}
	out["slackChannel"] = nil
	out["chatworkRoom"] = nil
	out["microsoftTeamsChannel"] = nil
	out["googleChatSetting"] = nil
	out["reportUploadGoogleDrive"] = nil
	out["reportUploadBox"] = nil
	return out, nil
}
func pf08EmptyStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func pf08Categories(ctx context.Context, q store.Querier, codes []string) []any {
	out := []any{}
	for _, code := range codes {
		id, e := store.CategoryIDByCode(ctx, q, code)
		if e != nil {
			continue
		}
		c, e := store.GetCategory(ctx, q, id)
		if e == nil {
			out = append(out, Category(c))
		}
	}
	return out
}
func pf08WorkflowViewers(ctx context.Context, q store.Querier, id string) []model.WorkflowViewer {
	d, e := store.WorkflowDefByID(ctx, q, id)
	if e != nil {
		return []model.WorkflowViewer{}
	}
	if d.TicketViewers == nil {
		return []model.WorkflowViewer{}
	}
	return d.TicketViewers
}

// pf08FormFieldsByCode indexes a workflow's form fields by code; condition fields reference them through Variable.
func pf08FormFieldsByCode(sections []model.WorkflowSection) map[string]model.FormField {
	out := map[string]model.FormField{}
	for _, s := range sections {
		for _, f := range s.FormFields {
			out[f.Code] = f
		}
	}
	return out
}

// pf08ConditionValue renders a stored threshold as the string-or-null value kickflow returns.
func pf08ConditionValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		raw, err := json.Marshal(x)
		if err != nil {
			return nil
		}
		return string(raw)
	}
}

func pf08WorkflowConditionField(ctx context.Context, q store.Querier, f model.ConditionField, formFields map[string]model.FormField) map[string]any {
	out := map[string]any{"id": f.ID, "symbol": f.Symbol, "value": pf08ConditionValue(f.Value), "formField": nil, "grade": nil, "team": nil, "generalMasterItem": nil}
	field, ok := formFields[f.Variable]
	if !ok {
		field, ok = formFields[model.VariableName(f.Variable)]
	}
	if ok {
		out["formField"] = pf08FormField(ctx, q, field)
	}
	code, _ := f.Value.(string)
	if code != "" {
		switch f.FieldKey {
		case "grade":
			if id, e := store.ResolveID(ctx, q, "grades", code); e == nil {
				if g, e := store.GetGrade(ctx, q, id); e == nil {
					out["grade"] = Grade(g)
				}
			}
		case "team":
			if chart, e := store.ActiveOrgChartID(ctx, q); e == nil {
				if id, e := store.TeamIDByCode(ctx, q, chart, code); e == nil {
					if x, e := Team(ctx, q, id); e == nil {
						out["team"] = x
					}
				}
			}
		case "general_master_item":
			if id, e := store.ResolveID(ctx, q, "general_master_items", code); e == nil {
				if x, e := GeneralMasterItem(ctx, q, id); e == nil {
					out["generalMasterItem"] = x
				}
			}
		}
	}
	return out
}

func pf08AvailableTeams(ctx context.Context, q store.Querier, values []model.WorkflowTeam) []any {
	out := []any{}
	for _, v := range values {
		var team any
		if id, e := store.TeamIDByCode(ctx, q, pf08ActiveChartID(ctx, q), v.TeamCode); e == nil {
			team, _ = Team(ctx, q, id)
		}
		grades := []any{}
		for _, code := range v.GradeCodes {
			if id, e := store.GradeIDByCode(ctx, q, code); e == nil {
				if g, e := store.GetGrade(ctx, q, id); e == nil {
					grades = append(grades, Grade(g))
				}
			}
		}
		out = append(out, map[string]any{"team": team, "descendants": v.Descendants, "gradeSymbol": v.GradeSymbol, "grades": grades})
	}
	return out
}
func pf08Watchers(ctx context.Context, q store.Querier, values []model.WorkflowWatcher) []any {
	out := []any{}
	for _, v := range values {
		var user, team any
		if v.UserCode != "" {
			if id, e := store.UserIDByCode(ctx, q, v.UserCode); e == nil {
				user, _ = User(ctx, q, id, UserOpts{})
			}
		}
		if v.TeamCode != "" {
			if id, e := store.TeamIDByCode(ctx, q, pf08ActiveChartID(ctx, q), v.TeamCode); e == nil {
				team, _ = Team(ctx, q, id)
			}
		}
		out = append(out, map[string]any{"id": v.ID, "watcherType": v.WatcherType, "user": user, "team": team})
	}
	return out
}
func pf08TicketViewers(ctx context.Context, q store.Querier, values []model.WorkflowViewer) []any {
	out := []any{}
	for _, v := range values {
		var user, team, grade any
		if v.UserCode != "" {
			if id, e := store.UserIDByCode(ctx, q, v.UserCode); e == nil {
				user, _ = User(ctx, q, id, UserOpts{})
			}
		}
		if v.TeamCode != "" {
			if id, e := store.TeamIDByCode(ctx, q, pf08ActiveChartID(ctx, q), v.TeamCode); e == nil {
				team, _ = Team(ctx, q, id)
			}
		}
		if v.GradeCode != "" {
			if id, e := store.GradeIDByCode(ctx, q, v.GradeCode); e == nil {
				if g, e := store.GetGrade(ctx, q, id); e == nil {
					grade = Grade(g)
				}
			}
		}
		out = append(out, map[string]any{"id": v.ID, "user": user, "team": team, "grade": grade, "descendants": v.Descendants})
	}
	return out
}
