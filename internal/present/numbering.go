package present

import (
	"context"
	"database/sql"
	"sort"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf08WorkflowNumbering(ctx context.Context, q store.Querier, def model.WorkflowDef) (any, any, any, error) {
	var key any
	var next any
	if def.TicketNumberKey != nil {
		key = def.TicketNumberKey
		var counter int64
		err := q.QueryRowContext(ctx, `SELECT counter FROM ticket_number_key_counters WHERE ticket_number_key_id=?`, def.TicketNumberKey.ID).Scan(&counter)
		if err == nil {
			next = counter + 1
		} else if err != sql.ErrNoRows {
			return nil, nil, nil, err
		}
	}
	definitions := append([]model.CustomNumbering(nil), def.CustomNumberings...)
	sort.SliceStable(definitions, func(i, j int) bool { return definitions[i].Code < definitions[j].Code })
	custom := make([]any, 0, len(definitions))
	for _, item := range definitions {
		value, err := pf08CustomNextValue(ctx, q, item)
		if err != nil {
			return nil, nil, nil, err
		}
		item.Value = value
		custom = append(custom, item)
	}
	return key, next, custom, nil
}

func pf08CustomNextValue(ctx context.Context, q store.Querier, item model.CustomNumbering) (int64, error) {
	key := "custom:" + item.ID
	if item.TicketNumberKeyID != nil && *item.TicketNumberKeyID != "" {
		key = "key:" + *item.TicketNumberKeyID
	}
	var counter int64
	err := q.QueryRowContext(ctx, `SELECT counter FROM custom_numbering_counters WHERE custom_numbering_id=?`, key).Scan(&counter)
	if err == nil {
		return counter + 1, nil
	}
	if err == sql.ErrNoRows {
		if item.Value < 1 {
			return 1, nil
		}
		return item.Value, nil
	}
	return 0, err
}

func pf08CustomNumberingSettings(ctx context.Context, q store.Querier, settings []model.CustomNumberingSetting) ([]any, error) {
	settings = append([]model.CustomNumberingSetting(nil), settings...)
	sort.SliceStable(settings, func(i, j int) bool { return settings[i].SortOrder < settings[j].SortOrder })
	out := make([]any, 0, len(settings))
	for _, setting := range settings {
		var subStatus any
		if setting.SubStatusID != nil {
			item, err := store.SubStatusByID(ctx, q, *setting.SubStatusID)
			if err != nil && err != store.ErrNotFound {
				return nil, err
			}
			if err == nil {
				subStatus = SubStatus(item)
			}
		}
		out = append(out, map[string]any{"id": setting.ID, "customNumberingId": setting.CustomNumberingID, "ticketNumberFormat": setting.TicketNumberFormat, "timingType": setting.TimingType, "subStatusId": setting.SubStatusID, "subStatus": subStatus, "sortOrder": setting.SortOrder})
	}
	return out, nil
}

func pf08TicketCustomNumberingValues(ctx context.Context, q store.Querier, ticketID string, def model.WorkflowDef) ([]any, error) {
	definitions := append([]model.CustomNumbering(nil), def.CustomNumberings...)
	sort.SliceStable(definitions, func(i, j int) bool { return definitions[i].Code < definitions[j].Code })
	values := make([]any, 0, len(definitions))
	for _, definition := range definitions {
		var formatted string
		err := q.QueryRowContext(ctx, `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? AND custom_numbering_id=?`, ticketID, definition.ID).Scan(&formatted)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		definition.Value, err = pf08CustomNextValue(ctx, q, definition)
		if err != nil {
			return nil, err
		}
		values = append(values, map[string]any{"id": definition.ID, "formattedNumber": formatted, "customNumbering": definition})
	}
	return values, nil
}
