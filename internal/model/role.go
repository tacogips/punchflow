package model

import "time"

// RolePermission is one entry of a role's permissionList as persisted. A
// restricted entry limits the permission to the listed folders, general masters
// or teams; the dimension that applies depends on the permission (see
// PermissionScopeKind).
type RolePermission struct {
	Permission       string   `json:"permission"`
	Scope            string   `json:"scope,omitempty"`
	Restricted       bool     `json:"restricted,omitempty"`
	FolderIDs        []string `json:"folderIds,omitempty"`
	GeneralMasterIDs []string `json:"generalMasterIds,omitempty"`
	TeamIDs          []string `json:"teamIds,omitempty"`
}

// Scope kinds identify which target list limits a restricted permission.
const (
	ScopeKindNone   = ""
	ScopeKindFolder = "folder"
	ScopeKindMaster = "master"
	ScopeKindTeam   = "team"
)

// PermissionScopeKind reports which target dimension can restrict permission.
// Permissions that kickflow does not allow to be scoped return ScopeKindNone
// and are always treated as unrestricted.
func PermissionScopeKind(permission string) string {
	switch permission {
	case PermissionWorkflow, PermissionRoute, PermissionPipeline, PermissionTicketRead:
		return ScopeKindFolder
	case PermissionMaster, PermissionMasterRead:
		return ScopeKindMaster
	case PermissionTeam:
		return ScopeKindTeam
	default:
		return ScopeKindNone
	}
}

// PermissionGrant is the effective scope of one permission after merging every
// role of a user. Unrestricted grants ignore Targets; otherwise Targets holds
// the union of the target IDs of all restricted entries (folders and teams are
// expanded to include their descendants).
type PermissionGrant struct {
	Unrestricted bool
	Targets      map[string]bool
}

// Allows reports whether the grant covers the target ID.
func (g PermissionGrant) Allows(id string) bool {
	return g.Unrestricted || g.Targets[id]
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
