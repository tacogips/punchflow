package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func compatNomineeRoute(t *testing.T, e *pf14TestEnv) {
	t.Helper()
	min := 1
	nominee := model.RouteStep{Code: "nominee", StepOrder: 1, Title: "nominee", StepType: model.StepTypeAuthorCustomizable, ActionType: model.ActionTypeApprove, MinCustomAssignees: &min}
	compatSetRoute(t, e, "expense-claim", nominee, compatUserStep("final", 2, model.ActionTypeApprove, "finance-manager"))
}

func compatNominations(t *testing.T, e *pf14TestEnv, id string) []string {
	t.Helper()
	ids, err := store.EngCustomApprovers(e.ctx, e.db.Q(), id, "nominee")
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// R05 / PF-04: applicants nominate approvers for author_customizable steps on
// draft and rejected tickets; draft creation persists them and submission uses
// them.
func TestApplicantNominationsOnDraftAndRejectedTickets(t *testing.T) {
	nominate := func(users ...string) *[]ApproverInput {
		return &[]ApproverInput{{RouteStepCode: "nominee", UserIDs: users}}
	}

	t.Run("draft creation persists nominations and submission uses them", func(t *testing.T) {
		e := pf14Setup(t)
		compatNomineeRoute(t, e)
		id, _, err := compatCreateExpense(t, e, "sales-rep", "sales", model.TicketStatusDraft, *nominate(e.user("engineer")))
		if err != nil {
			t.Fatal(err)
		}
		if got := compatNominations(t, e, id); len(got) != 1 || got[0] != e.user("engineer") {
			t.Fatalf("persisted nominations=%v", got)
		}
		if _, err = compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)}); err != nil {
			t.Fatalf("submit with persisted nominations: %v", err)
		}
		if got := pf14cCurrentApprover(t, e, id); got != e.user("engineer") {
			t.Fatalf("current approver=%s, want nominee", got)
		}
	})

	t.Run("author saves nominations on a draft and replaces them", func(t *testing.T) {
		e := pf14Setup(t)
		compatNomineeRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		events, err := compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: nominate(e.user("engineer"))})
		if err != nil {
			t.Fatalf("author nominates: %v", err)
		}
		compatAssertTypes(t, "nominate", events, model.WebhookEventTicketUpdated)
		if _, err = compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: nominate(e.user("eng-manager"))}); err != nil {
			t.Fatal(err)
		}
		if got := compatNominations(t, e, id); len(got) != 1 || got[0] != e.user("eng-manager") {
			t.Fatalf("nominations after replace=%v", got)
		}
	})

	t.Run("nominations and submission in one request", func(t *testing.T) {
		e := pf14Setup(t)
		compatNomineeRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		if _, err := compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress), Approvers: nominate(e.user("engineer"))}); err != nil {
			t.Fatal(err)
		}
		if got := pf14cCurrentApprover(t, e, id); got != e.user("engineer") {
			t.Fatalf("current approver=%s, want nominee", got)
		}
	})

	t.Run("submission without nominations still fails", func(t *testing.T) {
		e := pf14Setup(t)
		compatNomineeRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		_, err := compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)})
		assertPF14ErrorCode(t, err, "validation_failed")
		if got := e.ticket(id).Status; got != model.TicketStatusDraft {
			t.Fatalf("status=%s after failed submit", got)
		}
	})

	t.Run("rejected ticket renominates before resubmission", func(t *testing.T) {
		e := pf14Setup(t)
		compatNomineeRoute(t, e)
		id, _, err := compatCreateExpense(t, e, "sales-rep", "sales", model.TicketStatusInProgress, *nominate(e.user("engineer")))
		if err != nil {
			t.Fatal(err)
		}
		e.apply(func(tx *store.Tx) error { _, err := e.engine.Withdraw(e.ctx, tx, e.actor("sales-rep"), id); return err })
		if _, err = compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: nominate(e.user("eng-manager")), Status: compatStr(model.TicketStatusInProgress)}); err != nil {
			t.Fatal(err)
		}
		if got := pf14cCurrentApprover(t, e, id); got != e.user("eng-manager") {
			t.Fatalf("current approver=%s, want the new nominee", got)
		}
	})

	t.Run("validation", func(t *testing.T) {
		e := pf14Setup(t)
		compatNomineeRoute(t, e)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		_, err := compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: &[]ApproverInput{{RouteStepCode: "final", UserIDs: []string{e.user("engineer")}}}})
		assertPF14ErrorCode(t, err, "invalid_parameter")
		_, err = compatUpdate(e, "sales-rep", id, UpdateInput{Approvers: nominate("00000000-0000-4000-8000-0000000000ff")})
		assertPF14ErrorCode(t, err, "validation_failed")
		_, err = compatUpdate(e, "sales-manager", id, UpdateInput{Approvers: nominate(e.user("engineer"))})
		// Drafts are not shared with applicant-team members until application.
		assertPF14ErrorCode(t, err, "invisible_ticket")
	})
}

// R10 / PF-05: an explicit status that equals the current draft/rejected status
// is an ordinary save; real transitions are still validated.
func TestExplicitSameStateSaves(t *testing.T) {
	e := pf14Setup(t)
	draftID, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
	events, err := compatUpdate(e, "sales-rep", draftID, UpdateInput{Status: compatStr(model.TicketStatusDraft), Title: compatStr("saved draft")})
	if err != nil {
		t.Fatalf("draft save with status draft: %v", err)
	}
	compatAssertTypes(t, "draft save", events, model.WebhookEventTicketUpdated)
	if ticket := e.ticket(draftID); ticket.Status != model.TicketStatusDraft || ticket.Title == nil || *ticket.Title != "saved draft" {
		t.Fatalf("ticket after draft save: status=%s title=%v", ticket.Status, ticket.Title)
	}
	events, err = compatUpdate(e, "sales-rep", draftID, UpdateInput{Status: compatStr(model.TicketStatusDraft)})
	if err != nil {
		t.Fatalf("no-op draft save: %v", err)
	}
	compatAssertTypes(t, "no-op draft save", events)

	_, err = compatUpdate(e, "sales-rep", draftID, UpdateInput{Status: compatStr(model.TicketStatusRejected)})
	assertPF14ErrorCode(t, err, "invalid_state")
	_, err = compatUpdate(e, "sales-rep", draftID, UpdateInput{Status: compatStr(model.TicketStatusCompleted)})
	assertPF14ErrorCode(t, err, "invalid_state")

	rejectedID, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	e.apply(func(tx *store.Tx) error {
		_, err := e.engine.Withdraw(e.ctx, tx, e.actor("sales-rep"), rejectedID)
		return err
	})
	if _, err = compatUpdate(e, "sales-rep", rejectedID, UpdateInput{Status: compatStr(model.TicketStatusRejected), Title: compatStr("edited")}); err != nil {
		t.Fatalf("rejected save with status rejected: %v", err)
	}
	if got := e.ticket(rejectedID).Status; got != model.TicketStatusRejected {
		t.Fatalf("status=%s after rejected save", got)
	}
	_, err = compatUpdate(e, "sales-rep", rejectedID, UpdateInput{Status: compatStr(model.TicketStatusDraft)})
	assertPF14ErrorCode(t, err, "invalid_state")
}

// PF-18: application restrictions honor descendant teams and grades, and are
// re-checked when a draft is submitted.
func TestWorkflowApplicationRestrictions(t *testing.T) {
	restrict := func(e *pf14TestEnv, entries ...model.WorkflowTeam) {
		compatSetWorkflow(t, e, "expense-claim", "$.availableToEveryone", false)
		compatSetWorkflow(t, e, "expense-claim", "$.availableTeams", entries)
	}
	atLeast := model.GradeSymbolGreaterThanOrEqual
	create := func(e *pf14TestEnv, actor, team, status string) error {
		_, _, err := compatCreateExpense(t, e, actor, team, status, nil)
		return err
	}

	t.Run("descendant teams", func(t *testing.T) {
		e := pf14Setup(t)
		restrict(e, model.WorkflowTeam{TeamCode: "engineering", Descendants: true, GradeCodes: []string{}})
		if err := create(e, "engineer", "platform", model.TicketStatusDraft); err != nil {
			t.Fatalf("member of a descendant team: %v", err)
		}
		if err := create(e, "engineer", "engineering", model.TicketStatusDraft); err != nil {
			t.Fatalf("member of the listed team: %v", err)
		}
		assertPF14ErrorCode(t, create(e, "sales-rep", "sales", model.TicketStatusDraft), "ticket_create_restricted")

		restrict(e, model.WorkflowTeam{TeamCode: "engineering", Descendants: false, GradeCodes: []string{}})
		assertPF14ErrorCode(t, create(e, "engineer", "platform", model.TicketStatusDraft), "ticket_create_restricted")
		if err := create(e, "engineer", "engineering", model.TicketStatusDraft); err != nil {
			t.Fatalf("listed team without descendants: %v", err)
		}
	})

	t.Run("grade restriction", func(t *testing.T) {
		e := pf14Setup(t)
		restrict(e, model.WorkflowTeam{TeamCode: "company", Descendants: true, GradeSymbol: &atLeast, GradeCodes: []string{"manager"}})
		if err := create(e, "sales-manager", "sales", model.TicketStatusDraft); err != nil {
			t.Fatalf("manager in a descendant team: %v", err)
		}
		assertPF14ErrorCode(t, create(e, "sales-rep", "sales", model.TicketStatusDraft), "ticket_create_restricted")
		assertPF14ErrorCode(t, create(e, "engineer", "platform", model.TicketStatusInProgress), "ticket_create_restricted")
	})

	t.Run("submission rechecks eligibility", func(t *testing.T) {
		e := pf14Setup(t)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		restrict(e, model.WorkflowTeam{TeamCode: "company", Descendants: true, GradeSymbol: &atLeast, GradeCodes: []string{"manager"}})
		_, err := compatUpdate(e, "sales-rep", id, UpdateInput{Status: compatStr(model.TicketStatusInProgress)})
		assertPF14ErrorCode(t, err, "ticket_create_restricted")
		if got := e.ticket(id).Status; got != model.TicketStatusDraft {
			t.Fatalf("status=%s after restricted submit", got)
		}
		// Saving the draft stays possible; only submission is gated.
		if _, err = compatUpdate(e, "sales-rep", id, UpdateInput{Title: compatStr("still editable")}); err != nil {
			t.Fatalf("draft save: %v", err)
		}
	})
}
