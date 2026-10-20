package present

import (
	"context"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func UserFromModel(u model.User, o UserOpts) UserJSON {
	full := u.LastName + " " + u.FirstName
	image := map[string]any{"100x100": nil, "64x64": nil, "32x32": nil}
	out := UserJSON{"id": u.ID, "email": u.Email, "code": u.Code, "firstName": u.FirstName, "lastName": u.LastName, "fullName": full, "employeeId": u.EmployeeID, "image": image, "status": u.Status, "locale": u.Locale, "createdAt": pf08Time(u.CreatedAt), "updatedAt": pf08Time(u.UpdatedAt)}
	if o.LastUsedOn {
		out["lastUsedOn"] = u.LastUsedOn
	}
	if o.CustomFields && len(u.CustomFields) > 0 {
		out["customFields"] = pf08CustomFields(u.CustomFields)
	}
	if o.UserType {
		out["userType"] = u.UserType
	}
	return out
}
func pf08CustomFields(fields map[string]any) []any {
	out := []any{}
	for k, v := range fields {
		out = append(out, map[string]any{"code": k, "value": v, "fieldType": nil})
	}
	return out
}
func User(ctx context.Context, q store.Querier, id string, o UserOpts) (UserJSON, error) {
	u, e := store.GetUser(ctx, q, id)
	if e != nil {
		return nil, e
	}
	return UserFromModel(u, o), nil
}
func Users(ctx context.Context, q store.Querier, ids []string, o UserOpts) ([]UserJSON, error) {
	out := make([]UserJSON, 0, len(ids))
	xs, e := store.GetUsers(ctx, q, ids)
	if e != nil {
		return nil, e
	}
	by := map[string]model.User{}
	for _, u := range xs {
		by[u.ID] = u
	}
	for _, id := range ids {
		if u, ok := by[id]; ok {
			out = append(out, UserFromModel(u, o))
		}
	}
	return out, nil
}
func UserDetail(ctx context.Context, q store.Querier, id string, o UserOpts) (UserDetailJSON, error) {
	u, e := User(ctx, q, id, o)
	if e != nil {
		return nil, e
	}
	invite, ie := store.GetUserInvitation(ctx, q, id)
	var inv any
	if ie == nil {
		inv = map[string]any{"id": invite.ID, "expiresAt": pf08Time(invite.ExpiresAt), "userEmail": invite.UserEmail, "userLocale": invite.UserLocale, "createdAt": pf08Time(invite.CreatedAt), "updatedAt": pf08Time(invite.UpdatedAt)}
	} else if ie != store.ErrNotFound {
		return nil, ie
	}
	out := UserDetailJSON(u)
	out["userLineWorksAccount"] = nil
	out["invitation"] = inv
	return out, nil
}
