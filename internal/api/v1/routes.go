package v1

import (
	"net/http"
	"punchflow/internal/kferr"
)

// Operations returns the canonical 96-operation coverage matrix in its declared order.
func Operations() []Operation { return append([]Operation(nil), pf10Operations[:]...) }

var pf10Operations = [...]Operation{
	{OperationID: "getCurrentUser", Method: "GET", Pattern: "/v1/user", Handle: (*API).getCurrentUser},
	{OperationID: "listUsers", Method: "GET", Pattern: "/v1/users", Handle: (*API).listUsers},
	{OperationID: "createUser", Method: "POST", Pattern: "/v1/users", Handle: (*API).createUser},
	{OperationID: "getUser", Method: "GET", Pattern: "/v1/users/{userId}", Handle: (*API).getUser},
	{OperationID: "deleteUser", Method: "DELETE", Pattern: "/v1/users/{userId}", Handle: (*API).deleteUser},
	{OperationID: "updateUser", Method: "PATCH", Pattern: "/v1/users/{userId}", Handle: (*API).updateUser},
	{OperationID: "lookupUserByEmail", Method: "GET", Pattern: "/v1/users/lookupByEmail", Handle: (*API).lookupUserByEmail},
	{OperationID: "reinviteUser", Method: "POST", Pattern: "/v1/users/{userId}/reinvite", Handle: (*API).reinviteUser},
	{OperationID: "suspendUser", Method: "POST", Pattern: "/v1/users/{userId}/suspend", Handle: (*API).suspendUser},
	{OperationID: "reactivateUser", Method: "POST", Pattern: "/v1/users/{userId}/reactivate", Handle: (*API).reactivateUser},
	{OperationID: "listUserTeams", Method: "GET", Pattern: "/v1/users/{userId}/teams", Handle: (*API).listUserTeams},
	{OperationID: "listUserRoles", Method: "GET", Pattern: "/v1/users/{userId}/roles", Handle: (*API).listUserRoles},
	{OperationID: "listGrades", Method: "GET", Pattern: "/v1/grades", Handle: (*API).listGrades},
	{OperationID: "createGrade", Method: "POST", Pattern: "/v1/grades", Handle: (*API).createGrade},
	{OperationID: "getGrade", Method: "GET", Pattern: "/v1/grades/{gradeId}", Handle: (*API).getGrade},
	{OperationID: "deleteGrade", Method: "DELETE", Pattern: "/v1/grades/{gradeId}", Handle: (*API).deleteGrade},
	{OperationID: "updateGrade", Method: "PATCH", Pattern: "/v1/grades/{gradeId}", Handle: (*API).updateGrade},
	{OperationID: "setDefaultGrade", Method: "POST", Pattern: "/v1/grades/{gradeId}/default", Handle: (*API).setDefaultGrade},
	{OperationID: "listProxyApplicants", Method: "GET", Pattern: "/v1/proxyApplicants", Handle: (*API).listProxyApplicants},
	{OperationID: "createProxyApplicant", Method: "POST", Pattern: "/v1/proxyApplicants", Handle: (*API).createProxyApplicant},
	{OperationID: "updateProxyApplicant", Method: "PATCH", Pattern: "/v1/proxyApplicants/{proxyApplicantId}", Handle: (*API).updateProxyApplicant},
	{OperationID: "deleteProxyApplicant", Method: "DELETE", Pattern: "/v1/proxyApplicants/{proxyApplicantId}", Handle: (*API).deleteProxyApplicant},
	{OperationID: "listProxyApprovers", Method: "GET", Pattern: "/v1/proxyApprovers", Handle: (*API).listProxyApprovers},
	{OperationID: "createProxyApprover", Method: "POST", Pattern: "/v1/proxyApprovers", Handle: (*API).createProxyApprover},
	{OperationID: "updateProxyApprover", Method: "PATCH", Pattern: "/v1/proxyApprovers/{proxyApproverId}", Handle: (*API).updateProxyApprover},
	{OperationID: "deleteProxyApprover", Method: "DELETE", Pattern: "/v1/proxyApprovers/{proxyApproverId}", Handle: (*API).deleteProxyApprover},
	{OperationID: "listOrganizationCharts", Method: "GET", Pattern: "/v1/organizationCharts", Handle: (*API).listOrganizationCharts},
	{OperationID: "createOrganizationChart", Method: "POST", Pattern: "/v1/organizationCharts", Handle: (*API).createOrganizationChart},
	{OperationID: "deleteOrganizationChart", Method: "DELETE", Pattern: "/v1/organizationCharts/{organizationChartId}", Handle: (*API).deleteOrganizationChart},
	{OperationID: "getOrganizationChart", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}", Handle: (*API).getOrganizationChart},
	{OperationID: "updateOrganizationChart", Method: "PATCH", Pattern: "/v1/organizationCharts/{organizationChartId}", Handle: (*API).updateOrganizationChart},
	{OperationID: "getCurrentOrganizationChart", Method: "GET", Pattern: "/v1/organizationChart", Handle: (*API).getCurrentOrganizationChart},
	{OperationID: "activateOrganizationChart", Method: "POST", Pattern: "/v1/organizationCharts/{organizationChartId}/activate", Handle: (*API).activateOrganizationChart},
	{OperationID: "listTeams", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}/teams", Handle: (*API).listTeams},
	{OperationID: "createTeam", Method: "POST", Pattern: "/v1/organizationCharts/{organizationChartId}/teams", Handle: (*API).createTeam},
	{OperationID: "getTeam", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}", Handle: (*API).getTeam},
	{OperationID: "updateTeam", Method: "PATCH", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}", Handle: (*API).updateTeam},
	{OperationID: "deleteTeam", Method: "DELETE", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}", Handle: (*API).deleteTeam},
	{OperationID: "listTeamMembers", Method: "GET", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships", Handle: (*API).listTeamMembers},
	{OperationID: "createTeamMembers", Method: "POST", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships", Handle: (*API).createTeamMembers},
	{OperationID: "deleteTeamMembers", Method: "DELETE", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships", Handle: (*API).deleteTeamMembers},
	{OperationID: "updateTeamMember", Method: "PATCH", Pattern: "/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships/{userId}", Handle: (*API).updateTeamMember},
	{OperationID: "listCategories", Method: "GET", Pattern: "/v1/categories", Handle: (*API).listCategories},
	{OperationID: "createCategory", Method: "POST", Pattern: "/v1/categories", Handle: (*API).createCategory},
	{OperationID: "deleteCategory", Method: "DELETE", Pattern: "/v1/categories/{categoryId}", Handle: (*API).deleteCategory},
	{OperationID: "updateCategory", Method: "PATCH", Pattern: "/v1/categories/{categoryId}", Handle: (*API).updateCategory},
	{OperationID: "listFolders", Method: "GET", Pattern: "/v1/folders", Handle: (*API).listFolders},
	{OperationID: "createFolder", Method: "POST", Pattern: "/v1/folders", Handle: (*API).createFolder},
	{OperationID: "deleteFolder", Method: "DELETE", Pattern: "/v1/folders/{folderId}", Handle: (*API).deleteFolder},
	{OperationID: "updateFolder", Method: "PATCH", Pattern: "/v1/folders/{folderId}", Handle: (*API).updateFolder},
	{OperationID: "getFolder", Method: "GET", Pattern: "/v1/folders/{folderId}", Handle: (*API).getFolder},
	{OperationID: "listRoutes", Method: "GET", Pattern: "/v1/routes", Handle: (*API).listRoutes},
	{OperationID: "getRoute", Method: "GET", Pattern: "/v1/routes/{routeId}", Handle: (*API).getRoute},
	{OperationID: "listWorkflows", Method: "GET", Pattern: "/v1/workflows", Handle: (*API).listWorkflows},
	{OperationID: "getWorkflow", Method: "GET", Pattern: "/v1/workflows/{workflowId}", Handle: (*API).getWorkflow},
	{OperationID: "listTickets", Method: "GET", Pattern: "/v1/tickets", Handle: (*API).listTickets},
	{OperationID: "createTicket", Method: "POST", Pattern: "/v1/tickets", Handle: (*API).createTicket},
	{OperationID: "listTasks", Method: "GET", Pattern: "/v1/tickets/tasks", Handle: (*API).listTasks},
	{OperationID: "getTicket", Method: "GET", Pattern: "/v1/tickets/{ticketId}", Handle: (*API).getTicket},
	{OperationID: "updateTicket", Method: "PATCH", Pattern: "/v1/tickets/{ticketId}", Handle: (*API).updateTicket},
	{OperationID: "updateTicketForm", Method: "PATCH", Pattern: "/v1/tickets/{ticketId}/form", Handle: (*API).updateTicketForm},
	{OperationID: "approveTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/approve", Handle: (*API).approveTicket},
	{OperationID: "rejectTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/reject", Handle: (*API).rejectTicket},
	{OperationID: "denyTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/deny", Handle: (*API).denyTicket},
	{OperationID: "withdrawTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/withdraw", Handle: (*API).withdrawTicket},
	{OperationID: "archiveTicket", Method: "POST", Pattern: "/v1/tickets/{ticketId}/archive", Handle: (*API).archiveTicket},
	{OperationID: "listTicketLinks", Method: "GET", Pattern: "/v1/tickets/{ticketId}/links", Handle: (*API).listTicketLinks},
	{OperationID: "listViewers", Method: "GET", Pattern: "/v1/tickets/{ticketId}/viewers", Handle: (*API).listViewers},
	{OperationID: "createViewer", Method: "POST", Pattern: "/v1/tickets/{ticketId}/viewers", Handle: (*API).createViewer},
	{OperationID: "deleteViewer", Method: "DELETE", Pattern: "/v1/tickets/{ticketId}/viewers/{viewerId}", Handle: (*API).deleteViewer},
	{OperationID: "uploadFile", Method: "POST", Pattern: "/v1/files", Handle: (*API).uploadFile},
	{OperationID: "getFile", Method: "GET", Pattern: "/v1/files/{signedId}", Handle: (*API).getFile},
	{OperationID: "listGeneralMasters", Method: "GET", Pattern: "/v1/generalMasters", Handle: (*API).listGeneralMasters},
	{OperationID: "createGeneralMaster", Method: "POST", Pattern: "/v1/generalMasters", Handle: (*API).createGeneralMaster},
	{OperationID: "getGeneralMaster", Method: "GET", Pattern: "/v1/generalMasters/{generalMasterId}", Handle: (*API).getGeneralMaster},
	{OperationID: "updateGeneralMaster", Method: "PATCH", Pattern: "/v1/generalMasters/{generalMasterId}", Handle: (*API).updateGeneralMaster},
	{OperationID: "deleteGeneralMaster", Method: "DELETE", Pattern: "/v1/generalMasters/{generalMasterId}", Handle: (*API).deleteGeneralMaster},
	{OperationID: "listGeneralMasterItems", Method: "GET", Pattern: "/v1/generalMasters/{generalMasterId}/items", Handle: (*API).listGeneralMasterItems},
	{OperationID: "createGeneralMasterItem", Method: "POST", Pattern: "/v1/generalMasters/{generalMasterId}/items", Handle: (*API).createGeneralMasterItem},
	{OperationID: "getGeneralMasterItem", Method: "GET", Pattern: "/v1/generalMasters/{generalMasterId}/items/{itemId}", Handle: (*API).getGeneralMasterItem},
	{OperationID: "updateGeneralMasterItem", Method: "PATCH", Pattern: "/v1/generalMasters/{generalMasterId}/items/{itemId}", Handle: (*API).updateGeneralMasterItem},
	{OperationID: "deleteGeneralMasterItem", Method: "DELETE", Pattern: "/v1/generalMasters/{generalMasterId}/items/{itemId}", Handle: (*API).deleteGeneralMasterItem},
	{OperationID: "listRoles", Method: "GET", Pattern: "/v1/roles", Handle: (*API).listRoles},
	{OperationID: "createRole", Method: "POST", Pattern: "/v1/roles", Handle: (*API).createRole},
	{OperationID: "getRole", Method: "GET", Pattern: "/v1/roles/{roleId}", Handle: (*API).getRole},
	{OperationID: "updateRole", Method: "PATCH", Pattern: "/v1/roles/{roleId}", Handle: (*API).updateRole},
	{OperationID: "deleteRole", Method: "DELETE", Pattern: "/v1/roles/{roleId}", Handle: (*API).deleteRole},
	{OperationID: "createRoleMembers", Method: "POST", Pattern: "/v1/roles/{roleId}/memberships", Handle: (*API).createRoleMembers},
	{OperationID: "listRoleMembers", Method: "GET", Pattern: "/v1/roles/{roleId}/memberships", Handle: (*API).listRoleMembers},
	{OperationID: "deleteRoleMember", Method: "DELETE", Pattern: "/v1/roles/{roleId}/memberships/{userId}", Handle: (*API).deleteRoleMember},
	{OperationID: "listComments", Method: "GET", Pattern: "/v1/tickets/{ticketId}/comments", Handle: (*API).listComments},
	{OperationID: "createComment", Method: "POST", Pattern: "/v1/tickets/{ticketId}/comments", Handle: (*API).createComment},
	{OperationID: "getComment", Method: "GET", Pattern: "/v1/tickets/{ticketId}/comments/{commentId}", Handle: (*API).getComment},
	{OperationID: "updateComment", Method: "PATCH", Pattern: "/v1/tickets/{ticketId}/comments/{commentId}", Handle: (*API).updateComment},
	{OperationID: "deleteComment", Method: "DELETE", Pattern: "/v1/tickets/{ticketId}/comments/{commentId}", Handle: (*API).deleteComment},
	{OperationID: "listAuditLogs", Method: "GET", Pattern: "/v1/auditLogs", Handle: (*API).listAuditLogs},
}

// Register installs the operation routes and method-independent unknown /v1 path handler.
func (a *API) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	for _, op := range pf10Operations {
		operation := op
		h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { operation.Handle(a, w, r) }))
		if wrap != nil {
			h = wrap(h)
		}
		mux.Handle(op.Method+" "+op.Pattern, h)
	}
	mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { kferr.Write(w, kferr.EndpointNotFound()) }))
}
