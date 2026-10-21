package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestTicketActionsOptionalBodiesAndTransitions(t *testing.T) {
	env := testutil.New(t)

	approveID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	approved := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+approveID+"/approve", nil)
	if approved.Status != http.StatusOK {
		t.Fatalf("approve without body: %d %s", approved.Status, approved.Body)
	}

	rejectID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	missing := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+rejectID+"/reject", map[string]any{"comment": "needs changes"})
	testutil.AssertError(t, missing, http.StatusBadRequest, "invalid_parameter")
	wrongType := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+rejectID+"/reject", map[string]any{"to": "0"})
	testutil.AssertError(t, wrongType, http.StatusBadRequest, "invalid_parameter")
	rejected := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+rejectID+"/reject", map[string]any{"to": 0})
	if rejected.Status != http.StatusOK {
		t.Fatalf("reject to zero: %d %s", rejected.Status, rejected.Body)
	}
	var rejectedBody struct {
		Status string `json:"status"`
	}
	rejected.JSON(t, &rejectedBody)
	if rejectedBody.Status != "rejected" {
		t.Fatalf("reject status=%q, want rejected", rejectedBody.Status)
	}

	denyID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	denied := env.Do(t, testutil.Manager, http.MethodPost, "/v1/tickets/"+denyID+"/deny", nil)
	if denied.Status != http.StatusOK {
		t.Fatalf("deny without body: %d %s", denied.Status, denied.Body)
	}
	var deniedBody struct {
		Status string `json:"status"`
	}
	denied.JSON(t, &deniedBody)
	if deniedBody.Status != "denied" {
		t.Fatalf("deny status=%q, want denied", deniedBody.Status)
	}

	withdrawID := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	withdrawn := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+withdrawID+"/withdraw", nil)
	if withdrawn.Status != http.StatusOK {
		t.Fatalf("withdraw: %d %s", withdrawn.Status, withdrawn.Body)
	}
	var withdrawnBody struct {
		Status string `json:"status"`
	}
	withdrawn.JSON(t, &withdrawnBody)
	if withdrawnBody.Status != "rejected" {
		t.Fatalf("withdraw status=%q, want rejected", withdrawnBody.Status)
	}
}

func TestTicketActionsUnassignedApprove(t *testing.T) {
	env := testutil.New(t)
	id := pf22CreateTicket(t, env, testutil.Employee, "in_progress", 5000)
	result := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+id+"/approve", nil)
	testutil.AssertError(t, result, http.StatusForbidden, "user_not_assigned")
}
