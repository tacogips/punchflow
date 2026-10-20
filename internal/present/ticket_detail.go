package present

import (
	"context"
	"fmt"
	"reflect"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

type TicketDetailJSON map[string]any

func TicketDetail(ctx context.Context, q store.Querier, id string) (TicketDetailJSON, error) {
	ticket, err := Ticket(ctx, q, id)
	if err != nil {
		return nil, err
	}
	x, err := store.Ticket(ctx, q, id)
	if err != nil {
		return nil, err
	}
	wf, err := WorkflowInTicket(ctx, q, x.WorkflowID)
	if err != nil {
		return nil, err
	}
	var route any
	if x.RouteID != nil {
		route, err = RouteDetail(ctx, q, *x.RouteID)
		if err != nil && err != store.ErrNotFound {
			return nil, fmt.Errorf("present ticket route: %w", err)
		}
	}
	inputs, err := store.TicketInputs(ctx, q, id)
	if err != nil {
		return nil, err
	}
	def, err := store.WorkflowDefByID(ctx, q, x.WorkflowID)
	if err != nil {
		return nil, err
	}
	inputMaps := make([]any, 0, len(inputs))
	values := map[string]any{}
	for _, in := range inputs {
		values[in.FormFieldID] = in.Value
		var field any
		var fieldModel *model.FormField
		for _, section := range def.SectionList {
			for _, f := range section.FormFields {
				if f.ID == in.FormFieldID {
					field = pf08FormField(ctx, q, f)
					fieldModel = &f
					values[f.Code] = in.Value
				}
			}
		}
		if field == nil {
			field = map[string]any{"id": in.FormFieldID}
		}
		general := []any{}
		users := []any{}
		teams := []any{}
		tickets := []any{}
		files := []any{}
		refs := pf15ReferenceIDs(in.RefIDs, in.Value)
		if fieldModel != nil {
			switch fieldModel.FieldType {
			case "master", "general_master", "general_master_item":
				resolved, e := GeneralMasterItems(ctx, q, refs)
				if e != nil {
					return nil, e
				}
				for _, v := range resolved {
					general = append(general, v)
				}
			case "user":
				resolved, e := Users(ctx, q, refs, UserOpts{})
				if e != nil {
					return nil, e
				}
				for _, v := range resolved {
					users = append(users, v)
				}
			case "team":
				resolved, e := store.GetTeams(ctx, q, refs)
				if e != nil {
					return nil, e
				}
				for _, v := range resolved {
					team, e := Team(ctx, q, v.ID)
					if e != nil {
						return nil, e
					}
					teams = append(teams, team)
				}
			case "ticket":
				resolved, e := Tickets(ctx, q, refs)
				if e != nil {
					return nil, e
				}
				for _, v := range resolved {
					tickets = append(tickets, v)
				}
			}
		}
		attachments, e := store.TicketAttachments(ctx, q, id, nil, &in.FormFieldID)
		if e != nil {
			return nil, e
		}
		for _, f := range attachments {
			files = append(files, attachmentJSON(f))
		}
		inputMaps = append(inputMaps, map[string]any{"id": in.ID, "formFieldId": in.FormFieldID, "value": in.Value, "formField": field, "generalMasterItems": general, "users": users, "teams": teams, "inputTickets": tickets, "attachments": files})
	}
	sections := make([]any, 0, len(def.SectionList))
	for _, s := range def.SectionList {
		sectionInputs := []any{}
		for i, in := range inputs {
			for _, f := range s.FormFields {
				if f.ID == in.FormFieldID {
					sectionInputs = append(sectionInputs, inputMaps[i])
				}
			}
		}
		visible := true
		if s.Conditional {
			visible = pf15SectionVisible(s, values)
		}
		sections = append(sections, map[string]any{"id": s.ID, "sectionId": s.ID, "visible": visible, "viewRestriction": false, "allowViewAuthor": true, "allowViewApprover": true, "viewable": true, "inputs": sectionInputs, "sectionViewers": []any{}})
	}
	slipRows, err := store.TicketSlipItems(ctx, q, id)
	if err != nil {
		return nil, err
	}
	slips := make([]any, 0, len(slipRows))
	for _, row := range slipRows {
		var section *model.WorkflowSection
		for i := range def.SectionList {
			if def.SectionList[i].ID == row.SlipSectionID {
				section = &def.SectionList[i]
				break
			}
		}
		slipInputs := []any{}
		for fieldID, value := range row.Inputs {
			var field *model.SlipField
			if section != nil {
				for i := range section.SlipFields {
					if section.SlipFields[i].ID == fieldID || section.SlipFields[i].Code == fieldID {
						field = &section.SlipFields[i]
						break
					}
				}
			}
			fieldIDOut := fieldID
			if field != nil {
				fieldIDOut = field.ID
			}
			general, users, teams, relatedTickets := []any{}, []any{}, []any{}, []any{}
			refs := pf15ReferenceIDs(nil, value)
			if field != nil {
				switch field.FieldType {
				case "master", "general_master", "general_master_item":
					resolved, e := GeneralMasterItems(ctx, q, refs)
					if e != nil {
						return nil, e
					}
					for _, v := range resolved {
						general = append(general, v)
					}
				case "user":
					resolved, e := Users(ctx, q, refs, UserOpts{})
					if e != nil {
						return nil, e
					}
					for _, v := range resolved {
						users = append(users, v)
					}
				case "team":
					resolved, e := store.GetTeams(ctx, q, refs)
					if e != nil {
						return nil, e
					}
					for _, v := range resolved {
						team, e := Team(ctx, q, v.ID)
						if e != nil {
							return nil, e
						}
						teams = append(teams, team)
					}
				case "ticket":
					resolved, e := Tickets(ctx, q, refs)
					if e != nil {
						return nil, e
					}
					for _, v := range resolved {
						relatedTickets = append(relatedTickets, v)
					}
				}
			}
			slipInputs = append(slipInputs, map[string]any{"id": fieldID, "slipFieldId": fieldIDOut, "value": value, "createdAt": pf08Time(row.CreatedAt), "updatedAt": pf08Time(row.UpdatedAt), "generalMasterItems": general, "users": users, "teams": teams, "inputTickets": relatedTickets})
		}
		slips = append(slips, map[string]any{"id": row.ID, "slipSectionId": row.SlipSectionID, "position": row.Position, "createdAt": pf08Time(row.CreatedAt), "updatedAt": pf08Time(row.UpdatedAt), "inputs": slipInputs})
	}
	steps, err := ticketStepJSON(ctx, q, id, true)
	if err != nil {
		return nil, err
	}
	out := TicketDetailJSON(ticket)
	out["workflow"] = wf
	out["route"] = route
	out["ticketGuest"] = nil
	out["triggerTicket"] = nil
	out["nextTickets"] = []any{}
	out["slipItems"] = slips
	out["ticketSections"] = sections
	out["inputs"] = inputMaps
	out["cloudSignDocument"] = nil
	out["steps"] = steps
	out["customNumberingValues"] = []any{}
	return out, nil
}

func pf15ReferenceIDs(refs []string, value any) []string {
	if len(refs) > 0 {
		return append([]string{}, refs...)
	}
	ids := []string{}
	switch v := value.(type) {
	case string:
		if v != "" {
			ids = append(ids, v)
		}
	case []any:
		for _, item := range v {
			if id, ok := item.(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func pf15SectionVisible(s model.WorkflowSection, values map[string]any) bool {
	if !s.Conditional {
		return true
	}
	results := make([]bool, 0, len(s.ConditionFields))
	for _, c := range s.ConditionFields {
		actual := values[c.FieldKey]
		switch c.Symbol {
		case "equal":
			results = append(results, reflect.DeepEqual(actual, c.Value))
		case "not_equal":
			results = append(results, !reflect.DeepEqual(actual, c.Value))
		case "include":
			results = append(results, pf15Includes(actual, c.Value))
		case "exclude":
			results = append(results, !pf15Includes(actual, c.Value))
		case "is_empty":
			results = append(results, actual == nil || (reflect.TypeOf(actual) == reflect.TypeOf("") && actual.(string) == ""))
		case "is_not_empty":
			results = append(results, actual != nil && !(reflect.TypeOf(actual) == reflect.TypeOf("") && actual.(string) == ""))
		default:
			results = append(results, true)
		}
	}
	if len(results) == 0 {
		return true
	}
	if s.CombinationType == model.StepCombinationAny {
		for _, v := range results {
			if v {
				return true
			}
		}
		return false
	}
	for _, v := range results {
		if !v {
			return false
		}
	}
	return true
}
func pf15Includes(actual, want any) bool {
	switch xs := actual.(type) {
	case []any:
		for _, x := range xs {
			if reflect.DeepEqual(x, want) {
				return true
			}
		}
	case string:
		if w, ok := want.(string); ok {
			return containsString(xs, w)
		}
	}
	return false
}
func containsString(value, part string) bool {
	if part == "" {
		return true
	}
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
