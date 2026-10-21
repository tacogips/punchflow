package engine

import (
	"context"
	"encoding/json"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type Actor struct {
	UserID      string
	Permissions map[string]bool
	// ReadFolderIDs holds the folders (already expanded to descendants) that a
	// restricted ticket_read role covers. Permissions[ticket_read] must be false
	// when this is set; see ticketReadAllowed.
	ReadFolderIDs map[string]bool
}
type Engine struct{ clock kftime.Clock }

func New(clock kftime.Clock) *Engine { return &Engine{clock: clock} }

type FieldInput struct {
	FormFieldID         string          `json:"formFieldId"`
	FormFieldCode       string          `json:"formFieldCode"`
	Value               json.RawMessage `json:"value"`
	GeneralMasterItemID json.RawMessage `json:"generalMasterItemId"`
	UserID              json.RawMessage `json:"userId"`
	TeamID              json.RawMessage `json:"teamId"`
	TicketID            json.RawMessage `json:"ticketId"`
	Files               []string        `json:"files"`
}
type SlipFieldInput struct {
	SlipFieldID         string          `json:"slipFieldId"`
	SlipFieldCode       string          `json:"slipFieldCode"`
	Value               json.RawMessage `json:"value"`
	GeneralMasterItemID json.RawMessage `json:"generalMasterItemId"`
	UserID              json.RawMessage `json:"userId"`
	TeamID              json.RawMessage `json:"teamId"`
	TicketID            json.RawMessage `json:"ticketId"`
	Files               []string        `json:"files"`
}
type SlipItemInput struct {
	SlipSectionID   string           `json:"slipSectionId"`
	SlipSectionCode string           `json:"slipSectionCode"`
	Inputs          []SlipFieldInput `json:"inputs"`
}
type ApproverInput struct {
	RouteStepCode string   `json:"routeStepCode"`
	UserIDs       []string `json:"userId"`
}
type CreateInput struct {
	Status            string           `json:"status"`
	WorkflowID        string           `json:"workflowId"`
	AuthorTeamID      string           `json:"authorTeamId"`
	ProxyClientUserID *string          `json:"proxyClientUserId"`
	Title             *string          `json:"title"`
	Inputs            *[]FieldInput    `json:"inputs"`
	SlipItems         *[]SlipItemInput `json:"slipItems"`
	Approvers         []ApproverInput  `json:"approvers"`
	// Viewers and WatcherUserID follow the 2026-08-05 kickflow changelog.
	Viewers       *[]InlineViewer `json:"viewers"`
	WatcherUserID *string         `json:"watcherUserId"`
}
type UpdateInput struct {
	Status            *string          `json:"status"`
	AuthorTeamID      *string          `json:"authorTeamId"`
	ProxyClientUserID json.RawMessage  `json:"proxyClientUserId"`
	Title             *string          `json:"title"`
	Inputs            *[]FieldInput    `json:"inputs"`
	SlipItems         *[]SlipItemInput `json:"slipItems"`
	Approvers         *[]ApproverInput `json:"approvers"`
	Viewers           *[]InlineViewer  `json:"viewers"`
	WatcherUserID     *string          `json:"watcherUserId"`
}
type FormPatch struct {
	Inputs    []FieldInput    `json:"inputs"`
	SlipItems []SlipItemInput `json:"slipItems"`
}

func (e *Engine) Create(ctx context.Context, tx *store.Tx, a Actor, in CreateInput) (string, []model.WebhookEvent, error) {
	return e.pf14Create(ctx, tx, a, in)
}
func (e *Engine) Update(ctx context.Context, tx *store.Tx, a Actor, ticketID string, in UpdateInput) ([]model.WebhookEvent, error) {
	return e.pf14Update(ctx, tx, a, ticketID, in)
}
func (e *Engine) UpdateForm(ctx context.Context, tx *store.Tx, a Actor, ticketID string, in FormPatch) ([]model.WebhookEvent, error) {
	return e.pf14UpdateForm(ctx, tx, a, ticketID, in)
}
func (e *Engine) Approve(ctx context.Context, tx *store.Tx, a Actor, ticketID string, comment *string) ([]model.WebhookEvent, error) {
	return e.pf14Approve(ctx, tx, a, ticketID, comment)
}
func (e *Engine) SendBack(ctx context.Context, tx *store.Tx, a Actor, ticketID string, to int, comment *string) ([]model.WebhookEvent, error) {
	return e.pf14SendBack(ctx, tx, a, ticketID, to, comment)
}
func (e *Engine) Deny(ctx context.Context, tx *store.Tx, a Actor, ticketID string, comment *string) ([]model.WebhookEvent, error) {
	return e.pf14Deny(ctx, tx, a, ticketID, comment)
}
func (e *Engine) Withdraw(ctx context.Context, tx *store.Tx, a Actor, ticketID string) ([]model.WebhookEvent, error) {
	return e.pf14Withdraw(ctx, tx, a, ticketID)
}
func (e *Engine) Archive(ctx context.Context, tx *store.Tx, a Actor, ticketID string) ([]model.WebhookEvent, error) {
	return e.pf14Archive(ctx, tx, a, ticketID)
}
func (e *Engine) CanView(ctx context.Context, q store.Querier, a Actor, ticketID string) error {
	return e.pf14CanView(ctx, q, a, ticketID)
}
func (e *Engine) AddComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, body string, files []string) (string, []model.WebhookEvent, error) {
	return e.pf14AddComment(ctx, tx, a, ticketID, body, files)
}
func (e *Engine) EditComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, commentID, body string) ([]model.WebhookEvent, error) {
	return e.pf14EditComment(ctx, tx, a, ticketID, commentID, body)
}
func (e *Engine) DeleteComment(ctx context.Context, tx *store.Tx, a Actor, ticketID, commentID string) ([]model.WebhookEvent, error) {
	return e.pf14DeleteComment(ctx, tx, a, ticketID, commentID)
}
