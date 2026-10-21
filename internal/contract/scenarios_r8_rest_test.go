package contract_test

import (
	"net/http"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestContractStandardPlanUserListOmitsCustomFields(t *testing.T) {
	env := testutil.New(t)
	adminID := env.UserID(t, "admin")
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE users SET custom_fields=? WHERE id=?`, `{"cost-center":"CC-R8","hire-date":"2020-01-02","skills":["go"]}`, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tenant SET plan='standard'`); err != nil {
		t.Fatal(err)
	}
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	ops := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		ops[operation.OperationID] = operation
	}
	response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users?perPage=100", nil)
	pf26cCheck(t, "listUsers", response, ops, spec, compiler, make(map[string]*jsonschema.Schema))
	var users []map[string]any
	response.JSON(t, &users)
	populated := false
	for _, user := range users {
		if user["code"] == "admin" {
			populated = true
			if _, exists := user["customFields"]; exists {
				t.Fatalf("standard-plan user response includes customFields: %v", user)
			}
		}
	}
	if !populated {
		t.Fatal("expected populated admin user in list")
	}
}

func TestContractInvitedMembershipDefaultsGrade(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	ops := make(map[string]apiv1.Operation)
	for _, operation := range apiv1.Operations() {
		ops[operation.OperationID] = operation
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{
		"email": "contract-default-grade@example.test", "firstName": "Contract", "lastName": "Member",
		"memberships": []map[string]any{{"teamCode": "sales"}},
	})
	if created.Status != http.StatusOK {
		t.Fatalf("create invited user: status=%d body=%s", created.Status, created.Body)
	}
	var user map[string]any
	created.JSON(t, &user)
	chartID, err := store.ActiveOrgChartID(t.Context(), env.DB.Q())
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatal(err)
	}
	response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+chartID+"/teams/"+teamID+"/memberships?perPage=100", nil)
	pf26cCheck(t, "listTeamMembers", response, ops, spec, compiler, make(map[string]*jsonschema.Schema))
	var members []map[string]any
	response.JSON(t, &members)
	for _, member := range members {
		if member["id"] == user["id"] {
			grades, ok := member["grades"].([]any)
			if !ok || len(grades) == 0 {
				t.Fatalf("contract membership has no grades: %v", member)
			}
			return
		}
	}
	t.Fatal("created invited membership was absent from response")
}
