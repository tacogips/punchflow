package fixtures

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"punchflow/internal/model"
)

var pf06UUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// ticketNumberingTimings lists the accepted ticketNumberingTiming values (the
// kickflow TicketNumberingTiming.timingType enum); an empty value defaults to
// "opened".
var ticketNumberingTimings = []string{"created", "opened", "completed", model.TicketNumberingTimingSubStatusAttached}

// Validate checks fixture structure and all cross-file references.
func Validate(s *Set) error {
	if s == nil {
		return &ValidationError{Problems: []Problem{{File: "", Path: "$", Message: "fixture set is nil"}}}
	}
	var ps []Problem
	add := func(file, path, msg string) { ps = append(ps, Problem{File: file, Path: path, Message: msg}) }
	ids := map[string]string{}
	registerID := func(file, path, id string) {
		if !pf06UUIDPattern.MatchString(id) {
			add(file, path, "id must be a UUID")
		}
		if prior, ok := ids[id]; ok {
			add(file, path, "duplicate id (also used at "+prior+")")
		} else if id != "" {
			ids[id] = file + ":" + path
		}
	}

	// Validate every persisted identifier, including IDs nested in definitions.
	for _, item := range []struct {
		file  string
		value any
	}{
		{FileTenant, s.Tenant}, {FileGrades, s.Grades}, {FileUsers, s.Users}, {FileUserCustomFields, s.UserCustomFields}, {FileOrganizationCharts, s.OrgCharts},
		{FileRoles, s.Roles}, {FileCategories, s.Categories}, {FileFolders, s.Folders}, {FileGeneralMasters, s.GeneralMasters},
		{FileRoutes, s.Routes}, {FileWorkflows, s.Workflows}, {FileProxyApplicants, s.ProxyApplicants},
		{FileProxyApprovers, s.ProxyApprovers}, {FileTokens, s.Tokens}, {FileWebhooks, s.Webhooks},
		{FileLabels, s.Labels}, {FileSubStatuses, s.SubStatuses},
	} {
		pf06WalkIDs(reflect.ValueOf(item.value), item.file, "$", registerID)
	}
	if !pf06OneOf(s.Tenant.Tenant.Plan, model.TenantPlanStandard, model.TenantPlanEnterprise) {
		add(FileTenant, "tenant.plan", "invalid plan")
	}
	if !pf06OneOf(s.Tenant.Tenant.Mode, model.TenantModeNormal, model.TenantModeAdminWorking, model.TenantModeMaintenance, model.TenantModeSuspended) {
		add(FileTenant, "tenant.mode", "invalid mode")
	}

	userIDs, userCodes, gradeCodes, teamCodes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	categoryCodes, folderCodes, masterCodes, routeCodes, workflowCodes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, x := range s.Users {
		userIDs[x.ID], userCodes[x.Code] = true, true
	}
	for _, x := range s.Grades {
		gradeCodes[x.Code] = true
	}
	for _, x := range s.Categories {
		categoryCodes[x.Code] = true
	}
	for _, x := range s.Folders {
		folderCodes[x.Code] = true
	}
	for _, x := range s.GeneralMasters {
		masterCodes[x.Code] = true
	}
	for _, x := range s.Routes {
		routeCodes[x.Code] = true
	}
	for _, x := range s.Workflows {
		workflowCodes[x.Code] = true
	}
	checkUnique := func(file string, entries []pf06CodePath) {
		seen := map[string]string{}
		for _, e := range entries {
			if e.code == "" {
				add(file, e.path, "code is required")
				continue
			}
			if p, ok := seen[e.code]; ok {
				add(file, e.path, "duplicate code (also used at "+p+")")
			} else {
				seen[e.code] = e.path
			}
		}
	}
	gradeEntries := make([]pf06CodePath, 0, len(s.Grades))
	for i, x := range s.Grades {
		gradeEntries = append(gradeEntries, pf06CodePath{x.Code, fmt.Sprintf("[%d].code", i)})
	}
	checkUnique(FileGrades, gradeEntries)
	customDefs := map[string]model.UserCustomField{}
	customEntries := make([]pf06CodePath, 0, len(s.UserCustomFields))
	for i, x := range s.UserCustomFields {
		customEntries = append(customEntries, pf06CodePath{x.Code, fmt.Sprintf("[%d].code", i)})
		def := model.UserCustomField{ID: x.ID, Code: x.Code, Name: x.Name, FieldType: x.FieldType, Options: x.Options, VisibleToEveryone: x.VisibleToEveryone}
		customDefs[x.Code] = def
		if strings.TrimSpace(x.Name) == "" {
			add(FileUserCustomFields, fmt.Sprintf("[%d].name", i), "name is required")
		}
		if !pf06Contains(model.UserCustomFieldTypes, x.FieldType) {
			add(FileUserCustomFields, fmt.Sprintf("[%d].fieldType", i), "invalid user custom field type")
		}
		if (x.FieldType == model.UserCustomFieldTypeCheckbox || x.FieldType == model.UserCustomFieldTypePullDown) && len(x.Options) == 0 {
			add(FileUserCustomFields, fmt.Sprintf("[%d].options", i), "options are required for checkbox and pullDown fields")
		}
	}
	checkUnique(FileUserCustomFields, customEntries)
	userEntries := make([]pf06CodePath, 0, len(s.Users))
	for i, x := range s.Users {
		userEntries = append(userEntries, pf06CodePath{x.Code, fmt.Sprintf("[%d].code", i)})
		for code, value := range x.CustomFields {
			path := fmt.Sprintf("[%d].customFields.%s", i, code)
			def, ok := customDefs[code]
			if !ok {
				add(FileUsers, path, "unknown user custom field code")
			} else if _, err := def.NormalizeValue(value); err != nil {
				add(FileUsers, path, err.Error())
			}
		}
		if !pf06Contains(model.UserStatuses, x.Status) {
			add(FileUsers, fmt.Sprintf("[%d].status", i), "invalid user status")
		}
		if !pf06Contains(model.UserTypes, x.UserType) {
			add(FileUsers, fmt.Sprintf("[%d].userType", i), "invalid userType")
		}
	}
	checkUnique(FileUsers, userEntries)
	for _, group := range []struct {
		file    string
		entries []pf06CodePath
	}{
		{FileCategories, pf06MapCodes(len(s.Categories), func(i int) pf06CodePath { return pf06CodePath{s.Categories[i].Code, fmt.Sprintf("[%d].code", i)} })},
		{FileFolders, pf06MapCodes(len(s.Folders), func(i int) pf06CodePath { return pf06CodePath{s.Folders[i].Code, fmt.Sprintf("[%d].code", i)} })},
		{FileGeneralMasters, pf06MapCodes(len(s.GeneralMasters), func(i int) pf06CodePath { return pf06CodePath{s.GeneralMasters[i].Code, fmt.Sprintf("[%d].code", i)} })},
		{FileRoutes, pf06MapCodes(len(s.Routes), func(i int) pf06CodePath { return pf06CodePath{s.Routes[i].Code, fmt.Sprintf("[%d].code", i)} })},
		{FileWorkflows, pf06MapCodes(len(s.Workflows), func(i int) pf06CodePath { return pf06CodePath{s.Workflows[i].Code, fmt.Sprintf("[%d].code", i)} })},
	} {
		checkUnique(group.file, group.entries)
	}
	if n := pf06CountCurrent(s.OrgCharts); n != 1 {
		add(FileOrganizationCharts, "$", "exactly one organization chart must be current")
	}
	for ci, chart := range s.OrgCharts {
		seen := map[string]bool{}
		for ti, t := range chart.Teams {
			path := fmt.Sprintf("[%d].teams[%d]", ci, ti)
			if t.Code == "" {
				add(FileOrganizationCharts, path+".code", "code is required")
			}
			if seen[t.Code] {
				add(FileOrganizationCharts, path+".code", "duplicate team code in chart")
			}
			seen[t.Code] = true
			teamCodes[t.Code] = true
			for mi, m := range t.Memberships {
				mp := fmt.Sprintf("%s.memberships[%d]", path, mi)
				if !userCodes[m.UserCode] {
					add(FileOrganizationCharts, mp+".userCode", "unknown user code")
				}
				for gi, g := range m.GradeCodes {
					if !gradeCodes[g] {
						add(FileOrganizationCharts, fmt.Sprintf("%s.gradeCodes[%d]", mp, gi), "unknown grade code")
					}
				}
			}
		}
		pf06CheckTree(FileOrganizationCharts, chart.Teams, func(t TeamFixture) string { return t.Code }, func(t TeamFixture) *string { return t.ParentCode }, add)
	}
	if pf06CountSuper(s.Roles) != 1 {
		add(FileRoles, "$", "exactly one role must have editable false")
	}
	for i, r := range s.Roles {
		if !r.Editable && len(r.MemberCodes) == 0 {
			add(FileRoles, fmt.Sprintf("[%d].memberCodes", i), "super admin role needs at least one member")
		}
		for j, c := range r.MemberCodes {
			if !userCodes[c] {
				add(FileRoles, fmt.Sprintf("[%d].memberCodes[%d]", i, j), "unknown user code")
			}
		}
		for j, p := range r.PermissionList {
			if !pf06Contains(model.Permissions, p.Permission) {
				add(FileRoles, fmt.Sprintf("[%d].permissionList[%d].permission", i, j), "invalid permission")
			}
		}
	}
	rootFolders := 0
	for i, f := range s.Folders {
		if f.ParentCode == nil {
			rootFolders++
			if f.Editable {
				add(FileFolders, fmt.Sprintf("[%d].editable", i), "root folder must not be editable")
			}
		} else if !folderCodes[*f.ParentCode] {
			add(FileFolders, fmt.Sprintf("[%d].parentCode", i), "unknown parent folder code")
		}
	}
	if rootFolders != 1 {
		add(FileFolders, "$", "exactly one root folder is required")
	}
	pf06CheckTree(FileFolders, s.Folders, func(f FolderFixture) string { return f.Code }, func(f FolderFixture) *string { return f.ParentCode }, add)
	for i, m := range s.GeneralMasters {
		if !pf06Contains(model.MasterSortKeys, m.DefaultSortBy) {
			add(FileGeneralMasters, fmt.Sprintf("[%d].defaultSortBy", i), "invalid default sort")
		}
		fieldCodes := map[string]bool{}
		fieldEntries := make([]pf06CodePath, 0, len(m.Fields))
		for j, f := range m.Fields {
			p := fmt.Sprintf("[%d].fields[%d]", i, j)
			fieldCodes[f.Code] = true
			fieldEntries = append(fieldEntries, pf06CodePath{f.Code, p + ".code"})
			if !pf06Contains(model.MasterFieldTypes, f.FieldType) {
				add(FileGeneralMasters, p+".fieldType", "invalid field type")
			}
		}
		checkUnique(FileGeneralMasters, fieldEntries)
		itemSeen := map[string]bool{}
		for j, it := range m.Items {
			p := fmt.Sprintf("[%d].items[%d]", i, j)
			if itemSeen[it.Code] {
				add(FileGeneralMasters, p+".code", "duplicate item code in master")
			}
			itemSeen[it.Code] = true
			for k, in := range it.Inputs {
				if !fieldCodes[in.FieldCode] {
					add(FileGeneralMasters, fmt.Sprintf("%s.inputs[%d].fieldCode", p, k), "unknown field code")
				}
			}
		}
	}
	for i, r := range s.Routes {
		if !pf06Contains(model.RouteStatuses, r.Status) {
			add(FileRoutes, fmt.Sprintf("[%d].status", i), "invalid status")
		}
		if !folderCodes[r.FolderCode] {
			add(FileRoutes, fmt.Sprintf("[%d].folderCode", i), "unknown folder code")
		}
		stepSeen := map[string]bool{}
		for j, st := range r.Steps {
			p := fmt.Sprintf("[%d].steps[%d]", i, j)
			if stepSeen[st.Code] {
				add(FileRoutes, p+".code", "duplicate step code")
			}
			stepSeen[st.Code] = true
			if st.RequiredApprovalsNumber > 0 == (st.RequiredApprovalsPercent > 0) {
				add(FileRoutes, p, "exactly one approval threshold must be positive")
			}
			if st.RequiredApprovalsNumber < 0 || st.RequiredApprovalsPercent < 0 || st.RequiredApprovalsPercent > 100 {
				add(FileRoutes, p, "invalid approval threshold")
			}
			if !pf06Contains(model.StepTypes, st.StepType) {
				add(FileRoutes, p+".stepType", "invalid step type")
			}
			if !pf06Contains(model.ActionTypes, st.ActionType) {
				add(FileRoutes, p+".actionType", "invalid action type")
			}
			if st.FallbackType != nil && !pf06Contains(model.FallbackTypes, *st.FallbackType) {
				add(FileRoutes, p+".fallbackType", "invalid fallbackType")
			}
			if st.ManagerSelectionType != "" && !pf06Contains(model.ManagerSelectionTypes, st.ManagerSelectionType) {
				add(FileRoutes, p+".managerSelectionType", "invalid managerSelectionType")
			}
			if st.RouteStepCondition != nil {
				if !pf06Contains(model.StepConditionTypes, st.RouteStepCondition.ConditionType) {
					add(FileRoutes, p+".routeStepCondition.conditionType", "invalid condition type")
				}
				if !pf06Contains(model.StepCombinationTypes, st.RouteStepCondition.CombinationType) {
					add(FileRoutes, p+".routeStepCondition.combinationType", "invalid combination type")
				}
				for k, cf := range st.RouteStepCondition.RouteStepConditionFields {
					pf := fmt.Sprintf("%s.routeStepCondition.routeStepConditionFields[%d]", p, k)
					if cf.FieldKey == "other_variable" {
						add(FileRoutes, pf+".fieldKey", "other_variable is unsupported")
					} else if !pf06Contains(model.StepConditionFieldKeys, cf.FieldKey) {
						add(FileRoutes, pf+".fieldKey", "invalid or unsupported field key")
					}
					if !pf06OneOf(cf.Symbol, "equal", "not_equal", "greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal", "include", "exclude", "is_empty", "is_not_empty", "descendants_or_equal", "not_descendants_or_equal") {
						add(FileRoutes, pf+".symbol", "invalid condition symbol")
					}
				}
			}
			for k, code := range st.UserCodes {
				if !userCodes[code] {
					add(FileRoutes, fmt.Sprintf("%s.userCodes[%d]", p, k), "unknown user code")
				}
			}
			for k, t := range st.Targets {
				if t.GradeSymbol != nil && !pf06Contains(model.GradeSymbols, *t.GradeSymbol) {
					add(FileRoutes, fmt.Sprintf("%s.targets[%d].gradeSymbol", p, k), "invalid gradeSymbol")
				}
				if t.TeamCode != "" && !teamCodes[t.TeamCode] {
					add(FileRoutes, fmt.Sprintf("%s.targets[%d].teamCode", p, k), "unknown team code")
				}
				for n, g := range t.GradeCodes {
					if !gradeCodes[g] {
						add(FileRoutes, fmt.Sprintf("%s.targets[%d].gradeCodes[%d]", p, k, n), "unknown grade code")
					}
				}
			}
		}
	}
	for i, w := range s.Workflows {
		p := fmt.Sprintf("[%d]", i)
		if !pf06Contains(model.WorkflowStatuses, w.Status) {
			add(FileWorkflows, p+".status", "invalid status")
		}
		if w.TicketNumberingTiming != "" && !pf06Contains(ticketNumberingTimings, w.TicketNumberingTiming) {
			add(FileWorkflows, p+".ticketNumberingTiming", "invalid ticket numbering timing")
		}
		if !folderCodes[w.FolderCode] {
			add(FileWorkflows, p+".folderCode", "unknown folder code")
		}
		if !routeCodes[w.RouteCode] {
			add(FileWorkflows, p+".routeCode", "unknown route code")
		}
		for j, c := range w.CategoryCodes {
			if !categoryCodes[c] {
				add(FileWorkflows, fmt.Sprintf("%s.categoryCodes[%d]", p, j), "unknown category code")
			}
		}
		if len(w.RouteConditions) == 0 {
			add(FileWorkflows, p+".routeConditions", "must not be empty")
		} else {
			last := w.RouteConditions[len(w.RouteConditions)-1].ConditionType
			if last != "always" && last != "field_otherwise" {
				add(FileWorkflows, p+".routeConditions", "must end with always or field_otherwise")
			}
		}
		fields := map[string]bool{}
		for si, sec := range w.SectionList {
			sp := fmt.Sprintf("%s.sectionList[%d]", p, si)
			if !pf06Contains(model.SectionTypes, sec.SectionType) {
				add(FileWorkflows, sp+".sectionType", "invalid sectionType")
			}
			if sec.Conditional {
				if sec.SectionType != model.SectionTypeForm {
					add(FileWorkflows, sp+".sectionType", "conditional section must be form")
				}
				if sec.CombinationType == "custom" {
					add(FileWorkflows, sp+".combinationType", "combinationType custom is unsupported")
				} else if !pf06Contains(model.StepCombinationTypes, sec.CombinationType) {
					add(FileWorkflows, sp+".combinationType", "invalid combinationType")
				}
			}
			if (sec.ViewRestriction || len(sec.SectionViewers) > 0) && sec.SectionType != model.SectionTypeForm {
				add(FileWorkflows, sp+".viewRestriction", "only form sections support view restrictions")
			}
			for vi, v := range sec.SectionViewers {
				vp := fmt.Sprintf("%s.sectionViewers[%d]", sp, vi)
				if (v.UserCode == "") == (v.TeamCode == "") {
					add(FileWorkflows, vp, "exactly one of userCode and teamCode is required")
				}
				if v.UserCode != "" && !userCodes[v.UserCode] {
					add(FileWorkflows, vp+".userCode", "unknown user code")
				}
				if v.TeamCode != "" && !teamCodes[v.TeamCode] {
					add(FileWorkflows, vp+".teamCode", "unknown team code")
				}
				if v.GradeCode != "" && (v.TeamCode == "" || !gradeCodes[v.GradeCode]) {
					add(FileWorkflows, vp+".gradeCode", "gradeCode requires a team and a known grade code")
				}
			}
			for fi, f := range sec.FormFields {
				fields[f.Code] = true
				if !pf06Contains(model.FormFieldSizes, f.Size) {
					add(FileWorkflows, fmt.Sprintf("%s.sectionList[%d].formFields[%d].size", p, si, fi), "invalid size")
				}
				if f.GeneralMasterCode != "" && !masterCodes[f.GeneralMasterCode] {
					add(FileWorkflows, fmt.Sprintf("%s.sectionList[%d].formFields[%d].generalMasterCode", p, si, fi), "unknown general master code")
				}
			}
		}
		for j, c := range w.RouteConditions {
			cp := fmt.Sprintf("%s.routeConditions[%d]", p, j)
			if !pf06Contains(model.RouteConditionTypes, c.ConditionType) {
				add(FileWorkflows, cp+".conditionType", "invalid condition type")
			}
			if (c.RouteCode == nil) == (c.ErrorMessage == nil) {
				add(FileWorkflows, cp, "exactly one of routeCode and errorMessage is required")
			}
			if c.RouteCode != nil && !routeCodes[*c.RouteCode] {
				add(FileWorkflows, cp+".routeCode", "unknown route code")
			}
			if c.ConditionType == model.RouteConditionTypeField || c.CombinationType != "" {
				if c.CombinationType == "custom" {
					add(FileWorkflows, cp+".combinationType", "combinationType custom is unsupported")
				} else if !pf06Contains(model.RouteCombinationTypes, c.CombinationType) {
					add(FileWorkflows, cp+".combinationType", "invalid combination type")
				}
			}
			for k, cf := range c.ConditionFields {
				fp := fmt.Sprintf("%s.conditionFields[%d]", cp, k)
				if cf.FieldKey == "other_variable" {
					add(FileWorkflows, fp+".fieldKey", "other_variable is unsupported")
				} else if !pf06Contains(model.StepConditionFieldKeys, cf.FieldKey) || cf.FieldKey == model.StepConditionFieldKeyAuthorGrade || cf.FieldKey == model.StepConditionFieldKeyAuthorTeam {
					add(FileWorkflows, fp+".fieldKey", "invalid field key")
				}
				if !pf06OneOf(cf.Symbol, "equal", "not_equal", "greater_than", "greater_than_or_equal", "less_than", "less_than_or_equal", "include", "exclude", "is_empty", "is_not_empty", "descendants_or_equal") {
					add(FileWorkflows, fp+".symbol", "invalid condition symbol")
				}
				if cf.Variable != "" && !fields[cf.Variable] {
					add(FileWorkflows, fp+".variable", "unknown form field code")
				}
			}
		}
		for ti, team := range w.AvailableTeams {
			if team.GradeSymbol != nil && !pf06Contains(model.GradeSymbols, *team.GradeSymbol) {
				add(FileWorkflows, fmt.Sprintf("%s.availableTeams[%d].gradeSymbol", p, ti), "invalid gradeSymbol")
			}
		}
	}
	for i, p := range append(append([]ProxyFixture{}, s.ProxyApplicants...), s.ProxyApprovers...) {
		path := fmt.Sprintf("proxy[%d]", i)
		if !userCodes[p.UserCode] {
			add("proxy-registrations.json", path+".userCode", "unknown user code")
		}
		if !userCodes[p.ProxyUserCode] {
			add("proxy-registrations.json", path+".proxyUserCode", "unknown proxy user code")
		}
		for j, w := range p.WorkflowCodes {
			if !workflowCodes[w] {
				add("proxy-registrations.json", fmt.Sprintf("%s.workflowCodes[%d]", path, j), "unknown workflow code")
			}
		}
	}
	for i, t := range s.Tokens {
		p := fmt.Sprintf("[%d]", i)
		if t.Token == "" {
			add(FileTokens, p+".token", "token is required")
		}
		switch t.Kind {
		case model.TokenKindPAT:
			if t.UserCode == "" || !userCodes[t.UserCode] {
				add(FileTokens, p+".userCode", "pat requires a valid userCode")
			}
		case model.TokenKindSAT:
			if t.UserCode != "" {
				add(FileTokens, p+".userCode", "sat must not have userCode")
			}
		default:
			add(FileTokens, p+".kind", "invalid token kind")
		}
	}
	for i, w := range s.Webhooks {
		for j, c := range w.WorkflowCodes {
			if !workflowCodes[c] {
				add(FileWebhooks, fmt.Sprintf("[%d].workflowCodes[%d]", i, j), "unknown workflow code")
			}
		}
		for j, e := range w.EventTypes {
			if !pf06Contains(model.WebhookEventTypes, e) {
				add(FileWebhooks, fmt.Sprintf("[%d].eventTypes[%d]", i, j), "invalid event type")
			}
		}
	}
	validateLabelsAndSubStatuses(s, add)
	if len(ps) > 0 {
		sortProblems(ps)
		return &ValidationError{Problems: ps}
	}
	return nil
}

func pf06WalkIDs(v reflect.Value, file, path string, add func(string, string, string)) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		pf06WalkIDs(v.Elem(), file, path, add)
		return
	}
	switch v.Kind() {
	case reflect.Struct:
		if id := v.FieldByName("ID"); id.IsValid() && id.Kind() == reflect.String {
			add(file, path+".id", id.String())
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.Name == "ID" {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				name = f.Name
			}
			pf06WalkIDs(v.Field(i), file, path+"."+name, add)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			pf06WalkIDs(v.Index(i), file, fmt.Sprintf("%s[%d]", path, i), add)
		}
	}
}

type pf06CodePath struct{ code, path string }

func pf06MapCodes(n int, at func(int) pf06CodePath) []pf06CodePath {
	out := make([]pf06CodePath, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, at(i))
	}
	return out
}
func pf06OneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func pf06Contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func pf06CountCurrent(x []OrgChartFixture) int {
	n := 0
	for _, v := range x {
		if v.Current {
			n++
		}
	}
	return n
}
func pf06CountSuper(x []RoleFixture) int {
	n := 0
	for _, v := range x {
		if !v.Editable {
			n++
		}
	}
	return n
}
func pf06CheckTree[T any](file string, xs []T, code func(T) string, parent func(T) *string, add func(string, string, string)) {
	parents := map[string]*string{}
	for _, x := range xs {
		parents[code(x)] = parent(x)
	}
	state := map[string]int{}
	var visit func(string)
	visit = func(c string) {
		if state[c] == 1 {
			add(file, c, "parent cycle")
			return
		}
		if state[c] == 2 {
			return
		}
		state[c] = 1
		if p := parents[c]; p != nil && parents[*p] != nil {
			visit(*p)
		}
		state[c] = 2
	}
	for c := range parents {
		visit(c)
	}
}
