package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

type pf18InviteMembership struct {
	TeamCode   string   `json:"teamCode"`
	GradeCodes []string `json:"gradeCodes"`
	Leader     bool     `json:"leader"`
}

type pf18CustomField struct {
	Code  string `json:"code"`
	Value any    `json:"value"`
}

type pf18CreateUserBody struct {
	Email        string                 `json:"email"`
	Code         string                 `json:"code"`
	FirstName    string                 `json:"firstName"`
	LastName     string                 `json:"lastName"`
	EmployeeID   *string                `json:"employeeId"`
	Memberships  []pf18InviteMembership `json:"memberships"`
	CustomFields []pf18CustomField      `json:"customFields"`
	UserType     string                 `json:"userType"`
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	statuses := r.URL.Query()["status[]"]
	if len(statuses) == 0 {
		statuses = r.URL.Query()["status"]
	}
	for _, status := range statuses {
		if !pf18Contains(model.UserStatuses, status) {
			kferr.Write(w, kferr.InvalidParameter("Invalid status"))
			return
		}
	}
	sort := httpx.ParseSort(r, "email", "code", "lastUsedOn")
	ids, total, err := store.UserList(r.Context(), a.d.DB.Q(), store.UserListFilter{Statuses: statuses}, page.Offset(), page.PerPage, sort.Field, sort.Desc)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	items, err := present.Users(r.Context(), a.d.DB.Q(), ids, present.UserOpts{LastUsedOn: true, CustomFields: true, AllCustomFields: true})
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	var body pf18CreateUserBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.Email == "" || body.FirstName == "" || body.LastName == "" {
		kferr.Write(w, kferr.InvalidParameter("Missing required user fields"))
		return
	}
	if !pf08AddressOnlyEmail(body.Email) {
		kferr.Write(w, kferr.InvalidParameter("Invalid email"))
		return
	}
	if len(body.Email) > 254 || len([]rune(body.FirstName)) > 255 || len([]rune(body.LastName)) > 255 || (body.EmployeeID != nil && len([]rune(*body.EmployeeID)) > 30) || len(body.Memberships) > 5 {
		kferr.Write(w, kferr.InvalidParameter("Invalid user fields"))
		return
	}
	if body.Code == "" {
		body.Code = strings.ReplaceAll(store.NewID(), "-", "")[:16]
	}
	if len([]rune(body.Code)) > 100 {
		kferr.Write(w, kferr.InvalidParameter("Invalid code"))
		return
	}
	if body.UserType == "" {
		body.UserType = model.UserTypeNormal
	}
	if !pf18Contains(model.UserTypes, body.UserType) {
		kferr.Write(w, kferr.InvalidParameter("Invalid userType"))
		return
	}
	allowed, err := store.UserEmailAllowed(r.Context(), a.d.DB.Q(), body.Email)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	if !allowed {
		kferr.Write(w, kferr.DomainNotAllowed())
		return
	}
	custom, err := pf18ApplyCustomFields(r.Context(), a.d.DB.Q(), nil, body.CustomFields)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	now := a.d.Clock.Now()
	user := model.User{ID: store.NewID(), Code: body.Code, Email: body.Email, FirstName: body.FirstName, LastName: body.LastName, EmployeeID: body.EmployeeID, Status: model.UserStatusInvited, Locale: "en", UserType: body.UserType, CustomFields: custom, CreatedAt: now, UpdatedAt: now}
	principal, _ := auth.FromContext(r.Context())
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.UserCreate(r.Context(), tx, user); err != nil {
			return err
		}
		inv := model.UserInvitation{ID: store.NewID(), UserID: user.ID, UserEmail: user.Email, UserLocale: user.Locale, ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}
		if err := store.UserSetInvitation(r.Context(), tx, inv); err != nil {
			return err
		}
		if len(body.Memberships) > 0 {
			chart, err := store.ActiveOrgChartID(r.Context(), tx)
			if err != nil {
				return err
			}
			for _, membership := range body.Memberships {
				teamID, err := store.TeamIDByCode(r.Context(), tx, chart, membership.TeamCode)
				if err != nil {
					return kferr.ValidationFailed(map[string][]string{"memberships": {"contains an unknown teamCode"}})
				}
				gradeIDs := make([]string, 0, len(membership.GradeCodes))
				for _, code := range membership.GradeCodes {
					id, err := store.GradeIDByCode(r.Context(), tx, code)
					if err != nil {
						return kferr.ValidationFailed(map[string][]string{"memberships": {"contains an unknown gradeCode"}})
					}
					gradeIDs = append(gradeIDs, id)
				}
				if err := store.UserAddTeamMembership(r.Context(), tx, model.TeamMembership{ID: store.NewID(), TeamID: teamID, UserID: user.ID, Leader: membership.Leader, GradeIDs: gradeIDs, CreatedAt: now, UpdatedAt: now}); err != nil {
					return err
				}
			}
		}
		return pf18Audit(r, tx, principal.User.ID, "user.create", user.ID, []string{"email", "code", "firstName", "lastName"})
	})
	if err != nil {
		if pf18Unique(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"email": {"has already been taken"}, "code": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	out, err := pf18UserDetail(r.Context(), a.d.DB.Q(), user.ID, true)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	a.pf18TransitionUser(w, r, "delete")
}
func (a *API) reinviteUser(w http.ResponseWriter, r *http.Request) {
	a.pf18TransitionUser(w, r, "reinvite")
}
func (a *API) suspendUser(w http.ResponseWriter, r *http.Request) {
	a.pf18TransitionUser(w, r, "suspend")
}
func (a *API) reactivateUser(w http.ResponseWriter, r *http.Request) {
	a.pf18TransitionUser(w, r, "reactivate")
}

func (a *API) pf18TransitionUser(w http.ResponseWriter, r *http.Request, action string) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "users", r.PathValue("userId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	p, _ := auth.FromContext(r.Context())
	now := a.d.Clock.Now()
	hasEvents := false
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		u, err := store.GetUser(r.Context(), tx, id)
		if err != nil {
			return kferr.ResourceNotFound()
		}
		status, audit := u.Status, "user."+action
		var deactivated *time.Time
		switch action {
		case "delete":
			if u.Status == model.UserStatusDeactivated {
				return kferr.InvalidState("User is already deactivated")
			}
			status, deactivated = model.UserStatusDeactivated, &now
		case "reinvite":
			if u.Status != model.UserStatusDeactivated && u.Status != model.UserStatusInvited {
				return kferr.InvalidState("User cannot be reinvited")
			}
			status = model.UserStatusInvited
			if err := store.UserSetInvitation(r.Context(), tx, model.UserInvitation{ID: store.NewID(), UserID: u.ID, UserEmail: u.Email, UserLocale: u.Locale, ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		case "suspend":
			if u.Status != model.UserStatusActive {
				return kferr.InvalidState("Only activated users can be suspended")
			}
			status = model.UserStatusSuspended
		case "reactivate":
			if u.Status != model.UserStatusSuspended {
				return kferr.InvalidState("Only suspended users can be reactivated")
			}
			status = model.UserStatusActive
		}
		if err := store.UserSetStatus(r.Context(), tx, id, status, deactivated, store.Millis(now)); err != nil {
			return err
		}
		if action == "delete" {
			// Deletion (unlike suspension) archives the user's open tickets and
			// drops their outstanding approver assignments.
			events, err := a.d.Engine.CleanupDeletedUser(r.Context(), tx, actorFor(p), id)
			if err != nil {
				return err
			}
			if err = a.d.Events.Enqueue(r.Context(), tx, events); err != nil {
				return err
			}
			hasEvents = len(events) > 0
		}
		return pf18Audit(r, tx, p.User.ID, audit, id, []string{"status"})
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if hasEvents {
		a.d.Events.Notify()
	}
	if action == "suspend" || action == "delete" || action == "reactivate" || action == "reinvite" {
		out, err := pf18UserDetail(r.Context(), a.d.DB.Q(), id, true)
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	id, err := store.ResolveID(r.Context(), a.d.DB.Q(), "users", r.PathValue("userId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	var raw map[string]json.RawMessage
	if err := httpx.DecodeJSON(r, &raw); err != nil {
		kferr.Write(w, err)
		return
	}
	patch := store.UserPatch{}
	var fields []string
	for key, target := range map[string]**string{"code": &patch.Code, "email": &patch.Email, "firstName": &patch.FirstName, "lastName": &patch.LastName} {
		if value, ok := raw[key]; ok {
			var s string
			if json.Unmarshal(value, &s) != nil {
				kferr.Write(w, kferr.InvalidParameter("Invalid "+key))
				return
			}
			*target = &s
			fields = append(fields, key)
		}
	}
	if value, ok := raw["employeeId"]; ok {
		var v *string
		if string(value) != "null" && json.Unmarshal(value, &v) != nil {
			kferr.Write(w, kferr.InvalidParameter("Invalid employeeId"))
			return
		}
		patch.EmployeeID = &v
		fields = append(fields, "employeeId")
	}
	if value, ok := raw["customFields"]; ok {
		var list []pf18CustomField
		if json.Unmarshal(value, &list) != nil {
			kferr.Write(w, kferr.InvalidParameter("Invalid customFields"))
			return
		}
		current, e := store.GetUser(r.Context(), a.d.DB.Q(), id)
		if e != nil {
			kferr.Write(w, kferr.ResourceNotFound())
			return
		}
		// Entries are merged over the stored values; a null value clears a field.
		patch.CustomFields, e = pf18ApplyCustomFields(r.Context(), a.d.DB.Q(), current.CustomFields, list)
		if e != nil {
			kferr.Write(w, e)
			return
		}
		fields = append(fields, "customFields")
	}
	if len(fields) == 0 {
		kferr.Write(w, kferr.InvalidParameter("Empty update"))
		return
	}
	if patch.Email != nil {
		if len(*patch.Email) > 254 {
			kferr.Write(w, kferr.InvalidParameter("Invalid email"))
			return
		}
		if !pf08AddressOnlyEmail(*patch.Email) {
			kferr.Write(w, kferr.InvalidParameter("Invalid email"))
			return
		}
		allowed, err := store.UserEmailAllowed(r.Context(), a.d.DB.Q(), *patch.Email)
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		if !allowed {
			kferr.Write(w, kferr.DomainNotAllowed())
			return
		}
	}
	for key, value := range map[string]*string{"code": patch.Code, "firstName": patch.FirstName, "lastName": patch.LastName} {
		if value != nil && strings.TrimSpace(*value) == "" {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{key: {"must not be blank"}}))
			return
		}
	}
	if patch.Code != nil && len([]rune(*patch.Code)) > 100 || patch.FirstName != nil && len([]rune(*patch.FirstName)) > 255 || patch.LastName != nil && len([]rune(*patch.LastName)) > 255 || patch.EmployeeID != nil && *patch.EmployeeID != nil && len([]rune(**patch.EmployeeID)) > 30 {
		kferr.Write(w, kferr.InvalidParameter("Invalid user fields"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	now := a.d.Clock.Now()
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if err := store.UserUpdate(r.Context(), tx, id, patch, store.Millis(now)); err != nil {
			return err
		}
		return pf18Audit(r, tx, p.User.ID, "user.update", id, fields)
	})
	if err != nil {
		if pf18Unique(err) {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"code": {"has already been taken"}, "email": {"has already been taken"}}))
		} else {
			kferr.Write(w, err)
		}
		return
	}
	out, err := pf18UserDetail(r.Context(), a.d.DB.Q(), id, true)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) lookupUserByEmail(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionUser); err != nil {
		kferr.Write(w, err)
		return
	}
	email := r.URL.Query().Get("email")
	if email == "" {
		kferr.Write(w, kferr.InvalidParameter("Missing email"))
		return
	}
	u, err := store.UserByEmail(r.Context(), a.d.DB.Q(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			kferr.Write(w, kferr.ResourceNotFound())
		} else {
			kferr.Write(w, kferr.Internal())
		}
		return
	}
	out, err := pf18UserDetail(r.Context(), a.d.DB.Q(), u.ID, true)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (a *API) listUserTeams(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionTeam); err != nil {
		kferr.Write(w, err)
		return
	}
	userID, err := store.ResolveID(r.Context(), a.d.DB.Q(), "users", r.PathValue("userId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	if _, err := store.GetUser(r.Context(), a.d.DB.Q(), userID); err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	submittable := false
	if values, ok := r.URL.Query()["submittable"]; ok {
		if len(values) != 1 {
			kferr.Write(w, kferr.InvalidParameter("Invalid submittable"))
			return
		}
		parsed, err := strconv.ParseBool(values[0])
		if err != nil {
			kferr.Write(w, kferr.InvalidParameter("Invalid submittable"))
			return
		}
		submittable = parsed
	}
	ids, _, err := store.UserTeamIDs(r.Context(), a.d.DB.Q(), userID, 0, 10000)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	all := make([]present.TeamJSON, 0, len(ids))
	for _, id := range ids {
		team, err := store.GetTeam(r.Context(), a.d.DB.Q(), id)
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		if submittable && team.ApproveOnly {
			continue
		}
		if !principalOf(r).Allows(model.PermissionTeam, id) {
			continue
		}
		item, err := present.Team(r.Context(), a.d.DB.Q(), id)
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		all = append(all, item)
	}
	total := len(all)
	start := page.Offset()
	if start > len(all) {
		start = len(all)
	}
	end := start + page.PerPage
	if end > len(all) {
		end = len(all)
	}
	items := all[start:end]
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (a *API) listUserRoles(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionRole); err != nil {
		kferr.Write(w, err)
		return
	}
	userID, err := store.ResolveID(r.Context(), a.d.DB.Q(), "users", r.PathValue("userId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if _, err := store.GetUser(r.Context(), a.d.DB.Q(), userID); err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	ids, total, err := store.UserRoleIDs(r.Context(), a.d.DB.Q(), userID, page.Offset(), page.PerPage)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	items := make([]present.RoleJSON, 0, len(ids))
	for _, id := range ids {
		item, err := present.Role(r.Context(), a.d.DB.Q(), id)
		if err != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		items = append(items, item)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, items)
}

// pf18UserDetail renders a user. includeHiddenFields adds the custom fields that
// are not visible to everyone, which callers with the user permission may see.
func pf18UserDetail(ctx context.Context, q store.Querier, id string, includeHiddenFields bool) (present.UserDetailJSON, error) {
	out, err := present.UserDetail(ctx, q, id, present.UserOpts{LastUsedOn: true, CustomFields: true, UserType: true, AllCustomFields: includeHiddenFields})
	if err != nil {
		return nil, err
	}
	u, err := store.GetUser(ctx, q, id)
	if err != nil {
		return nil, err
	}
	out["deactivatedAt"] = nil
	if u.DeactivatedAt != nil {
		out["deactivatedAt"] = kftime.Format(*u.DeactivatedAt)
	}
	return out, nil
}

func pf18Audit(r *http.Request, q store.Querier, userID, action, id string, fields []string) error {
	data, err := json.Marshal(map[string]any{"id": id, "fields": fields})
	if err != nil {
		return err
	}
	ip, requestID, system := httpx.ClientIP(r), httpx.RequestIDFrom(r.Context()), "api"
	return store.InsertAuditLog(r.Context(), q, model.AuditLog{UserID: &userID, Action: action, Data: data, RemoteIP: &ip, RequestID: &requestID, SystemType: &system, CreatedAt: time.Now()})
}
func pf18Contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func pf18Unique(err error) bool {
	return err != nil && (strings.Contains(strings.ToLower(err.Error()), "unique constraint") || strings.Contains(strings.ToLower(err.Error()), "is not unique"))
}

// pf08AddressOnlyEmail reports whether s is a bare email address. The user
// schemas declare format: email, so RFC 5322 mailbox forms that mail.ParseAddress
// accepts ("Name <user@example.test>", trailing comments, quoted local parts)
// are rejected: the stored value is echoed back as User.email unchanged.
func pf08AddressOnlyEmail(s string) bool {
	if s == "" || strings.TrimSpace(s) != s || strings.ContainsAny(s, "<>\"(), \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Name == "" && addr.Address == s
}
