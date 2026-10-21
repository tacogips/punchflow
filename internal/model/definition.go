package model

import (
	"strings"
	"time"
)

type Workflow struct {
	ID                  string    `json:"id"`
	Code                string    `json:"code"`
	Status              string    `json:"status"`
	FolderID            string    `json:"folderId"`
	DefinitionJSON      []byte    `json:"-"`
	TicketNumberCounter int64     `json:"ticketNumberCounter"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
type Route struct {
	ID             string    `json:"id"`
	Code           string    `json:"code"`
	Status         string    `json:"status"`
	FolderID       string    `json:"folderId"`
	DefinitionJSON []byte    `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
type WorkflowDef struct {
	ID                            string            `json:"id"`
	Code                          string            `json:"code"`
	Status                        string            `json:"status"`
	Name                          string            `json:"name"`
	Description                   string            `json:"description"`
	Notes                         string            `json:"notes"`
	FolderCode                    string            `json:"folderCode"`
	CategoryCodes                 []string          `json:"categoryCodes"`
	RouteCode                     string            `json:"routeCode"`
	PublicTicket                  bool              `json:"publicTicket"`
	VisibleToManager              string            `json:"visibleToManager"`
	VisibleToTeamMembers          bool              `json:"visibleToTeamMembers"`
	TitleDescription              *string           `json:"titleDescription"`
	TicketNumberFormat            *string           `json:"ticketNumberFormat"`
	Overwritable                  bool              `json:"overwritable"`
	TitleInputMode                string            `json:"titleInputMode"`
	TitleFormula                  *string           `json:"titleFormula"`
	AllowEditingOfViewers         bool              `json:"allowEditingOfViewers"`
	CommentingEnabled             bool              `json:"commentingEnabled"`
	CommentingRequiredOnApproval  bool              `json:"commentingRequiredOnApproval"`
	CommentingRequiredOnRejection bool              `json:"commentingRequiredOnRejection"`
	CommentingRequiredOnDenial    bool              `json:"commentingRequiredOnDenial"`
	AvailableToEveryone           bool              `json:"availableToEveryone"`
	AvailableTeams                []WorkflowTeam    `json:"availableTeams"`
	SectionList                   []WorkflowSection `json:"sectionList"`
	RouteConditions               []RouteCondition  `json:"routeConditions"`
	TicketNumberingTiming         string            `json:"ticketNumberingTiming"`
	// TicketNumberingSubStatus is the code of the sub-status that triggers
	// numbering when TicketNumberingTiming is sub_status_attached.
	TicketNumberingSubStatus        *string                  `json:"ticketNumberingSubStatus,omitempty"`
	ReportFormats                   []string                 `json:"reportFormats"`
	HiddenOnWorkflowFilterForTicket bool                     `json:"hiddenOnWorkflowFilterForTicket"`
	HiddenOnWorkflowSelectionScreen bool                     `json:"hiddenOnWorkflowSelectionScreen"`
	ApprovalCancellable             bool                     `json:"approvalCancellable"`
	ReportFileNameFormat            *string                  `json:"reportFileNameFormat"`
	CollectEmailOnExternalPublish   bool                     `json:"collectEmailOnExternalPublish"`
	NotifyGuestOnCompletion         bool                     `json:"notifyGuestOnCompletion"`
	AllowCustomSteps                bool                     `json:"allowCustomSteps"`
	TicketNumberKey                 *TicketNumberKey         `json:"ticketNumberKey"`
	TicketViewers                   []WorkflowViewer         `json:"ticketViewers"`
	Watchers                        []WorkflowWatcher        `json:"watchers"`
	ExcelTemplate                   *ExcelTemplate           `json:"excelTemplate"`
	CustomNumberings                []CustomNumbering        `json:"customNumberings"`
	CustomNumberingSettings         []CustomNumberingSetting `json:"customNumberingSettings"`
}
type WorkflowTeam struct {
	TeamCode    string   `json:"teamCode"`
	Descendants bool     `json:"descendants"`
	GradeSymbol *string  `json:"gradeSymbol"`
	GradeCodes  []string `json:"gradeCodes"`
}
type WorkflowExternalPublish struct {
	ID                  string `json:"id"`
	ExternalPublishHash string `json:"externalPublishHash"`
}
type TicketNumberKey struct {
	ID      string  `json:"id"`
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Notes   *string `json:"notes"`
	Default bool    `json:"default"`
}
type WorkflowWatcher struct {
	ID          string `json:"id"`
	WatcherType string `json:"watcherType"`
	UserCode    string `json:"userCode,omitempty"`
	TeamCode    string `json:"teamCode,omitempty"`
}
type WorkflowViewer struct {
	ID          string `json:"id"`
	UserCode    string `json:"userCode,omitempty"`
	TeamCode    string `json:"teamCode,omitempty"`
	GradeCode   string `json:"gradeCode,omitempty"`
	Descendants bool   `json:"descendants"`
}
type ExcelTemplate struct {
	ID                    string   `json:"id"`
	URL                   string   `json:"url"`
	RestrictedExport      bool     `json:"restrictedExport"`
	AllowedTicketStatuses []string `json:"allowedTicketStatuses"`
}
type WorkflowSection struct {
	ID                      string           `json:"id"`
	Code                    string           `json:"code,omitempty"`
	Title                   string           `json:"title"`
	SectionType             string           `json:"sectionType"`
	Conditional             bool             `json:"conditional,omitempty"`
	CombinationType         string           `json:"combinationType,omitempty"`
	CombinationExpression   string           `json:"combinationExpression,omitempty"`
	AllowApproverToAddItems bool             `json:"allowApproverToAddItems,omitempty"`
	FormFields              []FormField      `json:"formFields,omitempty"`
	SlipFields              []SlipField      `json:"slipFields,omitempty"`
	ConditionFields         []ConditionField `json:"conditionFields,omitempty"`
	// ViewRestriction limits who may view and enter the section contents.
	// When true, only the author (AllowViewAuthor), current or past approvers
	// (AllowViewApprover), the SectionViewers and ticket_read administrators
	// may see the section's inputs.
	ViewRestriction   bool            `json:"viewRestriction,omitempty"`
	AllowViewAuthor   bool            `json:"allowViewAuthor,omitempty"`
	AllowViewApprover bool            `json:"allowViewApprover,omitempty"`
	SectionViewers    []SectionViewer `json:"sectionViewers,omitempty"`
}

// SectionViewer designates a user, or a team (optionally narrowed by grade),
// allowed to view a restricted section. Exactly one of UserCode and TeamCode is set.
type SectionViewer struct {
	ID          string `json:"id,omitempty"`
	UserCode    string `json:"userCode,omitempty"`
	TeamCode    string `json:"teamCode,omitempty"`
	GradeCode   string `json:"gradeCode,omitempty"`
	Descendants bool   `json:"descendants,omitempty"`
}
type FormField struct {
	ID                           string                 `json:"id"`
	Code                         string                 `json:"code"`
	Title                        string                 `json:"title"`
	Description                  *string                `json:"description"`
	FieldType                    string                 `json:"fieldType"`
	Required                     bool                   `json:"required"`
	Approver                     bool                   `json:"approver"`
	Author                       bool                   `json:"author"`
	Options                      []string               `json:"options"`
	Size                         string                 `json:"size"`
	RegexpFormat                 *string                `json:"regexpFormat"`
	Formula                      *string                `json:"formula"`
	DefaultValue                 *string                `json:"defaultValue"`
	MinValue                     *float64               `json:"minValue"`
	MaxValue                     *float64               `json:"maxValue"`
	MinLength                    *int                   `json:"minLength"`
	MaxLength                    *int                   `json:"maxLength"`
	DecimalDigit                 *int                   `json:"decimalDigit"`
	Delimited                    *bool                  `json:"delimited"`
	Prefix                       *string                `json:"prefix"`
	Suffix                       *string                `json:"suffix"`
	Hidden                       *bool                  `json:"hidden"`
	ReadonlyOnUI                 *bool                  `json:"readonlyOnUi"`
	Multiple                     bool                   `json:"multiple"`
	Orientation                  string                 `json:"orientation"`
	AllowedExtensions            []string               `json:"allowedExtensions"`
	AutoLink                     bool                   `json:"autoLink"`
	UseTodayForDefaultValue      bool                   `json:"useTodayForDefaultValue"`
	AllowedTicketStatus          []string               `json:"allowedTicketStatus"`
	GeneralMasterCode            string                 `json:"generalMasterCode,omitempty"`
	DefaultGeneralMasterItemCode string                 `json:"defaultGeneralMasterItemCode,omitempty"`
	CustomValidations            []FormCustomValidation `json:"customValidations,omitempty"`
}
type SlipField struct {
	ID                           string                 `json:"id"`
	Code                         string                 `json:"code"`
	Title                        string                 `json:"title"`
	FieldType                    string                 `json:"fieldType"`
	Required                     bool                   `json:"required"`
	ShowTotal                    bool                   `json:"showTotal"`
	Options                      []string               `json:"options"`
	RegexpFormat                 *string                `json:"regexpFormat"`
	Formula                      *string                `json:"formula"`
	MaxValue                     *float64               `json:"maxValue"`
	MinValue                     *float64               `json:"minValue"`
	DefaultValue                 *string                `json:"defaultValue"`
	DecimalDigit                 *int                   `json:"decimalDigit"`
	Delimited                    *bool                  `json:"delimited"`
	AllowedExtensions            []string               `json:"allowedExtensions"`
	Prefix                       *string                `json:"prefix"`
	Suffix                       *string                `json:"suffix"`
	Approver                     bool                   `json:"approver"`
	Author                       bool                   `json:"author"`
	Hidden                       *bool                  `json:"hidden"`
	ReadonlyOnUI                 *bool                  `json:"readonlyOnUi"`
	MinLength                    *int                   `json:"minLength"`
	MaxLength                    *int                   `json:"maxLength"`
	Multiple                     bool                   `json:"multiple"`
	AutoLink                     bool                   `json:"autoLink"`
	UseTodayForDefaultValue      bool                   `json:"useTodayForDefaultValue"`
	AllowedTicketStatus          []string               `json:"allowedTicketStatus"`
	GeneralMasterCode            string                 `json:"generalMasterCode,omitempty"`
	DefaultGeneralMasterItemCode string                 `json:"defaultGeneralMasterItemCode,omitempty"`
	CustomValidations            []SlipCustomValidation `json:"customValidations,omitempty"`
}

// FormCustomValidation describes a validation attached to a form field.
// It fires when the configured field conditions match.
type FormCustomValidation struct {
	ID              string                      `json:"id"`
	CombinationType string                      `json:"combinationType"`
	ErrorMessage    string                      `json:"errorMessage"`
	Fields          []FormCustomValidationField `json:"fields"`
}

// FormCustomValidationField identifies a form field condition by fixture code.
type FormCustomValidationField struct {
	ID            string  `json:"id"`
	FormFieldCode string  `json:"formFieldCode"`
	Symbol        string  `json:"symbol"`
	Value         *string `json:"value"`
}

// SlipCustomValidation describes a validation attached to a detail field.
type SlipCustomValidation struct {
	ID              string                      `json:"id"`
	CombinationType string                      `json:"combinationType"`
	ErrorMessage    string                      `json:"errorMessage"`
	Fields          []SlipCustomValidationField `json:"fields"`
}

// SlipCustomValidationField identifies a detail field condition by fixture code.
type SlipCustomValidationField struct {
	ID            string  `json:"id"`
	SlipFieldCode string  `json:"slipFieldCode"`
	Symbol        string  `json:"symbol"`
	Value         *string `json:"value"`
}

type ConditionField struct {
	ID                    string `json:"id"`
	Variable              string `json:"variable"`
	FieldKey              string `json:"fieldKey"`
	Symbol                string `json:"symbol"`
	Value                 any    `json:"value,omitempty"`
	GeneralMasterCode     string `json:"generalMasterCode,omitempty"`
	GeneralMasterItemCode string `json:"generalMasterItemCode,omitempty"`
}
type RouteCondition struct {
	ID                    string           `json:"id"`
	ConditionType         string           `json:"conditionType"`
	CombinationType       string           `json:"combinationType"`
	CombinationExpression string           `json:"combinationExpression"`
	RouteCode             *string          `json:"routeCode"`
	ErrorMessage          *string          `json:"errorMessage"`
	ConditionFields       []ConditionField `json:"conditionFields"`
}
type RouteDef struct {
	ID          string      `json:"id"`
	Code        string      `json:"code"`
	Status      string      `json:"status"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Notes       string      `json:"notes"`
	FolderCode  string      `json:"folderCode"`
	Steps       []RouteStep `json:"steps"`
}
type RouteStep struct {
	ID                            string              `json:"id"`
	Code                          string              `json:"code"`
	StepOrder                     int                 `json:"stepOrder"`
	Title                         string              `json:"title"`
	StepType                      string              `json:"stepType"`
	ActionType                    string              `json:"actionType"`
	Instruction                   *string             `json:"instruction"`
	RequiredApprovalsNumber       int                 `json:"requiredApprovalsNumber"`
	RequiredApprovalsPercent      int                 `json:"requiredApprovalsPercent"`
	FallbackType                  *string             `json:"fallbackType"`
	AllowSelfApproval             bool                `json:"allowSelfApproval"`
	ManagerSelectionType          string              `json:"managerSelectionType"`
	AllowRaising                  bool                `json:"allowRaising"`
	MinCustomAssignees            *int                `json:"minCustomAssignees"`
	ApproverAssignmentInstruction *string             `json:"approverAssignmentInstruction"`
	UserCodes                     []string            `json:"userCodes"`
	Targets                       []RouteTarget       `json:"targets"`
	RouteStepCondition            *RouteStepCondition `json:"routeStepCondition"`
	SubStatus                     *string             `json:"subStatus"`
}
type RouteTarget struct {
	TeamCode    string   `json:"teamCode,omitempty"`
	Descendants bool     `json:"descendants,omitempty"`
	GradeSymbol *string  `json:"gradeSymbol"`
	GradeCodes  []string `json:"gradeCodes,omitempty"`
	Variable    string   `json:"variable,omitempty"`
}
type RouteStepCondition struct {
	ID                       string           `json:"id"`
	ConditionType            string           `json:"conditionType"`
	CombinationType          string           `json:"combinationType"`
	RouteStepConditionFields []ConditionField `json:"routeStepConditionFields"`
}

// VariableName normalizes a form-field variable reference used by route and
// condition definitions. The documented syntax is "$field_code"; the
// "${field_code}" form and the bare field code are accepted as well. It is
// shared by the engine (value lookup) and the presenter (form field lookup) so
// both resolve the same field.
func VariableName(variable string) string {
	name := strings.TrimSpace(variable)
	name = strings.TrimPrefix(name, "$")
	if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
		name = strings.TrimSuffix(strings.TrimPrefix(name, "{"), "}")
	}
	return name
}
