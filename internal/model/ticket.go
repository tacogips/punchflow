package model

import "time"

type Ticket struct {
	ID                 string     `json:"id"`
	WorkflowID         string     `json:"workflowId"`
	Status             string     `json:"status"`
	CurrentStep        int        `json:"currentStep"`
	Title              *string    `json:"title"`
	TicketNumber       *string    `json:"ticketNumber"`
	AuthorID           *string    `json:"authorId"`
	AuthorTeamID       *string    `json:"authorTeamId"`
	AuthorTeamSnapshot []byte     `json:"authorTeamSnapshot,omitempty"`
	ProxyClientUserID  *string    `json:"proxyClientUserId"`
	ForcedPublicType   string     `json:"forcedPublicType"`
	RouteID            *string    `json:"routeId"`
	OpenedAt           *time.Time `json:"openedAt"`
	CompletedAt        *time.Time `json:"completedAt"`
	DeniedAt           *time.Time `json:"deniedAt"`
	ArchivedAt         *time.Time `json:"archivedAt"`
	ClosedAt           *time.Time `json:"closedAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
type TicketInput struct {
	ID          string    `json:"id"`
	TicketID    string    `json:"ticketId"`
	FormFieldID string    `json:"formFieldId"`
	Value       any       `json:"value"`
	RefIDs      []string  `json:"refIds"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type TicketSlipItem struct {
	ID            string         `json:"id"`
	TicketID      string         `json:"ticketId"`
	SlipSectionID string         `json:"slipSectionId"`
	Position      int            `json:"position"`
	Inputs        map[string]any `json:"inputs"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}
type TicketStep struct {
	ID                       string     `json:"id"`
	TicketID                 string     `json:"ticketId"`
	RouteStepID              *string    `json:"routeStepId"`
	RouteStepCode            *string    `json:"routeStepCode"`
	StepOrder                int        `json:"stepOrder"`
	DisplayOrder             int        `json:"displayOrder"`
	Title                    *string    `json:"title"`
	ActionType               string     `json:"actionType"`
	RequiredApprovalsNumber  int        `json:"requiredApprovalsNumber"`
	RequiredApprovalsPercent int        `json:"requiredApprovalsPercent"`
	Status                   string     `json:"status"`
	Skip                     bool       `json:"skip"`
	Fallback                 bool       `json:"fallback"`
	FallbackResult           *string    `json:"fallbackResult"`
	CompletedAt              *time.Time `json:"completedAt"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}
type TicketAssignee struct {
	ID               string     `json:"id"`
	TicketStepID     string     `json:"ticketStepId"`
	TicketID         string     `json:"ticketId"`
	UserID           string     `json:"userId"`
	Completed        bool       `json:"completed"`
	CompletedAt      *time.Time `json:"completedAt"`
	ActualApproverID *string    `json:"actualApproverId"`
	AssignedAt       *time.Time `json:"assignedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}
type TicketViewer struct {
	ID          string  `json:"id"`
	TicketID    string  `json:"ticketId"`
	UserID      *string `json:"userId"`
	TeamID      *string `json:"teamId"`
	GradeID     *string `json:"gradeId"`
	Descendants bool    `json:"descendants"`
}
type TicketLink struct {
	TicketID       string    `json:"ticketId"`
	LinkedTicketID string    `json:"linkedTicketId"`
	CreatedAt      time.Time `json:"createdAt"`
}
type TicketCustomApprover struct {
	TicketID      string   `json:"ticketId"`
	RouteStepCode string   `json:"routeStepCode"`
	UserIDs       []string `json:"userIds"`
}

// TicketIsPublic reports the effective public status of a ticket from its
// forced public type and the workflow-level publicTicket setting.
func TicketIsPublic(forcedPublicType string, workflowPublic bool) bool {
	switch forcedPublicType {
	case ForcedPublicTypeForcedPublic:
		return true
	case ForcedPublicTypeForcedPrivate:
		return false
	default:
		return workflowPublic
	}
}
