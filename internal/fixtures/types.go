// Package fixtures defines the stable JSON file contracts consumed by fixture loading.
package fixtures

import "punchflow/internal/model"

const (
	FileTenant             = "tenant.json"
	FileGrades             = "grades.json"
	FileUsers              = "users.json"
	FileOrganizationCharts = "organization-charts.json"
	FileRoles              = "roles.json"
	FileCategories         = "categories.json"
	FileFolders            = "folders.json"
	FileGeneralMasters     = "general-masters.json"
	FileRoutes             = "routes.json"
	FileWorkflows          = "workflows.json"
	FileProxyApplicants    = "proxy-applicants.json"
	FileProxyApprovers     = "proxy-approvers.json"
	FileTokens             = "tokens.json"
	FileWebhooks           = "webhooks.json"
)

type TenantFile struct {
	Tenant model.Tenant `json:"tenant"`
}
type GradeFixture struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	IsDefault bool   `json:"isDefault"`
}
type UserFixture struct {
	ID           string         `json:"id"`
	Code         string         `json:"code"`
	Email        string         `json:"email"`
	FirstName    string         `json:"firstName"`
	LastName     string         `json:"lastName"`
	EmployeeID   *string        `json:"employeeId,omitempty"`
	Status       string         `json:"status"`
	Locale       string         `json:"locale"`
	UserType     string         `json:"userType"`
	CustomFields map[string]any `json:"customFields,omitempty"`
}
type OrgChartFixture struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Current bool          `json:"current"`
	Teams   []TeamFixture `json:"teams"`
}
type TeamFixture struct {
	ID          string              `json:"id"`
	Code        string              `json:"code"`
	Name        string              `json:"name"`
	Notes       *string             `json:"notes,omitempty"`
	ParentCode  *string             `json:"parentCode,omitempty"`
	ApproveOnly bool                `json:"approveOnly"`
	Memberships []MembershipFixture `json:"memberships"`
}
type MembershipFixture struct {
	UserCode   string   `json:"userCode"`
	Leader     bool     `json:"leader"`
	GradeCodes []string `json:"gradeCodes"`
}
type RoleFixture struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Editable       bool                   `json:"editable"`
	PermissionList []model.RolePermission `json:"permissionList"`
	MemberCodes    []string               `json:"memberCodes"`
}
type CategoryFixture struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
type FolderFixture struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ParentCode  *string `json:"parentCode,omitempty"`
	Editable    bool    `json:"editable"`
}
type GeneralMasterFixture struct {
	ID                        string               `json:"id"`
	Code                      string               `json:"code"`
	Name                      string               `json:"name"`
	Description               *string              `json:"description,omitempty"`
	DefaultSortBy             string               `json:"defaultSortBy"`
	InitialDisplayCode        bool                 `json:"initialDisplayCode"`
	InitialDisplayCreatedAt   bool                 `json:"initialDisplayCreatedAt"`
	InitialDisplayDescription bool                 `json:"initialDisplayDescription"`
	Fields                    []MasterFieldFixture `json:"fields"`
	Items                     []MasterItemFixture  `json:"items"`
}
type MasterFieldFixture struct {
	ID             string   `json:"id"`
	Code           string   `json:"code"`
	Title          string   `json:"title"`
	Description    *string  `json:"description,omitempty"`
	FieldType      string   `json:"fieldType"`
	Required       bool     `json:"required"`
	FieldOrder     int      `json:"fieldOrder"`
	Visible        bool     `json:"visible"`
	InitialDisplay bool     `json:"initialDisplay"`
	Options        []string `json:"options,omitempty"`
}
type MasterItemFixture struct {
	ID          string               `json:"id"`
	Code        string               `json:"code"`
	Name        string               `json:"name"`
	Description *string              `json:"description,omitempty"`
	StartsOn    *string              `json:"startsOn,omitempty"`
	EndsOn      *string              `json:"endsOn,omitempty"`
	Inputs      []MasterInputFixture `json:"inputs"`
}
type MasterInputFixture struct {
	ID        string `json:"id"`
	FieldCode string `json:"fieldCode"`
	Value     any    `json:"value"`
}
type ProxyFixture struct {
	ID            string   `json:"id"`
	UserCode      string   `json:"userCode"`
	ProxyUserCode string   `json:"proxyUserCode"`
	StartsOn      *string  `json:"startsOn,omitempty"`
	EndsOn        *string  `json:"endsOn,omitempty"`
	WorkflowCodes []string `json:"workflowCodes"`
}
type TokenFixture struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	UserCode    string  `json:"userCode,omitempty"`
	Token       string  `json:"token"`
}
type WebhookFixture struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Secret        string   `json:"secret"`
	Enabled       bool     `json:"enabled"`
	WorkflowCodes []string `json:"workflowCodes"`
	EventTypes    []string `json:"eventTypes"`
}
type DefinitionsFile struct {
	Routes    []model.RouteDef    `json:"routes"`
	Workflows []model.WorkflowDef `json:"workflows"`
}
