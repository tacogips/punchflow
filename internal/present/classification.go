package present

import (
	"context"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func Category(m model.Category) CategoryJSON {
	return CategoryJSON{"id": m.ID, "code": m.Code, "name": m.Name, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}
}
func Folder(ctx context.Context, q store.Querier, id string) (FolderJSON, error) {
	m, e := store.GetFolder(ctx, q, id)
	if e != nil {
		return nil, e
	}
	full, e := store.FolderFullName(ctx, q, id)
	if e != nil {
		return nil, e
	}
	wc, e := store.CountFolderWorkflows(ctx, q, id)
	if e != nil {
		return nil, e
	}
	rc, e := store.CountFolderRoutes(ctx, q, id)
	if e != nil {
		return nil, e
	}
	return FolderJSON{"id": m.ID, "name": m.Name, "fullName": full, "code": m.Code, "workflowsCount": wc, "routesCount": rc, "pipelinesCount": 0, "editable": m.Editable, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt), "description": m.Description}, nil
}
func FolderDetail(ctx context.Context, q store.Querier, id string) (FolderDetailJSON, error) {
	base, e := Folder(ctx, q, id)
	if e != nil {
		return nil, e
	}
	anc, e := store.GetFolderAncestors(ctx, q, id)
	if e != nil {
		return nil, e
	}
	a := []any{}
	for _, x := range anc {
		j, er := Folder(ctx, q, x.ID)
		if er != nil {
			return nil, er
		}
		a = append(a, j)
	}
	out := FolderDetailJSON(base)
	children, e := store.GetFolderChildren(ctx, q, id)
	if e != nil {
		return nil, e
	}
	c := []any{}
	for _, x := range children {
		j, er := Folder(ctx, q, x.ID)
		if er != nil {
			return nil, er
		}
		c = append(c, j)
	}
	out["ancestors"] = a
	out["children"] = c
	return out, nil
}
