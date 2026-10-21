package engine

import (
	"encoding/json"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// slipAsFormField lets detail fields share the form field validators.
func slipAsFormField(f model.SlipField) model.FormField {
	return model.FormField{
		ID: f.ID, Code: f.Code, Title: f.Title, FieldType: f.FieldType, Required: f.Required, Approver: f.Approver, Author: f.Author,
		Options: f.Options, RegexpFormat: f.RegexpFormat, Formula: f.Formula, DefaultValue: f.DefaultValue,
		MinValue: f.MinValue, MaxValue: f.MaxValue, MinLength: f.MinLength, MaxLength: f.MaxLength, DecimalDigit: f.DecimalDigit,
		Delimited: f.Delimited, Hidden: f.Hidden, ReadonlyOnUI: f.ReadonlyOnUI, Multiple: f.Multiple, AutoLink: f.AutoLink,
		AllowedExtensions: f.AllowedExtensions, AllowedTicketStatus: f.AllowedTicketStatus, GeneralMasterCode: f.GeneralMasterCode,
	}
}

// resolveSlipSection picks the detail section of a submitted row. The ID wins
// over the code; when both are omitted the first detail section is used,
// except for the /form PATCH, which requires an explicit selector.
func (n *normalizer) resolveSlipSection(item SlipItemInput) (model.WorkflowSection, error) {
	switch {
	case item.SlipSectionID != "":
		if s, ok := n.idx.slipByID[item.SlipSectionID]; ok {
			return s, nil
		}
	case item.SlipSectionCode != "":
		if s, ok := n.idx.slipByCode[item.SlipSectionCode]; ok {
			return s, nil
		}
	case n.opts.Partial:
		return model.WorkflowSection{}, kferr.InvalidParameter("slipSectionId or slipSectionCode is required")
	case len(n.idx.slipOrder) > 0:
		return n.idx.slipOrder[0], nil
	}
	return model.WorkflowSection{}, kferr.ValidationFailed(map[string][]string{"slipItems": {"contains an unknown slip section"}})
}

// editableByActor reports whether the actor may enter the detail field. Only
// approvers are restricted (to approver fields unless the workflow lets them
// overwrite everything); applicants may enter every field.
func (n *normalizer) editableByActor(f model.SlipField) bool {
	return !n.opts.Approver || f.Approver || n.def.Overwritable
}

// preserveOmitted reports whether an omitted cell keeps its stored value: the
// fields the actor cannot (approvers) or need not (applicants, approver-only
// fields) enter.
func (n *normalizer) preserveOmitted(f model.SlipField) bool {
	if n.opts.Approver {
		return !n.editableByActor(f)
	}
	return f.Approver
}

func slipRawValue(in SlipFieldInput) (any, error) {
	raw := in.Value
	for _, alt := range []json.RawMessage{in.GeneralMasterItemID, in.UserID, in.TeamID, in.TicketID} {
		if len(raw) == 0 {
			raw = alt
		}
	}
	var v any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (n *normalizer) normalizeSlipRows(items []SlipItemInput) ([]model.TicketSlipItem, error) {
	existing := map[string][]model.TicketSlipItem{}
	for _, row := range n.opts.ExistingSlips {
		existing[row.SlipSectionID] = append(existing[row.SlipSectionID], row)
	}
	counters := map[string]int{}
	rows := make([]model.TicketSlipItem, 0, len(items))
	for _, item := range items {
		section, err := n.resolveSlipSection(item)
		if err != nil {
			return nil, err
		}
		if n.opts.Blocked[section.ID] {
			return nil, kferr.InvalidParameter("Invalid slip section")
		}
		position := counters[section.ID]
		counters[section.ID]++
		var prev *model.TicketSlipItem
		if position < len(existing[section.ID]) {
			prev = &existing[section.ID][position]
		}
		if n.opts.Approver && prev == nil && !section.AllowApproverToAddItems {
			return nil, kferr.InvalidParameter("Approver cannot add rows to this detail section")
		}
		cells, err := n.normalizeSlipCells(section, item, prev)
		if err != nil {
			return nil, err
		}
		row := model.TicketSlipItem{ID: store.NewID(), TicketID: n.ticketID, SlipSectionID: section.ID, Inputs: cells, CreatedAt: store.Time(n.nowMs), UpdatedAt: store.Time(n.nowMs)}
		if prev != nil {
			row.ID, row.CreatedAt = prev.ID, prev.CreatedAt
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (n *normalizer) normalizeSlipCells(section model.WorkflowSection, item SlipItemInput, prev *model.TicketSlipItem) (map[string]any, error) {
	fields := map[string]model.SlipField{}
	for _, f := range section.SlipFields {
		fields[f.ID] = f
		fields[f.Code] = f
	}
	cells := map[string]any{}
	supplied := map[string]bool{}
	for _, in := range item.Inputs {
		key := in.SlipFieldID
		if key == "" {
			key = in.SlipFieldCode
		}
		field, ok := fields[key]
		if !ok {
			return nil, kferr.ValidationFailed(map[string][]string{"slipItems": {"contains an unknown slip field"}})
		}
		if supplied[field.ID] {
			return nil, kferr.InvalidParameter("Slip field must be unique")
		}
		supplied[field.ID] = true
		if !n.editableByActor(field) {
			return nil, kferr.InvalidParameter("Invalid slip field code")
		}
		value, err := slipRawValue(in)
		if err != nil {
			n.fail(field.Code, "is invalid")
			continue
		}
		ff := slipAsFormField(field)
		if field.FieldType == "file" {
			value, _ = pf14FileValue(in.Files, value, nil)
		}
		if field.FieldType == "calculation" {
			value = nil
		}
		if err = pf14ValidateValue(ff, value); err != nil {
			n.fail(field.Code, err.Error())
			continue
		}
		if field.FieldType != "calculation" {
			if n.slipRadioMustBeSelected(field, value) {
				n.fail(field.Code, "must be selected")
				continue
			}
			if err = pf14cValidateFormValue(n.ctx, n.q, ff, value); err != nil {
				n.fail(field.Code, err.Error())
				continue
			}
		}
		cells[field.ID] = value
	}
	if prev != nil {
		for _, f := range section.SlipFields {
			if supplied[f.ID] || !n.preserveOmitted(f) {
				continue
			}
			if v, ok := slipCell(*prev, f); ok {
				cells[f.ID] = v
			}
		}
	}
	return cells, nil
}

func (n *normalizer) slipRadioMustBeSelected(f model.SlipField, value any) bool {
	if f.FieldType != "radio_button" || !pf14Empty(value) || !n.opts.Full && !n.opts.Approver {
		return false
	}
	return f.Hidden == nil || !*f.Hidden
}

// slipCell returns a stored cell by field ID, falling back to the code for
// rows written by older versions of the workflow.
func slipCell(row model.TicketSlipItem, f model.SlipField) (any, bool) {
	if v, ok := row.Inputs[f.ID]; ok {
		return v, true
	}
	v, ok := row.Inputs[f.Code]
	return v, ok
}

func cloneSlipRow(row model.TicketSlipItem) model.TicketSlipItem {
	cells := make(map[string]any, len(row.Inputs))
	for k, v := range row.Inputs {
		cells[k] = v
	}
	row.Inputs = cells
	return row
}

// assembleSlips merges the submitted rows with the stored ones. Rows of
// sections the actor cannot enter are always kept. It reports whether the
// stored set must be rewritten.
func (n *normalizer) assembleSlips(newRows []model.TicketSlipItem) ([]model.TicketSlipItem, bool) {
	existing := n.opts.ExistingSlips
	final := []model.TicketSlipItem{}
	switch n.opts.SlipMode {
	case slipReplaceAll:
		final = append(final, newRows...)
		for _, row := range existing {
			if n.opts.Blocked[row.SlipSectionID] {
				final = append(final, cloneSlipRow(row))
			}
		}
	case slipReplaceSections:
		bySection := map[string][]model.TicketSlipItem{}
		order := []string{}
		for _, row := range newRows {
			if _, ok := bySection[row.SlipSectionID]; !ok {
				order = append(order, row.SlipSectionID)
			}
			bySection[row.SlipSectionID] = append(bySection[row.SlipSectionID], row)
		}
		inserted := map[string]bool{}
		for _, row := range existing {
			rows, replaced := bySection[row.SlipSectionID]
			if !replaced {
				final = append(final, cloneSlipRow(row))
				continue
			}
			if !inserted[row.SlipSectionID] {
				inserted[row.SlipSectionID] = true
				final = append(final, rows...)
			}
		}
		for _, id := range order {
			if !inserted[id] {
				final = append(final, bySection[id]...)
			}
		}
	default:
		for _, row := range existing {
			final = append(final, cloneSlipRow(row))
		}
	}
	for i := range final {
		final[i].Position = i
	}
	return final, n.opts.SlipMode != slipKeep
}

// checkRequiredCells enforces required detail cells on every resulting row
// (including stored rows) when the form is complete (submission).
func (n *normalizer) checkRequiredCells(rows []model.TicketSlipItem) {
	if !n.opts.Full {
		return
	}
	for _, row := range rows {
		section, ok := n.idx.slipByID[row.SlipSectionID]
		if !ok || n.opts.Blocked[section.ID] {
			continue
		}
		for _, f := range section.SlipFields {
			hidden := f.Hidden != nil && *f.Hidden
			if hidden || f.FieldType == "calculation" {
				continue
			}
			v, exists := slipCell(row, f)
			if f.FieldType == "radio_button" && pf14Empty(v) {
				n.fail(f.Code, "must be selected")
				continue
			}
			if f.Required && !f.Approver && pf14Empty(v) {
				n.fail(f.Code, "is required")
				continue
			}
			if exists {
				field := model.FormField{
					Code: f.Code, FieldType: f.FieldType, Options: f.Options,
					RegexpFormat: f.RegexpFormat, MinValue: f.MinValue, MaxValue: f.MaxValue,
					MinLength: f.MinLength, MaxLength: f.MaxLength, DecimalDigit: f.DecimalDigit,
					AllowedExtensions: f.AllowedExtensions, AllowedTicketStatus: f.AllowedTicketStatus,
				}
				if err := pf14ValidateValue(field, v); err != nil {
					n.fail(f.Code, err.Error())
					continue
				}
				if err := pf14cValidateFormValue(n.ctx, n.q, field, v); err != nil {
					n.fail(f.Code, err.Error())
				}
			}
		}
	}
}
