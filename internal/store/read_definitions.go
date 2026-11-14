package store

import (
	"context"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
)

func GetWorkflow(ctx context.Context, q Querier, id string) (model.Workflow, error) {
	var x model.Workflow
	var counter, cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,code,status,folder_id,definition_json,ticket_number_counter,created_at,updated_at FROM workflows WHERE id=?`, id).Scan(&x.ID, &x.Code, &x.Status, &x.FolderID, &x.DefinitionJSON, &counter, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.TicketNumberCounter = counter
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func WorkflowDefByID(ctx context.Context, q Querier, id string) (model.WorkflowDef, error) {
	x, e := GetWorkflow(ctx, q, id)
	if e != nil {
		return model.WorkflowDef{}, e
	}
	var d model.WorkflowDef
	if e = json.Unmarshal(x.DefinitionJSON, &d); e != nil {
		return d, fmt.Errorf("store: decode workflow definition: %w", e)
	}
	d.ID = x.ID
	d.Code = x.Code
	d.Status = x.Status
	return d, nil
}
func WorkflowIDByCode(ctx context.Context, q Querier, code string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM workflows WHERE code=?`, code).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}
func GetRoute(ctx context.Context, q Querier, id string) (model.Route, error) {
	var x model.Route
	var cr, up int64
	err := q.QueryRowContext(ctx, `SELECT id,code,status,folder_id,definition_json,created_at,updated_at FROM routes WHERE id=?`, id).Scan(&x.ID, &x.Code, &x.Status, &x.FolderID, &x.DefinitionJSON, &cr, &up)
	if err != nil {
		return x, pf08NotFound(err)
	}
	x.CreatedAt = Time(cr)
	x.UpdatedAt = Time(up)
	return x, nil
}
func RouteDefByID(ctx context.Context, q Querier, id string) (model.RouteDef, error) {
	x, e := GetRoute(ctx, q, id)
	if e != nil {
		return model.RouteDef{}, e
	}
	var d model.RouteDef
	if e = json.Unmarshal(x.DefinitionJSON, &d); e != nil {
		return d, fmt.Errorf("store: decode route definition: %w", e)
	}
	d.ID = x.ID
	d.Code = x.Code
	d.Status = x.Status
	return d, nil
}
func RouteIDByCode(ctx context.Context, q Querier, code string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM routes WHERE code=?`, code).Scan(&id)
	if err != nil {
		return "", pf08NotFound(err)
	}
	return id, nil
}
func GetWorkflowByCode(ctx context.Context, q Querier, code string) (model.Workflow, error) {
	id, e := WorkflowIDByCode(ctx, q, code)
	if e != nil {
		return model.Workflow{}, e
	}
	return GetWorkflow(ctx, q, id)
}
func GetRouteByCode(ctx context.Context, q Querier, code string) (model.Route, error) {
	id, e := RouteIDByCode(ctx, q, code)
	if e != nil {
		return model.Route{}, e
	}
	return GetRoute(ctx, q, id)
}
