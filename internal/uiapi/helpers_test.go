package uiapi_test

import (
	"net/http"
	"testing"

	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

// expenseEnv wraps a seeded test server with the expense-claim workflow ids.
type expenseEnv struct {
	*testutil.Env
	workflowID  string
	teamID      string
	fields      map[string]string
	slipSection string
	slipFields  map[string]string
}

func newExpenseEnv(t *testing.T) *expenseEnv {
	t.Helper()
	env := testutil.New(t)
	response := env.Do(t, testutil.Employee, http.MethodGet, "/v1/workflows/expense-claim", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("get expense-claim workflow: status=%d body=%s", response.Status, response.Body)
	}
	var workflow struct {
		ID          string `json:"id"`
		SectionList []struct {
			ID          string `json:"id"`
			SectionType string `json:"sectionType"`
			FormFields  []struct{ ID, Code string }
			SlipFields  []struct{ ID, Code string }
		} `json:"sectionList"`
	}
	response.JSON(t, &workflow)
	out := &expenseEnv{Env: env, workflowID: workflow.ID, fields: map[string]string{}, slipFields: map[string]string{}}
	for _, section := range workflow.SectionList {
		for _, field := range section.FormFields {
			out.fields[field.Code] = field.ID
		}
		if section.SectionType == "slip" {
			out.slipSection = section.ID
			for _, field := range section.SlipFields {
				out.slipFields[field.Code] = field.ID
			}
		}
	}
	teamID, err := store.ResolveID(t.Context(), env.DB.Q(), "teams", "sales")
	if err != nil {
		t.Fatalf("resolve sales team: %v", err)
	}
	out.teamID = teamID
	return out
}

type ticketSpec struct {
	status  string
	title   string
	purpose string
	amount  int
	rows    []rowSpec
}

type rowSpec struct {
	item string
	qty  int
	unit int
}

func (e *expenseEnv) createTicket(t *testing.T, persona testutil.Persona, spec ticketSpec) string {
	t.Helper()
	if spec.status == "" {
		spec.status = "in_progress"
	}
	if spec.amount == 0 {
		spec.amount = 5000
	}
	masterID, err := store.ResolveID(t.Context(), e.DB.Q(), "general_masters", "cost-centers")
	if err != nil {
		t.Fatalf("resolve cost-centers master: %v", err)
	}
	masterItemID, err := store.MasterItemIDByCode(t.Context(), e.DB.Q(), masterID, "cc-sales")
	if err != nil {
		t.Fatalf("resolve cc-sales master item: %v", err)
	}
	body := map[string]any{
		"status": spec.status, "workflowId": e.workflowID, "authorTeamId": e.teamID, "title": spec.title,
		"inputs": []map[string]any{
			{"formFieldId": e.fields["amount"], "value": spec.amount},
			{"formFieldId": e.fields["purpose"], "value": spec.purpose},
			{"formFieldId": e.fields["expense-date"], "value": "2026-01-01"},
			{"formFieldId": e.fields["cost-center"], "generalMasterItemId": masterItemID},
			{"formFieldId": e.fields["receipt"], "files": []string{}},
			{"formFieldId": e.fields["approver-note"], "value": ""},
		},
	}
	if len(spec.rows) > 0 {
		items := make([]map[string]any, 0, len(spec.rows))
		for _, row := range spec.rows {
			items = append(items, map[string]any{"slipSectionId": e.slipSection, "inputs": []map[string]any{
				{"slipFieldId": e.slipFields["item"], "value": row.item},
				{"slipFieldId": e.slipFields["qty"], "value": row.qty},
				{"slipFieldId": e.slipFields["unit-price"], "value": row.unit},
			}})
		}
		body["slipItems"] = items
	}
	response := e.Do(t, persona, http.MethodPost, "/v1/tickets", body)
	if response.Status != http.StatusOK {
		t.Fatalf("create ticket: status=%d body=%s", response.Status, response.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	response.JSON(t, &created)
	return created.ID
}

func (e *expenseEnv) post(t *testing.T, persona testutil.Persona, path string, body any) *testutil.Response {
	t.Helper()
	return e.Do(t, persona, http.MethodPost, path, body)
}

type notification struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	TicketID      string  `json:"ticketId"`
	TicketTitle   *string `json:"ticketTitle"`
	TicketVisible bool    `json:"ticketVisible"`
	Excerpt       string  `json:"excerpt"`
	Read          bool    `json:"read"`
	Actor         *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"actor"`
}

func (e *expenseEnv) inbox(t *testing.T, persona testutil.Persona, query string) []notification {
	t.Helper()
	response := e.Do(t, persona, http.MethodGet, "/ui/api/notifications"+query, nil)
	if response.Status != http.StatusOK {
		t.Fatalf("list notifications: status=%d body=%s", response.Status, response.Body)
	}
	var items []notification
	response.JSON(t, &items)
	return items
}

func (e *expenseEnv) unread(t *testing.T, persona testutil.Persona) int {
	t.Helper()
	response := e.Do(t, persona, http.MethodGet, "/ui/api/notifications/unread-count", nil)
	if response.Status != http.StatusOK {
		t.Fatalf("unread count: status=%d body=%s", response.Status, response.Body)
	}
	var out struct {
		Count int `json:"count"`
	}
	response.JSON(t, &out)
	return out.Count
}

func kinds(items []notification) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Kind)
	}
	return out
}

func countKind(items []notification, kind, ticketID string) int {
	n := 0
	for _, item := range items {
		if item.Kind == kind && (ticketID == "" || item.TicketID == ticketID) {
			n++
		}
	}
	return n
}
