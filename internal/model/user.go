package model

import "time"

type User struct {
	ID            string         `json:"id"`
	Code          string         `json:"code"`
	Email         string         `json:"email"`
	FirstName     string         `json:"firstName"`
	LastName      string         `json:"lastName"`
	EmployeeID    *string        `json:"employeeId"`
	Status        string         `json:"status"`
	Locale        string         `json:"locale"`
	UserType      string         `json:"userType"`
	CustomFields  map[string]any `json:"customFields"`
	LastUsedOn    *string        `json:"lastUsedOn"`
	DeactivatedAt *time.Time     `json:"deactivatedAt"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}
type UserInvitation struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	UserEmail  string    `json:"userEmail"`
	UserLocale string    `json:"userLocale"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
