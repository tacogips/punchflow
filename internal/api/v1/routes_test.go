package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPF10OperationsMatchCoverageMatrix(t *testing.T) {
	expected := []Operation{
		{OperationID: "getCurrentUser", Method: "GET", Pattern: "/v1/user"},
		{OperationID: "listUsers", Method: "GET", Pattern: "/v1/users"},
		{OperationID: "createUser", Method: "POST", Pattern: "/v1/users"},
		{OperationID: "getUser", Method: "GET", Pattern: "/v1/users/{userId}"},
		{OperationID: "deleteUser", Method: "DELETE", Pattern: "/v1/users/{userId}"},
		{OperationID: "updateUser", Method: "PATCH", Pattern: "/v1/users/{userId}"},
		{OperationID: "lookupUserByEmail", Method: "GET", Pattern: "/v1/users/lookupByEmail"},
		{OperationID: "reinviteUser", Method: "POST", Pattern: "/v1/users/{userId}/reinvite"},
		{OperationID: "suspendUser", Method: "POST", Pattern: "/v1/users/{userId}/suspend"},
		{OperationID: "reactivateUser", Method: "POST", Pattern: "/v1/users/{userId}/reactivate"},
		{OperationID: "listUserTeams", Method: "GET", Pattern: "/v1/users/{userId}/teams"},
		{OperationID: "listUserRoles", Method: "GET", Pattern: "/v1/users/{userId}/roles"},
		{OperationID: "listGrades", Method: "GET", Pattern: "/v1/grades"},
		{OperationID: "createGrade", Method: "POST", Pattern: "/v1/grades"},
		{OperationID: "getGrade", Method: "GET", Pattern: "/v1/grades/{gradeId}"},
		{OperationID: "deleteGrade", Method: "DELETE", Pattern: "/v1/grades/{gradeId}"},
		{OperationID: "updateGrade", Method: "PATCH", Pattern: "/v1/grades/{gradeId}"},
		{OperationID: "setDefaultGrade", Method: "POST", Pattern: "/v1/grades/{gradeId}/default"},
		{OperationID: "listProxyApplicants", Method: "GET", Pattern: "/v1/proxyApplicants"},
		{OperationID: "createProxyApplicant", Method: "POST", Pattern: "/v1/proxyApplicants"},
		{OperationID: "updateProxyApplicant", Method: "PATCH", Pattern: "/v1/proxyApplicants/{proxyApplicantId}"},
		{OperationID: "deleteProxyApplicant", Method: "DELETE", Pattern: "/v1/proxyApplicants/{proxyApplicantId}"},
		{OperationID: "listProxyApprovers", Method: "GET", Pattern: "/v1/proxyApprovers"},
		{OperationID: "createProxyApprover", Method: "POST", Pattern: "/v1/proxyApprovers"},
		{OperationID: "updateProxyApprover", Method: "PATCH", Pattern: "/v1/proxyApprovers/{proxyApproverId}"},
		{OperationID: "deleteProxyApprover", Method: "DELETE", Pattern: "/v1/proxyApprovers/{proxyApproverId}"},
		{OperationID: "listOrganizationCharts", Method: "GET", Pattern: "/v1/organizationCharts"},
		{OperationID: "createOrganizationChart", Method: "POST", Pattern: "/v1/organizationCharts"},
		{OperationID: "deleteOrganizationChart", Method: "DELETE", Pattern: "/v1/organizationCharts/{organizationChartId}"},
		{OperationID: "getOrganizationChart", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}"},
		{OperationID: "updateOrganizationChart", Method: "PATCH", Pattern: "/v1/organizationCharts/{organizationChartId}"},
		{OperationID: "getCurrentOrganizationChart", Method: "GET", Pattern: "/v1/organizationChart"},
		{OperationID: "activateOrganizationChart", Method: "POST", Pattern: "/v1/organizationCharts/{organizationChartId}/activate"},
		{OperationID: "listTeams", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}/teams"},
		{OperationID: "createTeam", Method: "POST", Pattern: "/v1/organizationCharts/{organizationChartId}/teams"},
		{OperationID: "getTeam", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}"},
		{OperationID: "updateTeam", Method: "PATCH", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}"},
		{OperationID: "deleteTeam", Method: "DELETE", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}"},
		{OperationID: "listTeamMembers", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships"},
		{OperationID: "createTeamMembers", Method: "POST", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships"},
		{OperationID: "deleteTeamMembers", Method: "DELETE", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships"},
		{OperationID: "updateTeamMember", Method: "PATCH", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships/{userId}"},
		{OperationID: "listCategories", Method: "GET", Pattern: "/v1/categories"},
		{OperationID: "createCategory", Method: "POST", Pattern: "/v1/categories"},
		{OperationID: "deleteCategory", Method: "DELETE", Pattern: "/v1/categories/{categoryId}"},
		{OperationID: "updateCategory", Method: "PATCH", Pattern: "/v1/categories/{categoryId}"},
		{OperationID: "listFolders", Method: "GET", Pattern: "/v1/folders"},
		{OperationID: "createFolder", Method: "POST", Pattern: "/v1/folders"},
		{OperationID: "deleteFolder", Method: "DELETE", Pattern: "/v1/folders/{folderId}"},
		{OperationID: "updateFolder", Method: "PATCH", Pattern: "/v1/folders/{folderId}"},
		{OperationID: "getFolder", Method: "GET", Pattern: "/v1/folders/{folderId}"},
		{OperationID: "listRoutes", Method: "GET", Pattern: "/v1/routes"},
		{OperationID: "getRoute", Method: "GET", Pattern: "/v1/routes/{routeId}"},
		{OperationID: "listWorkflows", Method: "GET", Pattern: "/v1/workflows"},
		{OperationID: "getWorkflow", Method: "GET", Pattern: "/v1/workflows/{workflowId}"},
		{OperationID: "listTickets", Method: "GET", Pattern: "/v1/tickets"},
		{OperationID: "createTicket", Method: "POST", Pattern: "/v1/tickets"},
		{OperationID: "listTasks", Method: "GET", Pattern: "/v1/tickets/tasks"},
		{OperationID: "getTicket", Method: "GET", Pattern: "/v1/tickets/{ticketId}"},
		{OperationID: "updateTicket", Method: "PATCH", Pattern: "/v1/tickets/{ticketId}"},
		{OperationID: "updateTicketForm", Method: "PATCH", Pattern: "/v1/tickets/{ticketId}/form"},
		{OperationID: "approveTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/approve"},
		{OperationID: "rejectTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/reject"},
		{OperationID: "denyTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/deny"},
		{OperationID: "withdrawTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/withdraw"},
		{OperationID: "archiveTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/archive"},
		{OperationID: "listTicketLinks", Method: "GET", Pattern: "/v1/tickets/{ticketId}/links"},
		{OperationID: "listViewers", Method: "GET", Pattern: "/v1/tickets/{ticketId}/viewers"},
		{OperationID: "createViewer", Method: "POST", Pattern: "/v1/tickets/{ticketId}/viewers"},
		{OperationID: "deleteViewer", Method: "DELETE", Pattern: "/v1/tickets/{ticketId}/viewers/{viewerId}"},
		{OperationID: "uploadFile", Method: "POST", Pattern: "/v1/files"},
		{OperationID: "getFile", Method: "GET", Pattern: "/v1/files/{signedId}"},
		{OperationID: "listGeneralMasters", Method: "GET", Pattern: "/v1/generalMasters"},
		{OperationID: "createGeneralMaster", Method: "POST", Pattern: "/v1/generalMasters"},
		{OperationID: "getGeneralMaster", Method: "GET", Pattern: "/v1/generalMasters/{generalMasterId}"},
		{OperationID: "updateGeneralMaster", Method: "PATCH", Pattern: "/v1/generalMasters/{generalMasterId}"},
		{OperationID: "deleteGeneralMaster", Method: "DELETE", Pattern: "/v1/generalMasters/{generalMasterId}"},
		{OperationID: "listGeneralMasterItems", Method: "GET", Pattern: "/v1/generalMasters/{generalMasterId}/items"},
		{OperationID: "createGeneralMasterItem", Method: "POST", Pattern: "/v1/generalMasters/{generalMasterId}/items"},
		{OperationID: "getGeneralMasterItem", Method: "GET", Pattern: "/v1/generalMasters/{generalMasterId}/items/{itemId}"},
		{OperationID: "updateGeneralMasterItem", Method: "PATCH", Pattern: "/v1/generalMasters/{generalMasterId}/items/{itemId}"},
		{OperationID: "deleteGeneralMasterItem", Method: "DELETE", Pattern: "/v1/generalMasters/{generalMasterId}/items/{itemId}"},
		{OperationID: "listRoles", Method: "GET", Pattern: "/v1/roles"},
		{OperationID: "createRole", Method: "POST", Pattern: "/v1/roles"},
		{OperationID: "getRole", Method: "GET", Pattern: "/v1/roles/{roleId}"},
		{OperationID: "updateRole", Method: "PATCH", Pattern: "/v1/roles/{roleId}"},
		{OperationID: "deleteRole", Method: "DELETE", Pattern: "/v1/roles/{roleId}"},
		{OperationID: "createRoleMembers", Method: "POST", Pattern: "/v1/roles/{roleId}/memberships"},
		{OperationID: "listRoleMembers", Method: "GET", Pattern: "/v1/roles/{roleId}/memberships"},
		{OperationID: "deleteRoleMember", Method: "DELETE", Pattern: "/v1/roles/{roleId}/memberships/{userId}"},
		{OperationID: "listComments", Method: "GET", Pattern: "/v1/tickets/{ticketId}/comments"},
		{OperationID: "createComment", Method: "POST", Pattern: "/v1/tickets/{ticketId}/comments"},
		{OperationID: "getComment", Method: "GET", Pattern: "/v1/tickets/{ticketId}/comments/{commentId}"},
		{OperationID: "updateComment", Method: "PATCH", Pattern: "/v1/tickets/{ticketId}/comments/{commentId}"},
		{OperationID: "deleteComment", Method: "DELETE", Pattern: "/v1/tickets/{ticketId}/comments/{commentId}"},
		{OperationID: "listAuditLogs", Method: "GET", Pattern: "/v1/auditLogs"},
	}
	got := Operations()
	if len(got) != 96 {
		t.Fatalf("got %d operations", len(got))
	}
	ids, pairs, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, w := range expected {
		g := got[i]
		if g.OperationID != w.OperationID || g.Method != w.Method || g.Pattern != w.Pattern {
			t.Errorf("row %d mismatch: %+v want %+v", i, g, w)
		}
		if ids[g.OperationID] {
			t.Errorf("duplicate id %s", g.OperationID)
		}
		ids[g.OperationID] = true
		p := g.Method + " " + g.Pattern
		if pairs[p] {
			t.Errorf("duplicate route %s", p)
		}
		pairs[p] = true
		paths[g.Pattern] = true
	}
	if len(paths) != 57 {
		t.Errorf("got %d patterns", len(paths))
	}
}
func TestPF10DispatchPatternsAndCatchAll(t *testing.T) {
	a := New(Deps{})
	m := http.NewServeMux()
	a.Register(m, nil)
	for _, c := range []struct{ method, path, want string }{{"GET", "/v1/users/lookupByEmail", "GET /v1/users/lookupByEmail"}, {"GET", "/v1/tickets/tasks", "GET /v1/tickets/tasks"}} {
		_, p := m.Handler(httptest.NewRequest(c.method, c.path, nil))
		if p != c.want {
			t.Errorf("matched %q want %q", p, c.want)
		}
	}
	for _, op := range Operations() {
		p := op.Pattern
		for _, part := range strings.Split(p, "/") {
			if strings.HasPrefix(part, "{") {
				p = strings.Replace(p, part, "x", 1)
			}
		}
		_, matched := m.Handler(httptest.NewRequest(op.Method, p, nil))
		if want := op.Method + " " + op.Pattern; matched != want {
			t.Errorf("%s %s matched %q want %q", op.Method, p, matched, want)
		}
	}
	for _, p := range []string{"/v1/users", "/v1/nope"} {
		rr := httptest.NewRecorder()
		m.ServeHTTP(rr, httptest.NewRequest("PUT", p, nil))
		if rr.Code != 404 || !strings.Contains(rr.Body.String(), `"code":"endpoint_not_found"`) {
			t.Errorf("PUT %s = %d %s", p, rr.Code, rr.Body.String())
		}
	}
}
