package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"punchflow/internal/access"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

// Ticket CSV layouts.
const (
	// LayoutTicket writes one row per ticket with its form inputs.
	LayoutTicket = "ticket"
	// LayoutDetail writes one row per detail (slip) row.
	LayoutDetail = "detail"
)

type column struct{ key, header string }

// columnSet collects dynamic columns in first-seen order.
type columnSet struct {
	columns []column
	index   map[string]bool
}

func (c *columnSet) add(key, header string) {
	if c.index == nil {
		c.index = map[string]bool{}
	}
	if !c.index[key] {
		c.index[key] = true
		c.columns = append(c.columns, column{key: key, header: header})
	}
}

// WriteTickets writes the identified tickets in the requested layout. Ticket
// content is presented for the caller, so inputs and detail rows of restricted
// form sections the caller may not view are left empty. ids must already be
// restricted to tickets the caller may read.
func WriteTickets(ctx context.Context, q store.Querier, w io.Writer, layout string, ids []string, caller access.Caller) error {
	if layout != LayoutTicket && layout != LayoutDetail {
		return fmt.Errorf("export: unknown ticket layout %q", layout)
	}
	details := make([]map[string]any, 0, len(ids))
	defs := map[string]model.WorkflowDef{}
	var workflowOrder []string
	for _, id := range ids {
		detail, err := present.TicketDetailFor(ctx, q, id, &caller)
		if err != nil {
			return fmt.Errorf("export: present ticket: %w", err)
		}
		generic, err := toMap(detail)
		if err != nil {
			return err
		}
		details = append(details, generic)
		workflowID, _ := nested(generic, "workflow")["id"].(string)
		if _, ok := defs[workflowID]; !ok && workflowID != "" {
			def, err := store.WorkflowDefByID(ctx, q, workflowID)
			if err != nil {
				return fmt.Errorf("export: load workflow: %w", err)
			}
			defs[workflowID] = def
			workflowOrder = append(workflowOrder, workflowID)
		}
	}

	var fields columnSet
	fieldKeys := map[string]string{} // form/slip field ID -> column key
	for _, workflowID := range workflowOrder {
		for _, section := range defs[workflowID].SectionList {
			if layout == LayoutTicket && section.SectionType == model.SectionTypeForm {
				for _, f := range section.FormFields {
					key := fieldKey(f.Code, f.ID)
					fieldKeys[f.ID] = key
					fields.add(key, firstNonEmpty(f.Title, f.Code))
				}
			}
			if layout == LayoutDetail && section.SectionType != model.SectionTypeForm {
				for _, f := range section.SlipFields {
					key := fieldKey(f.Code, f.ID)
					fieldKeys[f.ID] = key
					fields.add(key, firstNonEmpty(f.Title, f.Code))
				}
			}
		}
	}

	out, err := NewWriter(w)
	if err != nil {
		return err
	}
	header := fixedHeader(layout)
	for _, c := range fields.columns {
		header = append(header, c.header)
	}
	if err = out.Write(header); err != nil {
		return err
	}
	for _, detail := range details {
		base := fixedCells(detail, layout)
		if layout == LayoutTicket {
			values := map[string]string{}
			for _, raw := range slice(detail, "inputs") {
				input, _ := raw.(map[string]any)
				if key, ok := fieldKeys[str(input, "formFieldId")]; ok {
					values[key] = inputText(input)
				}
			}
			if err = out.Write(withFields(base, fields.columns, values)); err != nil {
				return err
			}
			continue
		}
		workflowID, _ := nested(detail, "workflow")["id"].(string)
		sections := sectionTitles(defs[workflowID])
		for _, raw := range slice(detail, "slipItems") {
			row, _ := raw.(map[string]any)
			values := map[string]string{}
			for _, cell := range slice(row, "inputs") {
				input, _ := cell.(map[string]any)
				if key, ok := fieldKeys[str(input, "slipFieldId")]; ok {
					values[key] = inputText(input)
				}
			}
			position, _ := row["position"].(float64)
			cells := append(append([]string{}, base...), sections[str(row, "slipSectionId")], strconv.Itoa(int(position)+1))
			if err = out.Write(withFields(cells, fields.columns, values)); err != nil {
				return err
			}
		}
	}
	return out.Flush()
}

func fixedHeader(layout string) []string {
	if layout == LayoutDetail {
		return []string{"Ticket ID", "Ticket number", "Title", "Workflow", "Status", "Author", "Section", "Row no."}
	}
	return []string{"Ticket ID", "Ticket number", "Title", "Workflow", "Status", "Author", "Author email", "Author team", "Created at", "Opened at", "Completed at", "Archived at", "Updated at"}
}

// fixedCells returns the leading cells shared by both layouts; the detail
// layout appends the section and row number itself.
func fixedCells(detail map[string]any, layout string) []string {
	author := nested(detail, "author")
	cells := []string{
		str(detail, "id"), str(detail, "ticketNumber"), str(detail, "title"),
		str(nested(detail, "workflow"), "name"), str(detail, "status"), str(author, "fullName"),
	}
	if layout == LayoutDetail {
		return cells
	}
	return append(cells,
		str(author, "email"), str(nested(detail, "authorTeam"), "fullName"),
		str(detail, "createdAt"), str(detail, "openedAt"), str(detail, "completedAt"), str(detail, "archivedAt"), str(detail, "updatedAt"))
}

func withFields(cells []string, columns []column, values map[string]string) []string {
	out := append([]string{}, cells...)
	for _, c := range columns {
		out = append(out, values[c.key])
	}
	return out
}

func sectionTitles(def model.WorkflowDef) map[string]string {
	out := map[string]string{}
	for _, section := range def.SectionList {
		out[section.ID] = section.Title
	}
	return out
}

func fieldKey(code, id string) string {
	if code != "" {
		return "code:" + code
	}
	return "id:" + id
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// inputText renders one form input or detail cell for a spreadsheet: resolved
// references (master items, users, teams, tickets, files) by name, otherwise the
// stored value.
func inputText(input map[string]any) string {
	for _, source := range []struct {
		key  string
		name func(map[string]any) string
	}{
		{"generalMasterItems", func(m map[string]any) string { return str(m, "name") }},
		{"users", func(m map[string]any) string { return firstNonEmpty(str(m, "fullName"), str(m, "email")) }},
		{"teams", func(m map[string]any) string { return firstNonEmpty(str(m, "fullName"), str(m, "name")) }},
		{"inputTickets", func(m map[string]any) string { return firstNonEmpty(str(m, "ticketNumber"), str(m, "title")) }},
		{"attachments", func(m map[string]any) string { return str(m, "filename") }},
	} {
		items := slice(input, source.key)
		if len(items) == 0 {
			continue
		}
		names := make([]string, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				if name := source.name(m); name != "" {
					names = append(names, name)
				}
			}
		}
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}
	}
	return scalarText(input["value"])
}

func scalarText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := scalarText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

// toMap converts a presented value (which may use named map types) to a plain
// JSON object.
func toMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("export: encode presented ticket: %w", err)
	}
	var out map[string]any
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("export: decode presented ticket: %w", err)
	}
	return out, nil
}

func nested(m map[string]any, key string) map[string]any {
	out, _ := m[key].(map[string]any)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func slice(m map[string]any, key string) []any {
	out, _ := m[key].([]any)
	return out
}

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return scalarText(v)
	}
}
