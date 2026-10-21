package uiapi

import (
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/engine"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type pendingRequest struct {
	Pending *bool `json:"pending"`
}

type pendingResponse struct {
	Pending bool `json:"pending"`
}

// setAssigneePending updates the current assignee's hold state. An active
// proxy changes the primary assignee's state, so both users observe one hold.
func (a *API) setAssigneePending(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r); err != nil {
		kferr.Write(w, err)
		return
	}
	var body pendingRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		kferr.Write(w, err)
		return
	}
	if body.Pending == nil {
		kferr.Write(w, kferr.InvalidParameter("pending is required"))
		return
	}
	p, _ := auth.FromContext(r.Context())
	actor := engine.ActorFromPrincipal(p)
	var out pendingResponse
	err := a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		ticketID := r.PathValue("ticketId")
		if err := a.d.Engine.CanView(r.Context(), tx, actor, ticketID); err != nil {
			return err
		}
		ticket, err := store.EngLoadTicket(r.Context(), tx, ticketID)
		if err != nil {
			return kferr.ResourceNotFound()
		}
		if ticket.Status != model.TicketStatusInProgress {
			return kferr.InvalidState("Ticket is not in progress")
		}
		steps, err := store.EngTicketSteps(r.Context(), tx, ticketID)
		if err != nil {
			return err
		}
		stepID := ""
		for _, step := range steps {
			if step.StepOrder == ticket.CurrentStep && step.Status == model.StepStatusInProgress {
				stepID = step.ID
				break
			}
		}
		if stepID == "" {
			return kferr.InvalidState("No active approval step")
		}
		assignees, err := store.EngStepAssignees(r.Context(), tx, stepID)
		if err != nil {
			return err
		}
		for _, assignee := range assignees {
			if assignee.ID != r.PathValue("assigneeId") || assignee.Completed {
				continue
			}
			allowed := assignee.UserID == p.User.ID
			if !allowed {
				allowed, err = store.EngActiveProxyApprovers(r.Context(), tx, assignee.UserID, p.User.ID, ticket.WorkflowID, kftime.FormatDate(a.d.Clock.Now()))
				if err != nil {
					return err
				}
			}
			if !allowed {
				return kferr.UserNotAssigned()
			}
			assignee.Pending = *body.Pending
			assignee.UpdatedAt = a.d.Clock.Now()
			if err = store.EngUpdateAssignee(r.Context(), tx, assignee); err != nil {
				return err
			}
			out.Pending = assignee.Pending
			return nil
		}
		return kferr.ResourceNotFound()
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
