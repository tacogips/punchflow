package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestDeleteUserInvalidatesDirectApproverRouteAndWorkflow(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	workflow, err := store.GetWorkflowByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(ctx, env.DB.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(ctx, `UPDATE routes SET definition_json=json_set(definition_json,'$.steps[0].userCodes',json_array('assistant')) WHERE code=?`, definition.RouteCode); err != nil {
		t.Fatal(err)
	}
	userID := env.UserID(t, "assistant")
	response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/"+userID, nil)
	if response.Status != http.StatusOK {
		t.Fatalf("delete user: status=%d body=%s", response.Status, response.Body)
	}
	route, err := store.GetRouteByCode(ctx, env.DB.Q(), definition.RouteCode)
	if err != nil || route.Status != model.RouteStatusError {
		t.Fatalf("route status=%q, err=%v; want error", route.Status, err)
	}
	workflow, err = store.GetWorkflowByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil || workflow.Status != model.WorkflowStatusInvisible {
		t.Fatalf("workflow status=%q, err=%v; want invisible", workflow.Status, err)
	}
}

func TestDeleteTeamInvalidatesRouteAndRemovesRoleTarget(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	workflow, err := store.GetWorkflowByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(ctx, env.DB.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.TeamIDByCode(ctx, env.DB.Q(), "00000000-0000-4000-8000-000000000027", "sales-east")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(ctx, `UPDATE routes SET definition_json=json_set(definition_json,'$.steps[1].targets[0].teamCode','sales-east') WHERE code=?`, definition.RouteCode); err != nil {
		t.Fatal(err)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/roles", map[string]any{"name": "Scoped team role", "permissionList": []any{map[string]any{
		"permission": "team", "restricted": true, "teamIds": []string{teamID},
	}}})
	if created.Status != http.StatusOK {
		t.Fatalf("create scoped role: status=%d body=%s", created.Status, created.Body)
	}
	var role struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body, &role); err != nil || role.ID == "" {
		t.Fatalf("decode role id=%q err=%v", role.ID, err)
	}
	response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/organizationCharts/00000000-0000-4000-8000-000000000027/teams/sales-east", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("delete team: status=%d body=%s", response.Status, response.Body)
	}
	route, err := store.GetRouteByCode(ctx, env.DB.Q(), definition.RouteCode)
	if err != nil || route.Status != model.RouteStatusError {
		t.Fatalf("route status=%q, err=%v; want error", route.Status, err)
	}
	workflow, err = store.GetWorkflowByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil || workflow.Status != model.WorkflowStatusInvisible {
		t.Fatalf("workflow status=%q, err=%v; want invisible", workflow.Status, err)
	}
	roleResponse := env.Do(t, testutil.Admin, http.MethodGet, "/v1/roles/"+role.ID, nil)
	if roleResponse.Status != http.StatusOK {
		t.Fatalf("get role after team deletion: status=%d body=%s", roleResponse.Status, roleResponse.Body)
	}
	var decoded struct {
		PermissionList []struct {
			Teams []any `json:"teams"`
		} `json:"permissionList"`
	}
	if err := json.Unmarshal(roleResponse.Body, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.PermissionList) != 1 || len(decoded.PermissionList[0].Teams) != 0 {
		t.Fatalf("role permission targets after deletion=%+v, want empty team list", decoded.PermissionList)
	}
}
