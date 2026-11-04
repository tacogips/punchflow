package store

import (
	"context"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
)

func GetProxyApplicant(ctx context.Context, q Querier, id string) (model.ProxyApplicant, error) {
	var x model.ProxyApplicant
	var starts, ends, workflows pf08SQLNullString
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at FROM proxy_applicants WHERE id=?`, id).Scan(&x.ID, &x.UserID, &x.ProxyUserID, &starts, &ends, &workflows, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.StartsOn = starts.ptr()
	x.EndsOn = ends.ptr()
	if workflows.valid {
		if err = json.Unmarshal([]byte(workflows.s), &x.WorkflowIDs); err != nil {
			return x, fmt.Errorf("store: decode proxy workflows: %w", err)
		}
	}
	if x.WorkflowIDs == nil {
		x.WorkflowIDs = []string{}
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func GetProxyApprover(ctx context.Context, q Querier, id string) (model.ProxyApprover, error) {
	var x model.ProxyApprover
	var starts, ends, workflows pf08SQLNullString
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at FROM proxy_approvers WHERE id=?`, id).Scan(&x.ID, &x.UserID, &x.ProxyUserID, &starts, &ends, &workflows, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.StartsOn = starts.ptr()
	x.EndsOn = ends.ptr()
	if workflows.valid {
		if err = json.Unmarshal([]byte(workflows.s), &x.WorkflowIDs); err != nil {
			return x, fmt.Errorf("store: decode proxy workflows: %w", err)
		}
	}
	if x.WorkflowIDs == nil {
		x.WorkflowIDs = []string{}
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
