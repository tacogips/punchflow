package model

import "time"

type RolePermission struct {
	Permission string `json:"permission"`
	Scope      string `json:"scope,omitempty"`
}
type Role struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Editable       bool             `json:"editable"`
	PermissionList []RolePermission `json:"permissionList"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}
type RoleMembership struct {
	RoleID    string    `json:"roleId"`
	UserID    string    `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
}
