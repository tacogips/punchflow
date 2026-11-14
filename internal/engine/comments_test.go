package engine

import (
	"encoding/json"
	"reflect"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestCommentRules(t *testing.T) {
	t.Run("comments are disabled for leave approval", func(t *testing.T) {
		e := pf14Setup(t)
		ticketID := pf14CreateWorkflowTicket(t, e, "leave-request", "sales-rep", model.TicketStatusInProgress, map[string]any{
			"start-date": "2025-03-03",
			"end-date":   "2025-03-04",
			"reason":     "vacation",
		}, nil)
		comment := "approved"
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, err := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), ticketID, &comment)
			return err
		})
		assertPF14FieldError(t, err, "validation_failed", "comment")
	})

	t.Run("rejection comment is required", func(t *testing.T) {
		e := pf14Setup(t)
		ticketID := pf14CreateWorkflowTicket(t, e, "purchase-request", "sales-rep", model.TicketStatusInProgress, map[string]any{
			"amount":         "10000",
			"item":           "Laptop",
			"budget-code":    nil,
			"reviewer":       e.user("engineer"),
			"approving-team": e.team("engineering"),
		}, []ApproverInput{{RouteStepCode: "purchase-nominee", UserIDs: []string{e.user("accountant")}}})
		err := e.db.WithTx(e.ctx, func(tx *store.Tx) error {
			_, err := e.engine.SendBack(e.ctx, tx, e.actor("accountant"), ticketID, 0, nil)
			return err
		})
		assertPF14FieldError(t, err, "validation_failed", "comment")
	})

	t.Run("approval comment is stored after approval event", func(t *testing.T) {
		e := pf14Setup(t)
		ticketID := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
		comment := "Looks good"
		var events []model.WebhookEvent
		e.apply(func(tx *store.Tx) error {
			var err error
			events, err = e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), ticketID, &comment)
			return err
		})
		if got := pf14EventTypes(events); !reflect.DeepEqual(got, []string{model.WebhookEventTicketApproved, model.WebhookEventCommentCreated}) {
			t.Fatalf("events=%v", got)
		}
		if _, err := store.EngLoadComment(e.ctx, e.db.Q(), ticketID, events[1].CommentID); err != nil {
			t.Fatalf("action comment was not stored: %v", err)
		}
	})
}

func TestComments(t *testing.T) {
	e := pf14Setup(t)
	ticketID := pf14CreateWorkflowTicket(t, e, "expense-claim", "sales-rep", model.TicketStatusInProgress, map[string]any{"amount": "5000", "purpose": "client meal"}, nil)
	body := "Please review this with <@" + e.user("accountant") + ">"
	var commentID string
	var events []model.WebhookEvent
	e.apply(func(tx *store.Tx) error {
		var err error
		commentID, events, err = e.engine.AddComment(e.ctx, tx, e.actor("sales-rep"), ticketID, body, nil)
		return err
	})
	if got := pf14EventTypes(events); !reflect.DeepEqual(got, []string{model.WebhookEventCommentCreated}) {
		t.Fatalf("add comment events=%v", got)
	}
	comment, err := store.EngLoadComment(e.ctx, e.db.Q(), ticketID, commentID)
	if err != nil {
		t.Fatal(err)
	}
	if comment.Body != body || !reflect.DeepEqual(comment.MentionedUserIDs, []string{e.user("accountant")}) {
		t.Fatalf("stored comment=%+v", comment)
	}

	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, err := e.engine.EditComment(e.ctx, tx, e.actor("sales-manager"), ticketID, commentID, "unauthorized edit")
		return err
	})
	assertPF14ErrorCode(t, err, "missing_permission")

	e.apply(func(tx *store.Tx) error {
		var err error
		events, err = e.engine.DeleteComment(e.ctx, tx, e.actor("sales-rep"), ticketID, commentID)
		return err
	})
	if got := pf14EventTypes(events); !reflect.DeepEqual(got, []string{model.WebhookEventCommentUpdated}) {
		t.Fatalf("delete comment events=%v", got)
	}
	comment, err = store.EngLoadComment(e.ctx, e.db.Q(), ticketID, commentID)
	if err != nil {
		t.Fatal(err)
	}
	if comment.Body != "" || comment.DeletedAt == nil || len(comment.MentionedUserIDs) != 0 {
		t.Fatalf("deleted comment=%+v", comment)
	}
}

func pf14CreateWorkflowTicket(t *testing.T, e *pf14TestEnv, workflowCode, actorCode, status string, values map[string]any, approvers []ApproverInput) string {
	t.Helper()
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), workflowCode)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(e.ctx, e.db.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]FieldInput, 0)
	for _, section := range definition.SectionList {
		for _, field := range section.FormFields {
			value, ok := values[field.Code]
			if !ok {
				value = nil
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			input := FieldInput{FormFieldID: field.ID, FormFieldCode: field.Code}
			switch field.FieldType {
			case "user":
				input.UserID = encoded
			case "team":
				input.TeamID = encoded
			case "master":
				input.GeneralMasterItemID = encoded
			default:
				input.Value = encoded
			}
			inputs = append(inputs, input)
		}
	}
	var ticketID string
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		var err error
		ticketID, _, err = e.engine.Create(e.ctx, tx, e.actor(actorCode), CreateInput{
			Status:       status,
			WorkflowID:   workflow.ID,
			AuthorTeamID: e.team("sales"),
			Inputs:       &inputs,
			Approvers:    approvers,
		})
		return err
	})
	if err != nil {
		t.Fatalf("create %s ticket: %v", workflowCode, err)
	}
	return ticketID
}

func pf14EventTypes(events []model.WebhookEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func assertPF14FieldError(t *testing.T, err error, code, field string) {
	t.Helper()
	api, ok := kferr.As(err)
	if !ok || api.Code != code || len(api.Errors[field]) == 0 {
		t.Fatalf("error=%v, want API error %q with field %q", err, code, field)
	}
}
