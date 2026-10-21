package engine

import (
	"fmt"

	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

// INF-FRM-01 stores reference records alongside their identifiers so later
// master, team, or user edits cannot rewrite the ticket's saved field value.
func (n *normalizer) referenceSnapshot(fieldType string, ids []string) (any, error) {
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		var value map[string]any
		switch fieldType {
		case "master":
			item, err := store.GetGeneralMasterItem(n.ctx, n.q, id)
			if err != nil {
				return nil, fmt.Errorf("snapshot master reference: %w", err)
			}
			inputs := []any{}
			fields, err := store.GetGeneralMasterFields(n.ctx, n.q, item.GeneralMasterID)
			if err != nil {
				return nil, fmt.Errorf("snapshot master fields: %w", err)
			}
			for _, field := range fields {
				fieldValue, ok := item.Inputs[field.ID]
				if !ok {
					continue
				}
				inputID := item.InputIDs[field.ID]
				inputs = append(inputs, map[string]any{"id": inputID, "value": fieldValue, "createdAt": kftime.Format(item.CreatedAt), "updatedAt": kftime.Format(item.UpdatedAt), "field": map[string]any{"id": field.ID, "title": field.Title, "description": field.Description, "code": field.Code, "fieldType": field.FieldType, "required": field.Required, "fieldOrder": field.FieldOrder, "visible": field.Visible, "initialDisplay": field.InitialDisplay, "options": field.Options, "createdAt": kftime.Format(field.CreatedAt), "updatedAt": kftime.Format(field.UpdatedAt)}})
			}
			value = map[string]any{"id": item.ID, "code": item.Code, "name": item.Name, "description": item.Description, "startsOn": item.StartsOn, "endsOn": item.EndsOn, "inputs": inputs, "createdAt": kftime.Format(item.CreatedAt), "updatedAt": kftime.Format(item.UpdatedAt)}
		case "user":
			user, err := store.GetUser(n.ctx, n.q, id)
			if err != nil {
				return nil, fmt.Errorf("snapshot user reference: %w", err)
			}
			value = map[string]any{"id": user.ID, "code": user.Code, "email": user.Email, "firstName": user.FirstName, "lastName": user.LastName, "fullName": user.LastName + " " + user.FirstName, "employeeId": user.EmployeeID, "image": map[string]any{"100x100": nil, "64x64": nil, "32x32": nil}, "status": user.Status, "locale": user.Locale, "createdAt": kftime.Format(user.CreatedAt), "updatedAt": kftime.Format(user.UpdatedAt)}
		case "team":
			team, err := store.GetTeam(n.ctx, n.q, id)
			if err != nil {
				return nil, fmt.Errorf("snapshot team reference: %w", err)
			}
			fullName, err := store.TeamFullName(n.ctx, n.q, id)
			if err != nil {
				return nil, fmt.Errorf("snapshot team full name: %w", err)
			}
			usersCount, err := store.CountTeamUsers(n.ctx, n.q, id)
			if err != nil {
				return nil, fmt.Errorf("snapshot team users: %w", err)
			}
			value = map[string]any{"id": team.ID, "code": team.Code, "name": team.Name, "fullName": fullName, "notes": team.Notes, "approveOnly": team.ApproveOnly, "usersCount": usersCount, "createdAt": kftime.Format(team.CreatedAt), "updatedAt": kftime.Format(team.UpdatedAt)}
		case "ticket":
			ticket, err := store.Ticket(n.ctx, n.q, id)
			if err != nil {
				return nil, fmt.Errorf("snapshot ticket reference: %w", err)
			}
			value = map[string]any{"id": ticket.ID, "title": ticket.Title, "status": ticket.Status, "ticketNumber": ticket.TicketNumber}
		default:
			return nil, fmt.Errorf("unsupported reference type %q", fieldType)
		}
		values = append(values, value)
	}
	return values, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
