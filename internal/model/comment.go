package model

import "time"

type Comment struct {
	ID               string     `json:"id"`
	TicketID         string     `json:"ticketId"`
	UserID           string     `json:"userId"`
	Body             string     `json:"body"`
	MentionedUserIDs []string   `json:"mentionedUserIds"`
	DeletedAt        *time.Time `json:"deletedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}
