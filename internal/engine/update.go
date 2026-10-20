package engine

import (
	"context"
	"encoding/json"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (e *Engine) pf14Update(ctx context.Context, tx *store.Tx, a Actor, id string, in UpdateInput) ([]model.WebhookEvent, error) {
	return e.pf14cUpdate(ctx, tx, a, id, in, false)
}

func (e *Engine) pf14cUpdate(ctx context.Context, tx *store.Tx, a Actor, id string, in UpdateInput, partial bool) ([]model.WebhookEvent, error) {
	t, err := e.pf14Guard(ctx, tx, a, id)
	if err != nil {
		return nil, err
	}
	author := t.AuthorID != nil && *t.AuthorID == a.UserID
	approver := false
	if t.Status == model.TicketStatusInProgress {
		_, _, err = e.pf14CanAct(ctx, tx, a, t)
		if err == nil {
			approver = true
		} else {
			return nil, err
		}
	}
	if !approver && !author {
		return nil, kferr.MissingPermission("Only the author or current approver can update this ticket")
	}
	if t.Status != model.TicketStatusDraft && t.Status != model.TicketStatusRejected && t.Status != model.TicketStatusInProgress {
		return nil, kferr.InvalidState("Ticket cannot be updated")
	}
	if approver && in.Status != nil {
		return nil, kferr.InvalidState("Approvers cannot change ticket status")
	}
	def, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return nil, err
	}
	now := e.clock.Now()
	hasUpdate := false
	if !approver {
		if in.Title != nil {
			t.Title = in.Title
			hasUpdate = true
		}
		if in.AuthorTeamID != nil {
			team, er := store.GetTeam(ctx, tx, *in.AuthorTeamID)
			if er != nil {
				return nil, kferr.ValidationFailed(map[string][]string{"authorTeamId": {"is invalid"}})
			}
			member, er := store.EngUserIsActiveMember(ctx, tx, a.UserID, team.ID)
			if er != nil {
				return nil, er
			}
			if !member {
				return nil, kferr.ValidationFailed(map[string][]string{"authorTeamId": {"is invalid"}})
			}
			t.AuthorTeamID = &team.ID
			name, er := store.TeamFullName(ctx, tx, team.ID)
			if er != nil {
				return nil, er
			}
			t.AuthorTeamSnapshot, _ = json.Marshal(map[string]string{"id": team.ID, "name": team.Name, "fullName": name, "code": team.Code})
			hasUpdate = true
		}
	}
	if len(in.ProxyClientUserID) > 0 && !approver {
		var proxy *string
		if string(in.ProxyClientUserID) != "null" {
			if err = json.Unmarshal(in.ProxyClientUserID, &proxy); err != nil {
				return nil, kferr.InvalidParameter("Invalid proxy client")
			}
		}
		if proxy != nil {
			if err = pf14cValidateProxyClient(ctx, tx, a.UserID, *proxy, t.WorkflowID, kftime.FormatDate(now)); err != nil {
				return nil, err
			}
		}
		t.ProxyClientUserID = proxy
		hasUpdate = true
	}
	if in.Inputs != nil || in.SlipItems != nil {
		inputValues := []FieldInput{}
		slipValues := []SlipItemInput{}
		if in.Inputs != nil {
			if approver {
				if err = pf14cValidateApproverPatch(def, *in.Inputs); err != nil {
					return nil, err
				}
				existing, er := store.EngTicketInputs(ctx, tx, id)
				if er != nil {
					return nil, er
				}
				inputValues = pf14cMergeInputs(def, pf14cInputsFromRows(def, existing), *in.Inputs)
			} else if partial {
				existing, er := store.EngTicketInputs(ctx, tx, id)
				if er != nil {
					return nil, er
				}
				inputValues = pf14cMergeInputs(def, pf14cInputsFromRows(def, existing), *in.Inputs)
			} else {
				if err = pf14ValidateCreateCompleteness(def, *in.Inputs); err != nil {
					return nil, err
				}
				inputValues = *in.Inputs
			}
		} else {
			existing, er := store.EngTicketInputs(ctx, tx, id)
			if er != nil {
				return nil, er
			}
			if partial {
				inputValues = pf14cInputsFromRows(def, existing)
			} else {
				inputValues = pf14InputsFromRows(def, existing)
			}
		}
		if in.SlipItems != nil {
			slipValues = *in.SlipItems
		}
		full := in.Status != nil && *in.Status == model.TicketStatusInProgress
		inputs, slips, er := pf14NormalizeForm(ctx, tx, id, def, inputValues, slipValues, full, store.Millis(now))
		if er != nil {
			return nil, er
		}
		if err = store.EngReplaceInputs(ctx, tx, id, inputs); err != nil {
			return nil, err
		}
		if in.SlipItems != nil {
			if err = store.EngReplaceSlipItems(ctx, tx, id, slips); err != nil {
				return nil, err
			}
		}
		hasUpdate = true
	}
	if in.Approvers != nil {
		if !approver {
			return nil, kferr.MissingPermission("Only current approvers may customize assignees")
		}
		if t.RouteID == nil {
			return nil, kferr.InvalidState("Ticket route is unavailable")
		}
		route, er := store.RouteDefByID(ctx, tx, *t.RouteID)
		if er != nil {
			return nil, er
		}
		ticketSteps, er := store.EngTicketSteps(ctx, tx, id)
		if er != nil {
			return nil, er
		}
		for _, v := range *in.Approvers {
			validStep := false
			for _, routeStep := range route.Steps {
				if routeStep.Code == v.RouteStepCode && routeStep.StepType == model.StepTypeAssigneeCustomizable {
					validStep = true
					break
				}
			}
			var step *model.TicketStep
			for i := range ticketSteps {
				if ticketSteps[i].RouteStepCode != nil && *ticketSteps[i].RouteStepCode == v.RouteStepCode {
					step = &ticketSteps[i]
					break
				}
			}
			if !validStep || step == nil {
				return nil, kferr.InvalidParameter("Invalid customizable route step")
			}
			if err = store.EngInsertCustomApprovers(ctx, tx, id, v.RouteStepCode, v.UserIDs); err != nil {
				return nil, err
			}
			existing, er := store.EngStepAssignees(ctx, tx, step.ID)
			if er != nil {
				return nil, er
			}
			present := map[string]bool{}
			for _, assignee := range existing {
				present[assignee.UserID] = true
			}
			for _, userID := range v.UserIDs {
				if _, er = store.GetUser(ctx, tx, userID); er != nil {
					return nil, kferr.ValidationFailed(map[string][]string{"approvers": {"contains an unknown user"}})
				}
				if present[userID] {
					continue
				}
				assignee := model.TicketAssignee{ID: store.NewID(), TicketStepID: step.ID, TicketID: id, UserID: userID, CreatedAt: now, UpdatedAt: now}
				if err = store.EngInsertAssignee(ctx, tx, assignee); err != nil {
					return nil, err
				}
				present[userID] = true
			}
		}
		hasUpdate = true
	}
	if in.Status != nil {
		if *in.Status != model.TicketStatusInProgress {
			return nil, kferr.InvalidState("Only submission is supported")
		}
		if t.Status != model.TicketStatusDraft && t.Status != model.TicketStatusRejected {
			return nil, kferr.InvalidState("Ticket cannot be submitted")
		}
		inputs, er := store.EngTicketInputs(ctx, tx, id)
		if er != nil {
			return nil, er
		}
		inputValues := pf14InputsFromRows(def, inputs)
		normalized, _, er := pf14NormalizeForm(ctx, tx, id, def, inputValues, nil, true, store.Millis(now))
		if er != nil {
			return nil, er
		}
		if err = store.EngReplaceInputs(ctx, tx, id, normalized); err != nil {
			return nil, err
		}
		if err = store.EngDeleteTicketSteps(ctx, tx, id); err != nil {
			return nil, err
		}
		approvers := []ApproverInput{}
		if in.Approvers != nil {
			approvers = *in.Approvers
		}
		if err = e.pf14BuildRoute(ctx, tx, &t, def, inputValues, approvers, store.Millis(now)); err != nil {
			return nil, err
		}
		if t.TicketNumber == nil && def.TicketNumberingTiming == "opened" {
			if err = pf14Number(ctx, tx, &t, def, now); err != nil {
				return nil, err
			}
		}
		t.Status = model.TicketStatusInProgress
		if t.OpenedAt == nil {
			t.OpenedAt = &now
		}
		t.UpdatedAt = now
		if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
			return nil, err
		}
		evs := []model.WebhookEvent{}
		if hasUpdate {
			evs = append(evs, pf14Event(model.WebhookEventTicketUpdated, id, a.UserID))
		}
		evs = append(evs, pf14Event(model.WebhookEventTicketOpened, id, a.UserID))
		return evs, nil
	}
	if hasUpdate {
		t.UpdatedAt = now
		if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
			return nil, err
		}
		return []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, id, a.UserID)}, nil
	}
	return []model.WebhookEvent{}, nil
}

func (e *Engine) pf14UpdateForm(ctx context.Context, tx *store.Tx, a Actor, id string, in FormPatch) ([]model.WebhookEvent, error) {
	if len(in.Inputs) == 0 && len(in.SlipItems) == 0 {
		return nil, kferr.InvalidParameter("Form patch must contain inputs or slip items")
	}
	update := UpdateInput{}
	if len(in.Inputs) > 0 {
		update.Inputs = &in.Inputs
	}
	var slipPatch *[]SlipItemInput
	if len(in.SlipItems) > 0 {
		slipPatch = &in.SlipItems
	}
	update.SlipItems = slipPatch
	return e.pf14cUpdate(ctx, tx, a, id, update, true)
}

func pf14cValidateApproverPatch(def model.WorkflowDef, patch []FieldInput) error {
	if def.Overwritable {
		return nil
	}
	fieldsByID := map[string]model.FormField{}
	fieldsByCode := map[string]model.FormField{}
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			fieldsByID[field.ID] = field
			fieldsByCode[field.Code] = field
		}
	}
	for _, input := range patch {
		field, found := fieldsByID[input.FormFieldID]
		if !found {
			field, found = fieldsByCode[input.FormFieldCode]
		}
		if found && !field.Approver {
			return kferr.InvalidParameter("Invalid form field code")
		}
	}
	return nil
}

func pf14cInputsFromRows(def model.WorkflowDef, rows []model.TicketInput) []FieldInput {
	fields := map[string]model.FormField{}
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			fields[field.ID] = field
		}
	}
	out := make([]FieldInput, 0, len(rows))
	for _, row := range rows {
		field := fields[row.FormFieldID]
		raw, err := json.Marshal(row.Value)
		if err != nil {
			raw = json.RawMessage("null")
		}
		input := FieldInput{FormFieldID: row.FormFieldID, FormFieldCode: field.Code, Value: raw}
		if len(row.RefIDs) > 0 {
			refValue := any(row.RefIDs)
			if len(row.RefIDs) == 1 && !field.Multiple {
				refValue = row.RefIDs[0]
			}
			refs, marshalErr := json.Marshal(refValue)
			if marshalErr == nil {
				switch field.FieldType {
				case "master":
					input.GeneralMasterItemID = refs
				case "user":
					input.UserID = refs
				case "team":
					input.TeamID = refs
				case "ticket":
					input.TicketID = refs
				case "file":
					input.Files = append([]string(nil), row.RefIDs...)
				}
			}
		}
		out = append(out, input)
	}
	return out
}

func pf14cMergeInputs(def model.WorkflowDef, stored, patch []FieldInput) []FieldInput {
	fieldIDByCode := map[string]string{}
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			fieldIDByCode[field.Code] = field.ID
		}
	}
	merged := make([]FieldInput, 0, len(stored)+len(patch))
	index := map[string]int{}
	key := func(in FieldInput) string {
		if in.FormFieldID != "" {
			return "id:" + in.FormFieldID
		}
		if id := fieldIDByCode[in.FormFieldCode]; id != "" {
			return "id:" + id
		}
		return "code:" + in.FormFieldCode
	}
	for _, input := range stored {
		index[key(input)] = len(merged)
		merged = append(merged, input)
	}
	for _, input := range patch {
		k := key(input)
		if i, ok := index[k]; ok {
			merged[i] = input
			continue
		}
		index[k] = len(merged)
		merged = append(merged, input)
	}
	return merged
}

func pf14InputsFromRows(def model.WorkflowDef, rows []model.TicketInput) []FieldInput {
	byID := map[string]string{}
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			byID[field.ID] = field.Code
		}
	}
	out := []FieldInput{}
	for _, row := range rows {
		raw, _ := json.Marshal(row.Value)
		out = append(out, FieldInput{FormFieldID: row.FormFieldID, FormFieldCode: byID[row.FormFieldID], Value: raw})
	}
	return out
}
