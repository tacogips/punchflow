package present

import (
	"context"
	"fmt"

	"punchflow/internal/access"
	"punchflow/internal/auth"
	"punchflow/internal/conditions"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type TicketDetailJSON map[string]any

// TicketDetail presents a ticket without any per-caller section filtering
// (used for webhook payloads, which carry the complete ticket).
func TicketDetail(ctx context.Context, q store.Querier, id string) (TicketDetailJSON, error) {
	return ticketDetail(ctx, q, id, nil, false)
}

// TicketDetailWebhook presents ticket data with restricted form values redacted.
// INF-SEC-02: Webhook bodies retain field definitions but never restricted values.
func TicketDetailWebhook(ctx context.Context, q store.Querier, id string) (TicketDetailJSON, error) {
	return ticketDetail(ctx, q, id, nil, true)
}

// TicketDetailFor presents a ticket for the caller. Inputs of restricted form
// sections the caller may not view are omitted and the section is marked not
// viewable. A nil caller sees every section.
func TicketDetailFor(ctx context.Context, q store.Querier, id string, caller *access.Caller) (TicketDetailJSON, error) {
	return ticketDetail(ctx, q, id, caller, false)
}

// TicketDetailForPrincipal resolves ticket_read against the ticket's folder
// before applying section visibility.
func TicketDetailForPrincipal(ctx context.Context, q store.Querier, id string, principal *auth.Principal) (TicketDetailJSON, error) {
	caller, err := access.CallerForTicketID(ctx, q, principal, id)
	if err != nil {
		return nil, err
	}
	return ticketDetail(ctx, q, id, &caller, false)
}

func ticketDetail(ctx context.Context, q store.Querier, id string, caller *access.Caller, redactRestricted bool) (TicketDetailJSON, error) {
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
	viewable := map[string]bool{}
	for _, s := range def.SectionList {
		viewable[s.ID] = true
	}
	if caller != nil {
		if viewable, err = access.SectionAccess(ctx, q, *caller, x, def); err != nil {
			return nil, err
		}
	}
	fieldSection := map[string]string{}
	for _, s := range def.SectionList {
		for _, f := range s.FormFields {
			fieldSection[f.ID] = s.ID
		}
	}
	inputMaps := make([]any, 0, len(inputs))
	inputBySection := map[string][]any{}
	values := map[string]any{}
	for _, in := range inputs {
		values[in.FormFieldID] = in.Value
		for _, section := range def.SectionList {
			for _, f := range section.FormFields {
				if f.ID == in.FormFieldID {
					values[f.Code] = in.Value
				}
			}
		}
		sid, known := fieldSection[in.FormFieldID]
		if known && !viewable[sid] {
			continue
		}
		item, e := formInputJSON(ctx, q, def, in)
		if e != nil {
			return nil, e
		}
		if redactRestricted && known && restrictedFormSection(def, sid) {
			redactFormInput(item)
		}
		inputMaps = append(inputMaps, item)
		if sid, known := fieldSection[in.FormFieldID]; known {
			inputBySection[sid] = append(inputBySection[sid], item)
		}
	}
	sections := make([]any, 0, len(def.SectionList))
	for _, s := range def.SectionList {
		sectionInputs := inputBySection[s.ID]
		if sectionInputs == nil {
			sectionInputs = []any{}
		}
		visible := true
		if s.Conditional {
			visible = conditions.MatchExpression(s.ConditionFields, s.CombinationType, s.CombinationExpression, values)
		}
		restricted := s.SectionType == model.SectionTypeForm && s.ViewRestriction
		viewers := []any{}
		if restricted {
			if viewers, err = sectionViewersJSON(ctx, q, s, &x.CreatedAt); err != nil {
				return nil, err
			}
		}
		allowAuthor, allowApprover := !restricted || s.AllowViewAuthor, !restricted || s.AllowViewApprover
		sections = append(sections, map[string]any{"id": s.ID, "sectionId": s.ID, "visible": visible, "viewRestriction": restricted, "allowViewAuthor": allowAuthor, "allowViewApprover": allowApprover, "viewable": viewable[s.ID], "inputs": sectionInputs, "sectionViewers": viewers})
	}
	slips, err := slipItemsJSON(ctx, q, def, id, viewable)
	if err != nil {
		return nil, err
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
	customNumbers, err := pf08TicketCustomNumberingValues(ctx, q, id, def)
	if err != nil {
		return nil, err
	}
	out["customNumberingValues"] = customNumbers
	return out, nil
}

func restrictedFormSection(def model.WorkflowDef, sectionID string) bool {
	for _, section := range def.SectionList {
		if section.ID == sectionID {
			return section.SectionType == model.SectionTypeForm && section.ViewRestriction
		}
	}
	return false
}

func redactFormInput(item map[string]any) {
	item["value"] = nil
	item["generalMasterItems"] = []any{}
	item["users"] = []any{}
	item["teams"] = []any{}
	item["inputTickets"] = []any{}
	item["attachments"] = []any{}
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
