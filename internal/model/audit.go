package model

import "time"

type AuditLog struct {
	ID         string    `json:"id"`
	UserID     *string   `json:"userId"`
	Action     string    `json:"action"`
	Data       []byte    `json:"data,omitempty"`
	RemoteIP   *string   `json:"remoteIp"`
	RequestID  *string   `json:"requestId"`
	SystemType *string   `json:"systemType"`
	TicketID   *string   `json:"ticketId"`
	CreatedAt  time.Time `json:"createdAt"`
}
