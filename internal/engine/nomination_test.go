package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestPreviousApproverCanNominateOnlyImmediateNextActionableGroup(t *testing.T) {
	t.Run("rejects farther future customizable step", func(t *testing.T) {
		e := pf14Setup(t)
		target := pf15RouteStep("future-custom", 3, model.ActionTypeApprove, "", false)
		target.StepType = model.StepTypeAssigneeCustomizable
		minimum := 1
		target.MinCustomAssignees = &minimum
		compatSetRoute(t, e, "expense-claim",
			pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
			pf15RouteStep("intervening", 2, model.ActionTypeApprove, "", false, "finance-manager"),
			target,
		)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
			return e.engine.NominateNextApprovers(e.ctx, tx, e.actor("sales-manager"), id, "future-custom", []string{e.user("finance-manager")})
		})
		pf15WantCode(t, "farther future nomination", err, "invalid_state")
	})

	t.Run("allows a customizable step after an already skipped route step", func(t *testing.T) {
		e := pf14Setup(t)
		intervening := pf15RouteStep("skipped", 2, model.ActionTypeApprove, "", false, "finance-manager")
		intervening.RouteStepCondition = &model.RouteStepCondition{ID: "skip-condition", ConditionType: model.StepConditionTypeConditionalSkip, CombinationType: model.StepCombinationAll}
		target := pf15RouteStep("next-custom", 3, model.ActionTypeApprove, "", false)
		target.StepType = model.StepTypeAssigneeCustomizable
		minimum := 1
		target.MinCustomAssignees = &minimum
		compatSetRoute(t, e, "expense-claim",
			pf15RouteStep("review", 1, model.ActionTypeApprove, "", false, "sales-manager"),
			intervening,
			target,
		)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		_, err := pf15Run(e, func(tx *store.Tx) ([]model.WebhookEvent, error) {
			return e.engine.NominateNextApprovers(e.ctx, tx, e.actor("sales-manager"), id, "next-custom", []string{e.user("finance-manager")})
		})
		if err != nil {
			t.Fatalf("nominate after skipped group: %v", err)
		}
		var count int
		if err = e.db.Q().QueryRowContext(e.ctx, `SELECT count(*) FROM ticket_assignees a JOIN ticket_steps s ON s.id=a.ticket_step_id WHERE a.ticket_id=? AND s.route_step_code='next-custom' AND a.user_id=?`, id, e.user("finance-manager")).Scan(&count); err != nil || count != 1 {
			t.Fatalf("next customizable assignee count=%d err=%v, want one", count, err)
		}
	})
}
