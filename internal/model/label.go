package model

import "time"

// Label is a colored tag that viewers attach to tickets.
type Label struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// SubStatus is an admin-defined progress marker attached to a ticket when it
// reaches a route step. The Allow* flags gate the corresponding ticket actions.
type SubStatus struct {
	ID              string    `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Notes           *string   `json:"notes"`
	AllowWithdrawal bool      `json:"allowWithdrawal"`
	AllowRejection  bool      `json:"allowRejection"`
	AllowDenial     bool      `json:"allowDenial"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// TicketNumberingTimingSubStatusAttached is the workflow numbering timing that
// allocates the ticket number when a sub-status is attached to the ticket.
const TicketNumberingTimingSubStatusAttached = "sub_status_attached"
