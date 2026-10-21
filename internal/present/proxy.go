package present

import (
	"context"
	"punchflow/internal/store"
)

func ProxyApplicant(ctx context.Context, q store.Querier, id string) (ProxyApplicantJSON, error) {
	m, e := store.GetProxyApplicant(ctx, q, id)
	if e != nil {
		return nil, e
	}
	u, e := User(ctx, q, m.UserID, UserOpts{})
	if e != nil {
		return nil, e
	}
	pu, e := User(ctx, q, m.ProxyUserID, UserOpts{})
	if e != nil {
		return nil, e
	}
	workflows := []any{}
	for _, wid := range m.WorkflowIDs {
		w, er := Workflow(ctx, q, wid)
		if er == nil {
			workflows = append(workflows, w)
		} else if er != store.ErrNotFound {
			return nil, er
		}
	}
	return ProxyApplicantJSON{"id": m.ID, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "user": u, "proxyUser": pu, "startsOn": m.StartsOn, "endsOn": m.EndsOn, "workflows": workflows}, nil
}
func ProxyApprover(ctx context.Context, q store.Querier, id string) (ProxyApproverJSON, error) {
	m, e := store.GetProxyApprover(ctx, q, id)
	if e != nil {
		return nil, e
	}
	u, e := User(ctx, q, m.UserID, UserOpts{})
	if e != nil {
		return nil, e
	}
	pu, e := User(ctx, q, m.ProxyUserID, UserOpts{})
	if e != nil {
		return nil, e
	}
	workflows := []any{}
	for _, wid := range m.WorkflowIDs {
		w, er := Workflow(ctx, q, wid)
		if er == nil {
			workflows = append(workflows, w)
		} else if er != store.ErrNotFound {
			return nil, er
		}
	}
	return ProxyApproverJSON{"id": m.ID, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "user": u, "proxyUser": pu, "startsOn": m.StartsOn, "endsOn": m.EndsOn, "workflows": workflows}, nil
}
