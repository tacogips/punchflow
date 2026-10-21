package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
	"time"
)

func EngLoadTicket(ctx context.Context, q Querier, id string) (model.Ticket, error) {
	var x model.Ticket
	var title, number, author, team, snapshot, proxy, route sql.NullString
	var opened, completed, denied, archived, closed sql.NullInt64
	var created, updated int64
	err := q.QueryRowContext(ctx, `SELECT id,workflow_id,status,current_step,title,ticket_number,author_id,author_team_id,author_team_snapshot,proxy_client_user_id,forced_public_type,route_id,opened_at,completed_at,denied_at,archived_at,closed_at,created_at,updated_at FROM tickets WHERE id=?`, id).Scan(&x.ID, &x.WorkflowID, &x.Status, &x.CurrentStep, &title, &number, &author, &team, &snapshot, &proxy, &x.ForcedPublicType, &route, &opened, &completed, &denied, &archived, &closed, &created, &updated)
	if err != nil {
		return x, pf14NotFound(err)
	}
	x.Title = pf14StringPtr(title)
	x.TicketNumber = pf14StringPtr(number)
	x.AuthorID = pf14StringPtr(author)
	x.AuthorTeamID = pf14StringPtr(team)
	x.AuthorTeamSnapshot = []byte(snapshot.String)
	x.ProxyClientUserID = pf14StringPtr(proxy)
	x.RouteID = pf14StringPtr(route)
	x.OpenedAt = pf14TimePtr(opened)
	x.CompletedAt = pf14TimePtr(completed)
	x.DeniedAt = pf14TimePtr(denied)
	x.ArchivedAt = pf14TimePtr(archived)
	x.ClosedAt = pf14TimePtr(closed)
	x.CreatedAt = Time(created)
	x.UpdatedAt = Time(updated)
	return x, nil
}
func EngInsertTicket(ctx context.Context, q Querier, x model.Ticket) error {
	_, err := q.ExecContext(ctx, `INSERT INTO tickets(id,workflow_id,status,current_step,title,ticket_number,author_id,author_team_id,author_team_snapshot,proxy_client_user_id,forced_public_type,route_id,opened_at,completed_at,denied_at,archived_at,closed_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.ID, x.WorkflowID, x.Status, x.CurrentStep, x.Title, x.TicketNumber, x.AuthorID, x.AuthorTeamID, pf14Bytes(x.AuthorTeamSnapshot), x.ProxyClientUserID, x.ForcedPublicType, x.RouteID, NullMillis(x.OpenedAt), NullMillis(x.CompletedAt), NullMillis(x.DeniedAt), NullMillis(x.ArchivedAt), NullMillis(x.ClosedAt), Millis(x.CreatedAt), Millis(x.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: insert engine ticket: %w", err)
	}
	return nil
}
func EngUpdateTicket(ctx context.Context, q Querier, x model.Ticket) error {
	_, err := q.ExecContext(ctx, `UPDATE tickets SET status=?,current_step=?,title=?,ticket_number=?,author_id=?,author_team_id=?,author_team_snapshot=?,proxy_client_user_id=?,forced_public_type=?,route_id=?,opened_at=?,completed_at=?,denied_at=?,archived_at=?,closed_at=?,updated_at=? WHERE id=?`, x.Status, x.CurrentStep, x.Title, x.TicketNumber, x.AuthorID, x.AuthorTeamID, pf14Bytes(x.AuthorTeamSnapshot), x.ProxyClientUserID, x.ForcedPublicType, x.RouteID, NullMillis(x.OpenedAt), NullMillis(x.CompletedAt), NullMillis(x.DeniedAt), NullMillis(x.ArchivedAt), NullMillis(x.ClosedAt), Millis(x.UpdatedAt), x.ID)
	if err != nil {
		return fmt.Errorf("store: update engine ticket: %w", err)
	}
	_, err = q.ExecContext(ctx, `UPDATE ticket_assignees SET pending=0,updated_at=? WHERE ticket_id=? AND pending=1 AND NOT EXISTS (SELECT 1 FROM ticket_steps ts WHERE ts.id=ticket_assignees.ticket_step_id AND ts.step_order=? AND ts.status='in_progress' AND ?='in_progress')`, Millis(x.UpdatedAt), x.ID, x.CurrentStep, x.Status)
	if err != nil {
		return fmt.Errorf("store: clear inactive assignee holds: %w", err)
	}
	return nil
}
func EngTicketInputs(ctx context.Context, q Querier, ticketID string) ([]model.TicketInput, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_id,form_field_id,value,ref_ids,reference_snapshot,created_at,updated_at FROM ticket_inputs WHERE ticket_id=? ORDER BY created_at,id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: query engine inputs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketInput{}
	for rows.Next() {
		var x model.TicketInput
		var value, refs, snapshot string
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketID, &x.FormFieldID, &value, &refs, &snapshot, &c, &u); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(value), &x.Value); err != nil {
			return nil, fmt.Errorf("store: decode engine input: %w", err)
		}
		if err = json.Unmarshal([]byte(refs), &x.RefIDs); err != nil {
			return nil, fmt.Errorf("store: decode engine refs: %w", err)
		}
		if err = json.Unmarshal([]byte(snapshot), &x.ReferenceSnapshot); err != nil {
			return nil, fmt.Errorf("store: decode engine reference snapshot: %w", err)
		}
		x.CreatedAt = Time(c)
		x.UpdatedAt = Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}
func EngReplaceInputs(ctx context.Context, q Querier, ticketID string, values []model.TicketInput) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM ticket_inputs WHERE ticket_id=?`, ticketID); err != nil {
		return fmt.Errorf("store: clear engine inputs: %w", err)
	}
	for _, x := range values {
		v, e := json.Marshal(x.Value)
		if e != nil {
			return fmt.Errorf("store: encode engine input: %w", e)
		}
		refs, e := json.Marshal(x.RefIDs)
		if e != nil {
			return fmt.Errorf("store: encode engine refs: %w", e)
		}
		snapshot, e := json.Marshal(x.ReferenceSnapshot)
		if e != nil {
			return fmt.Errorf("store: encode engine reference snapshot: %w", e)
		}
		if _, e = q.ExecContext(ctx, `INSERT INTO ticket_inputs(id,ticket_id,form_field_id,value,ref_ids,reference_snapshot,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, x.ID, ticketID, x.FormFieldID, string(v), string(refs), string(snapshot), Millis(x.CreatedAt), Millis(x.UpdatedAt)); e != nil {
			return fmt.Errorf("store: insert engine input: %w", e)
		}
	}
	return nil
}
func EngReplaceSlipItems(ctx context.Context, q Querier, ticketID string, values []model.TicketSlipItem) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM ticket_slip_items WHERE ticket_id=?`, ticketID); err != nil {
		return fmt.Errorf("store: clear engine slip items: %w", err)
	}
	for _, x := range values {
		v, e := json.Marshal(x.Inputs)
		if e != nil {
			return fmt.Errorf("store: encode engine slip: %w", e)
		}
		snapshots, e := json.Marshal(x.ReferenceSnapshots)
		if e != nil {
			return fmt.Errorf("store: encode engine detail reference snapshots: %w", e)
		}
		if _, e = q.ExecContext(ctx, `INSERT INTO ticket_slip_items(id,ticket_id,slip_section_id,position,inputs,reference_snapshots,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, x.ID, ticketID, x.SlipSectionID, x.Position, string(v), string(snapshots), Millis(x.CreatedAt), Millis(x.UpdatedAt)); e != nil {
			return fmt.Errorf("store: insert engine slip: %w", e)
		}
	}
	return nil
}
func EngTicketSteps(ctx context.Context, q Querier, ticketID string) ([]model.TicketStep, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_id,route_step_id,route_step_code,step_order,display_order,title,action_type,required_approvals_number,required_approvals_percent,status,skip,fallback,fallback_result,completed_at,created_at,updated_at,author_id,instruction FROM ticket_steps WHERE ticket_id=? ORDER BY display_order,id`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("store: query engine steps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketStep{}
	for rows.Next() {
		var x model.TicketStep
		var rid, code, title, fallback, stepAuthor, stepInstruction sql.NullString
		var skip, fb int
		var done sql.NullInt64
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketID, &rid, &code, &x.StepOrder, &x.DisplayOrder, &title, &x.ActionType, &x.RequiredApprovalsNumber, &x.RequiredApprovalsPercent, &x.Status, &skip, &fb, &fallback, &done, &c, &u, &stepAuthor, &stepInstruction); err != nil {
			return nil, err
		}
		x.AuthorID, x.Instruction = pf14StringPtr(stepAuthor), pf14StringPtr(stepInstruction)
		x.RouteStepID = pf14StringPtr(rid)
		x.RouteStepCode = pf14StringPtr(code)
		x.Title = pf14StringPtr(title)
		x.FallbackResult = pf14StringPtr(fallback)
		x.Skip = skip != 0
		x.Fallback = fb != 0
		x.CompletedAt = pf14TimePtr(done)
		x.CreatedAt = Time(c)
		x.UpdatedAt = Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}
func EngDeleteTicketSteps(ctx context.Context, q Querier, ticketID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM ticket_steps WHERE ticket_id=?`, ticketID)
	if err != nil {
		return fmt.Errorf("store: delete engine steps: %w", err)
	}
	return nil
}
func EngInsertStep(ctx context.Context, q Querier, x model.TicketStep) error {
	_, err := q.ExecContext(ctx, `INSERT INTO ticket_steps(id,ticket_id,route_step_id,route_step_code,step_order,display_order,title,action_type,required_approvals_number,required_approvals_percent,status,skip,fallback,fallback_result,completed_at,created_at,updated_at,author_id,instruction) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.ID, x.TicketID, x.RouteStepID, x.RouteStepCode, x.StepOrder, x.DisplayOrder, x.Title, x.ActionType, x.RequiredApprovalsNumber, x.RequiredApprovalsPercent, x.Status, x.Skip, x.Fallback, x.FallbackResult, NullMillis(x.CompletedAt), Millis(x.CreatedAt), Millis(x.UpdatedAt), x.AuthorID, x.Instruction)
	if err != nil {
		return fmt.Errorf("store: insert engine step: %w", err)
	}
	return nil
}
func EngUpdateStep(ctx context.Context, q Querier, x model.TicketStep) error {
	_, err := q.ExecContext(ctx, `UPDATE ticket_steps SET status=?,skip=?,fallback=?,fallback_result=?,completed_at=?,updated_at=? WHERE id=?`, x.Status, x.Skip, x.Fallback, x.FallbackResult, NullMillis(x.CompletedAt), Millis(x.UpdatedAt), x.ID)
	if err != nil {
		return fmt.Errorf("store: update engine step: %w", err)
	}
	return nil
}
func EngStepAssignees(ctx context.Context, q Querier, stepID string) ([]model.TicketAssignee, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_step_id,ticket_id,user_id,completed,completed_at,pending,actual_approver_id,assigned_at,created_at,updated_at FROM ticket_assignees WHERE ticket_step_id=? ORDER BY created_at,id`, stepID)
	if err != nil {
		return nil, fmt.Errorf("store: query engine assignees: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketAssignee{}
	for rows.Next() {
		var x model.TicketAssignee
		var done, pending int
		var completed, actual, assigned sql.NullInt64
		var actualID sql.NullString
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketStepID, &x.TicketID, &x.UserID, &done, &completed, &pending, &actualID, &assigned, &c, &u); err != nil {
			return nil, err
		}
		x.Completed = done != 0
		x.Pending = pending != 0
		x.CompletedAt = pf14TimePtr(completed)
		x.ActualApproverID = pf14StringPtr(actualID)
		x.AssignedAt = pf14TimePtr(assigned)
		x.CreatedAt = Time(c)
		x.UpdatedAt = Time(u)
		_ = actual
		out = append(out, x)
	}
	return out, rows.Err()
}

// EngDeleteStepAssignees replaces the materialized assignment set when a
// team-based route step is entered under the current organization chart.
func EngDeleteStepAssignees(ctx context.Context, q Querier, stepID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM ticket_assignees WHERE ticket_step_id=?`, stepID)
	if err != nil {
		return fmt.Errorf("store: delete step assignees: %w", err)
	}
	return nil
}
func EngInsertAssignee(ctx context.Context, q Querier, x model.TicketAssignee) error {
	_, err := q.ExecContext(ctx, `INSERT INTO ticket_assignees(id,ticket_step_id,ticket_id,user_id,completed,completed_at,pending,actual_approver_id,assigned_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, x.ID, x.TicketStepID, x.TicketID, x.UserID, x.Completed, NullMillis(x.CompletedAt), x.Pending, x.ActualApproverID, NullMillis(x.AssignedAt), Millis(x.CreatedAt), Millis(x.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: insert engine assignee: %w", err)
	}
	return nil
}
func EngUpdateAssignee(ctx context.Context, q Querier, x model.TicketAssignee) error {
	_, err := q.ExecContext(ctx, `UPDATE ticket_assignees SET user_id=?,completed=?,completed_at=?,pending=CASE WHEN user_id<>? THEN 0 ELSE ? END,actual_approver_id=?,assigned_at=?,updated_at=? WHERE id=?`, x.UserID, x.Completed, NullMillis(x.CompletedAt), x.UserID, x.Pending, x.ActualApproverID, NullMillis(x.AssignedAt), Millis(x.UpdatedAt), x.ID)
	if err != nil {
		return fmt.Errorf("store: update engine assignee: %w", err)
	}
	return nil
}
func EngInsertCustomApprovers(ctx context.Context, q Querier, ticketID, code string, ids []string) error {
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO ticket_custom_approvers(ticket_id,route_step_code,user_ids) VALUES(?,?,?) ON CONFLICT(ticket_id,route_step_code) DO UPDATE SET user_ids=excluded.user_ids`, ticketID, code, string(raw))
	if err != nil {
		return fmt.Errorf("store: save custom approvers: %w", err)
	}
	return nil
}
func EngCustomApprovers(ctx context.Context, q Querier, ticketID, code string) ([]string, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT user_ids FROM ticket_custom_approvers WHERE ticket_id=? AND route_step_code=?`, ticketID, code).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: load custom approvers: %w", err)
	}
	var out []string
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("store: decode custom approvers: %w", err)
	}
	return out, nil
}
func EngAddComment(ctx context.Context, q Querier, x model.Comment) error {
	mentioned, err := json.Marshal(x.MentionedUserIDs)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO comments(id,ticket_id,user_id,body,mentioned_user_ids,deleted_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, x.ID, x.TicketID, x.UserID, x.Body, string(mentioned), NullMillis(x.DeletedAt), Millis(x.CreatedAt), Millis(x.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: insert engine comment: %w", err)
	}
	return nil
}
func EngLoadComment(ctx context.Context, q Querier, ticketID, id string) (model.Comment, error) {
	var x model.Comment
	var mentioned string
	var deleted sql.NullInt64
	var c, u int64
	err := q.QueryRowContext(ctx, `SELECT id,ticket_id,user_id,body,mentioned_user_ids,deleted_at,created_at,updated_at FROM comments WHERE ticket_id=? AND id=?`, ticketID, id).Scan(&x.ID, &x.TicketID, &x.UserID, &x.Body, &mentioned, &deleted, &c, &u)
	if err != nil {
		return x, pf14NotFound(err)
	}
	if err = json.Unmarshal([]byte(mentioned), &x.MentionedUserIDs); err != nil {
		return x, fmt.Errorf("store: decode mentioned users: %w", err)
	}
	x.DeletedAt = pf14TimePtr(deleted)
	x.CreatedAt = Time(c)
	x.UpdatedAt = Time(u)
	return x, nil
}
func EngUpdateComment(ctx context.Context, q Querier, x model.Comment) error {
	mentioned, err := json.Marshal(x.MentionedUserIDs)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE comments SET body=?,mentioned_user_ids=?,deleted_at=?,updated_at=? WHERE id=? AND ticket_id=?`, x.Body, string(mentioned), NullMillis(x.DeletedAt), Millis(x.UpdatedAt), x.ID, x.TicketID)
	if err != nil {
		return fmt.Errorf("store: update engine comment: %w", err)
	}
	return nil
}
func EngAttachCommentFiles(ctx context.Context, q Querier, ticketID, commentID string, signedIDs []string, now int64) error {
	for _, signed := range signedIDs {
		var id string
		if err := q.QueryRowContext(ctx, `SELECT id FROM files WHERE signed_id=?`, signed).Scan(&id); err != nil {
			return pf14NotFound(err)
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO file_attachments(file_id,ticket_id,comment_id,created_at) VALUES(?,?,?,?)`, id, ticketID, commentID, now); err != nil {
			return fmt.Errorf("store: attach engine comment file: %w", err)
		}
	}
	return nil
}
func EngLinkTickets(ctx context.Context, q Querier, a, b string, now int64) error {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO ticket_links(ticket_id,linked_ticket_id,created_at) VALUES(?,?,?)`, pair[0], pair[1], now); err != nil {
			return fmt.Errorf("store: link engine tickets: %w", err)
		}
	}
	return nil
}

func pf14StringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
func pf14TimePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := Time(v.Int64)
	return &t
}
func pf14Bytes(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return string(v)
}
func pf14NotFound(err error) error {
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	return fmt.Errorf("store: engine lookup: %w", err)
}
