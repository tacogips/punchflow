package ticketui_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestResetCustomNumberingRequiresWorkflowPermissionAndResetsOnlyCounter(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	const numberingID = "b12b19aa-98ac-4c9f-a38f-b512ba1e3511"
	const otherNumberingID = "403dcdab-4e45-4a35-b1a8-12e9b76a1815"
	const sharedKeyID = "a8e3a2f1-c7d0-4e5d-9102-6ec8f0726a10"
	format := "N-[number]"
	sharedKey := sharedKeyID
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	definition.CustomNumberings = []model.CustomNumbering{
		{ID: numberingID, Code: "RESET", Name: "Reset number", Value: 1, TicketNumberKeyID: &sharedKey},
		{ID: otherNumberingID, Code: "OTHER", Name: "Other number", Value: 1},
	}
	definition.CustomNumberingSettings = []model.CustomNumberingSetting{
		{ID: "676785c3-6c25-4ba5-a832-c9666c7f8f54", CustomNumberingID: numberingID, TicketNumberFormat: &format, TimingType: "opened"},
	}
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, definitionJSON, workflowID); err != nil {
		t.Fatal(err)
	}

	issuedTicketID := createTicket(t, env)
	var issuedNumber string
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? AND custom_numbering_id=?`, issuedTicketID, numberingID).Scan(&issuedNumber); err != nil || issuedNumber != "N-1" {
		t.Fatalf("first issued number=%q err=%v, want N-1", issuedNumber, err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `INSERT INTO custom_numbering_counters(custom_numbering_id,counter,last_allocated_at,last_period) VALUES(?,42,1,'period') ON CONFLICT(custom_numbering_id) DO UPDATE SET counter=42,last_allocated_at=1,last_period='period'`, "key:"+sharedKeyID); err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `INSERT INTO custom_numbering_counters(custom_numbering_id,counter,last_allocated_at,last_period) VALUES(?,19,1,'period')`, "custom:"+otherNumberingID); err != nil {
		t.Fatal(err)
	}
	path := "/ui/api/admin/workflows/" + workflowID + "/custom-numberings/" + numberingID + "/reset"
	workflow, err := store.GetWorkflow(t.Context(), env.DB.Q(), workflowID)
	if err != nil {
		t.Fatal(err)
	}
	var outOfScopeFolder string
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT id FROM folders WHERE id<>? ORDER BY id LIMIT 1`, workflow.FolderID).Scan(&outOfScopeFolder); err != nil {
		t.Fatal(err)
	}
	const restrictedRoleID = "00000000-0000-4000-8000-000000000095"
	if _, err = env.DB.Q().ExecContext(t.Context(), `INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES(?,?,1,?,1,1)`, restrictedRoleID, "Out of scope workflow manager", `[{"permission":"workflow","restricted":true,"folderIds":["`+outOfScopeFolder+`"]}]`); err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `INSERT INTO role_memberships(role_id,user_id,created_at) VALUES(?,?,1)`, restrictedRoleID, env.UserID(t, "hr-manager")); err != nil {
		t.Fatal(err)
	}
	testutil.AssertError(t, env.Do(t, testutil.Manager, http.MethodPost, path, nil), http.StatusForbidden, "missing_permission")

	response := env.Do(t, testutil.Admin, http.MethodPost, path, nil)
	if response.Status != http.StatusNoContent {
		t.Fatalf("reset custom numbering: %d %s", response.Status, response.Body)
	}
	var counter int64
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT counter FROM custom_numbering_counters WHERE custom_numbering_id=?`, "key:"+sharedKeyID).Scan(&counter); err != nil || counter != 0 {
		t.Fatalf("reset counter=%d err=%v, want 0 before next allocation", counter, err)
	}
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT counter FROM custom_numbering_counters WHERE custom_numbering_id=?`, "custom:"+otherNumberingID).Scan(&counter); err != nil || counter != 19 {
		t.Fatalf("unselected counter=%d err=%v, want 19", counter, err)
	}
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? AND custom_numbering_id=?`, issuedTicketID, numberingID).Scan(&issuedNumber); err != nil || issuedNumber != "N-1" {
		t.Fatalf("previously issued number=%q err=%v, want unchanged N-1", issuedNumber, err)
	}

	nextTicketID := createTicket(t, env)
	var nextNumber string
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT formatted_number FROM ticket_custom_numbering_values WHERE ticket_id=? AND custom_numbering_id=?`, nextTicketID, numberingID).Scan(&nextNumber); err != nil || nextNumber != "N-1" {
		t.Fatalf("next issued number=%q err=%v, want N-1 after reset", nextNumber, err)
	}
}

func TestResetCustomNumberingRejectsUnknownWorkflowAndNumbering(t *testing.T) {
	env := testutil.New(t)
	for _, path := range []string{
		"/ui/api/admin/workflows/missing/custom-numberings/00000000-0000-4000-8000-000000000001/reset",
		"/ui/api/admin/workflows/expense-claim/custom-numberings/00000000-0000-4000-8000-000000000001/reset",
	} {
		testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, path, nil), http.StatusNotFound, "resource_not_found")
	}
}
