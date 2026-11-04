package engine

import (
	"context"
	"sort"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf14SyncInputSideEffects maintains the data derived from stored form and
// detail values: file attachment associations (so replaced or removed files
// disappear) and the related-ticket links created by autoLink ticket fields.
// old* are the values before the edit; res holds the values now stored.
func pf14SyncInputSideEffects(ctx context.Context, q store.Querier, def model.WorkflowDef, ticketID string, oldInputs []model.TicketInput, oldSlips []model.TicketSlipItem, res formResult, nowMs int64) error {
	if err := store.EngSyncInputAttachments(ctx, q, ticketID, pf14InputAttachments(def, res.Inputs, res.Slips), nowMs); err != nil {
		return err
	}
	before := pf14AutoLinkTargets(def, oldInputs, oldSlips)
	after := pf14AutoLinkTargets(def, res.Inputs, res.Slips)
	for _, id := range sortedKeys(after) {
		if id == ticketID {
			continue
		}
		if err := store.EngLinkTickets(ctx, q, ticketID, id, nowMs); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(before) {
		if !after[id] && id != ticketID {
			if err := store.EngUnlinkTickets(ctx, q, ticketID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// pf14InputAttachments lists the file associations implied by the values.
func pf14InputAttachments(def model.WorkflowDef, inputs []model.TicketInput, slips []model.TicketSlipItem) []store.InputAttachment {
	idx := newFormIndex(def)
	out := []store.InputAttachment{}
	for _, in := range inputs {
		if f, ok := idx.fieldByID[in.FormFieldID]; ok && f.FieldType == "file" {
			out = append(out, store.InputAttachment{FormFieldID: f.ID, SignedIDs: pf14RefIDs(in.RefIDs, in.Value)})
		}
	}
	for _, row := range slips {
		section, ok := idx.slipByID[row.SlipSectionID]
		if !ok {
			continue
		}
		for _, f := range section.SlipFields {
			if f.FieldType != "file" {
				continue
			}
			v, _ := slipCell(row, f)
			out = append(out, store.InputAttachment{SlipItemID: row.ID, SignedIDs: pf14StringsOf(v)})
		}
	}
	return out
}

// pf14AutoLinkTargets collects the tickets referenced by autoLink ticket fields.
func pf14AutoLinkTargets(def model.WorkflowDef, inputs []model.TicketInput, slips []model.TicketSlipItem) map[string]bool {
	idx := newFormIndex(def)
	out := map[string]bool{}
	for _, in := range inputs {
		if f, ok := idx.fieldByID[in.FormFieldID]; ok && f.FieldType == "ticket" && f.AutoLink {
			for _, id := range pf14RefIDs(in.RefIDs, in.Value) {
				out[id] = true
			}
		}
	}
	for _, row := range slips {
		section, ok := idx.slipByID[row.SlipSectionID]
		if !ok {
			continue
		}
		for _, f := range section.SlipFields {
			if f.FieldType == "ticket" && f.AutoLink {
				v, _ := slipCell(row, f)
				for _, id := range pf14StringsOf(v) {
					out[id] = true
				}
			}
		}
	}
	return out
}

// pf14RefIDs prefers the stored reference list and falls back to the value.
func pf14RefIDs(refs []string, value any) []string {
	if len(refs) > 0 {
		return refs
	}
	return pf14StringsOf(value)
}
