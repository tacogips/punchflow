package model

import "time"

type Tenant struct {
	ID                  string    `json:"id"`
	PublicID            string    `json:"publicId"`
	Name                string    `json:"name"`
	Plan                string    `json:"plan"`
	Mode                string    `json:"mode"`
	IPAllowlist         []string  `json:"ipAllowlist"`
	AllowedEmailDomains []string  `json:"allowedEmailDomains"`
	SelfServiceProxy    bool      `json:"selfServiceProxy"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
