package ticketui

import (
	"net/http"

	"punchflow/internal/auth"
	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) resetCustomNumbering(w http.ResponseWriter, r *http.Request) {
	if err := auth.Require(r, model.PermissionWorkflow); err != nil {
		kferr.Write(w, err)
		return
	}
	q := a.d.DB.Q()
	workflowID, err := store.ResolveID(r.Context(), q, "workflows", r.PathValue("workflowId"))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	workflow, err := store.GetWorkflow(r.Context(), q, workflowID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	principal, ok := auth.FromContext(r.Context())
	if !ok || !principal.Allows(model.PermissionWorkflow, workflow.FolderID) {
		kferr.Write(w, kferr.MissingPermission("workflow management permission is required"))
		return
	}
	definition, err := store.WorkflowDefByID(r.Context(), q, workflowID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	numberingID := r.PathValue("customNumberingId")
	var counterKey string
	for _, numbering := range definition.CustomNumberings {
		if numbering.ID != numberingID {
			continue
		}
		counterKey = "custom:" + numbering.ID
		if numbering.TicketNumberKeyID != nil && *numbering.TicketNumberKeyID != "" {
			counterKey = "key:" + *numbering.TicketNumberKeyID
		}
		break
	}
	if counterKey == "" {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		_, execErr := tx.ExecContext(r.Context(), `INSERT INTO custom_numbering_counters(custom_numbering_id,counter,last_allocated_at,last_period)
			VALUES(?,0,NULL,'') ON CONFLICT(custom_numbering_id) DO UPDATE SET counter=0,last_allocated_at=NULL,last_period=''`, counterKey)
		return execErr
	}); err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
