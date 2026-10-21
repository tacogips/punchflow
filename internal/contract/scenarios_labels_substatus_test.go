package contract_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	apiv1 "punchflow/internal/api/v1"
	"punchflow/internal/store"
	"punchflow/internal/testutil"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const specPrefix = "file:///kickflow-openapi.json#/components/schemas/"

// PF-15: labels and sub-status are presented with the OpenAPI Label/SubStatus
// shapes in tickets, routes, lists and webhook payloads, and the ticket list
// filters by sub-status.
func TestContractLabelsAndSubStatus(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	compile := func(uri string) *jsonschema.Schema {
		t.Helper()
		schema, err := compiler.Compile(uri)
		if err != nil {
			t.Fatalf("compile %s: %v", uri, err)
		}
		return schema
	}
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	createSchemas := map[string]*jsonschema.Schema{"createTicket": compile(responseSchemaURI(operations["createTicket"].Pattern, "post"))}
	ticketDetail, labelSchema, subStatusSchema := compile(specPrefix+"TicketDetail"), compile(specPrefix+"Label"), compile(specPrefix+"SubStatus")
	listSchema := compile(responseSchemaURI(operations["listTickets"].Pattern, "get"))
	routeSchema := compile(responseSchemaURI(operations["getRoute"].Pattern, "get"))
	workflowSchema := compile(responseSchemaURI(operations["getWorkflow"].Pattern, "get"))

	ticket := pf26aCreateTicket(t, env, operations["createTicket"], createSchemas, spec, "in_progress")
	path := "/v1/tickets/" + ticket.ID

	var detail struct {
		SubStatus map[string]any   `json:"subStatus"`
		Labels    []map[string]any `json:"labels"`
	}
	fetch := func() *testutil.Response {
		response := env.Do(t, testutil.Employee, http.MethodGet, path, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("get ticket: %d %s", response.Status, response.Body)
		}
		validateJSON(t, ticketDetail, response.Body)
		detail.SubStatus, detail.Labels = nil, nil
		response.JSON(t, &detail)
		return response
	}

	fetch()
	if detail.SubStatus == nil || detail.SubStatus["code"] != "under-review" {
		t.Fatalf("subStatus after submission = %v, want under-review", detail.SubStatus)
	}
	if len(detail.Labels) != 0 {
		t.Fatalf("labels = %v, want none", detail.Labels)
	}
	subStatusBody, _ := json.Marshal(detail.SubStatus)
	validateJSON(t, subStatusSchema, subStatusBody)

	t.Run("attach label through the UI API", func(t *testing.T) {
		list := env.Do(t, testutil.Employee, http.MethodGet, "/ui/api/labels", nil)
		if list.Status != http.StatusOK {
			t.Fatalf("list labels: %d %s", list.Status, list.Body)
		}
		var labels []map[string]any
		list.JSON(t, &labels)
		if len(labels) != 3 {
			t.Fatalf("labels = %d, want the 3 fixture labels", len(labels))
		}
		for _, label := range labels {
			body, _ := json.Marshal(label)
			validateJSON(t, labelSchema, body)
		}
		labelID, _ := labels[0]["id"].(string)
		attached := env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+ticket.ID+"/labels", map[string]any{"labelId": labelID})
		if attached.Status != http.StatusOK {
			t.Fatalf("attach label: %d %s", attached.Status, attached.Body)
		}
		validateJSON(t, ticketDetail, attached.Body)
		fetch()
		if len(detail.Labels) != 1 || detail.Labels[0]["id"] != labelID {
			t.Fatalf("labels after attach = %v", detail.Labels)
		}
		checkTimestamps(t, detail.Labels[0])
		missing := env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+ticket.ID+"/labels", map[string]any{"labelId": "00000000-0000-4000-8000-00000000ffff"})
		if missing.Status != http.StatusUnprocessableEntity {
			t.Fatalf("attach unknown label: %d %s", missing.Status, missing.Body)
		}
		removed := env.Do(t, testutil.Employee, http.MethodDelete, "/ui/api/tickets/"+ticket.ID+"/labels/"+labelID, nil)
		if removed.Status != http.StatusOK {
			t.Fatalf("detach label: %d %s", removed.Status, removed.Body)
		}
		fetch()
		if len(detail.Labels) != 0 {
			t.Fatalf("labels after detach = %v", detail.Labels)
		}
		unauthenticated := env.Do(t, "", http.MethodGet, "/ui/api/labels", nil)
		if unauthenticated.Status != http.StatusUnauthorized {
			t.Fatalf("unauthenticated list labels: %d", unauthenticated.Status)
		}
	})

	t.Run("sub-status list filter", func(t *testing.T) {
		underReview, err := store.SubStatusByCode(t.Context(), env.DB.Q(), "under-review")
		if err != nil {
			t.Fatal(err)
		}
		query := url.Values{"subStatusIds[]": {underReview.ID}}.Encode()
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets?"+query, nil)
		if response.Status != http.StatusOK {
			t.Fatalf("list by sub-status: %d %s", response.Status, response.Body)
		}
		validateJSON(t, listSchema, response.Body)
		var tickets []struct {
			ID        string         `json:"id"`
			SubStatus map[string]any `json:"subStatus"`
		}
		response.JSON(t, &tickets)
		found := false
		for _, item := range tickets {
			if item.SubStatus == nil || item.SubStatus["id"] != underReview.ID {
				t.Fatalf("filtered list contains ticket %s with sub-status %v", item.ID, item.SubStatus)
			}
			found = found || item.ID == ticket.ID
		}
		if !found {
			t.Fatalf("filtered list misses %s: %s", ticket.ID, response.Body)
		}
		finance, err := store.SubStatusByCode(t.Context(), env.DB.Q(), "finance-check")
		if err != nil {
			t.Fatal(err)
		}
		other := env.Do(t, testutil.Admin, http.MethodGet, "/v1/tickets?"+url.Values{"subStatusIds[]": {finance.ID}}.Encode(), nil)
		var none []map[string]any
		other.JSON(t, &none)
		if len(none) != 0 {
			t.Fatalf("finance-check filter matched %d tickets before the finance step", len(none))
		}
	})

	t.Run("route and workflow presentation", func(t *testing.T) {
		routeID, err := store.ResolveID(t.Context(), env.DB.Q(), "routes", "expense-standard")
		if err != nil {
			t.Fatal(err)
		}
		response := env.Do(t, testutil.Admin, http.MethodGet, "/v1/routes/"+routeID, nil)
		validateJSON(t, routeSchema, response.Body)
		var route struct {
			Steps []struct {
				Code      string         `json:"code"`
				SubStatus map[string]any `json:"subStatus"`
			} `json:"steps"`
		}
		response.JSON(t, &route)
		got := map[string]any{}
		for _, step := range route.Steps {
			if step.SubStatus != nil {
				got[step.Code] = step.SubStatus["code"]
			}
		}
		if got["expense-manager"] != "under-review" || got["expense-finance"] != "finance-check" {
			t.Fatalf("route step sub-statuses = %v", got)
		}
		workflowID, err := store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")
		if err != nil {
			t.Fatal(err)
		}
		validateJSON(t, workflowSchema, env.Do(t, testutil.Admin, http.MethodGet, "/v1/workflows/"+workflowID, nil).Body)
	})

	t.Run("sub-status restrictions through the API", func(t *testing.T) {
		errorSchema := compile("file:///kickflow-openapi.json#/components/schemas/ErrorResponse")
		approved := env.Do(t, testutil.Manager, http.MethodPost, path+"/approve", nil)
		if approved.Status != http.StatusOK {
			t.Fatalf("approve: %d %s", approved.Status, approved.Body)
		}
		validateJSON(t, ticketDetail, approved.Body)
		var after struct {
			SubStatus map[string]any `json:"subStatus"`
		}
		approved.JSON(t, &after)
		if after.SubStatus == nil || after.SubStatus["code"] != "finance-check" || after.SubStatus["allowWithdrawal"] != false || after.SubStatus["allowDenial"] != false {
			t.Fatalf("subStatus at the finance step = %v", after.SubStatus)
		}
		withdraw := env.Do(t, testutil.Employee, http.MethodPost, path+"/withdraw", nil)
		pf26aCheckError(t, withdraw, http.StatusUnprocessableEntity, "invalid_state", errorSchema)
		deny := env.Do(t, testutil.Finance, http.MethodPost, path+"/deny", nil)
		pf26aCheckError(t, deny, http.StatusUnprocessableEntity, "invalid_state", errorSchema)
		reject := env.Do(t, testutil.Finance, http.MethodPost, path+"/reject", map[string]any{"to": 0})
		if reject.Status != http.StatusOK {
			t.Fatalf("send back is allowed by finance-check: %d %s", reject.Status, reject.Body)
		}
		validateJSON(t, ticketDetail, reject.Body)
		fetch()
		if detail.SubStatus != nil {
			t.Fatalf("rejected ticket keeps sub-status %v", detail.SubStatus)
		}
	})
}

// PF-15: webhook payloads carry the ticket's labels and sub-status.
func TestContractLabelsAndSubStatusWebhook(t *testing.T) {
	env := testutil.New(t)
	spec := loadSpec(t)
	compiler := schemaCompiler(t, spec)
	payloadSchema, err := compiler.Compile("file:///kickflow-openapi.json#/webhooks/ticket/post/requestBody/content/application~1json/schema")
	if err != nil {
		t.Fatal(err)
	}
	operations := map[string]apiv1.Operation{}
	for _, operation := range apiv1.Operations() {
		operations[operation.OperationID] = operation
	}
	createSchema, err := compiler.Compile(responseSchemaURI(operations["createTicket"].Pattern, "post"))
	if err != nil {
		t.Fatal(err)
	}
	env.RegisterReceiverWebhook(t, "secret", "ticket_updated")
	ticket := pf26aCreateTicket(t, env, operations["createTicket"], map[string]*jsonschema.Schema{"createTicket": createSchema}, spec, "in_progress")
	label, err := store.Labels(t.Context(), env.DB.Q())
	if err != nil || len(label) == 0 {
		t.Fatalf("labels: %v %v", label, err)
	}
	response := env.Do(t, testutil.Employee, http.MethodPost, "/ui/api/tickets/"+ticket.ID+"/labels", map[string]any{"labelId": label[0].ID})
	if response.Status != http.StatusOK {
		t.Fatalf("attach label: %d %s", response.Status, response.Body)
	}
	if env.DeliverWebhooks(t) == 0 {
		t.Fatal("label change queued no ticket_updated delivery")
	}
	requests := env.Receiver.Requests()
	if len(requests) == 0 {
		t.Fatal("receiver got no request")
	}
	body := requests[len(requests)-1].Body
	validateJSON(t, payloadSchema, body)
	var payload struct {
		EventType string `json:"eventType"`
		Data      struct {
			Ticket struct {
				SubStatus map[string]any   `json:"subStatus"`
				Labels    []map[string]any `json:"labels"`
			} `json:"ticket"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.EventType != "ticket_updated" || len(payload.Data.Ticket.Labels) != 1 || payload.Data.Ticket.Labels[0]["id"] != label[0].ID {
		t.Fatalf("payload labels = %+v (%s)", payload, body)
	}
	if payload.Data.Ticket.SubStatus == nil || payload.Data.Ticket.SubStatus["code"] != "under-review" {
		t.Fatalf("payload subStatus = %v", payload.Data.Ticket.SubStatus)
	}
}
