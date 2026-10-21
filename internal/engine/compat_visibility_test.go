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
		id, _ := compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		company := e.team("company")
		compatAddViewer(t, e, id, model.TicketViewer{TeamID: &company})
		compatAssertVisible(t, e, "ceo", id, true)
		compatAssertVisible(t, e, "engineer", id, false) // member of a descendant only

		e = pf14Setup(t)
		id, _ = compatMustCreate(t, e, "sales-rep", model.TicketStatusInProgress)
		company = e.team("company")
		manager := compatGradeID(t, e, "manager")
		compatAddViewer(t, e, id, model.TicketViewer{TeamID: &company, Descendants: true, GradeID: &manager})
		compatAssertVisible(t, e, "eng-manager", id, true)
		compatAssertVisible(t, e, "hr-manager", id, true)
		compatAssertVisible(t, e, "engineer", id, false) // staff grade
		compatAssertVisible(t, e, "ceo", id, false)      // executive grade

		e = pf14Setup(t)
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
