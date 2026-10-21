package webhook

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestPayloadsBuildSupportedEventsAndEscaping(t *testing.T) {
	db, ctx := pf09TestDB(t)
	q := db.Q()
	now := kftime.ToMillis(time.Date(2026, 4, 5, 6, 7, 8, 123000000, time.UTC))
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenant(id,public_id,name,plan,mode,created_at,updated_at) VALUES('tenant','public','Tenant','standard','normal',?,?)`, []any{now, now}},
		{`INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('actor','A','a@example.test','Ada','Lovelace','activated','en','normal','{}',?,?)`, []any{now, now}},
		{`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('folder','F','Folder',?,?)`, []any{now, now}},
		{`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('workflow','W','visible','folder',?,?,?)`, []any{`{"id":"workflow","code":"W","name":"Workflow","titleInputMode":"manual","sectionList":[],"ticketViewers":[]}`, now, now}},
		{`INSERT INTO tickets(id,workflow_id,status,current_step,title,forced_public_type,created_at,updated_at) VALUES('ticket','workflow','in_progress',0,'<b>','forced_private',?,?)`, []any{now, now}},
		{`INSERT INTO comments(id,ticket_id,user_id,body,mentioned_user_ids,created_at,updated_at) VALUES('comment','ticket','actor','hello','[]',?,?)`, []any{now, now}},
	} {
		if _, err := q.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	builder := NewPayloads()
	for _, eventType := range model.WebhookEventTypes {
		t.Run(eventType, func(t *testing.T) {
			ev := model.WebhookEvent{Type: eventType, TicketID: "ticket", CommentID: "comment", ActorUserID: "actor"}
			body, err := builder.Build(ctx, q, ev)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "<b>") {
				t.Fatalf("payload contains raw HTML: %s", body)
			}
			if eventType == model.WebhookEventTicketCreated && !strings.Contains(string(body), `\u003cb\u003e`) {
				t.Fatalf("payload lacks escaped HTML sequence: %s", body)
			}
			var payload map[string]any
			if err = json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["eventType"] != eventType {
				t.Fatalf("eventType=%v", payload["eventType"])
			}
			data := payload["data"].(map[string]any)
			if strings.HasPrefix(eventType, "ticket_") {
				if data["ticket"].(map[string]any)["id"] != "ticket" {
					t.Fatalf("ticket data=%v", data)
				}
			} else if strings.HasPrefix(eventType, "comment_") {
				if data["comment"].(map[string]any)["id"] != "comment" || data["ticket"].(map[string]any)["steps"] == nil {
					t.Fatalf("comment data=%v", data)
				}
			} else if data["message"] != "ping" {
				t.Fatalf("ping data=%v", data)
			}
			ts := payload["user"].(map[string]any)["createdAt"].(string)
			if !regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}\+09:00$`).MatchString(ts) {
				t.Fatalf("timestamp=%s", ts)
			}
		})
	}
	for _, removed := range []string{"ticket_submitted", "comment_deleted", "future_event"} {
		body, err := builder.Build(ctx, q, model.WebhookEvent{Type: removed, TicketID: "ticket", CommentID: "comment", ActorUserID: "actor"})
		if err == nil {
			t.Errorf("Build(%q) accepted removed event", removed)
			continue
		}
		if body != nil || !strings.Contains(err.Error(), "unsupported event type") {
			t.Errorf("Build(%q) = (%q, %v), want (nil, unsupported event type error)", removed, body, err)
		}
	}
}

func TestPayloadsRedactRestrictedSectionValuesForTicketEvents(t *testing.T) {
	db, ctx := pf09TestDB(t)
	q := db.Q()
	now := kftime.ToMillis(time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC))
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tenant(id,public_id,name,plan,mode,created_at,updated_at) VALUES('tenant','public','Tenant','standard','normal',?,?)`, []any{now, now}},
		{`INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('actor','A','a@example.test','Ada','Lovelace','activated','en','normal','{}',?,?)`, []any{now, now}},
		{`INSERT INTO folders(id,code,name,created_at,updated_at) VALUES('folder','F','Folder',?,?)`, []any{now, now}},
		{`INSERT INTO workflows(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('workflow','W','visible','folder',?,?,?)`, []any{`{"id":"workflow","code":"W","name":"Workflow","titleInputMode":"manual","sectionList":[],"ticketViewers":[]}`, now, now}},
		{`INSERT INTO tickets(id,workflow_id,status,current_step,title,ticket_number,forced_public_type,created_at,updated_at) VALUES('ticket','workflow','in_progress',0,'Request','W-1','forced_private',?,?)`, []any{now, now}},
		{`INSERT INTO files(id,signed_id,filename,content_type,byte_size,checksum,storage_name,uploaded_by,created_at) VALUES('file','signed-file','classified.pdf','application/pdf',12,'sum','blob','actor',?)`, []any{now}},
	} {
		if _, err := q.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	def, err := store.WorkflowDefByID(ctx, q, "workflow")
	if err != nil {
		t.Fatal(err)
	}
	def.SectionList = []model.WorkflowSection{{
		ID: "restricted", SectionType: model.SectionTypeForm, ViewRestriction: true,
		FormFields: []model.FormField{
			{ID: "secret", Code: "secret_code", Title: "Secret", FieldType: "text"},
			{ID: "secret-user", Code: "secret_user", Title: "Secret user", FieldType: "user"},
			{ID: "secret-file", Code: "secret_file", Title: "Secret file", FieldType: "file"},
		},
	}}
	encoded, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.ExecContext(ctx, `UPDATE workflows SET definition_json=? WHERE id='workflow'`, string(encoded)); err != nil {
		t.Fatal(err)
	}
	if _, err = q.ExecContext(ctx, `INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at) VALUES('secret-input','ticket','secret','"classified-value"','[]',1,1)`); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ id, field, value, refs string }{
		{"secret-user-input", "secret-user", `"actor"`, `["actor"]`},
		{"secret-file-input", "secret-file", `["signed-file"]`, `["signed-file"]`},
	} {
		if _, err = q.ExecContext(ctx, `INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at) VALUES(?, 'ticket', ?, ?, ?, 1, 1)`, input.id, input.field, input.value, input.refs); err != nil {
			t.Fatal(err)
		}
	}
	for _, eventType := range []string{model.WebhookEventTicketCreated, model.WebhookEventTicketUpdated, model.WebhookEventTicketApproved, model.WebhookEventTicketCompleted} {
		t.Run(eventType, func(t *testing.T) {
			body, buildErr := NewPayloads().Build(ctx, q, model.WebhookEvent{Type: eventType, TicketID: "ticket", ActorUserID: "actor"})
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			if strings.Contains(string(body), "classified-value") || strings.Contains(string(body), "classified.pdf") {
				t.Fatalf("restricted value leaked: %s", body)
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			ticket := payload["data"].(map[string]any)["ticket"].(map[string]any)
			inputs := ticket["inputs"].([]any)
			if len(inputs) != 3 {
				t.Fatalf("restricted input definition missing: %v", inputs)
			}
			for _, raw := range inputs {
				input := raw.(map[string]any)
				if input["value"] != nil {
					t.Fatalf("restricted input value was not redacted: %v", input)
				}
				if input["formField"].(map[string]any)["code"] == nil {
					t.Fatalf("restricted field definition was not retained: %v", input)
				}
				for _, key := range []string{"generalMasterItems", "users", "teams", "inputTickets", "attachments"} {
					if len(input[key].([]any)) != 0 {
						t.Fatalf("redacted input %s=%v", key, input[key])
					}
				}
			}
		})
	}
}
