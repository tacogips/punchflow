package v1

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

type pf19TeamBody struct {
	Name        *string `json:"name"`
	Code        *string `json:"code"`
	Notes       *string `json:"notes"`
	ParentID    *string `json:"parentId"`
	ApproveOnly *bool   `json:"approveOnly"`
}
type pf19MemberInput struct {
	ID       string          `json:"id"`
	Leader   *bool           `json:"leader"`
	GradeIDs json.RawMessage `json:"gradeIds"`
}
type pf19MembersBody struct {
	Users   []pf19MemberInput `json:"users"`
	UserIDs []string          `json:"userIds"`
}
type pf19MemberUpdate struct {
	Leader   *bool           `json:"leader"`
	GradeIDs json.RawMessage `json:"gradeIds"`
}

func (a *API) pf19TeamAuth(r *http.Request) error { return auth.Require(r, model.PermissionTeam) }
func (a *API) pf19ChartID(r *http.Request) (string, error) {
	id := r.PathValue("organizationChartId")
	if id == "" {
		return "", kferr.ResourceNotFound()
	}
	if _, e := store.GetOrganizationChart(r.Context(), a.d.DB.Q(), id); e != nil {
		return "", kferr.ResourceNotFound()
	}
	return id, nil
}
func (a *API) pf19TeamID(r *http.Request, q store.Querier, chartID string) (string, error) {
	raw := r.PathValue("teamId")
	if raw == "" {
		return "", kferr.ResourceNotFound()
	}
	if t, e := store.GetTeam(r.Context(), q, raw); e == nil && t.OrganizationChartID == chartID {
		return t.ID, nil
	}
	id, e := store.TeamIDByCode(r.Context(), q, chartID, raw)
	if e != nil {
		return "", kferr.ResourceNotFound()
	}
	return id, nil
}
func (a *API) listTeams(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	var parent *string
	if raw, ok := r.URL.Query()["parentId"]; ok {
		if len(raw) != 1 {
			kferr.Write(w, kferr.InvalidParameter("Invalid parentId"))
			return
		}
		id := raw[0]
		if t, er := store.GetTeam(r.Context(), a.d.DB.Q(), id); er == nil && t.OrganizationChartID == chart {
			id = t.ID
		} else {
			id, er = store.TeamIDByCode(r.Context(), a.d.DB.Q(), chart, id)
			if er != nil {
				kferr.Write(w, kferr.ResourceNotFound())
				return
			}
		}
		parent = &id
	}
	page, e := httpx.ParsePage(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	xs, n, e := store.TeamList(r.Context(), a.d.DB.Q(), chart, parent, page.Offset(), page.PerPage, teamListFilter(principalOf(r)))
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out := make([]any, 0, len(xs))
	for _, x := range xs {
		v, er := present.Team(r.Context(), a.d.DB.Q(), x.ID)
		if er != nil {
			kferr.Write(w, kferr.Internal())
			return
		}
		out = append(out, v)
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, n)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createTeam(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	var b pf19TeamBody
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Name == nil {
		kferr.Write(w, kferr.InvalidParameter("name is required"))
		return
	}
	if *b.Name == "" {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"name": {"is required"}}))
		return
	}
	if e := pf20ValidateLength("name", *b.Name, 300); e != nil {
		kferr.Write(w, e)
		return
	}
	now := time.Now()
	x := model.Team{ID: store.NewID(), OrganizationChartID: chart, Name: *b.Name, Code: "team-" + strings.ReplaceAll(store.NewID()[24:], "-", ""), CreatedAt: now, UpdatedAt: now}
	if b.Code != nil && *b.Code != "" {
		x.Code = *b.Code
	}
	if e := pf20ValidateLength("code", x.Code, 100); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Notes != nil {
		if e := pf20ValidateLength("notes", *b.Notes, 10000); e != nil {
			kferr.Write(w, e)
			return
		}
		x.Notes = b.Notes
	}
	if b.ParentID != nil && *b.ParentID != "" {
		pid, er := a.pf19ResolveParent(r, chart, *b.ParentID)
		if er != nil {
			kferr.Write(w, er)
			return
		}
		x.ParentID = &pid
	}
	if e = requireTeamParent(principalOf(r), x.ParentID); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.ApproveOnly != nil {
		x.ApproveOnly = *b.ApproveOnly
	}
	p, _ := auth.FromContext(r.Context())
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if x.ParentID != nil {
			ok, er := store.TeamParentInChart(r.Context(), tx, chart, *x.ParentID)
			if er != nil {
				return er
			}
			if !ok {
				return kferr.ValidationFailed(map[string][]string{"parentId": {"must be in the same organization chart"}})
			}
		}
		exists, er := store.TeamCodeExists(r.Context(), tx, chart, x.Code, "")
		if er != nil {
			return er
		}
		if exists {
			return kferr.ValidationFailed(map[string][]string{"code": {"already exists in organization chart"}})
		}
		if er = store.TeamInsert(r.Context(), tx, x); er != nil {
			return er
		}
		return a.pf19TeamAudit(r, tx, p.User.ID, "team.create", x.ID)
	})
	if e != nil {
		kferr.Write(w, e)
		return
	}
	out, e := present.TeamDetail(r.Context(), a.d.DB.Q(), x.ID)
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) getTeam(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	out, e := present.TeamDetail(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) updateTeam(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	var b pf19TeamBody
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	x, e := store.GetTeam(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	oldTeamCode := x.Code
	if b.Name != nil {
		x.Name = *b.Name
	}
	if e := pf20ValidateLength("name", x.Name, 300); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Notes != nil {
		if e := pf20ValidateLength("notes", *b.Notes, 10000); e != nil {
			kferr.Write(w, e)
			return
		}
		x.Notes = b.Notes
	}
	if b.ApproveOnly != nil {
		x.ApproveOnly = *b.ApproveOnly
	}
	if b.Code != nil && *b.Code != "" {
		x.Code = *b.Code
	}
	if e := pf20ValidateLength("code", x.Code, 100); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.ParentID != nil {
		if *b.ParentID == "" {
			x.ParentID = nil
		} else {
			pid, er := a.pf19ResolveParent(r, chart, *b.ParentID)
			if er != nil {
				kferr.Write(w, er)
				return
			}
			x.ParentID = &pid
		}
	}
	if b.ParentID != nil {
		if e = requireTeamParent(principalOf(r), x.ParentID); e != nil {
			kferr.Write(w, e)
			return
		}
	}
	x.UpdatedAt = time.Now()
	p, _ := auth.FromContext(r.Context())
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		currentChart, er := store.OrgChartIsCurrent(r.Context(), tx, chart)
		if er != nil {
			return er
		}
		if x.ParentID != nil {
			ok, er := store.TeamParentInChart(r.Context(), tx, chart, *x.ParentID)
			if er != nil {
				return er
			}
			if !ok {
				return kferr.ValidationFailed(map[string][]string{"parentId": {"must be in the same organization chart"}})
			}
			cycle, er := store.TeamParentWouldCycle(r.Context(), tx, id, *x.ParentID)
			if er != nil {
				return er
			}
			if cycle {
				return kferr.ValidationFailed(map[string][]string{"parentId": {"would create a parent cycle"}})
			}
		}
		exists, er := store.TeamCodeExists(r.Context(), tx, chart, x.Code, id)
		if er != nil {
			return er
		}
		if exists {
			return kferr.ValidationFailed(map[string][]string{"code": {"already exists in organization chart"}})
		}
		if currentChart {
			if er = store.ReconcileDefinitionCodeChange(r.Context(), tx, "team", oldTeamCode, x.Code, x.UpdatedAt); er != nil {
				return er
			}
		}
		if er = store.TeamUpdate(r.Context(), tx, x); er != nil {
			return er
		}
		return a.pf19TeamAudit(r, tx, p.User.ID, "team.update", id)
	})
	if e != nil {
		kferr.Write(w, e)
		return
	}
	out, e := present.TeamDetail(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) deleteTeam(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	team, e := store.GetTeam(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	p, _ := auth.FromContext(r.Context())
	var events []model.WebhookEvent
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		currentChart, currentErr := store.OrgChartIsCurrent(r.Context(), tx, chart)
		if currentErr != nil {
			return currentErr
		}
		var er error
		events, er = a.d.Engine.CleanupDeletedTeam(r.Context(), tx, actorFor(p), id)
		if er != nil {
			return er
		}
		if currentChart {
			if er = store.InvalidateRoutesReferencing(r.Context(), tx, team.Code, false, a.d.Clock.Now()); er != nil {
				return er
			}
			if er = store.RemoveTeamDefinitionReferences(r.Context(), tx, team.Code, a.d.Clock.Now()); er != nil {
				return er
			}
			if er = store.RemoveTeamFromRoleTargets(r.Context(), tx, id, a.d.Clock.Now()); er != nil {
				return er
			}
		}
		if er = store.TeamReparentChildren(r.Context(), tx, id, team.ParentID, a.d.Clock.Now()); er != nil {
			return er
		}
		if er = store.TeamDelete(r.Context(), tx, id, chart); er != nil {
			return er
		}
		if er = a.pf19TeamAudit(r, tx, p.User.ID, "team.delete", id); er != nil {
			return er
		}
		return a.d.Events.Enqueue(r.Context(), tx, events)
	})
	if e != nil {
		kferr.Write(w, e)
		return
	}
	a.d.Events.Notify()
	httpx.WriteEmpty(w)
}
func (a *API) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	page, e := httpx.ParsePage(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	total, e := store.CountTeamUsers(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	out, e := present.MemberUsers(r.Context(), a.d.DB.Q(), id, page.Offset(), page.PerPage)
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	httpx.SetPagination(w, r, a.d.BaseURL, page, total)
	httpx.WriteJSON(w, http.StatusOK, out)
}
func (a *API) createTeamMembers(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	var b pf19MembersBody
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if len(b.Users) == 0 || len(b.Users) > 10 {
		kferr.Write(w, kferr.InvalidParameter("users must contain between 1 and 10 entries"))
		return
	}
	now := time.Now()
	p, _ := auth.FromContext(r.Context())
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		members, er := store.GetTeamMemberships(r.Context(), tx, id)
		if er != nil {
			return er
		}
		leaders := false
		for _, m := range members {
			leaders = leaders || m.Leader
		}
		seen := map[string]bool{}
		for _, u := range b.Users {
			if u.ID == "" {
				return kferr.InvalidParameter("id is required")
			}
			if u.Leader == nil {
				return kferr.InvalidParameter("leader is required")
			}
			if string(u.GradeIDs) == "null" {
				return kferr.InvalidParameter("gradeIds must be an array")
			}
			uid, er := store.ResolveID(r.Context(), tx, "users", u.ID)
			if er != nil {
				return kferr.ValidationFailed(map[string][]string{"users": {"must reference existing users"}})
			}
			if seen[uid] {
				return kferr.InvalidParameter("duplicate user id")
			}
			seen[uid] = true
			exists, er := store.TeamMembershipExists(r.Context(), tx, id, uid)
			if er != nil {
				return er
			}
			if exists {
				return kferr.ValidationFailed(map[string][]string{"users": {"user is already a team member"}})
			}
			gradeRefs := []string{}
			if len(u.GradeIDs) == 0 {
				defaultGrade, err := store.DefaultGradeID(r.Context(), tx)
				if err != nil {
					return kferr.ValidationFailed(map[string][]string{"gradeIds": {"default grade is not configured"}})
				}
				gradeRefs = append(gradeRefs, defaultGrade)
			} else {
				if string(u.GradeIDs) == "null" {
					return kferr.InvalidParameter("gradeIds must be an array")
				}
				if err := json.Unmarshal(u.GradeIDs, &gradeRefs); err != nil {
					return kferr.InvalidParameter("gradeIds must be an array")
				}
			}
			if len(gradeRefs) == 0 {
				return kferr.ValidationFailed(map[string][]string{"gradeIds": {"must contain at least one grade"}})
			}
			grades := []string{}
			for _, g := range gradeRefs {
				gid, er := store.ResolveID(r.Context(), tx, "grades", g)
				if er != nil {
					return kferr.ValidationFailed(map[string][]string{"gradeIds": {"must reference existing grades"}})
				}
				grades = append(grades, gid)
			}
			if len(grades) == 0 {
				return kferr.ValidationFailed(map[string][]string{"gradeIds": {"must contain at least one grade"}})
			}
			user, er := store.GetUser(r.Context(), tx, uid)
			if er != nil || (user.Status != model.UserStatusInvited && user.Status != model.UserStatusActive) {
				return kferr.ValidationFailed(map[string][]string{"users": {"must be invited or active users"}})
			}
			if er = store.TeamMembershipInsert(r.Context(), tx, model.TeamMembership{ID: store.NewID(), TeamID: id, UserID: uid, Leader: *u.Leader, GradeIDs: grades, CreatedAt: now, UpdatedAt: now}); er != nil {
				return er
			}
			leaders = leaders || *u.Leader
		}
		if !leaders {
			return kferr.ValidationFailed(map[string][]string{"users": {"team must have at least one leader"}})
		}
		return a.pf19TeamAudit(r, tx, p.User.ID, "team.members.create", id)
	})
	if e != nil {
		kferr.Write(w, a.pf19TeamError(e))
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) deleteTeamMembers(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	var b pf19MembersBody
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if len(b.UserIDs) == 0 || len(b.UserIDs) > 10 {
		kferr.Write(w, kferr.InvalidParameter("userIds must contain between 1 and 10 entries"))
		return
	}
	if pf19HasDuplicateStrings(b.UserIDs) {
		kferr.Write(w, kferr.InvalidParameter("userIds must not contain duplicates"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		members, er := store.GetTeamMemberships(r.Context(), tx, id)
		if er != nil {
			return er
		}
		remove := map[string]bool{}
		for _, raw := range b.UserIDs {
			uid, er := store.ResolveID(r.Context(), tx, "users", raw)
			if er != nil {
				return kferr.ResourceNotFound()
			}
			if remove[uid] {
				return kferr.InvalidParameter("userIds must not contain duplicates")
			}
			remove[uid] = true
		}
		remaining, leaders := 0, 0
		for _, m := range members {
			if !remove[m.UserID] {
				remaining++
				if m.Leader {
					leaders++
				}
			}
		}
		if remaining > 0 && leaders == 0 {
			return kferr.ValidationFailed(map[string][]string{"userIds": {"team must retain at least one leader"}})
		}
		resolved := make([]string, 0, len(remove))
		for uid := range remove {
			resolved = append(resolved, uid)
		}
		if er = store.TeamMembershipDelete(r.Context(), tx, id, resolved); er != nil {
			return er
		}
		return a.pf19TeamAudit(r, tx, p.User.ID, "team.members.delete", id)
	})
	if e != nil {
		kferr.Write(w, a.pf19TeamError(e))
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) updateTeamMember(w http.ResponseWriter, r *http.Request) {
	if e := a.pf19TeamAuth(r); e != nil {
		kferr.Write(w, e)
		return
	}
	chart, e := a.pf19ChartID(r)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	id, e := a.pf19TeamID(r, a.d.DB.Q(), chart)
	if e != nil {
		kferr.Write(w, e)
		return
	}
	if !a.guardTeam(w, r, id) {
		return
	}
	uid, e := store.ResolveID(r.Context(), a.d.DB.Q(), "users", r.PathValue("userId"))
	if e != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	var b pf19MemberUpdate
	if e = httpx.DecodeJSON(r, &b); e != nil {
		kferr.Write(w, e)
		return
	}
	if b.Leader == nil || len(b.GradeIDs) == 0 {
		kferr.Write(w, kferr.InvalidParameter("leader and gradeIds are required"))
		return
	}
	if string(b.GradeIDs) == "null" {
		kferr.Write(w, kferr.InvalidParameter("gradeIds must be an array"))
		return
	}
	var gradeRefs []string
	if json.Unmarshal(b.GradeIDs, &gradeRefs) != nil {
		kferr.Write(w, kferr.InvalidParameter("gradeIds must be an array"))
		return
	}
	members, e := store.GetTeamMemberships(r.Context(), a.d.DB.Q(), id)
	if e != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	var target *model.TeamMembership
	leaders := 0
	for i := range members {
		if members[i].UserID == uid {
			target = &members[i]
		}
		if members[i].Leader {
			leaders++
		}
	}
	if target == nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	leader := *b.Leader
	grades := []string{}
	for _, g := range gradeRefs {
		gid, er := store.ResolveID(r.Context(), a.d.DB.Q(), "grades", g)
		if er != nil {
			kferr.Write(w, kferr.ValidationFailed(map[string][]string{"gradeIds": {"must reference existing grades"}}))
			return
		}
		grades = append(grades, gid)
	}
	if len(grades) == 0 {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"gradeIds": {"must contain at least one grade"}}))
		return
	}
	if target.Leader && !leader {
		leaders--
	}
	if !target.Leader && leader {
		leaders++
	}
	if len(members) > 0 && leaders == 0 {
		kferr.Write(w, kferr.ValidationFailed(map[string][]string{"leader": {"team must retain at least one leader"}}))
		return
	}
	p, _ := auth.FromContext(r.Context())
	e = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		if er := store.TeamMembershipUpdate(r.Context(), tx, id, uid, leader, grades, time.Now()); er != nil {
			return er
		}
		return a.pf19TeamAudit(r, tx, p.User.ID, "team.member.update", id)
	})
	if e != nil {
		kferr.Write(w, a.pf19TeamError(e))
		return
	}
	httpx.WriteEmpty(w)
}
func (a *API) pf19ResolveParent(r *http.Request, chart, raw string) (string, error) {
	if store.IsUUID(raw) {
		t, e := store.GetTeam(r.Context(), a.d.DB.Q(), raw)
		if e != nil || t.OrganizationChartID != chart {
			return "", kferr.ValidationFailed(map[string][]string{"parentId": {"must identify a team in this organization chart"}})
		}
		return t.ID, nil
	}
	id, e := store.TeamIDByCode(r.Context(), a.d.DB.Q(), chart, raw)
	if e != nil {
		return "", kferr.ValidationFailed(map[string][]string{"parentId": {"must identify a team in this organization chart"}})
	}
	return id, nil
}
func (a *API) pf19TeamAudit(r *http.Request, q store.Querier, user, action, id string) error {
	data, _ := json.Marshal(map[string]string{"teamId": id})
	ip := httpx.ClientIP(r)
	rid := httpx.RequestIDFrom(r.Context())
	return store.InsertAuditLog(r.Context(), q, model.AuditLog{UserID: &user, Action: action, Data: data, RemoteIP: &ip, RequestID: &rid, CreatedAt: time.Now()})
}
func (a *API) pf19TeamError(e error) error {
	if e == store.ErrNotFound {
		return kferr.ResourceNotFound()
	}
	if strings.Contains(strings.ToLower(e.Error()), "unique constraint") {
		return kferr.ValidationFailed(map[string][]string{"base": {"already exists"}})
	}
	if _, ok := kferr.As(e); ok {
		return e
	}
	return kferr.Internal()
}
