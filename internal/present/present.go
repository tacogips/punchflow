// Package present converts stored Punchflow models into kickflow-compatible JSON objects.
package present

import (
	"encoding/json"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"time"
)

type UserJSON map[string]any
type UserDetailJSON map[string]any
type GradeJSON map[string]any
type TeamJSON map[string]any
type TeamDetailJSON map[string]any
type MemberUserJSON map[string]any
type OrganizationChartJSON map[string]any
type RoleJSON map[string]any
type RoleDetailJSON map[string]any
type CategoryJSON map[string]any
type FolderJSON map[string]any
type FolderDetailJSON map[string]any
type GeneralMasterJSON map[string]any
type GeneralMasterItemJSON map[string]any
type WorkflowJSON map[string]any
type WorkflowInTicketJSON map[string]any
type WorkflowDetailJSON map[string]any
type RouteJSON map[string]any
type RouteDetailJSON map[string]any
type ProxyApplicantJSON map[string]any
type ProxyApproverJSON map[string]any
type AuditLogJSON map[string]any

type UserOpts struct {
	LastUsedOn   bool
	CustomFields bool
	UserType     bool
	// AllCustomFields includes custom fields that are not visible to everyone;
	// set it for callers holding the user-management permission.
	AllCustomFields bool
	// CustomFieldDefs are loaded on demand by User and Users when CustomFields is set.
	CustomFieldDefs []model.UserCustomField
}

func pf08Time(t time.Time) string { return kftime.Format(t) }
func pf08Object(v any) any {
	b, e := json.Marshal(v)
	if e != nil {
		return map[string]any{}
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return map[string]any{}
	}
	return out
}
func pf08Array[T any](xs []T) []any {
	out := make([]any, 0, len(xs))
	for _, x := range xs {
		out = append(out, pf08Object(x))
	}
	return out
}
