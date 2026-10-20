package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"punchflow/internal/engine"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"punchflow/internal/testutil"
)

func TestCommentLifecycleAndWebhookEvents(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf23CreateExpenseTicket(t, env, "sales-rep", "sales", "")
	mentionedID := env.UserID(t, "accountant")
	env.RegisterReceiverWebhook(t, "pf23-webhook-secret", model.WebhookEventCommentCreated, model.WebhookEventCommentUpdated)
	base := "/v1/tickets/" + ticketID + "/comments"

	created := env.Do(t, testutil.Employee, http.MethodPost, base, map[string]any{"body": "Please review <@" + mentionedID + ">"})
	if created.Status != http.StatusOK {
		t.Fatalf("create comment: status=%d body=%s", created.Status, created.Body)
	}
	var comment struct {
		ID             string `json:"id"`
		Body           string `json:"body"`
		MentionedUsers []any  `json:"mentionedUsers"`
		Attachments    []any  `json:"attachments"`
	}
	created.JSON(t, &comment)
	if comment.ID == "" || comment.Body == "" || len(comment.MentionedUsers) != 1 || comment.Attachments == nil {
		t.Fatalf("unexpected created comment: %+v", comment)
	}
	second := env.Do(t, testutil.Employee, http.MethodPost, base, map[string]any{"body": "Second comment"})
	if second.Status != http.StatusOK {
		t.Fatalf("create second comment: status=%d body=%s", second.Status, second.Body)
	}
	updated := env.Do(t, testutil.Employee, http.MethodPatch, base+"/"+comment.ID, map[string]any{"body": "Updated comment"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update comment: status=%d body=%s", updated.Status, updated.Body)
	}
	var updatedBody struct {
		Body string `json:"body"`
	}
	updated.JSON(t, &updatedBody)
	if updatedBody.Body != "Updated comment" {
		t.Fatalf("updated body=%q", updatedBody.Body)
	}
	deleted := env.Do(t, testutil.Employee, http.MethodDelete, base+"/"+comment.ID, nil)
	if deleted.Status != http.StatusOK || len(deleted.Body) != 0 {
		t.Fatalf("delete comment: status=%d body=%q", deleted.Status, deleted.Body)
	}
	list := env.Do(t, testutil.Employee, http.MethodGet, base+"?perPage=1", nil)
	if list.Status != http.StatusOK {
		t.Fatalf("list comments: status=%d body=%s", list.Status, list.Body)
	}
	testutil.AssertPagination(t, list, 1, 1, 2)
	var listed []struct {
		ID        string  `json:"id"`
		Body      string  `json:"body"`
		DeletedAt *string `json:"deletedAt"`
	}
	list.JSON(t, &listed)
	if len(listed) != 1 {
		t.Fatalf("page contains %d comments", len(listed))
	}
	gotDeleted := env.Do(t, testutil.Employee, http.MethodGet, base+"/"+comment.ID, nil)
	if gotDeleted.Status != http.StatusOK {
		t.Fatalf("get deleted comment: status=%d body=%s", gotDeleted.Status, gotDeleted.Body)
	}
	var deletedComment struct {
		Body      string  `json:"body"`
		DeletedAt *string `json:"deletedAt"`
	}
	gotDeleted.JSON(t, &deletedComment)
	if deletedComment.Body != "" || deletedComment.DeletedAt == nil {
		t.Fatalf("soft-deleted comment body/deletedAt = %q/%v", deletedComment.Body, deletedComment.DeletedAt)
	}
	env.DeliverWebhooks(t)
	received := env.Receiver.Requests()
	wantEvents := []string{model.WebhookEventCommentCreated, model.WebhookEventCommentCreated, model.WebhookEventCommentUpdated, model.WebhookEventCommentUpdated}
	gotEvents := []string{}
	for _, request := range received {
		var payload struct {
			EventType string `json:"eventType"`
			Data      struct {
				Comment map[string]any `json:"comment"`
				Ticket  map[string]any `json:"ticket"`
			} `json:"data"`
		}
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if payload.EventType == model.WebhookEventCommentCreated || payload.EventType == model.WebhookEventCommentUpdated {
			if payload.Data.Comment == nil || payload.Data.Ticket == nil {
				t.Fatalf("comment event data = %+v", payload.Data)
			}
			gotEvents = append(gotEvents, payload.EventType)
		}
		if payload.EventType == "comment_deleted" {
			t.Fatalf("unexpected deleted-comment event: %s", request.Body)
		}
	}
	if len(gotEvents) != len(wantEvents) {
		t.Fatalf("comment events=%v, want %v", gotEvents, wantEvents)
	}
	counts := map[string]int{}
	for _, event := range gotEvents {
		counts[event]++
	}
	if counts[model.WebhookEventCommentCreated] != 2 || counts[model.WebhookEventCommentUpdated] != 2 {
		t.Fatalf("comment event counts=%v", counts)
	}
}

func TestCommentAuthorAndVisibilityRules(t *testing.T) {
	env := testutil.New(t)
	ticketID := pf23CreateExpenseTicket(t, env, "sales-rep", "sales", "")
	base := "/v1/tickets/" + ticketID + "/comments"
	created := env.Do(t, testutil.Employee, http.MethodPost, base, map[string]any{"body": "author comment"})
	var comment struct {
		ID string `json:"id"`
	}
	created.JSON(t, &comment)
	other := env.Do(t, testutil.Manager, http.MethodPatch, base+"/"+comment.ID, map[string]any{"body": "unauthorized"})
	testutil.AssertError(t, other, http.StatusForbidden, "missing_permission")
	invisibleTicket := pf23CreateExpenseTicket(t, env, "accountant", "finance", "")
	invisible := env.Do(t, testutil.Employee, http.MethodGet, "/v1/tickets/"+invisibleTicket+"/comments", nil)
	testutil.AssertError(t, invisible, http.StatusForbidden, "invisible_ticket")
}

func TestCommentingDisabledValidation(t *testing.T) {
	env := testutil.New(t)
	workflowID, err := store.WorkflowIDByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=json_set(definition_json,'$.commentingEnabled',json('false')) WHERE id=?`, workflowID); err != nil {
		t.Fatal(err)
	}
	ticketID := pf23CreateExpenseTicket(t, env, "sales-rep", "sales", "")
	res := env.Do(t, testutil.Employee, http.MethodPost, "/v1/tickets/"+ticketID+"/comments", map[string]any{"body": "disabled"})
	testutil.AssertError(t, res, http.StatusUnprocessableEntity, "validation_failed")
	var errorBody struct {
		Errors map[string][]string `json:"errors"`
	}
	res.JSON(t, &errorBody)
	if len(errorBody.Errors["base"]) == 0 {
		t.Fatalf("disabled commenting errors=%v; want errors.base", errorBody.Errors)
	}
}

func pf23CreateExpenseTicket(t *testing.T, env *testutil.Env, actorCode, teamCode, signedID string) string {
	t.Helper()
	ctx := context.Background()
	workflow, err := store.GetWorkflowByCode(ctx, env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(ctx, env.DB.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []engine.FieldInput{}
	for _, section := range definition.SectionList {
		if section.SectionType != model.SectionTypeForm {
			continue
		}
		for _, field := range section.FormFields {
			value := any(nil)
			switch field.Code {
			case "amount":
				value = 100
			case "purpose":
				value = "Test expense"
			}
			raw, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			input := engine.FieldInput{FormFieldID: field.ID, FormFieldCode: field.Code, Value: raw}
			if field.FieldType == "file" && signedID != "" {
				input.Files = []string{signedID}
			}
			inputs = append(inputs, input)
		}
	}
	chart, err := store.ActiveOrgChartID(ctx, env.DB.Q())
	if err != nil {
		t.Fatal(err)
	}
	teamID, err := store.TeamIDByCode(ctx, env.DB.Q(), chart, teamCode)
	if err != nil {
		t.Fatal(err)
	}
	engineService := engine.New(env.Clock)
	actorID := env.UserID(t, actorCode)
	var ticketID string
	err = env.DB.WithTx(ctx, func(tx *store.Tx) error {
		var createErr error
		ticketID, _, createErr = engineService.Create(ctx, tx, engine.Actor{UserID: actorID}, engine.CreateInput{
			Status: model.TicketStatusDraft, WorkflowID: workflow.ID, AuthorTeamID: teamID, Inputs: &inputs,
		})
		return createErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return ticketID
}
