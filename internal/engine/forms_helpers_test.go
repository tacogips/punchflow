package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf16Redefine rewrites a workflow definition in place.
func pf16Redefine(t *testing.T, e *pf14TestEnv, code string, mutate func(*model.WorkflowDef)) {
	t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), code)
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&def)
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflow.ID)
		return err
	})
}

func pf16Def(t *testing.T, e *pf14TestEnv, code string) model.WorkflowDef {
	t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), code)
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func pf16Section(def *model.WorkflowDef, code string) *model.WorkflowSection {
	for i := range def.SectionList {
		if def.SectionList[i].Code == code {
			return &def.SectionList[i]
		}
	}
	return nil
}

func pf16SlipField(code, fieldType string) model.SlipField {
	return model.SlipField{ID: "slip-" + code + "-id", Code: code, Title: code, FieldType: fieldType, Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}}
}

func pf16FormField(code, fieldType string) model.FormField {
	return model.FormField{ID: "form-" + code + "-id", Code: code, Title: code, FieldType: fieldType, Size: "full", Orientation: "horizontal", Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}}
}

// pf16Row builds one submitted detail row addressed by section and field codes.
func pf16Row(section string, cells map[string]any) SlipItemInput {
	item := SlipItemInput{SlipSectionCode: section}
	for code, value := range cells {
		raw, _ := json.Marshal(value)
		item.Inputs = append(item.Inputs, SlipFieldInput{SlipFieldCode: code, Value: raw})
	}
	return item
}

func pf16Inputs(t *testing.T, e *pf14TestEnv, values map[string]any, skip ...string) []FieldInput {
	t.Helper()
	_, fields := pf14FormDefinition(t, e, "expense-claim")
	for _, code := range skip {
		delete(fields, code)
	}
	return pf14CompleteInputs(fields, values)
}

func pf16Create(e *pf14TestEnv, actor, status string, inputs []FieldInput, slips []SlipItemInput) (string, error) {
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	if err != nil {
		return "", err
	}
	var id string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var er error
		id, _, er = e.engine.Create(e.ctx, tx, e.actor(actor), CreateInput{Status: status, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs, SlipItems: &slips})
		return er
	})
	return id, err
}

func pf16UpdateForm(e *pf14TestEnv, actor Actor, id string, patch FormPatch) error {
	return e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.UpdateForm(e.ctx, tx, actor, id, patch)
		return err
	})
}

func pf16Update(e *pf14TestEnv, actor Actor, id string, in UpdateInput) error {
	return e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.Update(e.ctx, tx, actor, id, in)
		return err
	})
}

func pf16Slips(t *testing.T, e *pf14TestEnv, id string) []model.TicketSlipItem {
	t.Helper()
	rows, err := store.TicketSlipItems(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// pf16Cell reads a stored detail cell by field code.
func pf16Cell(t *testing.T, def model.WorkflowDef, row model.TicketSlipItem, field string) any {
	t.Helper()
	for _, s := range def.SectionList {
		if s.ID != row.SlipSectionID {
			continue
		}
		for _, f := range s.SlipFields {
			if f.Code == field {
				return row.Inputs[f.ID]
			}
		}
	}
	t.Fatalf("no detail field %q in section %s", field, row.SlipSectionID)
	return nil
}

func pf16CountSlipRows(rows []model.TicketSlipItem, sectionID string) int {
	n := 0
	for _, r := range rows {
		if r.SlipSectionID == sectionID {
			n++
		}
	}
	return n
}
