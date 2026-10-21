package engine

import (
	"context"
	"encoding/json"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// INF-F7-09: pf27ValidateFormWireValues checks the value shape accepted by createTicket
// and updateTicket. The differential /form endpoint has a broader contract
// and deliberately does not call this validator.
func pf27ValidateFormWireValues(def model.WorkflowDef, inputs []FieldInput, slips []SlipItemInput) error {
	formFields := make(map[string]model.FormField)
	slipFields := make(map[string]model.SlipField)
	for _, section := range def.SectionList {
		for _, field := range section.FormFields {
			formFields[field.ID] = field
			formFields[field.Code] = field
		}
		for _, field := range section.SlipFields {
			slipFields[field.ID] = field
			slipFields[field.Code] = field
		}
	}
	for _, input := range inputs {
		field, ok := formFields[input.FormFieldID]
		if !ok {
			field, ok = formFields[input.FormFieldCode]
		}
		if ok && !pf27ValidWireValue(input.Value, field.FieldType == "checkbox") {
			return kferr.InvalidParameter("Invalid form input value")
		}
	}
	for _, slip := range slips {
		for _, input := range slip.Inputs {
			field, ok := slipFields[input.SlipFieldID]
			if !ok {
				field, ok = slipFields[input.SlipFieldCode]
			}
			if ok && !pf27ValidWireValue(input.Value, field.FieldType == "checkbox") {
				return kferr.InvalidParameter("Invalid detail input value")
			}
		}
	}
	return nil
}

// ValidateRESTFormWireValues applies the createTicket/updateTicket value
// schema using a workflow ID. The differential updateTicketForm operation
// intentionally uses a broader value contract.
func (e *Engine) ValidateRESTFormWireValues(ctx context.Context, q store.Querier, workflowID string, inputs []FieldInput, slips []SlipItemInput) error {
	workflow, err := store.GetWorkflow(ctx, q, workflowID)
	if err != nil {
		return kferr.ResourceNotFound()
	}
	def, err := store.WorkflowDefByID(ctx, q, workflow.ID)
	if err != nil {
		return err
	}
	return pf27ValidateFormWireValues(def, inputs, slips)
}

func pf27ValidWireValue(raw json.RawMessage, checkbox bool) bool {
	if len(raw) == 0 {
		return true
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return !checkbox
	case []any:
		for _, item := range typed {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func pf27ValidateApproverInputCompleteness(def model.WorkflowDef, inputs []FieldInput, blocked map[string]bool) error {
	provided := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		provided[input.FormFieldID] = true
		provided[input.FormFieldCode] = true
	}
	for _, section := range def.SectionList {
		if blocked[section.ID] {
			continue
		}
		for _, field := range section.FormFields {
			if !field.Approver && !def.Overwritable || field.Hidden != nil && *field.Hidden {
				continue
			}
			if !provided[field.ID] && !provided[field.Code] {
				return kferr.InvalidParameter("Missing form field: " + field.Code)
			}
		}
	}
	return nil
}
