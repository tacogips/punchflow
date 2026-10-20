package present

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestTicketPresenters(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	def, err := store.WorkflowDefByID(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	def.SectionList[0].FormFields = append(def.SectionList[0].FormFields, model.FormField{ID: "ffgm", Code: "MASTER", Title: "Master", FieldType: "master", GeneralMasterCode: "GM", Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}})
	definition, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.ExecContext(ctx, `UPDATE workflows SET definition_json=? WHERE id='wf'`, string(definition)); err != nil {
		t.Fatal(err)
	}
	now := kftime.ToMillis(time.Date(2026, 2, 3, 4, 5, 6, 123000000, time.UTC))
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO tickets(id,workflow_id,status,current_step,title,ticket_number,author_id,author_team_id,forced_public_type,route_id,created_at,updated_at,opened_at) VALUES('tk','wf','in_progress',1,'Request','WF-1','u1','tm2','follow_workflow','rt',?,?,?)`, []any{now, now, now}},
		{`INSERT INTO ticket_steps(id,ticket_id,route_step_id,route_step_code,step_order,display_order,title,action_type,required_approvals_number,required_approvals_percent,status,skip,fallback,created_at,updated_at) VALUES('st','tk','s1','S1',1,1,'Review','approve',1,100,'in_progress',0,0,?,?)`, []any{now, now}},
		{`INSERT INTO ticket_steps(id,ticket_id,route_step_id,route_step_code,step_order,display_order,title,action_type,required_approvals_number,required_approvals_percent,status,skip,fallback,created_at,updated_at) VALUES('st2','tk','s2','S2',2,2,'Confirm','confirm',1,100,'pending',0,0,?,?)`, []any{now, now}},
		{`INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,created_at,updated_at) VALUES('as','st','tk','u1',0,?,?)`, []any{now, now}},
		{`INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,created_at,updated_at) VALUES('as2','st2','tk','u2',0,?,?)`, []any{now, now}},
		{`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at) VALUES('in','tk','ff','"Hello"','[]',?,?)`, []any{now, now}},
		{`INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at) VALUES('in-master','tk','ffgm','["it"]','["it"]',?,?)`, []any{now, now}},
		{`INSERT INTO ticket_viewers(id,ticket_id,user_id,descendants) VALUES('vw','tk','u2',0)`, nil},
	} {
		if _, err := q.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := TicketSummary(ctx, q, "tk")
	if err != nil {
		t.Fatal(err)
	}
	if summary["publicStatus"] != false || summary["commentsCount"] != 0 {
		t.Fatalf("summary=%v", summary)
	}
	if !regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}\+09:00$`).MatchString(summary["createdAt"].(string)) {
		t.Fatalf("createdAt=%v", summary["createdAt"])
	}
	list, err := TicketWithStep(ctx, q, "tk")
	if err != nil {
		t.Fatal(err)
	}
	step := list["steps"].([]any)[0].(map[string]any)
	if len(list["steps"].([]any)) != 2 {
		t.Fatalf("steps=%v", list["steps"])
	}
	pf08Keys(t, step, "id routeStepId title actionType requiredApprovalsNumber requiredApprovalsPercent stepOrder skip fallback fallbackResult author assignees completed completedAt instruction displayOrder status")
	assignee := step["assignees"].([]any)[0].(map[string]any)
	pf08Keys(t, assignee, "id stepOrder current completed completedAt pending user proxyUserIds")
	if assignee["current"] != true || assignee["actualApprover"] != nil {
		t.Fatalf("assignee=%v", assignee)
	}
	if got := assignee["proxyUserIds"].([]string); len(got) != 1 || got[0] != "u2" {
		t.Fatalf("proxyUserIds=%v", got)
	}
	if _, ok := assignee["actualApprover"]; ok {
		t.Fatal("list assignee contains actualApprover")
	}
	detail, err := TicketDetail(ctx, q, "tk")
	if err != nil {
		t.Fatal(err)
	}
	pf08Keys(t, detail, "id ticketNumber title status currentStep createdAt openedAt completedAt archivedAt updatedAt publicStatus forcedPublicType workflow authorTeamId authorTeamSnapshot commentsCount logsCount linkedTicketsCount closedAt deniedAt useRouteStepCondition author authorTeam proxyClientUser subStatus labels route ticketGuest triggerTicket nextTickets slipItems ticketSections inputs cloudSignDocument steps customNumberingValues")
	for _, key := range []string{"route", "ticketGuest", "triggerTicket", "nextTickets", "slipItems", "ticketSections", "inputs", "cloudSignDocument", "steps", "customNumberingValues"} {
		if _, ok := detail[key]; !ok {
			t.Errorf("detail missing %s", key)
		}
	}
	detailStep := detail["steps"].([]any)[0].(map[string]any)
	pf08Keys(t, detailStep, "id routeStepId title actionType requiredApprovalsNumber requiredApprovalsPercent stepOrder skip fallback fallbackResult author assignees completed completedAt instruction displayOrder status")
	detailAssignee := detailStep["assignees"].([]any)[0].(map[string]any)
	pf08Keys(t, detailAssignee, "id stepOrder current completed completedAt pending user proxyUserIds actualApprover")
	if value, ok := detailAssignee["actualApprover"]; !ok || value != nil {
		t.Fatalf("detail actualApprover=%v present=%v", value, ok)
	}
	input := detail["inputs"].([]any)[0].(map[string]any)
	if input["generalMasterItems"] == nil || input["attachments"] == nil {
		t.Fatalf("input arrays missing: %v", input)
	}
	for _, raw := range detail["inputs"].([]any) {
		in := raw.(map[string]any)
		if in["formFieldId"] == "ffgm" && len(in["generalMasterItems"].([]any)) != 1 {
			t.Fatalf("master items=%v", in["generalMasterItems"])
		}
	}
	viewers, err := TicketViewers(ctx, q, []string{"tk"})
	if err != nil || len(viewers) != 1 {
		t.Fatalf("viewers=%v err=%v", viewers, err)
	}
	tenant, err := WebhookTenant(ctx, q)
	if err != nil || tenant["publicId"] != "pub" {
		t.Fatalf("tenant=%v err=%v", tenant, err)
	}
}
