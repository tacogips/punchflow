package model

const (
	TicketStatusDraft      = "draft"
	TicketStatusInProgress = "in_progress"
	TicketStatusCompleted  = "completed"
	TicketStatusRejected   = "rejected"
	TicketStatusDenied     = "denied"
	TicketStatusArchived   = "archived"

	StepStatusPending    = "pending"
	StepStatusInProgress = "in_progress"
	StepStatusCompleted  = "completed"
	StepStatusSkipped    = "skipped"
	// StepStatusRaised marks a step that was bypassed by raising (PF-19).
	StepStatusRaised = "raised"

	ActionTypeApprove = "approve"
	ActionTypeConfirm = "confirm"
	ActionTypeNone    = "none"

	StepTypeAuthor               = "author"
	StepTypeManager              = "manager"
	StepTypeTeam                 = "team"
	StepTypeUser                 = "user"
	StepTypeAuthorCustomizable   = "author_customizable"
	StepTypeAssigneeCustomizable = "assignee_customizable"
	StepTypeDynamicTeam          = "dynamic_team"
	StepTypeDynamicUser          = "dynamic_user"

	FallbackDirectManager = "direct_manager"
	FallbackHigherManager = "higher_manager"
	FallbackSkip          = "skip"
	FallbackNoFallback    = "no_fallback"
	FallbackHigherTeam    = "higher_team"

	ManagerSelectionClosest = "closest"
	ManagerSelectionAll     = "all"

	GradeSymbolEqual              = "equal"
	GradeSymbolGreaterThan        = "greater_than"
	GradeSymbolGreaterThanOrEqual = "greater_than_or_equal"
	GradeSymbolLessThan           = "less_than"
	GradeSymbolLessThanOrEqual    = "less_than_or_equal"
	GradeSymbolAnyOf              = "any_of"

	StepConditionTypeAlways          = "always"
	StepConditionTypeConditional     = "conditional"
	StepConditionTypeConditionalSkip = "conditional_skip"
	StepCombinationAll               = "all"
	StepCombinationAny               = "any"

	StepConditionFieldKeyAuthorGrade           = "author_grade"
	StepConditionFieldKeyAuthorTeam            = "author_team"
	StepConditionFieldKeyTextVariable          = "text_variable"
	StepConditionFieldKeyNumberVariable        = "number_variable"
	StepConditionFieldKeyCheckboxVariable      = "checkbox_variable"
	StepConditionFieldKeyGeneralMasterVariable = "general_master_variable"

	RouteConditionTypeAlways         = "always"
	RouteConditionTypeField          = "field"
	RouteConditionTypeFieldOtherwise = "field_otherwise"
	RouteCombinationAll              = "all"
	RouteCombinationAny              = "any"
	RouteCombinationCustom           = "custom"

	SectionTypeForm = "form"
	SectionTypeSlip = "slip"

	RouteStatusVisible = "visible"
	RouteStatusDeleted = "deleted"
	RouteStatusError   = "error"

	WorkflowStatusVisible   = "visible"
	WorkflowStatusInvisible = "invisible"
	WorkflowStatusDeleted   = "deleted"

	UserStatusInvited     = "invited"
	UserStatusActive      = "activated"
	UserStatusSuspended   = "suspended"
	UserStatusDeactivated = "deactivated"

	UserTypeNormal    = "normal"
	UserTypeAssistant = "assistant"

	PermissionTenant       = "tenant"
	PermissionBilling      = "billing"
	PermissionIntegration  = "integration"
	PermissionSecurity     = "security"
	PermissionAudit        = "audit"
	PermissionStats        = "stats"
	PermissionWorkflow     = "workflow"
	PermissionRoute        = "route"
	PermissionPipeline     = "pipeline"
	PermissionWorkflowMisc = "workflow_misc"
	PermissionUser         = "user"
	PermissionTeam         = "team"
	PermissionRole         = "role"
	PermissionMaster       = "master"
	PermissionMasterRead   = "master_read"
	PermissionTicketRead   = "ticket_read"
	PermissionTicketWrite  = "ticket_write"
	PermissionLabel        = "label"
	PermissionAutomation   = "automation"

	MasterFieldTypeText     = "text"
	MasterFieldTypeTextLong = "text_long"
	MasterFieldTypeNumber   = "number"
	MasterFieldTypeInteger  = "integer"
	MasterFieldTypeCheckbox = "checkbox"
	MasterFieldTypePullDown = "pull_down"
	MasterFieldTypeDate     = "date"

	MasterSortKeyName = "name"
	MasterSortKeyCode = "code"

	WebhookEventPing            = "ping"
	WebhookEventTicketCreated   = "ticket_created"
	WebhookEventTicketUpdated   = "ticket_updated"
	WebhookEventTicketOpened    = "ticket_opened"
	WebhookEventTicketApproved  = "ticket_approved"
	WebhookEventTicketConfirmed = "ticket_confirmed"
	WebhookEventTicketRejected  = "ticket_rejected"
	WebhookEventTicketDenied    = "ticket_denied"
	WebhookEventTicketCompleted = "ticket_completed"
	WebhookEventTicketWithdrawn = "ticket_withdrawn"
	WebhookEventTicketArchived  = "ticket_archived"
	WebhookEventCommentCreated  = "comment_created"
	WebhookEventCommentUpdated  = "comment_updated"

	TenantPlanStandard   = "standard"
	TenantPlanEnterprise = "enterprise"
	TenantPlanTrial      = "trial"

	TenantModeNormal       = "normal"
	TenantModeAdminWorking = "admin_working"
	TenantModeMaintenance  = "maintenance"
	TenantModeSuspended    = "suspended"

	TokenKindPAT = "pat"
	TokenKindSAT = "sat"

	WebhookStatusPending   = "pending"
	WebhookStatusSucceeded = "succeeded"
	WebhookStatusFailed    = "failed"
)

var TicketStatuses = []string{TicketStatusDraft, TicketStatusInProgress, TicketStatusCompleted, TicketStatusRejected, TicketStatusDenied, TicketStatusArchived}
var StepStatuses = []string{StepStatusPending, StepStatusInProgress, StepStatusCompleted, StepStatusSkipped, StepStatusRaised}
var ActionTypes = []string{ActionTypeApprove, ActionTypeConfirm, ActionTypeNone}
var StepTypes = []string{StepTypeAuthor, StepTypeManager, StepTypeTeam, StepTypeUser, StepTypeAuthorCustomizable, StepTypeAssigneeCustomizable, StepTypeDynamicTeam, StepTypeDynamicUser}
var FallbackTypes = []string{FallbackDirectManager, FallbackHigherManager, FallbackSkip, FallbackNoFallback, FallbackHigherTeam}
var ManagerSelectionTypes = []string{ManagerSelectionClosest, ManagerSelectionAll}
var GradeSymbols = []string{GradeSymbolEqual, GradeSymbolGreaterThan, GradeSymbolGreaterThanOrEqual, GradeSymbolLessThan, GradeSymbolLessThanOrEqual, GradeSymbolAnyOf}
var StepConditionTypes = []string{StepConditionTypeAlways, StepConditionTypeConditional, StepConditionTypeConditionalSkip}
var StepCombinationTypes = []string{StepCombinationAll, StepCombinationAny}
var StepConditionFieldKeys = []string{StepConditionFieldKeyAuthorGrade, StepConditionFieldKeyAuthorTeam, StepConditionFieldKeyTextVariable, StepConditionFieldKeyNumberVariable, StepConditionFieldKeyCheckboxVariable, StepConditionFieldKeyGeneralMasterVariable}
var RouteConditionTypes = []string{RouteConditionTypeAlways, RouteConditionTypeField, RouteConditionTypeFieldOtherwise}
var RouteCombinationTypes = []string{RouteCombinationAll, RouteCombinationAny, RouteCombinationCustom}
var SectionCombinationTypes = []string{RouteCombinationAll, RouteCombinationAny, RouteCombinationCustom}
var SectionTypes = []string{SectionTypeForm, SectionTypeSlip}
var RouteStatuses = []string{RouteStatusVisible, RouteStatusDeleted, RouteStatusError}
var WorkflowStatuses = []string{WorkflowStatusVisible, WorkflowStatusInvisible, WorkflowStatusDeleted}
var UserStatuses = []string{UserStatusInvited, UserStatusActive, UserStatusSuspended, UserStatusDeactivated}
var UserTypes = []string{UserTypeNormal, UserTypeAssistant}
var Permissions = []string{PermissionTenant, PermissionBilling, PermissionIntegration, PermissionSecurity, PermissionAudit, PermissionStats, PermissionWorkflow, PermissionRoute, PermissionPipeline, PermissionWorkflowMisc, PermissionUser, PermissionTeam, PermissionRole, PermissionMaster, PermissionMasterRead, PermissionTicketRead, PermissionTicketWrite, PermissionLabel, PermissionAutomation}
var MasterFieldTypes = []string{MasterFieldTypeText, MasterFieldTypeTextLong, MasterFieldTypeNumber, MasterFieldTypeInteger, MasterFieldTypeCheckbox, MasterFieldTypePullDown, MasterFieldTypeDate}
var MasterSortKeys = []string{MasterSortKeyName, MasterSortKeyCode}

const (
	FormFieldSizeFull = "full"
	FormFieldSizeHalf = "half"

	ForcedPublicTypeFollowWorkflow = "follow_workflow"
	ForcedPublicTypeForcedPublic   = "forced_public"
	ForcedPublicTypeForcedPrivate  = "forced_private"
)

var FormFieldSizes = []string{FormFieldSizeFull, FormFieldSizeHalf}
var ForcedPublicTypes = []string{ForcedPublicTypeFollowWorkflow, ForcedPublicTypeForcedPublic, ForcedPublicTypeForcedPrivate}
var WebhookEventTypes = []string{WebhookEventPing, WebhookEventTicketCreated, WebhookEventTicketUpdated, WebhookEventTicketOpened, WebhookEventTicketApproved, WebhookEventTicketConfirmed, WebhookEventTicketRejected, WebhookEventTicketDenied, WebhookEventTicketCompleted, WebhookEventTicketWithdrawn, WebhookEventTicketArchived, WebhookEventCommentCreated, WebhookEventCommentUpdated}
