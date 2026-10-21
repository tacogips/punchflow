package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	if err = pf14ValidateStatusRequest(t.Status, in.Status); err != nil {
		return nil, err
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
			if team.ApproveOnly {
				return nil, kferr.ValidationFailed(map[string][]string{"authorTeamId": {"is invalid"}})
			}
			applicantID := a.UserID
			if t.ProxyClientUserID != nil {
				applicantID = *t.ProxyClientUserID
			}
			if len(in.ProxyClientUserID) > 0 {
				if string(in.ProxyClientUserID) == "null" {
					applicantID = a.UserID
				} else {
					var requestedProxyClientID *string
					if er = json.Unmarshal(in.ProxyClientUserID, &requestedProxyClientID); er != nil {
						return nil, kferr.InvalidParameter("Invalid proxy client")
					}
					if requestedProxyClientID != nil {
						applicantID = *requestedProxyClientID
					}
				}
			}
			member, er := store.EngUserIsActiveMember(ctx, tx, applicantID, team.ID)
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
			if t.ProxyClientUserID != nil && *t.ProxyClientUserID == *proxy {
				err = pf14cValidateExistingProxyClient(ctx, tx, *proxy)
			} else {
				err = pf14cValidateProxyClient(ctx, tx, a.UserID, *proxy, t.WorkflowID, kftime.FormatDate(now))
			}
			if err != nil {
				return nil, err
			}
		}
		t.ProxyClientUserID = proxy
		hasUpdate = true
	}
	if !approver && t.ProxyClientUserID != nil {
		if err = pf14cValidateExistingProxyClient(ctx, tx, *t.ProxyClientUserID); err != nil {
			return nil, err
		}
	}
	if in.Inputs != nil || in.SlipItems != nil {
		if err = e.pf14cApplyFormEdit(ctx, tx, a, &t, def, in, partial, approver, now); err != nil {
			return nil, err
		}
		hasUpdate = true
	}
	if in.Approvers != nil {
		switch {
		case approver:
			if err = pf14cCustomizeAssignees(ctx, tx, t, *in.Approvers, now); err != nil {
				return nil, err
			}
		case author && (t.Status == model.TicketStatusDraft || t.Status == model.TicketStatusRejected):
			// Applicant nominations for author_customizable steps (R05/PF-04).
			if err = pf14SaveAuthorNominations(ctx, tx, id, def, *in.Approvers); err != nil {
				return nil, err
			}
		default:
			return nil, kferr.MissingPermission("Only the author or current approvers may customize assignees")
		}
		hasUpdate = true
	}
	if in.Viewers != nil || in.WatcherUserID != nil {
		if err = applyInlineViewers(ctx, tx, a, def, id, in.Viewers, in.WatcherUserID, now); err != nil {
			return nil, err
		}
		hasUpdate = true
	}
	if pf14IsSubmission(t.Status, in.Status) {
		if err = e.pf14cSubmit(ctx, tx, a, &t, def, in.Approvers, now); err != nil {
			return nil, err
		}
		evs := []model.WebhookEvent{}
		if hasUpdate {
			evs = append(evs, pf14Event(model.WebhookEventTicketUpdated, id, a.UserID))
		}
		submitted, er := pf14FinishSubmission(ctx, tx, &t, def, a.UserID, now)
		if er != nil {
			return nil, er
		}
		return append(evs, submitted...), nil
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

// pf14ValidateStatusRequest validates the optional status of a ticket update.
// Same-state saves (draft on a draft, rejected on a rejected ticket) are plain
// saves, and in_progress on a draft or rejected ticket is a submission. Every
// other transition is refused (R10/PF-05).
func pf14ValidateStatusRequest(current string, requested *string) error {
	if requested == nil {
		return nil
	}
	editable := current == model.TicketStatusDraft || current == model.TicketStatusRejected
	switch {
	case editable && (*requested == current || *requested == model.TicketStatusInProgress):
		return nil
	case *requested == model.TicketStatusInProgress:
		return kferr.InvalidState("Ticket cannot be submitted")
	}
	return kferr.InvalidState("Invalid ticket status transition")
}

// pf14IsSubmission reports whether the update submits a draft or rejected
// ticket for approval.
func pf14IsSubmission(current string, requested *string) bool {
	return requested != nil && *requested == model.TicketStatusInProgress &&
		(current == model.TicketStatusDraft || current == model.TicketStatusRejected)
}

// pf14cSubmit rebuilds the route of a draft or rejected ticket and advances it
// to its first actionable group, or to completion when no user action is
// needed. The application restriction is re-checked at this point.
func (e *Engine) pf14cSubmit(ctx context.Context, tx *store.Tx, a Actor, t *model.Ticket, def model.WorkflowDef, nominated *[]ApproverInput, now time.Time) error {
	applicantID := a.UserID
	if t.ProxyClientUserID != nil {
		applicantID = *t.ProxyClientUserID
	}
	if t.AuthorTeamID != nil {
		if err := pf14CheckApplicable(ctx, tx, applicantID, def, *t.AuthorTeamID); err != nil {
			return err
		}
		team, err := store.GetTeam(ctx, tx, *t.AuthorTeamID)
		if err != nil {
			return err
		}
		fullName, err := store.TeamFullName(ctx, tx, team.ID)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(map[string]string{"id": team.ID, "name": team.Name, "fullName": fullName, "code": team.Code})
		if err != nil {
			return fmt.Errorf("encode applicant team snapshot: %w", err)
		}
		t.AuthorTeamSnapshot = snapshot
	}
	inputValues, err := e.pf14cRevalidateForSubmit(ctx, tx, a, t, def, now)
	if err != nil {
		return err
	}
	if err = store.EngDeleteTicketSteps(ctx, tx, t.ID); err != nil {
		return err
	}
	approvers := []ApproverInput{}
	if nominated != nil {
		approvers = *nominated
	}
	t.Status = model.TicketStatusInProgress
	return e.pf14BuildRoute(ctx, tx, t, def, inputValues, approvers, store.Millis(now))
}

// pf14cCustomizeAssignees applies approver customization by a current approver
// for assignee_customizable steps of the materialized route.
func pf14cCustomizeAssignees(ctx context.Context, tx *store.Tx, t model.Ticket, approvers []ApproverInput, now time.Time) error {
	if t.RouteID == nil {
		return kferr.InvalidState("Ticket route is unavailable")
	}
	route, err := store.RouteDefByID(ctx, tx, *t.RouteID)
	if err != nil {
		return err
	}
	definition, err := store.WorkflowDefByID(ctx, tx, t.WorkflowID)
	if err != nil {
		return err
	}
	inputRows, err := store.EngTicketInputs(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	nominationValues := pf14RouteStepValues(definition, pf14InputValues(definition, pf14InputsFromRows(definition, inputRows)))
	ticketSteps, err := store.EngTicketSteps(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	for _, v := range approvers {
		var selected *model.RouteStep
		for _, routeStep := range route.Steps {
			if routeStep.Code == v.RouteStepCode && routeStep.StepType == model.StepTypeAssigneeCustomizable {
				selected = &routeStep
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
		if selected == nil || step == nil {
			return kferr.InvalidParameter("Invalid customizable route step")
		}
		for _, userID := range v.UserIDs {
			if err = pf14bValidateNominee(ctx, tx, userID, selected.Targets, nominationValues); err != nil {
				return err
			}
		}
		uniqueUserIDs := pf14UniqueUserIDs(v.UserIDs)
		if err = store.EngInsertCustomApprovers(ctx, tx, t.ID, v.RouteStepCode, uniqueUserIDs); err != nil {
			return err
		}
		existing, err := store.EngStepAssignees(ctx, tx, step.ID)
		if err != nil {
			return err
		}
		present := map[string]bool{}
		for _, assignee := range existing {
			present[assignee.UserID] = true
		}
		for _, userID := range uniqueUserIDs {
			if _, err = store.GetUser(ctx, tx, userID); err != nil {
				return kferr.ValidationFailed(map[string][]string{"approvers": {"contains an unknown user"}})
			}
			if present[userID] {
				continue
			}
			assignee := model.TicketAssignee{ID: store.NewID(), TicketStepID: step.ID, TicketID: t.ID, UserID: userID, CreatedAt: now, UpdatedAt: now}
			if err = store.EngInsertAssignee(ctx, tx, assignee); err != nil {
				return err
			}
			present[userID] = true
		}
	}
	return nil
}
