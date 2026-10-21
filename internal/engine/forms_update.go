package engine

import (
	"context"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf16StringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// pf14cApplyFormEdit applies the inputs/slipItems of an update to a stored
// ticket. partial selects the /form PATCH contract (per-field and
// per-detail-section differential update); otherwise inputs replace the whole
// form and slipItems replace every detail row. Values the actor cannot edit are
// preserved. The calculated title, if any, is set on t.
func (e *Engine) pf14cApplyFormEdit(ctx context.Context, tx *store.Tx, a Actor, t *model.Ticket, def model.WorkflowDef, in UpdateInput, partial, approver bool, now time.Time) error {
	blocked, err := pf16BlockedSections(ctx, tx, a, *t, def)
	if err != nil {
		return err
	}
	existingInputs, err := store.EngTicketInputs(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	existingSlips, err := store.TicketSlipItems(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	stored := pf14cInputsFromRows(def, existingInputs)
	inputValues := stored
	supplied := map[string]bool{}
	if in.Inputs != nil {
		if supplied, err = pf14cSuppliedFields(def, *in.Inputs); err != nil {
			return err
		}
		if err = pf16RejectBlockedInputs(def, blocked, *in.Inputs); err != nil {
			return err
		}
		switch {
		case approver:
			if err = pf14cValidateApproverPatch(def, *in.Inputs); err != nil {
				return err
			}
			inputValues = pf14cMergeInputs(def, stored, *in.Inputs)
		case partial:
			inputValues = pf14cMergeInputs(def, stored, *in.Inputs)
		default:
			if err = pf14ValidateCreateCompleteness(def, *in.Inputs, blocked); err != nil {
				return err
			}
			inputValues = *in.Inputs
		}
	}
	opts := formOpts{
		Full: in.Status != nil && *in.Status == model.TicketStatusInProgress, Partial: partial, Approver: approver,
		Blocked: blocked, Supplied: supplied, ExistingInputs: existingInputs, ExistingSlips: existingSlips,
		AuthorID: pf16StringOrEmpty(t.AuthorID), ProxyClientID: t.ProxyClientUserID, AuthorTeamJSON: t.AuthorTeamSnapshot,
	}
	slipValues := []SlipItemInput{}
	if in.SlipItems != nil {
		slipValues = *in.SlipItems
		opts.SlipMode = slipReplaceAll
		if partial {
			opts.SlipMode = slipReplaceSections
		}
	}
	return e.pf14cStoreForm(ctx, tx, t, def, inputValues, slipValues, opts, existingInputs, existingSlips, now)
}

// pf14cRevalidateForSubmit revalidates the stored form and detail rows as a
// complete submission and returns the calculated values as route inputs.
func (e *Engine) pf14cRevalidateForSubmit(ctx context.Context, tx *store.Tx, a Actor, t *model.Ticket, def model.WorkflowDef, now time.Time) ([]FieldInput, error) {
	blocked, err := pf16BlockedSections(ctx, tx, a, *t, def)
	if err != nil {
		return nil, err
	}
	existingInputs, err := store.EngTicketInputs(ctx, tx, t.ID)
	if err != nil {
		return nil, err
	}
	existingSlips, err := store.TicketSlipItems(ctx, tx, t.ID)
	if err != nil {
		return nil, err
	}
	opts := formOpts{Full: true, Blocked: blocked, ExistingInputs: existingInputs, ExistingSlips: existingSlips, AuthorID: pf16StringOrEmpty(t.AuthorID), ProxyClientID: t.ProxyClientUserID, AuthorTeamJSON: t.AuthorTeamSnapshot}
	stored := pf14cInputsFromRows(def, existingInputs)
	res, err := pf14NormalizeForm(ctx, tx, t.ID, def, stored, nil, opts, store.Millis(now))
	if err != nil {
		return nil, err
	}
	if err = e.pf14cWriteForm(ctx, tx, t, def, res, existingInputs, existingSlips, now); err != nil {
		return nil, err
	}
	return pf14cInputsFromRows(def, res.Inputs), nil
}

func (e *Engine) pf14cStoreForm(ctx context.Context, tx *store.Tx, t *model.Ticket, def model.WorkflowDef, inputs []FieldInput, slips []SlipItemInput, opts formOpts, oldInputs []model.TicketInput, oldSlips []model.TicketSlipItem, now time.Time) error {
	res, err := pf14NormalizeForm(ctx, tx, t.ID, def, inputs, slips, opts, store.Millis(now))
	if err != nil {
		return err
	}
	return e.pf14cWriteForm(ctx, tx, t, def, res, oldInputs, oldSlips, now)
}

func (e *Engine) pf14cWriteForm(ctx context.Context, tx *store.Tx, t *model.Ticket, def model.WorkflowDef, res formResult, oldInputs []model.TicketInput, oldSlips []model.TicketSlipItem, now time.Time) error {
	if err := store.EngReplaceInputs(ctx, tx, t.ID, res.Inputs); err != nil {
		return err
	}
	if res.SlipsChanged {
		if err := store.EngReplaceSlipItems(ctx, tx, t.ID, res.Slips); err != nil {
			return err
		}
	}
	if err := pf14SyncInputSideEffects(ctx, tx, def, t.ID, oldInputs, oldSlips, res, store.Millis(now)); err != nil {
		return err
	}
	if res.Title != nil {
		t.Title = res.Title
	}
	return nil
}
