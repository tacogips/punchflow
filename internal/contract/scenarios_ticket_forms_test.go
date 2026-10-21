package contract_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	pf16PhotoFieldID    = "00000000-0000-4000-8000-0000000000a1"
	pf16SubtotalFieldID = "00000000-0000-4000-8000-0000000000a2"
	pf16TotalFieldID    = "00000000-0000-4000-8000-0000000000a3"
	pf16ConfSectionID   = "00000000-0000-4000-8000-0000000000a4"
	pf16ConfViewerID    = "00000000-0000-4000-8000-0000000000a5"
)

func pf16SlipFieldDef(id, code, fieldType string) model.SlipField {
	return model.SlipField{ID: id, Code: code, Title: code, FieldType: fieldType, Author: true, Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}}
}

// pf16PrepareExpenseWorkflow adds a file and a calculation detail column, a
// calculation form field and a restricted section to expense-claim.
func pf16PrepareExpenseWorkflow(t *testing.T, env *testutil.Env) {
	t.Helper()
	ctx := t.Context()
	id, err := store.WorkflowIDByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(ctx, env.DB.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	subtotalFormula, totalFormula := "$qty * ${unit-price}", "$amount * 2 + $sum${unit-price}"
	var note model.FormField
	for i := range def.SectionList {
		section := &def.SectionList[i]
		switch section.SectionType {
		case model.SectionTypeSlip:
			for i := range section.SlipFields {
				if section.SlipFields[i].Code == "unit-price" {
					section.SlipFields[i].ShowTotal = true
				}
			}
			photo := pf16SlipFieldDef(pf16PhotoFieldID, "photo", "file")
			photo.Multiple = true
			subtotal := pf16SlipFieldDef(pf16SubtotalFieldID, "subtotal", "calculation")
			subtotal.Formula = &subtotalFormula
			section.SlipFields = append(section.SlipFields, photo, subtotal)
		case model.SectionTypeForm:
			kept := make([]model.FormField, 0, len(section.FormFields))
			for _, f := range section.FormFields {
				switch f.Code {
				case "approver-note":
					note = f
					continue
				case "receipt":
					f.Multiple = true
				}
				kept = append(kept, f)
			}
			total := model.FormField{ID: pf16TotalFieldID, Code: "total", Title: "Total", FieldType: "calculation", Formula: &totalFormula, Author: true, Size: "full", Orientation: "horizontal", Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}}
			section.FormFields = append(kept, total)
		}
	}
	def.SectionList = append(def.SectionList, model.WorkflowSection{
		ID: pf16ConfSectionID, Title: "Confidential", SectionType: model.SectionTypeForm, FormFields: []model.FormField{note},
		ViewRestriction: true, AllowViewAuthor: false, AllowViewApprover: true,
		SectionViewers: []model.SectionViewer{{ID: pf16ConfViewerID, UserCode: "ceo"}},
	})
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err = env.DB.WithTx(ctx, func(tx *store.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), id)
		return e
	}); err != nil {
		t.Fatal(err)
	}
}

func TestContractSelectableTicketWorkflowsAndDetailMasterDefault(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	def, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	for sectionIndex := range def.SectionList {
		section := &def.SectionList[sectionIndex]
		for fieldIndex := range section.FormFields {
			if section.FormFields[fieldIndex].Code == "related" {
				section.FormFields[fieldIndex].SelectableTicketWorkflowCodes = []string{"purchase-request"}
			}
		}
		for fieldIndex := range section.SlipFields {
			if section.SlipFields[fieldIndex].Code == "item" {
				section.SlipFields[fieldIndex].FieldType = "master"
				section.SlipFields[fieldIndex].GeneralMasterCode = "cost-centers"
				section.SlipFields[fieldIndex].DefaultGeneralMasterItemCode = "cc-sales"
				section.SlipFields[fieldIndex].SelectableTicketWorkflowCodes = []string{"purchase-request"}
			}
		}
	}
	def.SectionList[0].FormFields = append(def.SectionList[0].FormFields, model.FormField{
		ID: store.NewID(), Code: "contract-source", Title: "Source ticket", FieldType: "ticket", Author: true, Size: "full", Orientation: "horizontal",
		Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{},
		SelectableTicketWorkflowCodes: []string{"purchase-request"},
	})
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(raw), workflowID); err != nil {
		t.Fatal(err)
	}
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	operation := operations["getWorkflow"]
	response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	compiler := schemaCompiler(t, loadSpec(t))
	pf26aCheck(t, operation, response, pf16ResponseSchema(t, compiler, operation), loadSpec(t))
	var body struct {
		SectionList []struct {
			FormFields []struct {
				Code    string           `json:"code"`
				Choices []map[string]any `json:"selectableTicketWorkflows"`
			} `json:"formFields"`
			SlipFields []struct {
				Code    string           `json:"code"`
				Default map[string]any   `json:"defaultGeneralMasterItem"`
				Choices []map[string]any `json:"selectableTicketWorkflows"`
			} `json:"slipFields"`
		} `json:"sectionList"`
	}
	response.JSON(t, &body)
	var sourceChoice bool
	var detailDefault, detailChoice bool
	for _, section := range body.SectionList {
		for _, field := range section.FormFields {
			if field.Code == "contract-source" && len(field.Choices) == 1 && field.Choices[0]["code"] == "purchase-request" {
				sourceChoice = true
			}
		}
		for _, field := range section.SlipFields {
			if field.Code == "item" && field.Default["code"] == "cc-sales" && len(field.Choices) == 1 && field.Choices[0]["code"] == "purchase-request" {
				detailDefault, detailChoice = true, true
			}
		}
	}
	if !sourceChoice || !detailDefault || !detailChoice {
		t.Fatalf("populated ticket/detail field metadata missing: %s", response.Body)
	}
}

func pf16ResponseSchema(t *testing.T, compiler *jsonschema.Compiler, operation apiv1.Operation) *jsonschema.Schema {
	t.Helper()
	schema, err := compiler.Compile(responseSchemaURI(operation.Pattern, strings.ToLower(operation.Method)))
	if err != nil {
		t.Fatalf("compile %s response schema: %v", operation.OperationID, err)
	}
	return schema
}

// TestContractTicketDetailInputsAndSections validates a ticket with a
// populated detail section, file attachments, calculated values and a
// restricted section against the OpenAPI response schemas.
func TestContractTicketDetailInputsAndSections(t *testing.T) {
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	env := testutil.New(t, testutil.WithPublicBaseURL("http://punchflow.test"))
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	pf16PrepareExpenseWorkflow(t, env)

	upload := pf26bUploadFile(t, env, []byte("%PDF-1.4 receipt"), true)
	var uploaded struct {
		SignedID string `json:"signedId"`
	}
	upload.JSON(t, &uploaded)
	if uploaded.SignedID == "" {
		t.Fatalf("upload returned no signedId: %s", upload.Body)
	}

	workflowID, _ := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	salesTeamID, _ := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	costCenterID, err := store.ResolveID(t.Context(), env.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	costCenterItemID, err := store.MasterItemIDByCode(t.Context(), env.DB.Q(), costCenterID, "cc-sales")
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	slipFields := map[string]string{}
	var linesSection string
	wf := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	var workflow struct {
		SectionList []struct {
			ID         string `json:"id"`
			Code       string `json:"code"`
			FormFields []struct {
				ID, Code string
			} `json:"formFields"`
			SlipFields []struct {
				ID, Code string
			} `json:"slipFields"`
		} `json:"sectionList"`
	}
	wf.JSON(t, &workflow)
	for _, s := range workflow.SectionList {
		for _, f := range s.FormFields {
			fields[f.Code] = f.ID
		}
		for _, f := range s.SlipFields {
			slipFields[f.Code] = f.ID
		}
		if s.Code == "lines" {
			linesSection = s.ID
		}
	}

	slipRow := func(item string, qty, price float64, files []string) map[string]any {
		return map[string]any{"slipSectionId": linesSection, "inputs": []map[string]any{
			{"slipFieldId": slipFields["item"], "value": item},
			{"slipFieldId": slipFields["qty"], "value": fmt.Sprint(qty)},
			{"slipFieldId": slipFields["unit-price"], "value": fmt.Sprint(price)},
			{"slipFieldId": slipFields["photo"], "files": files},
			{"slipFieldId": slipFields["subtotal"], "value": nil},
		}}
	}
	created := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets", map[string]any{
		"status": "draft", "workflowId": workflowID, "authorTeamId": salesTeamID, "title": "PF-16 detail contract",
		"inputs": []map[string]any{
			{"formFieldId": fields["amount"], "value": "100"},
			{"formFieldId": fields["purpose"], "value": "detail rows"},
			{"formFieldId": fields["expense-date"], "value": "2026-01-01"},
			{"formFieldId": fields["cost-center"], "generalMasterItemId": costCenterItemID},
			{"formFieldId": fields["receipt"], "files": []string{uploaded.SignedID}},
			{"formFieldId": fields["total"], "value": nil},
		},
		"slipItems": []map[string]any{slipRow("pen", 2, 10.5, []string{uploaded.SignedID}), slipRow("pad", 1, 4, []string{})},
	})
	pf26aCheck(t, operations["createTicket"], created, pf16ResponseSchema(t, compiler, operations["createTicket"]), spec)

	var ticket struct {
		ID        string           `json:"id"`
		Inputs    []map[string]any `json:"inputs"`
		SlipItems []struct {
			Inputs []struct {
				Value       any              `json:"value"`
				Attachments []map[string]any `json:"attachments"`
				SlipField   map[string]any   `json:"slipField"`
			} `json:"inputs"`
		} `json:"slipItems"`
		TicketSections []map[string]any `json:"ticketSections"`
	}
	created.JSON(t, &ticket)
	if len(ticket.SlipItems) != 2 {
		t.Fatalf("slipItems=%d want 2: %s", len(ticket.SlipItems), created.Body)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at) VALUES(?,?,?,?,?,0,0)`, store.NewID(), ticket.ID, fields["approver-note"], `"restricted value"`, `[]`); err != nil {
		t.Fatal(err)
	}
	populated := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+ticket.ID, nil)
	pf26aCheck(t, operations["getTicket"], populated, pf16ResponseSchema(t, compiler, operations["getTicket"]), spec)
	var populatedTicket struct {
		Inputs []map[string]any `json:"inputs"`
	}
	populated.JSON(t, &populatedTicket)
	foundRestricted := false
	for _, input := range populatedTicket.Inputs {
		field, _ := input["formField"].(map[string]any)
		if field["code"] != "approver-note" {
			continue
		}
		foundRestricted = true
		if input["id"] == nil || input["formFieldId"] != fields["approver-note"] || input["value"] != nil {
			t.Errorf("restricted input identity/definition/redaction=%v", input)
		}
		for _, key := range []string{"generalMasterItems", "users", "teams", "inputTickets", "attachments"} {
			if values, _ := input[key].([]any); len(values) != 0 {
				t.Errorf("restricted input %s=%v, want empty", key, values)
			}
		}
	}
	if !foundRestricted {
		t.Fatal("restricted input was omitted instead of retaining its input and formField identity")
	}
	first := ticket.SlipItems[0].Inputs
	var photoAttachments, subtotal = 0, any(nil)
	for _, cell := range first {
		switch cell.SlipField["code"] {
		case "photo":
			photoAttachments = len(cell.Attachments)
			if len(cell.Attachments) == 1 && cell.Attachments[0]["signedId"] != uploaded.SignedID {
				t.Errorf("detail attachment=%v", cell.Attachments[0])
			}
		case "subtotal":
			subtotal = cell.Value
		}
	}
	if photoAttachments != 1 || subtotal != 21.0 {
		t.Errorf("first detail row photo attachments=%d subtotal=%v, want 1 and 21", photoAttachments, subtotal)
	}
	for _, in := range ticket.Inputs {
		field, _ := in["formField"].(map[string]any)
		attachments, _ := in["attachments"].([]any)
		switch field["code"] {
		case "cost-center":
			value, ok := in["value"].([]any)
			if !ok || len(value) != 1 {
				t.Errorf("master input value=%T %v, want one cached record", in["value"], in["value"])
				break
			}
			record, ok := value[0].(map[string]any)
			if !ok || record["code"] != "cc-sales" {
				t.Errorf("master input cached record=%v, want code cc-sales", value[0])
			}
		case "receipt":
			if len(attachments) != 1 {
				t.Errorf("receipt attachments=%v want one", attachments)
			}
		case "total":
			// $amount*2 + $sum${unit-price} = 200 + 10.5 + 4.
			if in["value"] != 214.5 {
				t.Errorf("total=%v want 214.5", in["value"])
			}
		}
	}
	for _, section := range ticket.TicketSections {
		if section["sectionId"] == pf16ConfSectionID {
			if section["viewable"] != false || section["viewRestriction"] != true {
				t.Errorf("restricted section for the author=%v", section)
			}
			if viewers, _ := section["sectionViewers"].([]any); len(viewers) != 1 {
				t.Errorf("sectionViewers=%v", viewers)
			}
		}
	}

	patch := env.Do(t, testutil.Employee, http.MethodPatch, "/v1/tickets/"+ticket.ID+"/form", map[string]any{
		"slipItems": []map[string]any{slipRow("pen", 3, 10, []string{})},
	})
	pf26aCheck(t, operations["updateTicketForm"], patch, pf16ResponseSchema(t, compiler, operations["updateTicketForm"]), spec)
	var patched struct {
		SlipItems []json.RawMessage `json:"slipItems"`
	}
	patch.JSON(t, &patched)
	if len(patched.SlipItems) != 1 {
		t.Errorf("form patch left %d detail rows, want the 1 submitted row", len(patched.SlipItems))
	}

	// An administrator with ticket_read sees the restricted section.
	admin := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets/"+ticket.ID, nil)
	pf26aCheck(t, operations["getTicket"], admin, pf16ResponseSchema(t, compiler, operations["getTicket"]), spec)
	var adminView struct {
		TicketSections []map[string]any `json:"ticketSections"`
	}
	admin.JSON(t, &adminView)
	for _, section := range adminView.TicketSections {
		if section["sectionId"] == pf16ConfSectionID && section["viewable"] != true {
			t.Errorf("ticket_read administrator cannot view restricted section: %v", section)
		}
	}
}
