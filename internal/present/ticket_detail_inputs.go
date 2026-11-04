package present

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"sort"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// stableUUID derives a deterministic UUID from the parts, for identifiers the
// storage model does not keep (for example one detail cell).
func stableUUID(parts ...string) string {
	h := sha1.New()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	sum := h.Sum(nil)
	sum[6] = sum[6]&0x0f | 0x50
	sum[8] = sum[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// resolvedRefs holds the objects a value references, keyed like the API's
// generalMasterItems/users/teams/inputTickets arrays.
type resolvedRefs struct {
	general, users, teams, tickets []any
}

func newResolvedRefs() resolvedRefs {
	return resolvedRefs{general: []any{}, users: []any{}, teams: []any{}, tickets: []any{}}
}

func resolveFieldRefs(ctx context.Context, q store.Querier, fieldType string, refs []string) (resolvedRefs, error) {
	out := newResolvedRefs()
	switch fieldType {
	case "master", "general_master", "general_master_item":
		resolved, e := GeneralMasterItems(ctx, q, refs)
		if e != nil {
			return out, e
		}
		for _, v := range resolved {
			out.general = append(out.general, v)
		}
	case "user":
		resolved, e := Users(ctx, q, refs, UserOpts{})
		if e != nil {
			return out, e
		}
		for _, v := range resolved {
			out.users = append(out.users, v)
		}
	case "team":
		resolved, e := store.GetTeams(ctx, q, refs)
		if e != nil {
			return out, e
		}
		for _, v := range resolved {
			team, e := Team(ctx, q, v.ID)
			if e != nil {
				return out, e
			}
			out.teams = append(out.teams, team)
		}
	case "ticket":
		resolved, e := Tickets(ctx, q, refs)
		if e != nil {
			return out, e
		}
		for _, v := range resolved {
			out.tickets = append(out.tickets, v)
		}
	}
	return out, nil
}

// attachmentsJSON presents the uploaded files referenced by signed ID, in the
// order the values list them. Unknown IDs are skipped.
func attachmentsJSON(ctx context.Context, q store.Querier, signedIDs []string) ([]any, error) {
	out := []any{}
	for _, signed := range signedIDs {
		file, err := store.FileBySignedID(ctx, q, signed)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, attachmentJSON(file))
	}
	return out, nil
}

func formInputJSON(ctx context.Context, q store.Querier, def model.WorkflowDef, in model.TicketInput) (map[string]any, error) {
	var field any = map[string]any{"id": in.FormFieldID}
	var fieldModel *model.FormField
	for _, section := range def.SectionList {
		for i := range section.FormFields {
			if section.FormFields[i].ID == in.FormFieldID {
				fieldModel = &section.FormFields[i]
				field = pf08FormFieldWithCustomValidations(ctx, q, section.FormFields[i], pf08FormFieldsByCode(def.SectionList))
			}
		}
	}
	refs := pf15ReferenceIDs(in.RefIDs, in.Value)
	resolved := newResolvedRefs()
	attachments := []any{}
	if fieldModel != nil {
		var err error
		if resolved, err = resolveFieldRefs(ctx, q, fieldModel.FieldType, refs); err != nil {
			return nil, err
		}
		if fieldModel.FieldType == "file" {
			if attachments, err = attachmentsJSON(ctx, q, refs); err != nil {
				return nil, err
			}
		}
	}
	value := in.Value
	if fieldModel != nil {
		switch fieldModel.FieldType {
		case "master":
			if in.ReferenceSnapshot != nil {
				value = in.ReferenceSnapshot
			} else {
				value = resolved.general
			}
		case "user":
			if in.ReferenceSnapshot != nil {
				value = in.ReferenceSnapshot
			} else {
				value = resolved.users
			}
		case "team":
			if in.ReferenceSnapshot != nil {
				value = in.ReferenceSnapshot
			} else {
				value = resolved.teams
			}
		case "ticket":
			if in.ReferenceSnapshot != nil {
				value = in.ReferenceSnapshot
			} else {
				value = resolved.tickets
			}
		case "file":
			value = nil
		}
	}
	return map[string]any{"id": in.ID, "formFieldId": in.FormFieldID, "value": value, "formField": field, "generalMasterItems": resolved.general, "users": resolved.users, "teams": resolved.teams, "inputTickets": resolved.tickets, "attachments": attachments}, nil
}

func slipFieldInTicket(ctx context.Context, q store.Querier, f model.SlipField, fields []model.SlipField) map[string]any {
	return pf08SlipFieldInContext(ctx, q, f, fields)
}

// slipItemsJSON presents the detail rows. Every field of the row's section is
// emitted in definition order (empty cells as null) with its attachments and
// field definition. Rows of sections the caller cannot view are omitted.
func slipItemsJSON(ctx context.Context, q store.Querier, def model.WorkflowDef, ticketID string, viewable map[string]bool) ([]any, error) {
	rows, err := store.TicketSlipItems(ctx, q, ticketID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Position < rows[j].Position })
	slips := make([]any, 0, len(rows))
	for _, row := range rows {
		if ok, known := viewable[row.SlipSectionID]; known && !ok {
			continue
		}
		var section *model.WorkflowSection
		for i := range def.SectionList {
			if def.SectionList[i].ID == row.SlipSectionID {
				section = &def.SectionList[i]
				break
			}
		}
		cells := []any{}
		if section != nil {
			for _, f := range section.SlipFields {
				cell, err := slipCellJSON(ctx, q, row, f, section.SlipFields)
				if err != nil {
					return nil, err
				}
				cells = append(cells, cell)
			}
		}
		slips = append(slips, map[string]any{"id": row.ID, "slipSectionId": row.SlipSectionID, "position": row.Position, "createdAt": pf08Time(row.CreatedAt), "updatedAt": pf08Time(row.UpdatedAt), "inputs": cells})
	}
	return slips, nil
}

func slipCellJSON(ctx context.Context, q store.Querier, row model.TicketSlipItem, f model.SlipField, fields []model.SlipField) (map[string]any, error) {
	value, ok := row.Inputs[f.ID]
	if !ok {
		value = row.Inputs[f.Code]
	}
	refs := pf15ReferenceIDs(nil, value)
	resolved, err := resolveFieldRefs(ctx, q, f.FieldType, refs)
	if err != nil {
		return nil, err
	}
	attachments := []any{}
	if f.FieldType == "file" {
		if attachments, err = attachmentsJSON(ctx, q, refs); err != nil {
			return nil, err
		}
	}
	publicValue := value
	switch f.FieldType {
	case "master", "general_master", "general_master_item":
		if snapshot, ok := row.ReferenceSnapshots[f.ID]; ok {
			publicValue = snapshot
		} else {
			publicValue = resolved.general
		}
	case "user":
		if snapshot, ok := row.ReferenceSnapshots[f.ID]; ok {
			publicValue = snapshot
		} else {
			publicValue = resolved.users
		}
	case "team":
		if snapshot, ok := row.ReferenceSnapshots[f.ID]; ok {
			publicValue = snapshot
		} else {
			publicValue = resolved.teams
		}
	case "ticket":
		if snapshot, ok := row.ReferenceSnapshots[f.ID]; ok {
			publicValue = snapshot
		} else {
			publicValue = resolved.tickets
		}
	case "file":
		publicValue = nil
	}
	return map[string]any{
		"id": stableUUID(row.ID, f.ID), "slipFieldId": f.ID, "value": publicValue, "createdAt": pf08Time(row.CreatedAt), "updatedAt": pf08Time(row.UpdatedAt),
		"generalMasterItems": resolved.general, "users": resolved.users, "teams": resolved.teams, "inputTickets": resolved.tickets,
		"attachments": attachments, "slipField": slipFieldInTicket(ctx, q, f, fields),
	}, nil
}
