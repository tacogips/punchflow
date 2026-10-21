package engine

import (
	"context"

	"punchflow/internal/access"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf14cSuppliedFields resolves the form fields a client patch refers to
// (formFieldId wins over formFieldCode) and rejects duplicate references with
// 400 before the patch is merged with stored values. Unknown fields are left
// for normalization to report.
func pf14cSuppliedFields(def model.WorkflowDef, patch []FieldInput) (map[string]bool, error) {
	idx := newFormIndex(def)
	supplied := map[string]bool{}
	for _, in := range patch {
		field, ok := idx.field(in)
		if !ok {
			continue
		}
		if supplied[field.ID] {
			return nil, kferr.InvalidParameter("Form field must be unique")
		}
		supplied[field.ID] = true
	}
	return supplied, nil
}

// pf16BlockedSections returns the sections the actor may not view or enter.
// It is empty (and cheap) for workflows without restricted sections.
func pf16BlockedSections(ctx context.Context, q store.Querier, a Actor, t model.Ticket, def model.WorkflowDef) (map[string]bool, error) {
	restricted := false
	for _, s := range def.SectionList {
		restricted = restricted || s.ViewRestriction
	}
	if !restricted {
		return nil, nil
	}
	allowed, err := access.SectionAccess(ctx, q, access.Caller{UserID: a.UserID, TicketRead: a.Permissions[model.PermissionTicketRead]}, t, def)
	if err != nil {
		return nil, err
	}
	blocked := map[string]bool{}
	for id, ok := range allowed {
		if !ok {
			blocked[id] = true
		}
	}
	return blocked, nil
}

// pf16RejectBlockedInputs refuses client-supplied values for fields in
// sections the actor cannot view or enter.
func pf16RejectBlockedInputs(def model.WorkflowDef, blocked map[string]bool, patch []FieldInput) error {
	if len(blocked) == 0 {
		return nil
	}
	idx := newFormIndex(def)
	for _, in := range patch {
		if field, ok := idx.field(in); ok && blocked[idx.fieldSection[field.ID]] {
			return kferr.InvalidParameter("Invalid form field code")
		}
	}
	return nil
}
