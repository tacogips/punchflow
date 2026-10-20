package v1_test

import (
	"database/sql"
	"net/http"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestUsersListPermissionsAndLookup(t *testing.T) {
	env := testutil.New(t)
	current := env.Do(t, testutil.Admin, http.MethodGet, "/v1/user", nil)
	if current.Status != http.StatusOK {
		t.Fatalf("current user status=%d body=%s", current.Status, current.Body)
	}
	var currentBody map[string]any
	current.JSON(t, &currentBody)
	if currentBody["code"] != "admin" || currentBody["userType"] != model.UserTypeNormal || currentBody["deactivatedAt"] != nil {
		t.Fatalf("current user=%v", currentBody)
	}
	list := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users?perPage=5", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Status, list.Body)
	}
	var rows []map[string]any
	list.JSON(t, &rows)
	if len(rows) != 5 {
		t.Fatalf("list count=%d", len(rows))
	}
	testutil.AssertPagination(t, list, 1, 5, 13)
	if denied := env.Do(t, testutil.Employee, http.MethodGet, "/v1/users", nil); denied.Status != http.StatusForbidden {
		t.Fatalf("denied status=%d body=%s", denied.Status, denied.Body)
	}
	if bad := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users?status=invalid", nil); bad.Status != http.StatusBadRequest {
		t.Fatalf("bad status filter=%d body=%s", bad.Status, bad.Body)
	}
	byCode := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/admin", nil)
	if byCode.Status != http.StatusOK {
		t.Fatalf("get by code status=%d body=%s", byCode.Status, byCode.Body)
	}
	lookup := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/lookupByEmail?email=ADMIN%40punchflow.test", nil)
	if lookup.Status != http.StatusOK {
		t.Fatalf("lookup status=%d body=%s", lookup.Status, lookup.Body)
	}
	unknown := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/lookupByEmail?email=missing%40punchflow.test", nil)
	testutil.AssertError(t, unknown, http.StatusNotFound, "resource_not_found")
}

func TestUserCreateUpdateAndDomainRules(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "new@example.test", "firstName": "New", "lastName": "Person"})
	if created.Status != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Status, created.Body)
	}
	var user map[string]any
	created.JSON(t, &user)
	if user["status"] != model.UserStatusInvited || user["email"] != "new@example.test" {
		t.Fatalf("created user=%v", user)
	}
	id := user["id"].(string)
	duplicate := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "NEW@example.test", "firstName": "Other", "lastName": "Person"})
	testutil.AssertError(t, duplicate, http.StatusUnprocessableEntity, "validation_failed")
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tenant SET allowed_email_domains='["example.com"]'`); err != nil {
		t.Fatal(err)
	}
	domain := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "outside@punchflow.test", "firstName": "Out", "lastName": "Side"})
	testutil.AssertError(t, domain, http.StatusUnprocessableEntity, "domain_not_allowed")
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+id, map[string]any{"firstName": "Updated", "employeeId": nil})
	if updated.Status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Status, updated.Body)
	}
	var changed map[string]any
	updated.JSON(t, &changed)
	if changed["firstName"] != "Updated" || changed["employeeId"] != nil {
		t.Fatalf("updated user=%v", changed)
	}
	var inviteCount int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM user_invitations WHERE user_id=?`, id).Scan(&inviteCount); err != nil || inviteCount != 1 {
		t.Fatalf("invitation count=%d err=%v", inviteCount, err)
	}
}

func TestUserStatusTransitionsAndAudit(t *testing.T) {
	env := testutil.New(t)
	id := env.UserID(t, "sales-rep")
	suspended := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+id+"/suspend", nil)
	if suspended.Status != http.StatusOK {
		t.Fatalf("suspend status=%d body=%s", suspended.Status, suspended.Body)
	}
	var user map[string]any
	suspended.JSON(t, &user)
	if user["status"] != model.UserStatusSuspended {
		t.Fatalf("suspended user=%v", user)
	}
	again := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+id+"/suspend", nil)
	testutil.AssertError(t, again, http.StatusUnprocessableEntity, "invalid_state")
	reactivated := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+id+"/reactivate", nil)
	if reactivated.Status != http.StatusOK {
		t.Fatalf("reactivate status=%d body=%s", reactivated.Status, reactivated.Body)
	}
	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/"+id, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Status, deleted.Body)
	}
	var count int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='user.suspend'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count=%d err=%v", count, err)
	}
}

func TestUsersTeamsAndRoles(t *testing.T) {
	env := testutil.New(t)
	id := env.UserID(t, "sales-rep")
	teams := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+id+"/teams", nil)
	if teams.Status != http.StatusOK {
		t.Fatalf("teams status=%d body=%s", teams.Status, teams.Body)
	}
	var teamRows []map[string]any
	teams.JSON(t, &teamRows)
	if len(teamRows) != 2 {
		t.Fatalf("teams=%v", teamRows)
	}
	filtered := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+id+"/teams?submittable=true&perPage=1", nil)
	if filtered.Status != http.StatusOK {
		t.Fatalf("submittable teams status=%d body=%s", filtered.Status, filtered.Body)
	}
	testutil.AssertPagination(t, filtered, 1, 1, 2)
	badFilter := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+id+"/teams?submittable=maybe", nil)
	testutil.AssertError(t, badFilter, http.StatusBadRequest, "invalid_parameter")
	unknown := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/no-such-user/teams", nil)
	testutil.AssertError(t, unknown, http.StatusNotFound, "resource_not_found")
	roles := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+env.UserID(t, "admin")+"/roles", nil)
	if roles.Status != http.StatusOK {
		t.Fatalf("roles status=%d body=%s", roles.Status, roles.Body)
	}
	var roleRows []map[string]any
	roles.JSON(t, &roleRows)
	if len(roleRows) == 0 {
		t.Fatal("admin roles are empty")
	}
}

func TestUserReinviteAndSuspendedTokenInvalidation(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "invite-again@example.test", "firstName": "Invite", "lastName": "Again"})
	if created.Status != http.StatusOK {
		t.Fatalf("create invited user status=%d body=%s", created.Status, created.Body)
	}
	var invited map[string]any
	created.JSON(t, &invited)
	id := invited["id"].(string)
	reinvite := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+id+"/reinvite", nil)
	if reinvite.Status != http.StatusOK {
		t.Fatalf("reinvite status=%d body=%s", reinvite.Status, reinvite.Body)
	}
	var tokenUser string
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT user_id FROM api_tokens WHERE fixture_plaintext='pf_pat_dev_employee'`).Scan(&tokenUser); err != nil {
		t.Fatal(err)
	}
	if err := store.UserSetStatus(t.Context(), env.DB.Q(), tokenUser, model.UserStatusSuspended, nil, store.Millis(env.Clock.Now())); err != nil {
		t.Fatal(err)
	}
	resp := env.Do(t, testutil.Employee, http.MethodGet, "/v1/user", nil)
	testutil.AssertError(t, resp, http.StatusUnauthorized, "user_inactive")
}

func TestUserReinviteDeactivatedAndInvalidState(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "reinvite-deactivated@example.test", "firstName": "Reinvite", "lastName": "Deactivated"})
	if created.Status != http.StatusOK {
		t.Fatalf("create user status=%d body=%s", created.Status, created.Body)
	}
	var createdUser map[string]any
	created.JSON(t, &createdUser)
	id := createdUser["id"].(string)

	deleted := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/users/"+id, nil)
	if deleted.Status != http.StatusOK {
		t.Fatalf("delete user status=%d body=%s", deleted.Status, deleted.Body)
	}
	var deletedUser map[string]any
	deleted.JSON(t, &deletedUser)
	if deletedUser["status"] != model.UserStatusDeactivated || deletedUser["deactivatedAt"] == nil {
		t.Fatalf("deleted user=%v", deletedUser)
	}

	reinvited := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+id+"/reinvite", nil)
	if reinvited.Status != http.StatusOK {
		t.Fatalf("reinvite user status=%d body=%s", reinvited.Status, reinvited.Body)
	}
	var reinvitedUser map[string]any
	reinvited.JSON(t, &reinvitedUser)
	if reinvitedUser["status"] != model.UserStatusInvited || reinvitedUser["deactivatedAt"] != nil {
		t.Fatalf("reinvited user=%v", reinvitedUser)
	}

	var status string
	var deactivatedAt sql.NullInt64
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT status,deactivated_at FROM users WHERE id=?`, id).Scan(&status, &deactivatedAt); err != nil {
		t.Fatal(err)
	}
	if status != model.UserStatusInvited || deactivatedAt.Valid {
		t.Fatalf("persisted reinvited user status=%q deactivatedAt=%+v", status, deactivatedAt)
	}
	var reinviteAudits int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='user.reinvite' AND data LIKE ?`, "%"+id+"%").Scan(&reinviteAudits); err != nil || reinviteAudits != 1 {
		t.Fatalf("reinvite audit count=%d err=%v", reinviteAudits, err)
	}

	activatedID := env.UserID(t, "admin")
	invalid := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users/"+activatedID+"/reinvite", nil)
	testutil.AssertError(t, invalid, http.StatusUnprocessableEntity, "invalid_state")
}
