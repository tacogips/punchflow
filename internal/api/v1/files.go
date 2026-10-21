package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"punchflow/internal/auth"
	"punchflow/internal/httpx"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func (a *API) uploadFile(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		kferr.Write(w, kferr.InvalidContentType())
		return
	}
	tenant, err := store.Tenant(r.Context(), a.d.DB.Q())
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	if tenant.Plan == "standard" {
		kferr.Write(w, kferr.UpgradeRequired())
		return
	}
	maxRequest := a.d.Files.MaxBytes() + (8 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequest)
	parseErr := r.ParseMultipartForm(8 << 20)
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	if parseErr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(parseErr, &maxErr) {
			kferr.Write(w, kferr.PayloadTooLarge())
		} else {
			kferr.Write(w, kferr.InvalidParameter("Invalid multipart form"))
		}
		return
	}
	part, header, err := r.FormFile("file")
	if err != nil {
		kferr.Write(w, kferr.InvalidParameter("Missing file"))
		return
	}
	defer func() { _ = part.Close() }()
	contentType := header.Header.Get("Content-Type")
	var saved model.File
	err = a.d.DB.WithTx(r.Context(), func(tx *store.Tx) error {
		var saveErr error
		saved, saveErr = a.d.Files.Save(r.Context(), tx, p.User.ID, header.Filename, contentType, part)
		if saveErr != nil {
			if strings.Contains(saveErr.Error(), "exceeds maximum size") {
				return kferr.PayloadTooLarge()
			}
			return saveErr
		}
		return pf23Audit(r, tx, p, "file.upload", saved.ID, "", []string{"filename", "byteSize", "contentType"}, a.d.Clock.Now())
	})
	if err != nil {
		kferr.Write(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"signedId": saved.SignedID})
}
func (a *API) getFile(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	signedID := r.PathValue("signedId")
	file, err := store.FileBySignedID(r.Context(), a.d.DB.Q(), signedID)
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	ticketIDs, err := store.FileAttachmentTicketIDs(r.Context(), a.d.DB.Q(), signedID)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	if len(ticketIDs) == 0 {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	actor := actorFor(p)
	visible := false
	for _, ticketID := range ticketIDs {
		if err := a.d.Engine.CanView(r.Context(), a.d.DB.Q(), actor, ticketID); err == nil {
			visible = true
			break
		} else if apiErr, isAPIError := kferr.As(err); !isAPIError || apiErr.Code != "invisible_ticket" {
			kferr.Write(w, err)
			return
		}
	}
	if !visible {
		kferr.Write(w, kferr.InvisibleTicket())
		return
	}
	fileURL := a.d.Files.SignedURL(file)
	if strings.HasPrefix(fileURL, "/") {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		fileURL = scheme + "://" + r.Host + fileURL
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"url": fileURL, "filename": file.Filename, "checksum": file.Checksum,
		"byteSize": file.ByteSize, "contentType": file.ContentType, "createdAt": kftime.Format(file.CreatedAt),
	})
}

func pf23Audit(r *http.Request, q store.Querier, p *auth.Principal, action, id, ticketID string, fields []string, now time.Time) error {
	data, err := json.Marshal(map[string]any{"id": id, "fields": fields})
	if err != nil {
		return fmt.Errorf("pf23 audit data: %w", err)
	}
	userID, remoteIP, requestID := p.User.ID, r.RemoteAddr, r.Header.Get("X-Request-ID")
	var ticket *string
	if ticketID != "" {
		ticket = &ticketID
	}
	return store.InsertAuditLog(r.Context(), q, model.AuditLog{UserID: &userID, Action: action, Data: data, RemoteIP: &remoteIP, RequestID: &requestID, TicketID: ticket, CreatedAt: now})
}
