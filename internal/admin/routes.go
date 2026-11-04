package admin

import (
	"net/http"

	"punchflow/internal/kferr"
)

var operations = [...]Operation{
	{Name: "listTokens", Method: http.MethodGet, Pattern: "/admin/api/tokens", Handle: (*API).listTokens},
	{Name: "createToken", Method: http.MethodPost, Pattern: "/admin/api/tokens", Handle: (*API).createToken},
	{Name: "deleteToken", Method: http.MethodDelete, Pattern: "/admin/api/tokens/{tokenId}", Handle: (*API).deleteToken},
	{Name: "listServiceAccounts", Method: http.MethodGet, Pattern: "/admin/api/service-accounts", Handle: (*API).listServiceAccounts},
	{Name: "createServiceAccount", Method: http.MethodPost, Pattern: "/admin/api/service-accounts", Handle: (*API).createServiceAccount},
	{Name: "deleteServiceAccount", Method: http.MethodDelete, Pattern: "/admin/api/service-accounts/{id}", Handle: (*API).deleteServiceAccount},
	{Name: "listWebhooks", Method: http.MethodGet, Pattern: "/admin/api/webhooks", Handle: (*API).listWebhooks},
	{Name: "createWebhook", Method: http.MethodPost, Pattern: "/admin/api/webhooks", Handle: (*API).createWebhook},
	{Name: "getWebhook", Method: http.MethodGet, Pattern: "/admin/api/webhooks/{id}", Handle: (*API).getWebhook},
	{Name: "updateWebhook", Method: http.MethodPatch, Pattern: "/admin/api/webhooks/{id}", Handle: (*API).updateWebhook},
	{Name: "deleteWebhook", Method: http.MethodDelete, Pattern: "/admin/api/webhooks/{id}", Handle: (*API).deleteWebhook},
	{Name: "pingWebhook", Method: http.MethodPost, Pattern: "/admin/api/webhooks/{id}/ping", Handle: (*API).pingWebhook},
	{Name: "listWebhookEvents", Method: http.MethodGet, Pattern: "/admin/api/webhooks/{id}/events", Handle: (*API).listWebhookEvents},
	{Name: "getWebhookEvent", Method: http.MethodGet, Pattern: "/admin/api/webhook-events/{eventId}", Handle: (*API).getWebhookEvent},
	{Name: "resendWebhookEvent", Method: http.MethodPost, Pattern: "/admin/api/webhook-events/{eventId}/resend", Handle: (*API).resendWebhookEvent},
	{Name: "createWebhookEvent", Method: http.MethodPost, Pattern: "/admin/api/webhook-events", Handle: (*API).createWebhookEvent},
	{Name: "getTenant", Method: http.MethodGet, Pattern: "/admin/api/tenant", Handle: (*API).getTenant},
	{Name: "updateTenant", Method: http.MethodPatch, Pattern: "/admin/api/tenant", Handle: (*API).updateTenant},
	{Name: "listFaults", Method: http.MethodGet, Pattern: "/admin/api/faults", Handle: (*API).listFaults},
	{Name: "createFault", Method: http.MethodPost, Pattern: "/admin/api/faults", Handle: (*API).createFault},
	{Name: "deleteFault", Method: http.MethodDelete, Pattern: "/admin/api/faults/{id}", Handle: (*API).deleteFault},
	{Name: "exportFixtures", Method: http.MethodGet, Pattern: "/admin/api/fixtures/export", Handle: (*API).exportFixtures},
	{Name: "resetFixtures", Method: http.MethodPost, Pattern: "/admin/api/fixtures/reset", Handle: (*API).resetFixtures},
}

// Operations returns a copy of the stable admin operation table.
func Operations() []Operation { return append([]Operation(nil), operations[:]...) }

// Register installs every declared route and a JSON not-found handler for the
// reserved admin namespace.
func (a *API) Register(mux *http.ServeMux) {
	for _, op := range operations {
		operation := op
		mux.Handle(operation.Method+" "+operation.Pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			operation.Handle(a, w, r)
		}))
	}
	mux.HandleFunc("/admin/api/", func(w http.ResponseWriter, _ *http.Request) {
		kferr.Write(w, kferr.EndpointNotFound())
	})
}
