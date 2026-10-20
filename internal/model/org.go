package model

import "time"

type OrganizationChart struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Team struct {
	ID                  string    `json:"id"`
	OrganizationChartID string    `json:"organizationChartId"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	Notes               *string   `json:"notes"`
	ParentID            *string   `json:"parentId"`
	ApproveOnly         bool      `json:"approveOnly"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
type TeamMembership struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"teamId"`
	UserID    string    `json:"userId"`
	Leader    bool      `json:"leader"`
	GradeIDs  []string  `json:"gradeIds"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ProxyApplicant and ProxyApprover store delegation registrations.
type ProxyApplicant struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	ProxyUserID string    `json:"proxyUserId"`
	StartsOn    *string   `json:"startsOn"`
	EndsOn      *string   `json:"endsOn"`
	WorkflowIDs []string  `json:"workflowIds"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type ProxyApprover struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	ProxyUserID string    `json:"proxyUserId"`
	StartsOn    *string   `json:"startsOn"`
	EndsOn      *string   `json:"endsOn"`
	WorkflowIDs []string  `json:"workflowIds"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
