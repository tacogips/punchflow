package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// PF-15: a sub-status is attached when its route step becomes current and is
// replaced or cleared as the ticket moves on (INF-60).
func TestSubStatusFollowsRoute(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "finance-check", false, "finance-manager"),
		pf15RouteStep("final", 3, model.ActionTypeApprove, "", false, "ceo"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if got := pf15SubStatusCode(t, e, id); got != "under-review" {
		t.Fatalf("sub-status after submission=%q, want under-review", got)
	}
	compatApprove(t, e, "sales-manager", id)
	if got := pf15SubStatusCode(t, e, id); got != "finance-check" {
		t.Fatalf("sub-status at finance step=%q, want finance-check", got)
	}
	compatApprove(t, e, "finance-manager", id)
	if got := pf15SubStatusCode(t, e, id); got != "finance-check" {
		t.Fatalf("sub-status at a step without one=%q, want retained finance-check", got)
	}
	compatApprove(t, e, "ceo", id)
	if got := e.ticket(id).Status; got != model.TicketStatusCompleted {
		t.Fatalf("status=%s, want completed", got)
	}
	if got := pf15SubStatusCode(t, e, id); got != "" {
		t.Fatalf("completed ticket keeps sub-status %q", got)
	}
}

func TestSubStatusReappliedOnSendBackAndResubmission(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "finance-check", false, "finance-manager"),
	)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "sales-manager", id)
	if _, err := pf15SendBack(e, "finance-manager", id, 1); err != nil {
		t.Fatal(err)
	}
	if got := pf15SubStatusCode(t, e, id); got != "under-review" {
		t.Fatalf("sub-status after sending back to step 1=%q, want under-review", got)
	}
	if _, err := pf15SendBack(e, "sales-manager", id, 0); err != nil {
		t.Fatal(err)
	}
	if got := pf15SubStatusCode(t, e, id); got != "" {
		t.Fatalf("rejected ticket keeps sub-status %q", got)
	}
	if _, err := compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)}); err != nil {
		t.Fatal(err)
	}
	if got := pf15SubStatusCode(t, e, id); got != "under-review" {
		t.Fatalf("sub-status after resubmission=%q, want under-review", got)
	}
}

// PF-15: allowWithdrawal/allowRejection/allowDenial gate the actions (INF-61).
func TestSubStatusRestrictsActions(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("finance", 1, model.ActionTypeApprove, "finance-check", false, "finance-manager"),
	)
	// finance-check: withdrawal and denial are forbidden, sending back is allowed.
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	_, err := pf15Withdraw(e, "sales-rep", id)
	pf15WantCode(t, "withdraw", err, "invalid_state")
	_, err = pf15Deny(e, "finance-manager", id)
	pf15WantCode(t, "deny", err, "invalid_state")
	if got := e.ticket(id).Status; got != model.TicketStatusInProgress {
		t.Fatalf("refused actions changed status to %s", got)
	}
	if got := pf15SubStatusCode(t, e, id); got != "finance-check" {
		t.Fatalf("refused actions changed the sub-status to %q", got)
	}
	events, err := pf15SendBack(e, "finance-manager", id, 0)
	if err != nil {
		t.Fatalf("send back is allowed by finance-check: %v", err)
	}
	compatAssertTypes(t, "send back", events, model.WebhookEventTicketRejected)

	// under-review allows everything.
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"),
	)
	open, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if _, err = pf15Withdraw(e, "sales-rep", open); err != nil {
		t.Fatalf("withdraw under-review: %v", err)
	}
	denied, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if _, err = pf15Deny(e, "sales-manager", denied); err != nil {
		t.Fatalf("deny under-review: %v", err)
	}
	if got := pf15SubStatusCode(t, e, denied); got != "" {
		t.Fatalf("denied ticket keeps sub-status %q", got)
	}
}

func TestSubStatusRestrictionDoesNotApplyWithoutSubStatus(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim", pf15RouteStep("plain", 1, model.ActionTypeApprove, "", false, "sales-manager"))
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if _, err := pf15Withdraw(e, "sales-rep", id); err != nil {
		t.Fatalf("withdraw without sub-status: %v", err)
	}
}

// PF-15: sub_status_attached numbering allocates the number when the
// configured sub-status (or, unconfigured, any sub-status) is attached.
func TestNumberingAtSubStatusAttached(t *testing.T) {
	setup := func(t *testing.T, numberingSubStatus any) *pf14TestEnv {
		e := pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberingTiming", "sub_status_attached")
		compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberFormat", "S-[number]")
		if numberingSubStatus != nil {
			compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberingSubStatus", numberingSubStatus)
		}
		compatSetRoute(t, e, "expense-claim",
			pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"),
			pf15RouteStep("finance", 2, model.ActionTypeApprove, "finance-check", false, "finance-manager"),
		)
		return e
	}
	t.Run("configured sub-status", func(t *testing.T) {
		e := setup(t, "finance-check")
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		if got := compatNumber(t, e, id); got != "" {
			t.Fatalf("number at under-review=%q, want none", got)
		}
		compatApprove(t, e, "sales-manager", id)
		if got := compatNumber(t, e, id); got != "S-1" {
			t.Fatalf("number at finance-check=%q, want S-1", got)
		}
		compatApprove(t, e, "finance-manager", id)
		if got := compatNumber(t, e, id); got != "S-1" {
			t.Fatalf("number changed on completion: %q", got)
		}
	})
	t.Run("any sub-status", func(t *testing.T) {
		e := setup(t, nil)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		if got := compatNumber(t, e, id); got != "S-1" {
			t.Fatalf("number at first sub-status=%q, want S-1", got)
		}
		compatApprove(t, e, "sales-manager", id)
		if got := compatNumber(t, e, id); got != "S-1" {
			t.Fatalf("number reallocated at second sub-status: %q", got)
		}
	})
	t.Run("never attached", func(t *testing.T) {
		e := pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberingTiming", "sub_status_attached")
		compatSetWorkflow(t, e, "expense-claim", "$.ticketNumberFormat", "S-[number]")
		compatSetRoute(t, e, "expense-claim", pf15RouteStep("plain", 1, model.ActionTypeApprove, "", false, "sales-manager"))
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		compatApprove(t, e, "sales-manager", id)
		if got := compatNumber(t, e, id); got != "" {
			t.Fatalf("ticket without a sub-status got number %q", got)
		}
	})
}

func TestSubStatusListFilter(t *testing.T) {
	e := pf14Setup(t)
	compatSetRoute(t, e, "expense-claim",
		pf15RouteStep("review", 1, model.ActionTypeApprove, "under-review", false, "sales-manager"),
		pf15RouteStep("finance", 2, model.ActionTypeApprove, "finance-check", false, "finance-manager"),
	)
	first, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	second, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatApprove(t, e, "sales-manager", second)
	statuses, err := store.SubStatuses(e.ctx, e.db.Q())
	if err != nil {
		t.Fatal(err)
	}
	idByCode := map[string]string{}
	for _, item := range statuses {
		idByCode[item.Code] = item.ID
	}
	list := func(codes ...string) []string {
		filter := store.TicketQueryFilter{UserID: e.user("admin")}
		for _, code := range codes {
			filter.SubStatusIDs = append(filter.SubStatusIDs, idByCode[code])
		}
		ids, _, err := store.TicketQueryList(e.ctx, e.db.Q(), filter, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}
	if got := list("under-review"); len(got) != 1 || got[0] != first {
		t.Fatalf("under-review filter=%v, want [%s]", got, first)
	}
	if got := list("finance-check"); len(got) != 1 || got[0] != second {
		t.Fatalf("finance-check filter=%v, want [%s]", got, second)
	}
	if got := list("under-review", "finance-check"); len(got) != 2 {
		t.Fatalf("combined filter=%v, want both tickets", got)
	}
	if got := list(); len(got) < 2 {
		t.Fatalf("empty filter must not restrict: %v", got)
	}
}
