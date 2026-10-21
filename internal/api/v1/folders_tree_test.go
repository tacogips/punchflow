package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func createFolder(t *testing.T, env *testutil.Env, body map[string]any) map[string]any {
	t.Helper()
	return decodeObject(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", body))
}

func TestCreateFolderWithoutParentGoesUnderRoot(t *testing.T) {
	env := testutil.New(t)
	omitted := createFolder(t, env, map[string]any{"name": "No parent given"})
	if omitted["fullName"] != "Root/No parent given" {
		t.Fatalf("omitted parentFolderId fullName = %v, want Root/No parent given", omitted["fullName"])
	}
	explicitNull := createFolder(t, env, map[string]any{"name": "Null parent", "parentFolderId": nil})
	if explicitNull["fullName"] != "Root/Null parent" {
		t.Fatalf("null parentFolderId fullName = %v, want Root/Null parent", explicitNull["fullName"])
	}
	// Moving a folder with parentFolderId:null returns it to the root, never creating a second root.
	nested := createFolder(t, env, map[string]any{"name": "Nested", "parentFolderId": "general"})
	if nested["fullName"] != "Root/General/Nested" {
		t.Fatalf("nested fullName = %v", nested["fullName"])
	}
	moved := decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/folders/"+nested["id"].(string), map[string]any{"parentFolderId": nil}))
	if moved["fullName"] != "Root/Nested" {
		t.Fatalf("moved fullName = %v, want Root/Nested", moved["fullName"])
	}
	var roots int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM folders WHERE parent_id IS NULL`).Scan(&roots); err != nil || roots != 1 {
		t.Fatalf("root folders = %d err=%v, want exactly 1", roots, err)
	}
	// An unknown parent is still a 404 and a missing name still a 400.
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", map[string]any{"name": "Orphan", "parentFolderId": "missing"}), http.StatusNotFound, "resource_not_found")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/folders", map[string]any{"parentFolderId": nil}), http.StatusBadRequest, "invalid_parameter")
}

func folderExists(t *testing.T, env *testutil.Env, id string) bool {
	t.Helper()
	var n int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM folders WHERE id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestDeleteFolderRemovesDescendantsAndRetiresDefinitions(t *testing.T) {
	env := testutil.New(t)
	parent := createFolder(t, env, map[string]any{"name": "Parent"})
	child := createFolder(t, env, map[string]any{"name": "Child", "parentFolderId": parent["id"]})
	grandchild := createFolder(t, env, map[string]any{"name": "Grandchild", "parentFolderId": child["id"]})
	sibling := createFolder(t, env, map[string]any{"name": "Sibling"})

	// A ticket exists for the workflow before its folder is deleted.
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	for _, query := range []string{
		`UPDATE workflows SET folder_id=? WHERE code='expense-claim'`,
		`UPDATE routes SET folder_id=? WHERE code LIKE 'expense-%'`,
	} {
		if _, err := env.DB.Q().ExecContext(t.Context(), query, grandchild["id"]); err != nil {
			t.Fatal(err)
		}
	}

	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/folders/"+parent["id"].(string), nil)
	if deleted.Status != http.StatusOK || len(deleted.Body) != 0 {
		t.Fatalf("delete status=%d body=%q, want 200 and empty", deleted.Status, deleted.Body)
	}
	for name, folder := range map[string]map[string]any{"parent": parent, "child": child, "grandchild": grandchild} {
		if folderExists(t, env, folder["id"].(string)) {
			t.Errorf("%s folder survived the recursive delete", name)
		}
	}
	if !folderExists(t, env, sibling["id"].(string)) {
		t.Error("an unrelated folder was deleted")
	}

	// Workflows and routes of the deleted tree are gone from listings...
	if got := codesOf(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows", nil)); !equalStrings(got, []string{"leave-request", "purchase-request"}) {
		t.Fatalf("workflows after delete = %v", got)
	}
	if got := codesOf(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/routes", nil)); !equalStrings(got, []string{"leave", "purchase"}) {
		t.Fatalf("routes after delete = %v", got)
	}
	// ...but the existing ticket keeps working and stays presentable.
	ticket := decodeObject(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+ticketID, nil))
	workflow, _ := ticket["workflow"].(map[string]any)
	if ticket["status"] != "in_progress" || workflow["code"] != "expense-claim" {
		t.Fatalf("ticket after folder delete = status %v workflow %v", ticket["status"], workflow)
	}
	if res := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+ticketID+"/approve", nil); res.Status != http.StatusOK {
		t.Fatalf("approving a ticket of a deleted workflow: %d %s", res.Status, res.Body)
	}
	detail := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if detail.Status != http.StatusOK {
		t.Fatalf("retired workflow detail: %d %s", detail.Status, detail.Body)
	}
	testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/folders/"+parent["id"].(string), nil), http.StatusNotFound, "resource_not_found")

	// The root folder itself can never be deleted.
	if res := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/folders/root", nil); res.Status != http.StatusForbidden {
		t.Fatalf("delete root status=%d want 403 body=%s", res.Status, res.Body)
	}
}

func TestDeleteFolderTreeRequiresPermissionForRetiredDefinitions(t *testing.T) {
	env := testutil.New(t)
	grantRole(t, env, "sales-rep",
		map[string]any{"permission": "workflow_misc", "restricted": false},
		restricted("workflow", []string{scopeFolderGeneral}, nil, nil),
		restricted("route", []string{scopeFolderGeneral}, nil, nil),
	)
	// finance holds workflows outside the delegated scope: nothing may be deleted.
	denied := env.Do(t, testutil.Employee, http.MethodDelete, "/v1/folders/finance", nil)
	testutil.AssertError(t, denied, http.StatusForbidden, "missing_permission")
	if !folderExists(t, env, scopeFolderFinance) {
		t.Fatal("rejected delete removed the folder")
	}
	if got := codesOf(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows", nil)); len(got) != 3 {
		t.Fatalf("rejected delete changed workflows: %v", got)
	}
	// general is inside the scope, so the same user may delete it.
	if res := env.Do(t, testutil.Employee, http.MethodDelete, "/v1/folders/general", nil); res.Status != http.StatusOK {
		t.Fatalf("scoped delete status=%d body=%s", res.Status, res.Body)
	}
	if folderExists(t, env, scopeFolderGeneral) {
		t.Fatal("scoped delete left the folder")
	}
	if got := codesOf(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/workflows", nil)); !equalStrings(got, []string{"expense-claim"}) {
		t.Fatalf("workflows after scoped delete = %v, want only expense-claim", got)
	}
}
