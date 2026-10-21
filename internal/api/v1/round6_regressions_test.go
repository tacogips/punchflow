package v1_test

import (
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestRound6InvitedUserCanJoinTeamMembership(t *testing.T) {
	env := testutil.New(t)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "invite-membership@example.test", "firstName": "Invite", "lastName": "Member"})
	if created.Status != http.StatusOK {
		t.Fatalf("create invited user: %d %s", created.Status, created.Body)
	}
	var user map[string]any
	created.JSON(t, &user)
	if user["status"] != model.UserStatusInvited {
		t.Fatalf("created user status=%v, want invited", user["status"])
	}
	base := "/v1/organizationCharts/" + pf19Chart + "/teams/finance/memberships"
	added := env.Do(t, testutil.Admin, http.MethodPost, base, map[string]any{"users": []any{map[string]any{"id": user["id"], "leader": false, "gradeIds": []string{"00000000-0000-4000-8000-000000000006"}}}})
	if added.Status != http.StatusOK {
		t.Fatalf("add invited user to team: %d %s", added.Status, added.Body)
	}
}

func TestRound6MissingRequiredNamesAreInvalidParameter(t *testing.T) {
	env := testutil.New(t)
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts", map[string]any{}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts", map[string]any{"name": nil}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+pf19Chart+"/teams", map[string]any{}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+pf19Chart+"/teams", map[string]any{"name": nil}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+pf19Chart+"/teams", map[string]any{"name": ""}), http.StatusUnprocessableEntity, "validation_failed")
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts", map[string]any{"name": "Round 6 chart"})
	if created.Status != http.StatusOK {
		t.Fatalf("create chart: %d %s", created.Status, created.Body)
	}
	var chart map[string]any
	created.JSON(t, &chart)
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart["id"].(string), map[string]any{}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart["id"].(string), map[string]any{"name": nil}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+chart["id"].(string), map[string]any{"name": ""}), http.StatusUnprocessableEntity, "validation_failed")
}

func TestRound6RequiredMasterFieldBooleanMustBePresent(t *testing.T) {
	for _, value := range []any{nil, missingRound6Value{}} {
		t.Run(map[bool]string{true: "null", false: "omitted"}[value != nil], func(t *testing.T) {
			env := testutil.New(t)
			field := map[string]any{"code": "note", "title": "Note", "fieldType": "text"}
			if _, omitted := value.(missingRound6Value); !omitted {
				field["required"] = value
			}
			testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, "/v1/generalMasters", map[string]any{"name": "Invalid master", "fields": []any{field}}), http.StatusBadRequest, "invalid_parameter")
		})
	}
}

type missingRound6Value struct{}

func TestRound6FullNameLengthIncludesBothNames(t *testing.T) {
	env := testutil.New(t)
	tooLong := strings.Repeat("界", 128)
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "long-name@example.test", "firstName": tooLong, "lastName": tooLong})
	testutil.AssertError(t, created, http.StatusBadRequest, "invalid_parameter")
	valid := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "valid-long-name@example.test", "firstName": strings.Repeat("界", 127), "lastName": strings.Repeat("語", 127)})
	if valid.Status != http.StatusOK {
		t.Fatalf("create boundary-length user: %d %s", valid.Status, valid.Body)
	}
	var user map[string]any
	valid.JSON(t, &user)
	userID := user["id"].(string)
	update := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+userID, map[string]any{"firstName": strings.Repeat("界", 128)})
	testutil.AssertError(t, update, http.StatusBadRequest, "invalid_parameter")
}

func TestRound6DefaultViewerOrderGroupsTeamsBeforeUsers(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf22CreateTicket(t, env, testutil.Employee, "draft", 5000)
	teamA, _ := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	teamZ, _ := store.ResolveID(t.Context(), env.DB.Q(), "teams", "finance")
	accountantID := env.UserID(t, "accountant")
	ceoID := env.UserID(t, "ceo")
	for i, viewer := range []model.TicketViewer{
		{ID: store.NewID(), TicketID: ticketID, TeamID: &teamZ},
		{ID: store.NewID(), TicketID: ticketID, TeamID: &teamA},
		{ID: store.NewID(), TicketID: ticketID, UserID: &ceoID},
		{ID: store.NewID(), TicketID: ticketID, UserID: &accountantID},
	} {
		if err := store.TicketQueryInsertViewer(t.Context(), env.DB.Q(), viewer); err != nil {
			t.Fatalf("insert viewer %d: %v", i, err)
		}
	}
	listed := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+ticketID+"/viewers?perPage=2", nil)
	if listed.Status != http.StatusOK {
		t.Fatalf("list viewers: %d %s", listed.Status, listed.Body)
	}
	var rows []map[string]any
	listed.JSON(t, &rows)
	if len(rows) != 2 || rows[0]["team"] == nil || rows[1]["team"] == nil {
		t.Fatalf("viewer order must group teams before users: %s", listed.Body)
	}
	firstTeam := rows[0]["team"].(map[string]any)
	secondTeam := rows[1]["team"].(map[string]any)
	if firstTeam["name"] != "Finance" || secondTeam["name"] != "Sales" {
		t.Fatalf("team viewers are not ordered by name: %s", listed.Body)
	}
	pageTwo := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+ticketID+"/viewers?perPage=2&page=2", nil)
	var lastPage []map[string]any
	pageTwo.JSON(t, &lastPage)
	if len(lastPage) != 2 || lastPage[0]["user"] == nil || lastPage[1]["user"] == nil {
		t.Fatalf("viewer pagination must place users after all teams: %s", pageTwo.Body)
	}
	firstUser := lastPage[0]["user"].(map[string]any)
	secondUser := lastPage[1]["user"].(map[string]any)
	if firstUser["email"] != "accountant@punchflow.test" || secondUser["email"] != "ceo@punchflow.test" {
		t.Fatalf("user viewers are not ordered by email: %s", pageTwo.Body)
	}
}

func TestRound6CommentBodyMustBePresentAndNonNull(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf23CreateExpenseTicket(t, env, "sales-rep", "sales", "")
	for name, payload := range map[string]map[string]any{"omitted": {"files": []string{"not-a-valid-file"}}, "null": {"body": nil, "files": []string{"not-a-valid-file"}}} {
		t.Run(name, func(t *testing.T) {
			testutil.AssertError(t, env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/comments", payload), http.StatusBadRequest, "invalid_parameter")
		})
	}
	uploaded := pf23UploadFile(t, env, []byte("attachment"), true)
	if uploaded.Status != http.StatusOK {
		t.Fatalf("upload attachment: %d %s", uploaded.Status, uploaded.Body)
	}
	var file struct {
		SignedID string `json:"signedId"`
	}
	uploaded.JSON(t, &file)
	withEmptyBody := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/comments", map[string]any{"body": "", "files": []string{file.SignedID}})
	if withEmptyBody.Status != http.StatusOK {
		t.Fatalf("explicit empty body with attachment: %d %s", withEmptyBody.Status, withEmptyBody.Body)
	}
	var count int
	if err := env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM comments WHERE ticket_id=?`, ticketID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("valid attachment-only comment count=%d, want 1", count)
	}
}
