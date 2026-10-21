package v1_test

import (
	"net/http"
	"strings"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

const pf19Chart = "00000000-0000-4000-8000-000000000027"

func TestTeamTreeCreateUpdateAndPermissions(t *testing.T) {
	env := testutil.New(t)
	roots := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams?perPage=2", nil)
	if roots.Status != http.StatusOK {
		t.Fatalf("roots status=%d body=%s", roots.Status, roots.Body)
	}
	if roots.Header.Get("Page") != "1" || roots.Header.Get("Per-Page") != "2" || roots.Header.Get("Total") != "1" {
		t.Fatalf("pagination headers: %v", roots.Header)
	}
	children := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams?parentId=sales", nil)
	if children.Status != http.StatusOK {
		t.Fatalf("children status=%d body=%s", children.Status, children.Body)
	}
	var rows []map[string]any
	children.JSON(t, &rows)
	if len(rows) != 1 || rows[0]["code"] != "sales-east" {
		t.Fatalf("sales children=%v", rows)
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+pf19Chart+"/teams", map[string]any{"name": "New Sales", "parentId": "company"})
	if created.Status != http.StatusOK {
		t.Fatalf("create team status=%d body=%s", created.Status, created.Body)
	}
	var team map[string]any
	created.JSON(t, &team)
	id := team["id"].(string)
	code := team["code"].(string)
	if got := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams/"+code, nil); got.Status != http.StatusOK {
		t.Fatalf("get by code status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/00000000-0000-4000-8000-000000000028/teams", map[string]any{"name": "Wrong parent", "parentId": "00000000-0000-4000-8000-000000000020"}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("cross-chart parent status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+pf19Chart+"/teams/company", map[string]any{"parentId": id}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("cycle status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Employee, http.MethodGet, "/v1/organizationCharts/"+pf19Chart+"/teams", nil); got.Status != http.StatusForbidden {
		t.Fatalf("employee status=%d want 403", got.Status)
	}
}

func TestDeleteTeamReparentsChildrenAndArchivesOpenTickets(t *testing.T) {
	env := testutil.New(t)
	env.RegisterReceiverWebhook(t, "team-delete-secret")
	ticketID := pf23CreateExpenseTicket(t, env, "sales-rep", "sales", "")
	deletedID, err := store.TeamIDByCode(t.Context(), env.DB.Q(), pf19Chart, "sales")
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := store.TeamIDByCode(t.Context(), env.DB.Q(), pf19Chart, "company")
	if err != nil {
		t.Fatal(err)
	}
	response := env.Do(t, testutil.Admin, http.MethodDelete, "/v1/organizationCharts/"+pf19Chart+"/teams/sales", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", response.Status, response.Body)
	}
	var parentID string
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT parent_id FROM teams WHERE code='sales-east' AND organization_chart_id=?`, pf19Chart).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if parentID != companyID {
		t.Fatalf("child parent=%s, want company %s", parentID, companyID)
	}
	ticket, err := store.EngLoadTicket(t.Context(), env.DB.Q(), ticketID)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Status != model.TicketStatusArchived || ticket.ArchivedAt == nil || ticket.ClosedAt == nil {
		t.Fatalf("deleted-team ticket=%+v, want archived and closed", ticket)
	}
	if ticket.AuthorTeamID != nil {
		t.Fatalf("archived ticket authorTeamID=%v, want cleared by team deletion", ticket.AuthorTeamID)
	}
	var archivedEvents int
	if err = env.DB.Q().QueryRowContext(t.Context(), `SELECT count(*) FROM webhook_events WHERE ticket_id=? AND event_type=?`, ticketID, model.WebhookEventTicketArchived).Scan(&archivedEvents); err != nil {
		t.Fatal(err)
	}
	if archivedEvents != 1 {
		t.Fatalf("ticket_archived events=%d, want 1", archivedEvents)
	}
	if _, err = store.GetTeam(t.Context(), env.DB.Q(), deletedID); err == nil {
		t.Fatal("deleted team still exists")
	}
}
func TestTeamMembershipLeaderAndPaginationRules(t *testing.T) {
	env := testutil.New(t)
	base := "/v1/organizationCharts/" + pf19Chart + "/teams/finance/memberships"
	members := env.Do(t, testutil.Admin, http.MethodGet, base+"?perPage=1", nil)
	if members.Status != http.StatusOK {
		t.Fatalf("members status=%d body=%s", members.Status, members.Body)
	}
	testutil.AssertPagination(t, members, 1, 1, 2)
	var rows []map[string]any
	members.JSON(t, &rows)
	if len(rows) != 1 {
		t.Fatalf("page length=%d", len(rows))
	}
	if _, ok := rows[0]["userType"]; ok {
		t.Fatalf("MemberUser must omit userType: %v", rows[0])
	}
	adminID := env.UserID(t, "admin")
	grades := []string{"00000000-0000-4000-8000-000000000006"}
	if got := env.Do(t, testutil.Admin, http.MethodPost, base, map[string]any{"users": []any{map[string]any{"id": adminID, "leader": false, "gradeIds": grades}}}); got.Status != http.StatusOK {
		t.Fatalf("add member status=%d body=%s", got.Status, got.Body)
	}
	if got := env.Do(t, testutil.Admin, http.MethodDelete, base, map[string]any{"userIds": []string{env.UserID(t, "finance-manager")}}); got.Status != http.StatusUnprocessableEntity {
		t.Fatalf("remove only leader status=%d body=%s", got.Status, got.Body)
	}
	many := make([]any, 11)
	for i := range many {
		many[i] = map[string]any{"id": adminID, "leader": false, "gradeIds": grades}
	}
	tooMany := env.Do(t, testutil.Admin, http.MethodPost, base, map[string]any{"users": many})
	if tooMany.Status != http.StatusBadRequest {
		t.Fatalf("11 users status=%d body=%s", tooMany.Status, tooMany.Body)
	}
	var sizeLimitError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	tooMany.JSON(t, &sizeLimitError)
	if sizeLimitError.Code != "invalid_parameter" || !strings.Contains(sizeLimitError.Message, "between 1 and 10") {
		t.Fatalf("11 users error=%+v", sizeLimitError)
	}
	if got := env.Do(t, testutil.Admin, http.MethodPatch, base+"/accountant", map[string]any{"leader": true, "gradeIds": grades}); got.Status != http.StatusOK {
		t.Fatalf("update member status=%d body=%s", got.Status, got.Body)
	}
}

func TestTeamMembershipRequiredFieldsAndNullGradeIDs(t *testing.T) {
	env := testutil.New(t)
	base := "/v1/organizationCharts/" + pf19Chart + "/teams/finance/memberships"
	adminID := env.UserID(t, "admin")
	for name, body := range map[string]any{
		"missing create leader": map[string]any{"users": []any{map[string]any{"id": adminID}}},
		"null create gradeIds":  map[string]any{"users": []any{map[string]any{"id": adminID, "leader": false, "gradeIds": nil}}},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, base, body), http.StatusBadRequest, "invalid_parameter")
		})
	}
	for name, body := range map[string]any{
		"missing update leader":   map[string]any{"gradeIds": []string{"00000000-0000-4000-8000-000000000006"}},
		"missing update gradeIds": map[string]any{"leader": true},
		"null update gradeIds":    map[string]any{"leader": true, "gradeIds": nil},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPatch, base+"/accountant", body), http.StatusBadRequest, "invalid_parameter")
		})
	}
}

func TestTeamCodeRenamePreservesFixtureRouteReferences(t *testing.T) {
	env := testutil.New(t)
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/organizationCharts/"+pf19Chart+"/teams/finance", map[string]any{"code": "finance-renamed"})
	if updated.Status != http.StatusOK {
		t.Fatalf("rename team status=%d body=%s", updated.Status, updated.Body)
	}
	route := env.Do(t, testutil.Admin, http.MethodGet, "/v1/routes/expense-standard", nil)
	if route.Status != http.StatusOK || !strings.Contains(string(route.Body), "finance-renamed") {
		t.Fatalf("route after team code rename status=%d body=%s", route.Status, route.Body)
	}
}
