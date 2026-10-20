package model

import "time"

type WebhookEvent struct {
	Type        string `json:"type"`
	TicketID    string `json:"ticketId,omitempty"`
	CommentID   string `json:"commentId,omitempty"`
	ActorUserID string `json:"actorUserId,omitempty"`
}
type Webhook struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Secret         *string    `json:"-"`
	Enabled        bool       `json:"enabled"`
	WorkflowIDs    []string   `json:"workflowIds"`
	EventTypes     []string   `json:"eventTypes"`
	FirstFailureAt *time.Time `json:"firstFailureAt"`
	DisabledAt     *time.Time `json:"disabledAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
type WebhookDelivery struct {
	ID             string    `json:"id"`
	WebhookID      string    `json:"webhookId"`
	EventType      string    `json:"eventType"`
	TicketID       *string   `json:"ticketId"`
	CommentID      *string   `json:"commentId"`
	ActorUserID    *string   `json:"actorUserId"`
	Body           []byte    `json:"-"`
	Status         string    `json:"status"`
	AttemptCount   int       `json:"attemptCount"`
	NextAttemptAt  time.Time `json:"nextAttemptAt"`
	LastStatusCode *int      `json:"lastStatusCode"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
type WebhookAttempt struct {
	ID             string            `json:"id"`
	EventID        string            `json:"eventId"`
	AttemptNo      int               `json:"attemptNo"`
	StartedAt      time.Time         `json:"startedAt"`
	DurationMS     int64             `json:"durationMs"`
	RequestHeaders map[string]string `json:"requestHeaders"`
	StatusCode     *int              `json:"statusCode"`
	ResponseBody   []byte            `json:"-"`
	Error          *string           `json:"error"`
	Manual         bool              `json:"manual"`
	CreatedAt      time.Time         `json:"createdAt"`
}
