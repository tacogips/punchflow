package uiapi_test

import (
	"fmt"
	"net/http"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/testutil"
)

func TestAssigneePendingControlsTaskFilterAndClearsOnStepChange(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Held ticket", purpose: "review"})
	managerID := e.UserID(t, "sales-manager")
	var assigneeID string
	err := e.DB.Q().QueryRowContext(t.Context(), `SELECT ta.id FROM ticket_assignees ta JOIN ticket_steps ts ON ts.id=ta.ticket_step_id WHERE ta.ticket_id=? AND ta.user_id=? AND ts.status=?`, id, managerID, model.StepStatusInProgress).Scan(&assigneeID)
	if err != nil {
		t.Fatalf("find current manager assignment: %v", err)
	}
	path := fmt.Sprintf("/ui/api/tickets/%s/assignees/%s/pending", id, assigneeID)
	e.CallerFor = "sales-deputy"
	held := e.Do(t, testutil.Integration, http.MethodPost, path, map[string]any{"pending": true})
	if held.Status != http.StatusOK || string(held.Body) != `{"pending":true}` {
		t.Fatalf("hold response: status=%d body=%s", held.Status, held.Body)
	}
	released := e.Do(t, testutil.Manager, http.MethodPost, path, map[string]any{"pending": false})
	if released.Status != http.StatusOK || string(released.Body) != `{"pending":false}` {
		t.Fatalf("release response: status=%d body=%s", released.Status, released.Body)
	}
	if heldAgain := e.Do(t, testutil.Integration, http.MethodPost, path, map[string]any{"pending": true}); heldAgain.Status != http.StatusOK {
		t.Fatalf("hold again through proxy: status=%d body=%s", heldAgain.Status, heldAgain.Body)
	}
	detail := e.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/"+id, nil)
	if detail.Status != http.StatusOK {
		t.Fatalf("ticket detail: status=%d body=%s", detail.Status, detail.Body)
	}
	var ticket struct {
		Steps []struct {
			Assignees []struct {
				ID      string `json:"id"`
				Pending bool   `json:"pending"`
			} `json:"assignees"`
		} `json:"steps"`
	}
	detail.JSON(t, &ticket)
	found := false
	for _, step := range ticket.Steps {
		for _, assignee := range step.Assignees {
			if assignee.ID == assigneeID {
				found = true
				if !assignee.Pending {
					t.Fatal("ticket detail did not expose pending=true")
				}
			}
		}
	}
	if !found {
		t.Fatalf("ticket detail omitted assignment %s", assigneeID)
	}
	tasks := e.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/tasks?pending=true", nil)
	if tasks.Status != http.StatusOK {
		t.Fatalf("pending tasks: status=%d body=%s", tasks.Status, tasks.Body)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	tasks.JSON(t, &rows)
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("pending tasks = %+v, want ticket %s", rows, id)
	}

	// The hold remains while the current step is still active, then clears
	// automatically when approval advances the ticket to its next step.
	approved := e.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+id+"/approve", nil)
	if approved.Status != http.StatusOK {
		t.Fatalf("approve held ticket: status=%d body=%s", approved.Status, approved.Body)
	}
	check := e.Do(t, testutil.Manager, http.MethodGet, "/v1/tickets/"+id, nil)
	check.JSON(t, &ticket)
	for _, step := range ticket.Steps {
		for _, assignee := range step.Assignees {
			if assignee.ID == assigneeID && assignee.Pending {
				t.Fatal("step transition did not clear the assignment hold")
			}
		}
	}
}

func TestAssigneePendingRejectsOtherUsersAndInvalidState(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Pending access", purpose: "review"})
	managerID := e.UserID(t, "sales-manager")
	var assigneeID string
	err := e.DB.Q().QueryRowContext(t.Context(), `SELECT ta.id FROM ticket_assignees ta JOIN ticket_steps ts ON ts.id=ta.ticket_step_id WHERE ta.ticket_id=? AND ta.user_id=? AND ts.status=?`, id, managerID, model.StepStatusInProgress).Scan(&assigneeID)
	if err != nil {
		t.Fatalf("find current manager assignment: %v", err)
	}
	path := fmt.Sprintf("/ui/api/tickets/%s/assignees/%s/pending", id, assigneeID)
	testutil.AssertError(t, e.Do(t, testutil.Employee, http.MethodPost, path, map[string]any{"pending": true}), http.StatusForbidden, "user_not_assigned")
	testutil.AssertError(t, e.Do(t, testutil.Manager, http.MethodPost, path, map[string]any{}), http.StatusBadRequest, "invalid_parameter")

	if _, err := e.DB.Q().ExecContext(t.Context(), `UPDATE tickets SET status=? WHERE id=?`, model.TicketStatusRejected, id); err != nil {
		t.Fatalf("reject ticket for invalid state case: %v", err)
	}
	testutil.AssertError(t, e.Do(t, testutil.Manager, http.MethodPost, path, map[string]any{"pending": true}), http.StatusUnprocessableEntity, "invalid_state")
}

func TestAssigneePendingFindsAssignmentsAcrossCurrentParallelGroup(t *testing.T) {
	e := newExpenseEnv(t)
	id := e.createTicket(t, testutil.Employee, ticketSpec{title: "Parallel hold", purpose: "review"})
	var currentStep int
	if err := e.DB.Q().QueryRowContext(t.Context(), `SELECT current_step FROM tickets WHERE id=?`, id).Scan(&currentStep); err != nil {
		t.Fatal(err)
	}
	stepID, assigneeID := "parallel-step", "parallel-assignee"
	managerID := e.UserID(t, "sales-manager")
	if _, err := e.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_steps(id,ticket_id,step_order,display_order,title,action_type,required_approvals_number,required_approvals_percent,status,skip,fallback,created_at,updated_at) VALUES(?,?,?,?,?,'approve',1,100,'in_progress',0,0,1,1)`, stepID, id, currentStep, 99, "Parallel review"); err != nil {
		t.Fatalf("insert parallel active step: %v", err)
	}
	if _, err := e.DB.Q().ExecContext(t.Context(), `INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,assigned_at,created_at,updated_at) VALUES(?,?,?, ?,0,1,1,1)`, assigneeID, stepID, id, managerID); err != nil {
		t.Fatalf("insert parallel assignee: %v", err)
	}
	path := fmt.Sprintf("/ui/api/tickets/%s/assignees/%s/pending", id, assigneeID)
	// sales-deputy is an active proxy for sales-manager; the proxy authority
	// check must still apply when the assignment is on a later parallel step.
	e.CallerFor = "sales-deputy"
	response := e.Do(t, testutil.Integration, http.MethodPost, path, map[string]any{"pending": true})
	if response.Status != http.StatusOK || string(response.Body) != `{"pending":true}` {
		t.Fatalf("hold later parallel assignment: status=%d body=%s", response.Status, response.Body)
	}
	var pending bool
	if err := e.DB.Q().QueryRowContext(t.Context(), `SELECT pending FROM ticket_assignees WHERE id=?`, assigneeID).Scan(&pending); err != nil || !pending {
		t.Fatalf("later parallel assignment pending=%t err=%v", pending, err)
	}
}
