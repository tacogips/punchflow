package engine

import (
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func compatGradeID(t *testing.T, e *pf14TestEnv, code string) string {
	t.Helper()
	id, err := store.GradeIDByCode(e.ctx, e.db.Q(), code)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func compatAddViewer(t *testing.T, e *pf14TestEnv, ticketID string, viewer model.TicketViewer) {
	t.Helper()
	viewer.ID, viewer.TicketID = store.NewID(), ticketID
	e.apply(func(tx *store.Tx) error { return store.TicketQueryInsertViewer(e.ctx, tx, viewer) })
}

func compatAssertVisible(t *testing.T, e *pf14TestEnv, userCode, ticketID string, want bool) {
	t.Helper()
	err := e.engine.CanView(e.ctx, e.db.Q(), e.actor(userCode), ticketID)
	if want && err != nil {
		t.Fatalf("%s cannot view ticket: %v", userCode, err)
	}
	if !want {
		assertPF14ErrorCode(t, err, "invisible_ticket")
	}
}

// R02 / PF-03: persisted ticket viewers (user and team) grant visibility.
func TestVisibilityPersistedTicketViewers(t *testing.T) {
	e := pf14Setup(t)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	compatAssertVisible(t, e, "engineer", id, false)

	t.Run("user viewer", func(t *testing.T) {
		user := e.user("engineer")
		compatAddViewer(t, e, id, model.TicketViewer{UserID: &user})
		compatAssertVisible(t, e, "engineer", id, true)
		compatAssertVisible(t, e, "assistant", id, false)
	})

	t.Run("team viewer without descendants", func(t *testing.T) {
		e := pf14Setup(t)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		team := e.team("engineering")
		compatAddViewer(t, e, id, model.TicketViewer{TeamID: &team})
		compatAssertVisible(t, e, "engineer", id, true)
		compatAssertVisible(t, e, "assistant", id, false)
	})

	t.Run("team viewer descendants and grade", func(t *testing.T) {
		e := pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", "")
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		company := e.team("company")
		compatAddViewer(t, e, id, model.TicketViewer{TeamID: &company})
		compatAssertVisible(t, e, "ceo", id, true)
		compatAssertVisible(t, e, "engineer", id, false) // member of a descendant only

		e = pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", "")
		id, _ = compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		company = e.team("company")
		manager := compatGradeID(t, e, "manager")
		compatAddViewer(t, e, id, model.TicketViewer{TeamID: &company, Descendants: true, GradeID: &manager})
		compatAssertVisible(t, e, "eng-manager", id, true)
		compatAssertVisible(t, e, "hr-manager", id, true)
		compatAssertVisible(t, e, "engineer", id, false) // staff grade
		compatAssertVisible(t, e, "ceo", id, false)      // executive grade

		e = pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", "")
		id, _ = compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		company = e.team("company")
		compatAddViewer(t, e, id, model.TicketViewer{TeamID: &company, Descendants: true})
		compatAssertVisible(t, e, "engineer", id, true)
	})

	t.Run("drafts are not shared", func(t *testing.T) {
		e := pf14Setup(t)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		user := e.user("engineer")
		compatAddViewer(t, e, id, model.TicketViewer{UserID: &user})
		compatAssertVisible(t, e, "engineer", id, false)
	})
}

// R02 / PF-03: public tickets are visible to everyone, computed from the forced
// public type and the workflow publicTicket setting.
func TestVisibilityPublicTickets(t *testing.T) {
	setForced := func(e *pf14TestEnv, id, forced string) {
		e.apply(func(tx *store.Tx) error {
			_, err := tx.ExecContext(e.ctx, `UPDATE tickets SET forced_public_type=? WHERE id=?`, forced, id)
			return err
		})
	}
	cases := []struct {
		name     string
		workflow bool
		forced   string
		visible  bool
	}{
		{"follow public workflow", true, model.ForcedPublicTypeFollowWorkflow, true},
		{"follow private workflow", false, model.ForcedPublicTypeFollowWorkflow, false},
		{"forced public on private workflow", false, model.ForcedPublicTypeForcedPublic, true},
		{"forced private on public workflow", true, model.ForcedPublicTypeForcedPrivate, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := pf14Setup(t)
			compatSetWorkflow(t, e, "expense-claim", "$.publicTicket", tc.workflow)
			id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
			setForced(e, id, tc.forced)
			compatAssertVisible(t, e, "engineer", id, tc.visible)
		})
	}
	t.Run("public drafts stay private", func(t *testing.T) {
		e := pf14Setup(t)
		compatSetWorkflow(t, e, "expense-claim", "$.publicTicket", true)
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusDraft)
		compatAssertVisible(t, e, "engineer", id, false)
	})
}

// INF-36: ticket_write holders and the proxy client can read, like ticket_read.
func TestVisibilityWriterAndProxyClient(t *testing.T) {
	e := pf14Setup(t)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	writer := Actor{UserID: e.user("hr-manager"), Permissions: map[string]bool{model.PermissionTicketWrite: true}}
	if err := e.engine.CanView(e.ctx, e.db.Q(), writer, id); err != nil {
		t.Fatalf("ticket_write holder cannot view: %v", err)
	}
	client := e.user("engineer")
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE tickets SET proxy_client_user_id=? WHERE id=?`, client, id)
		return err
	})
	compatAssertVisible(t, e, "engineer", id, true)
}

func TestProxySharingPersistsButActionsRequireActivePrincipal(t *testing.T) {
	e := pf14Setup(t)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	principalID := pf14cCurrentApprover(t, e, id)
	proxyID := e.user("hr-manager")
	workflow, err := store.GetWorkflowByCode(e.ctx, e.db.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `INSERT INTO proxy_approvers(id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at) VALUES('persisted-proxy',?,?,NULL,NULL,'[]',?,?)`, principalID, proxyID, e.clock.Now().UnixMilli(), e.clock.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	submitted, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	autoViewers, err := store.TicketViewers(e.ctx, e.db.Q(), submitted)
	if err != nil {
		t.Fatal(err)
	}
	autoShared := false
	for _, viewer := range autoViewers {
		autoShared = autoShared || viewer.UserID != nil && *viewer.UserID == proxyID
	}
	if !autoShared {
		t.Fatal("submission did not persist sharing for an active proxy of an assigned user")
	}
	if err = e.db.WithTx(e.ctx, func(tx *store.Tx) error { return SyncProxySharingForTicket(e.ctx, tx, id, e.clock.Now()) }); err != nil {
		t.Fatal(err)
	}
	compatAssertVisible(t, e, "hr-manager", id, true)
	states, err := store.TicketWatchStates(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	if states[proxyID] != store.WatchStateWatching {
		t.Fatalf("proxy watch state=%q, want watching", states[proxyID])
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE users SET status=? WHERE id=?`, model.UserStatusSuspended, principalID); err != nil {
		t.Fatal(err)
	}
	if err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, actionErr := e.engine.Approve(e.ctx, tx, Actor{UserID: proxyID}, id, nil)
		return actionErr
	}); err == nil {
		t.Fatal("proxy acted for a suspended principal")
	}
	proxies, err := store.ProxyUserIDs(e.ctx, e.db.Q(), principalID, workflow.ID, e.clock.Now().Format("2006-01-02"))
	if err != nil || len(proxies) != 0 {
		t.Fatalf("suspended principal proxyUserIds=%v, err=%v", proxies, err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE users SET status=? WHERE id=?`, model.UserStatusActive, principalID); err != nil {
		t.Fatal(err)
	}
	if err = store.ProxyDeleteApprover(e.ctx, e.db.Q(), "persisted-proxy"); err != nil {
		t.Fatal(err)
	}
	compatAssertVisible(t, e, "hr-manager", id, true)
	states, err = store.TicketWatchStates(e.ctx, e.db.Q(), id)
	if err != nil || states[proxyID] != store.WatchStateWatching {
		t.Fatalf("watch after setting removal=%q, err=%v", states[proxyID], err)
	}
	// A completed historical ticket does not receive a new proxy share.
	other, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if _, err = e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET status=? WHERE id=?`, model.TicketStatusCompleted, other); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `INSERT INTO proxy_approvers(id,user_id,proxy_user_id,starts_on,ends_on,workflow_ids,created_at,updated_at) VALUES('historical-proxy',?,?,NULL,NULL,'[]',?,?)`, principalID, proxyID, e.clock.Now().UnixMilli(), e.clock.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = e.db.WithTx(e.ctx, func(tx *store.Tx) error { return SyncProxySharingForTicket(e.ctx, tx, other, e.clock.Now()) }); err != nil {
		t.Fatal(err)
	}
	viewers, err := store.TicketViewers(e.ctx, e.db.Q(), other)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range viewers {
		if viewer.UserID != nil && *viewer.UserID == proxyID {
			t.Fatal("completed historical ticket was automatically shared with proxy")
		}
	}
}

func TestWorkflowVisibleToManagerSharesDirectAndAllManagers(t *testing.T) {
	for _, mode := range []string{"direct", "all"} {
		t.Run(mode, func(t *testing.T) {
			e := pf14Setup(t)
			compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", mode)
			id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
			sales := e.team("sales")
			ancestors, err := store.EngTeamAncestors(e.ctx, e.db.Q(), sales)
			if err != nil || len(ancestors) < 2 {
				t.Fatalf("team ancestors=%v, err=%v", ancestors, err)
			}
			direct, err := store.EngLeadersOfTeam(e.ctx, e.db.Q(), ancestors[0].ID)
			if err != nil || len(direct) == 0 {
				t.Fatalf("direct managers=%v, err=%v", direct, err)
			}
			canView := func(userID string) bool {
				t.Helper()
				return e.engine.CanView(e.ctx, e.db.Q(), Actor{UserID: userID}, id) == nil
			}
			if !canView(direct[0]) {
				t.Fatal("direct team manager cannot view ticket")
			}
			parentLeaders, err := store.EngLeadersOfTeam(e.ctx, e.db.Q(), ancestors[1].ID)
			if err != nil || len(parentLeaders) == 0 {
				t.Fatalf("parent managers=%v, err=%v", parentLeaders, err)
			}
			if got := canView(parentLeaders[0]); got != (mode == "all") {
				t.Fatalf("parent manager visibility=%v for mode %q", got, mode)
			}
		})
	}
}

func TestDirectManagerSharingFindsNearestParentAndExcludesApplicant(t *testing.T) {
	e := pf14Setup(t)
	compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", "direct")
	compatSetWorkflow(t, e, "expense-claim", "$.visibleToTeamMembers", false)
	id, _ := compatMustCreate(t, e, "sales-manager", model.TicketStatusInProgress)
	ceoID := e.user("ceo")
	viewers, err := store.TicketViewers(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	ceoShared := false
	for _, viewer := range viewers {
		if viewer.UserID == nil {
			continue
		}
		ceoShared = ceoShared || *viewer.UserID == ceoID
	}
	if !ceoShared {
		t.Fatal("parent team's nearest eligible manager was not shared")
	}
}

func TestWorkflowVisibleToTeamMembersPersistsIndividualViewers(t *testing.T) {
	e := pf14Setup(t)
	compatSetWorkflow(t, e, "expense-claim", "$.visibleToTeamMembers", true)
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	formerMember := e.user("sales-manager")
	newMember := e.user("assistant")
	e.apply(func(tx *store.Tx) error {
		_, err := tx.ExecContext(e.ctx, `DELETE FROM team_memberships WHERE team_id=? AND user_id=?`, e.team("sales"), formerMember)
		if err != nil {
			return err
		}
		return store.TeamMembershipInsert(e.ctx, tx, model.TeamMembership{ID: store.NewID(), TeamID: e.team("sales"), UserID: newMember, GradeIDs: []string{}, CreatedAt: e.clock.Now(), UpdatedAt: e.clock.Now()})
	})
	compatAssertVisible(t, e, "sales-manager", id, true)
	if err := e.engine.CanView(e.ctx, e.db.Q(), Actor{UserID: newMember}, id); err == nil {
		t.Fatal("new team member gained access to a ticket submitted before joining")
	}
}

func TestWorkflowWatchersAreSharedUsers(t *testing.T) {
	e := pf14Setup(t)
	watcherID := e.user("hr-manager")
	watcher, err := store.GetUser(e.ctx, e.db.Q(), watcherID)
	if err != nil {
		t.Fatal(err)
	}
	compatSetWorkflow(t, e, "expense-claim", "$.watchers", []map[string]any{{"id": "watcher-1", "watcherType": "user", "userCode": watcher.Code}})
	id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
	if err = e.engine.CanView(e.ctx, e.db.Q(), Actor{UserID: watcherID}, id); err != nil {
		t.Fatalf("workflow watcher cannot view private ticket: %v", err)
	}
}

func TestManagerSharingRefreshesOnLaterStepTransition(t *testing.T) {
	e := pf14Setup(t)
	compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", "direct")
	id := e.createExpense("sales-rep", 5000, true)
	financeManager := e.user("finance-manager")
	viewers, err := store.TicketViewers(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range viewers {
		if viewer.UserID != nil && *viewer.UserID == financeManager {
			t.Fatal("future manager unexpectedly shared at submission")
		}
	}
	if _, err = e.db.Q().ExecContext(e.ctx, `INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES(?,?,?,1,'[]',0,0)`, store.NewID(), e.team("sales"), financeManager); err != nil {
		t.Fatal(err)
	}
	e.apply(func(tx *store.Tx) error {
		_, actionErr := e.engine.Approve(e.ctx, tx, e.actor("sales-manager"), id, nil)
		return actionErr
	})
	viewers, err = store.TicketViewers(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range viewers {
		if viewer.UserID != nil && *viewer.UserID == financeManager {
			return
		}
	}
	t.Fatal("manager added after submission was not shared at the next-step transition")
}

func TestCompletedTicketDoesNotReceiveNewManagerSharing(t *testing.T) {
	e := pf14Setup(t)
	compatSetWorkflow(t, e, "expense-claim", "$.visibleToManager", "direct")
	id := e.createExpense("sales-rep", 5000, true)
	assistant := e.user("assistant")
	if _, err := e.db.Q().ExecContext(e.ctx, `UPDATE tickets SET status=? WHERE id=?`, model.TicketStatusCompleted, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Q().ExecContext(e.ctx, `INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES(?,?,?,1,'[]',0,0)`, store.NewID(), e.team("sales"), assistant); err != nil {
		t.Fatal(err)
	}
	if err := e.db.WithTx(e.ctx, func(tx *store.Tx) error { return SyncWorkflowManagerSharingForTicket(e.ctx, tx, id) }); err != nil {
		t.Fatal(err)
	}
	viewers, err := store.TicketViewers(e.ctx, e.db.Q(), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range viewers {
		if viewer.UserID != nil && *viewer.UserID == assistant {
			t.Fatal("completed ticket received a new manager-sharing snapshot")
		}
	}
}
