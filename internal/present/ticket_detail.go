package present

import (
	"context"
	"fmt"
	"reflect"

	"punchflow/internal/access"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type TicketDetailJSON map[string]any

// TicketDetail presents a ticket without any per-caller section filtering
// (used for webhook payloads, which carry the complete ticket).
func TicketDetail(ctx context.Context, q store.Querier, id string) (TicketDetailJSON, error) {
	return TicketDetailFor(ctx, q, id, nil)
}

// TicketDetailFor presents a ticket for the caller. Inputs of restricted form
// sections the caller may not view are omitted and the section is marked not
// viewable. A nil caller sees every section.
func TicketDetailFor(ctx context.Context, q store.Querier, id string, caller *access.Caller) (TicketDetailJSON, error) {
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
		if sid, known := fieldSection[in.FormFieldID]; known && !viewable[sid] {
			continue
		}
		item, e := formInputJSON(ctx, q, def, in)
		if e != nil {
			return nil, e
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
			visible = pf15SectionVisible(s, values)
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
			results = append(results, actual != nil && (reflect.TypeOf(actual) != reflect.TypeOf("") || actual.(string) != ""))
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
