package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// slipMode selects how submitted detail rows are applied.
type slipMode int

const (
	// slipKeep leaves stored detail rows untouched (they are still revalidated).
	slipKeep slipMode = iota
	// slipReplaceAll replaces every detail row (POST/PATCH /v1/tickets).
	slipReplaceAll
	// slipReplaceSections replaces rows only for the sections that were
	// explicitly supplied (PATCH /v1/tickets/{id}/form).
	slipReplaceSections
)

// formOpts carries the actor-aware context of one form normalization.
type formOpts struct {
	// Full enforces completeness (required cells, selected radio buttons), as
	// on submission.
	Full bool
	// Partial is the /form PATCH contract: detail selectors are mandatory.
	Partial bool
	// Approver marks edits made by a current approver.
	Approver bool
	// LockReferenceSnapshots retains reference records after application.
	LockReferenceSnapshots bool
	// SlipMode selects the detail-row write behavior.
	SlipMode slipMode
	// Blocked lists section IDs the actor cannot view or enter; their stored
	// values are carried over untouched.
	Blocked map[string]bool
	// Supplied lists form field IDs the client actually sent (nil means all),
	// used to scope approver-only checks to the patch.
	Supplied map[string]bool
	// ExistingInputs and ExistingSlips are the stored values before this edit.
	ExistingInputs []model.TicketInput
	ExistingSlips  []model.TicketSlipItem
	AuthorID       string
	ProxyClientID  *string
	AuthorTeamJSON []byte
}

// formResult is the normalized outcome.
type formResult struct {
	Inputs []model.TicketInput
	// Slips is the complete resulting detail row set.
	Slips []model.TicketSlipItem
	// SlipsChanged reports whether Slips must be written.
	SlipsChanged bool
	// Title is the calculated title, when the workflow computes it.
	Title *string
}

type formIndex struct {
	fieldByID, fieldByCode map[string]model.FormField
	fieldSection           map[string]string
	formSections           map[string]model.WorkflowSection
	slipByID, slipByCode   map[string]model.WorkflowSection
	slipOrder              []model.WorkflowSection
}

func newFormIndex(def model.WorkflowDef) *formIndex {
	idx := &formIndex{
		fieldByID: map[string]model.FormField{}, fieldByCode: map[string]model.FormField{}, fieldSection: map[string]string{},
		formSections: map[string]model.WorkflowSection{}, slipByID: map[string]model.WorkflowSection{}, slipByCode: map[string]model.WorkflowSection{},
	}
	for _, section := range def.SectionList {
		switch section.SectionType {
		case model.SectionTypeForm:
			idx.formSections[section.ID] = section
			for _, f := range section.FormFields {
				idx.fieldByID[f.ID] = f
				idx.fieldByCode[f.Code] = f
				idx.fieldSection[f.ID] = section.ID
			}
		case model.SectionTypeSlip:
			idx.slipByID[section.ID] = section
			idx.slipByCode[section.Code] = section
			idx.slipOrder = append(idx.slipOrder, section)
		}
	}
	return idx
}

func (x *formIndex) field(in FieldInput) (model.FormField, bool) {
	if in.FormFieldID != "" {
		f, ok := x.fieldByID[in.FormFieldID]
		return f, ok
	}
	if in.FormFieldCode != "" {
		f, ok := x.fieldByCode[in.FormFieldCode]
		return f, ok
	}
	return model.FormField{}, false
}

type normalizer struct {
	ctx      context.Context
	q        store.Querier
	ticketID string
	def      model.WorkflowDef
	opts     formOpts
	idx      *formIndex
	errs     map[string][]string
	nowMs    int64
}

func (n *normalizer) fail(code, message string) { n.errs[code] = append(n.errs[code], message) }

// pf14NormalizeForm validates and normalizes submitted form inputs and detail
// rows against the workflow definition, computes calculation fields, and
// merges the detail rows with the stored ones according to opts.
func pf14NormalizeForm(ctx context.Context, q store.Querier, ticketID string, def model.WorkflowDef, inputs []FieldInput, slips []SlipItemInput, opts formOpts, nowTime int64) (formResult, error) {
	n := &normalizer{ctx: ctx, q: q, ticketID: ticketID, def: def, opts: opts, idx: newFormIndex(def), errs: map[string][]string{}, nowMs: nowTime}
	normalized, err := n.normalizeInputs(inputs)
	if err != nil {
		return formResult{}, err
	}
	newRows, err := n.normalizeSlipRows(slips)
	if err != nil {
		return formResult{}, err
	}
	finalSlips, changed := n.assembleSlips(newRows)
	n.checkRequiredCells(finalSlips)
	normalized = n.carryOverBlocked(normalized)
	title, calcChanged := n.calculate(normalized, finalSlips)
	n.validateCustomFields(normalized, finalSlips)
	if len(n.errs) > 0 {
		return formResult{}, kferr.ValidationFailed(n.errs)
	}
	for i := range normalized {
		if normalized[i].CreatedAt.IsZero() {
			normalized[i].CreatedAt = store.Time(nowTime)
		}
		normalized[i].UpdatedAt = store.Time(nowTime)
	}
	return formResult{Inputs: normalized, Slips: finalSlips, SlipsChanged: changed || calcChanged, Title: title}, nil
}

func (n *normalizer) normalizeInputs(inputs []FieldInput) ([]model.TicketInput, error) {
	seen := map[string]bool{}
	normalized := []model.TicketInput{}
	values := n.actorInputValues(inputs)
	for _, in := range inputs {
		field, ok := n.idx.field(in)
		if !ok {
			return nil, kferr.InvalidParameter("Invalid form field code")
		}
		if n.opts.Blocked[n.idx.fieldSection[field.ID]] {
			continue
		}
		if seen[field.ID] {
			return nil, kferr.InvalidParameter("Form field must be unique")
		}
		seen[field.ID] = true
		value, refs, err := pf14FieldValue(in)
		if err != nil {
			n.fail(field.Code, "is invalid")
			continue
		}
		if field.FieldType == "file" {
			value, refs = pf14FileValue(in.Files, value, refs)
		}
		if field.FieldType == "calculation" {
			value, refs = nil, nil
		}
		if err = pf14ValidateValue(field, value); err != nil {
			n.fail(field.Code, err.Error())
			continue
		}
		if field.FieldType != "calculation" {
			if n.radioMustBeSelected(field, value, values) {
				n.fail(field.Code, "must be selected")
				continue
			}
			if err = pf14cValidateFormValue(n.ctx, n.q, field, value); err != nil {
				n.fail(field.Code, err.Error())
				continue
			}
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("engine: encode form value: %w", err)
		}
		var decoded any
		if err = json.Unmarshal(raw, &decoded); err != nil {
			return nil, err
		}
		var snapshot any
		if field.FieldType == "master" || field.FieldType == "user" || field.FieldType == "team" || field.FieldType == "ticket" {
			snapshot, err = n.referenceSnapshot(field.FieldType, refs)
			if err != nil {
				return nil, err
			}
			for _, old := range n.opts.ExistingInputs {
				if n.opts.LockReferenceSnapshots && old.FormFieldID == field.ID && sameStrings(old.RefIDs, refs) && old.ReferenceSnapshot != nil {
					snapshot = old.ReferenceSnapshot
					break
				}
			}
		}
		normalized = append(normalized, model.TicketInput{ID: store.NewID(), TicketID: n.ticketID, FormFieldID: field.ID, Value: decoded, RefIDs: refs, ReferenceSnapshot: snapshot})
	}
	if n.opts.Full {
		for _, section := range n.def.SectionList {
			if section.SectionType != model.SectionTypeForm || !pf14SectionVisible(section, values) || n.opts.Blocked[section.ID] {
				continue
			}
			for _, field := range section.FormFields {
				hidden := field.Hidden != nil && *field.Hidden
				if field.Required && !field.Approver && !hidden && field.FieldType != "calculation" {
					if !seen[field.ID] || pf14Empty(values[field.Code]) {
						n.fail(field.Code, "is required")
					}
				}
			}
		}
	}
	return normalized, nil
}

// INF-FRM-05 applies section visibility before conditional completeness and
// radio checks; restricted stored values remain untouched for persistence.
func (n *normalizer) actorInputValues(inputs []FieldInput) map[string]any {
	values := pf14InputValues(n.def, inputs)
	for _, section := range n.def.SectionList {
		if !n.opts.Blocked[section.ID] {
			continue
		}
		for _, field := range section.FormFields {
			values[field.ID], values[field.Code] = nil, nil
		}
	}
	return values
}

// radioMustBeSelected implements the type-specific "unselected" check that
// kickflow runs on submission and on approver form updates (drafts may keep an
// empty radio button).
func (n *normalizer) radioMustBeSelected(field model.FormField, value any, values map[string]any) bool {
	if field.FieldType != "radio_button" || !pf14Empty(value) {
		return false
	}
	if !n.opts.Full && (!n.opts.Approver || n.opts.Supplied != nil && !n.opts.Supplied[field.ID]) {
		return false
	}
	if field.Hidden != nil && *field.Hidden {
		return false
	}
	section, ok := n.idx.formSections[n.idx.fieldSection[field.ID]]
	return !ok || pf14SectionVisible(section, values)
}

// carryOverBlocked keeps stored rows of fields in sections the actor cannot
// view or enter.
func (n *normalizer) carryOverBlocked(normalized []model.TicketInput) []model.TicketInput {
	for _, row := range n.opts.ExistingInputs {
		if n.opts.Blocked[n.idx.fieldSection[row.FormFieldID]] {
			normalized = append(normalized, row)
		}
	}
	return normalized
}

// pf14FileValue normalizes a file field to its signed IDs: `files` wins, and a
// plain value (string or string array) is accepted as well.
func pf14FileValue(files []string, value any, refs []string) (any, []string) {
	ids := append([]string(nil), files...)
	if len(ids) == 0 {
		ids = pf14StringsOf(value)
	}
	if len(ids) == 0 {
		return value, refs
	}
	return ids, append([]string(nil), ids...)
}

// pf14StringsOf extracts non-empty strings from a string or a string list.
func pf14StringsOf(value any) []string {
	switch v := value.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
