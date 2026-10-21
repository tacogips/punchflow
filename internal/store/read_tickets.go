package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"punchflow/internal/model"
)

func Ticket(ctx context.Context, q Querier, id string) (model.Ticket, error) {
	var x model.Ticket
	var title, number, author, team, snapshot, proxy, route sql.NullString
	var opened, completed, denied, archived, closed sql.NullInt64
	var created, updated int64
	err := q.QueryRowContext(ctx, `SELECT id,workflow_id,status,current_step,title,ticket_number,author_id,author_team_id,author_team_snapshot,proxy_client_user_id,forced_public_type,route_id,opened_at,completed_at,denied_at,archived_at,closed_at,created_at,updated_at FROM tickets WHERE id=?`, id).Scan(&x.ID, &x.WorkflowID, &x.Status, &x.CurrentStep, &title, &number, &author, &team, &snapshot, &proxy, &x.ForcedPublicType, &route, &opened, &completed, &denied, &archived, &closed, &created, &updated)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.Title, x.TicketNumber, x.AuthorID, x.AuthorTeamID = pf15StringPtr(title), pf15StringPtr(number), pf15StringPtr(author), pf15StringPtr(team)
	x.AuthorTeamSnapshot = []byte(snapshot.String)
	x.ProxyClientUserID, x.RouteID = pf15StringPtr(proxy), pf15StringPtr(route)
	x.OpenedAt, x.CompletedAt, x.DeniedAt, x.ArchivedAt, x.ClosedAt = pf15TimePtr(opened), pf15TimePtr(completed), pf15TimePtr(denied), pf15TimePtr(archived), pf15TimePtr(closed)
	x.CreatedAt, x.UpdatedAt = Time(created), Time(updated)
	return x, nil
}

func Tickets(ctx context.Context, q Querier, ids []string) ([]model.Ticket, error) {
	out := make([]model.Ticket, 0, len(ids))
	for _, id := range ids {
		x, err := Ticket(ctx, q, id)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func TicketCounts(ctx context.Context, q Querier, id string) (comments, logs, links int, err error) {
	err = q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM comments WHERE ticket_id=? AND deleted_at IS NULL),(SELECT count(*) FROM audit_logs WHERE ticket_id=?),(SELECT count(*) FROM ticket_links WHERE ticket_id=? OR linked_ticket_id=?)`, id, id, id, id).Scan(&comments, &logs, &links)
	if err != nil {
		err = fmt.Errorf("store: count ticket relations: %w", err)
	}
	return
}

func TicketViewers(ctx context.Context, q Querier, id string) ([]model.TicketViewer, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_id,user_id,team_id,grade_id,descendants FROM ticket_viewers WHERE ticket_id=? ORDER BY rowid`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query ticket viewers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketViewer{}
	for rows.Next() {
		var x model.TicketViewer
		var u, t, g sql.NullString
		var d int
		if err = rows.Scan(&x.ID, &x.TicketID, &u, &t, &g, &d); err != nil {
			return nil, err
		}
		x.UserID, x.TeamID, x.GradeID = pf15StringPtr(u), pf15StringPtr(t), pf15StringPtr(g)
		x.Descendants = d != 0
		out = append(out, x)
	}
	return out, rows.Err()
}

func TicketInputs(ctx context.Context, q Querier, id string) ([]model.TicketInput, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_id,form_field_id,value,ref_ids,created_at,updated_at FROM ticket_inputs WHERE ticket_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query ticket inputs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketInput{}
	for rows.Next() {
		var x model.TicketInput
		var value, refs string
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketID, &x.FormFieldID, &value, &refs, &c, &u); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(value), &x.Value); err != nil {
			return nil, fmt.Errorf("store: decode ticket input: %w", err)
		}
		if err = json.Unmarshal([]byte(refs), &x.RefIDs); err != nil {
			return nil, fmt.Errorf("store: decode ticket input refs: %w", err)
		}
		x.CreatedAt, x.UpdatedAt = Time(c), Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}

func TicketSlipItems(ctx context.Context, q Querier, id string) ([]model.TicketSlipItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_id,slip_section_id,position,inputs,created_at,updated_at FROM ticket_slip_items WHERE ticket_id=? ORDER BY position,id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query slip items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketSlipItem{}
	for rows.Next() {
		var x model.TicketSlipItem
		var inputs string
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketID, &x.SlipSectionID, &x.Position, &inputs, &c, &u); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(inputs), &x.Inputs); err != nil {
			return nil, fmt.Errorf("store: decode slip item: %w", err)
		}
		x.CreatedAt, x.UpdatedAt = Time(c), Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}

func TicketSteps(ctx context.Context, q Querier, id string) ([]model.TicketStep, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_id,route_step_id,route_step_code,step_order,display_order,title,action_type,required_approvals_number,required_approvals_percent,status,skip,fallback,fallback_result,completed_at,created_at,updated_at,author_id,instruction FROM ticket_steps WHERE ticket_id=? ORDER BY display_order,step_order,id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: query ticket steps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketStep{}
	for rows.Next() {
		var x model.TicketStep
		var rid, code, title, result, stepAuthor, stepInstruction sql.NullString
		var skip, fallback int
		var done sql.NullInt64
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketID, &rid, &code, &x.StepOrder, &x.DisplayOrder, &title, &x.ActionType, &x.RequiredApprovalsNumber, &x.RequiredApprovalsPercent, &x.Status, &skip, &fallback, &result, &done, &c, &u, &stepAuthor, &stepInstruction); err != nil {
			return nil, err
		}
		x.AuthorID, x.Instruction = pf15StringPtr(stepAuthor), pf15StringPtr(stepInstruction)
		x.RouteStepID, x.RouteStepCode, x.Title = pf15StringPtr(rid), pf15StringPtr(code), pf15StringPtr(title)
		x.Skip, x.Fallback = skip != 0, fallback != 0
		x.FallbackResult = pf15StringPtr(result)
		x.CompletedAt = pf15TimePtr(done)
		x.CreatedAt, x.UpdatedAt = Time(c), Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}

func TicketAssignees(ctx context.Context, q Querier, stepID string) ([]model.TicketAssignee, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,ticket_step_id,ticket_id,user_id,completed,completed_at,actual_approver_id,assigned_at,created_at,updated_at FROM ticket_assignees WHERE ticket_step_id=? ORDER BY created_at,id`, stepID)
	if err != nil {
		return nil, fmt.Errorf("store: query ticket assignees: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.TicketAssignee{}
	for rows.Next() {
		var x model.TicketAssignee
		var done, assigned sql.NullInt64
		var actual sql.NullString
		var completed int
		var c, u int64
		if err = rows.Scan(&x.ID, &x.TicketStepID, &x.TicketID, &x.UserID, &completed, &done, &actual, &assigned, &c, &u); err != nil {
			return nil, err
		}
		x.Completed = completed != 0
		x.CompletedAt, x.AssignedAt = pf15TimePtr(done), pf15TimePtr(assigned)
		x.ActualApproverID = pf15StringPtr(actual)
		x.CreatedAt, x.UpdatedAt = Time(c), Time(u)
		out = append(out, x)
	}
	return out, rows.Err()
}

func Comment(ctx context.Context, q Querier, id string) (model.Comment, error) {
	var x model.Comment
	var mentions string
	var deleted sql.NullInt64
	var c, u int64
	err := q.QueryRowContext(ctx, `SELECT id,ticket_id,user_id,body,mentioned_user_ids,deleted_at,created_at,updated_at FROM comments WHERE id=?`, id).Scan(&x.ID, &x.TicketID, &x.UserID, &x.Body, &mentions, &deleted, &c, &u)
	if err != nil {
		return x, pf08NotFound(err)
	}
	if err = json.Unmarshal([]byte(mentions), &x.MentionedUserIDs); err != nil {
		return x, fmt.Errorf("store: decode comment mentions: %w", err)
	}
	if x.MentionedUserIDs == nil {
		x.MentionedUserIDs = []string{}
	}
	x.DeletedAt = pf15TimePtr(deleted)
	x.CreatedAt, x.UpdatedAt = Time(c), Time(u)
	return x, nil
}

func Comments(ctx context.Context, q Querier, ids []string) ([]model.Comment, error) {
	out := make([]model.Comment, 0, len(ids))
	for _, id := range ids {
		x, e := Comment(ctx, q, id)
		if e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, nil
}

func TicketAttachments(ctx context.Context, q Querier, ticketID string, commentID *string, fieldID *string) ([]model.File, error) {
	query := `SELECT f.id,f.signed_id,f.filename,f.content_type,f.byte_size,f.checksum,f.storage_name,f.uploaded_by,f.created_at FROM files f JOIN file_attachments a ON a.file_id=f.id WHERE a.ticket_id=?`
	args := []any{ticketID}
	if commentID != nil {
		query += ` AND a.comment_id=?`
		args = append(args, *commentID)
	}
	if fieldID != nil {
		query += ` AND a.form_field_id=?`
		args = append(args, *fieldID)
	}
	query += ` ORDER BY a.created_at,f.id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query ticket attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.File{}
	for rows.Next() {
		var x model.File
		var uploaded sql.NullString
		var created int64
		if err = rows.Scan(&x.ID, &x.SignedID, &x.Filename, &x.ContentType, &x.ByteSize, &x.Checksum, &x.StorageName, &uploaded, &created); err != nil {
			return nil, err
		}
		x.UploadedBy = pf15StringPtr(uploaded)
		x.CreatedAt = Time(created)
		out = append(out, x)
	}
	return out, rows.Err()
}

func ProxyUserIDs(ctx context.Context, q Querier, userID, workflowID, today string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT proxy_user_id FROM proxy_approvers WHERE user_id=? AND (starts_on IS NULL OR starts_on<=?) AND (ends_on IS NULL OR ends_on>=?) AND (workflow_ids='[]' OR workflow_ids LIKE ?) ORDER BY created_at,proxy_user_id`, userID, today, today, "%\""+workflowID+"\"%")
	if err != nil {
		return nil, fmt.Errorf("store: query proxy approvers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func pf15TimePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := Time(v.Int64)
	return &t
}
func pf15StringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
