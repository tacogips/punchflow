package present

import (
	"testing"
)

func TestWorkflowPresenterContracts(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	inTicket, err := WorkflowInTicket(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	pf08Keys(t, inTicket, "id code versionId versionNumber name description status publicTicket visibleToManager visibleToTeamMembers titleDescription ticketNumberFormat overwritable createdAt updatedAt titleInputMode titleFormula availableToEveryone reportFormats hiddenOnWorkflowFilterForTicket hiddenOnWorkflowSelectionScreen approvalCancellable reportFileNameFormat current notes versionCreatedAt collectEmailOnExternalPublish notifyGuestOnCompletion allowCustomSteps commentingRequiredOnApproval commentingRequiredOnRejection commentingRequiredOnDenial allowEditingOfViewers commentingEnabled nextTicketNumberValue nextTicketNumberValueByKey author versionAuthor externalPublish folder categories sectionList ticketViewers cloudSignSetting ticketNumberKey excelTemplate customNumberings customNumberingSettings")
	for _, key := range []string{"externalPublish", "sectionList", "ticketViewers", "cloudSignSetting", "ticketNumberKey", "excelTemplate", "customNumberings", "customNumberingSettings"} {
		if _, ok := inTicket[key]; !ok {
			t.Errorf("WorkflowInTicket missing %q", key)
		}
	}
	detail, err := WorkflowDetail(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	pf08Keys(t, detail, "id code versionId versionNumber name description status publicTicket visibleToManager visibleToTeamMembers titleDescription ticketNumberFormat overwritable createdAt updatedAt titleInputMode titleFormula availableToEveryone reportFormats hiddenOnWorkflowFilterForTicket hiddenOnWorkflowSelectionScreen approvalCancellable reportFileNameFormat current notes versionCreatedAt collectEmailOnExternalPublish notifyGuestOnCompletion allowCustomSteps commentingRequiredOnApproval commentingRequiredOnRejection commentingRequiredOnDenial allowEditingOfViewers commentingEnabled nextTicketNumberValue nextTicketNumberValueByKey author versionAuthor externalPublish folder categories sectionList ticketViewers cloudSignSetting ticketNumberKey excelTemplate customNumberings customNumberingSettings routeConditions availableTeams watchers workflowFilterVisibilities slackChannel chatworkRoom microsoftTeamsChannel googleChatSetting reportUploadGoogleDrive reportUploadBox ticketNumberingTiming")
	for _, key := range []string{"routeConditions", "availableTeams", "watchers", "workflowFilterVisibilities", "slackChannel", "chatworkRoom", "microsoftTeamsChannel", "googleChatSetting", "reportUploadGoogleDrive", "reportUploadBox", "ticketNumberingTiming"} {
		if _, ok := detail[key]; !ok {
			t.Errorf("WorkflowDetail missing %q", key)
		}
	}
	timing := detail["ticketNumberingTiming"].(map[string]any)
	pf08Keys(t, timing, "id timingType subStatus")
	available := detail["availableTeams"].([]any)[0]
	pf08Keys(t, available, "team descendants gradeSymbol grades")
	pf08Keys(t, available.(map[string]any)["team"], "id name fullName code notes approveOnly usersCount createdAt updatedAt")
	pf08Keys(t, available.(map[string]any)["grades"].([]any)[0], "id code name level isDefault createdAt updatedAt")
	watcher := detail["watchers"].([]any)[0]
	pf08Keys(t, watcher, "id watcherType user team")
	viewer := inTicket["ticketViewers"].([]any)[0]
	pf08Keys(t, viewer, "id user team grade descendants")
	section := detail["sectionList"].([]any)[0].(map[string]any)
	pf08Keys(t, section, "id title sectionType description viewRestriction sectionViewers formFields slipFields conditionFields")
	field := section["formFields"].([]any)[0]
	slip := section["slipFields"].([]any)[0]
	pf08Keys(t, slip, "id fieldType code title required showTotal options regexpFormat formula maxValue minValue defaultValue decimalDigit delimited allowedExtensions prefix suffix approver author hidden readonlyOnUi minLength maxLength multiple autoLink useTodayForDefaultValue allowedTicketStatus generalMaster defaultGeneralMasterItem generalMasterSearchFilters approverEditRestriction approverEditSetting customValidations selectableTicketWorkflows")
	pf08Keys(t, field, "id title description fieldType required approver options code size regexpFormat formula defaultValue minValue maxValue minLength maxLength decimalDigit delimited prefix suffix multiple orientation author hidden readonlyOnUi allowedExtensions autoLink useTodayForDefaultValue allowedTicketStatus generalMaster defaultGeneralMasterItem externalApiSetting kintoneAppSetting climberCloudSetting generalMasterSearchFilters approverEditRestriction approverEditSetting customValidations ticketCopySetting selectableTicketWorkflows")
	pf08Keys(t, field.(map[string]any)["approverEditSetting"], "permissionType descendants gradeSymbol grade team users")
	conditions := detail["routeConditions"].([]any)
	pf08Keys(t, conditions[0], "id conditionType combinationType combinationExpression conditionFields route errorMessage")
	wfField := conditions[0].(map[string]any)["conditionFields"].([]any)[0]
	pf08Keys(t, wfField, "id symbol value formField grade team generalMasterItem")
	if wfField.(map[string]any)["grade"] == nil {
		t.Fatal("workflow condition grade was not resolved")
	}
	route, ok := conditions[0].(map[string]any)["route"]
	if !ok || route == nil {
		t.Fatal("route condition did not resolve route")
	}
	routeDetail, err := RouteDetail(ctx, q, "rt")
	if err != nil {
		t.Fatal(err)
	}
	step := routeDetail["steps"].([]any)[0].(map[string]any)
	target := step["targets"].([]any)[0]
	pf08Keys(t, target, "team descendants gradeSymbol grades variable")
	if target.(map[string]any)["team"] == nil || len(target.(map[string]any)["grades"].([]any)) != 1 {
		t.Fatal("route target team/grade references were not resolved")
	}
	cond := step["routeStepCondition"].(map[string]any)
	pf08Keys(t, cond, "id conditionType combinationType routeStepConditionFields")
	cf := cond["routeStepConditionFields"].([]any)[0]
	pf08Keys(t, cf, "id variable fieldKey symbol value grade team generalMasterItem generalMaster")
	if cf.(map[string]any)["grade"] == nil {
		t.Fatal("route condition grade was not resolved")
	}
	master, err := GeneralMaster(ctx, q, "gm")
	if err != nil {
		t.Fatal(err)
	}
	mf := master["fields"].([]any)[0]
	pf08Keys(t, mf, "id code title description fieldType required fieldOrder visible initialDisplay options createdAt updatedAt")
	item, err := GeneralMasterItem(ctx, q, "it")
	if err != nil {
		t.Fatal(err)
	}
	input := item["inputs"].([]any)[0]
	pf08Keys(t, input, "id value createdAt updatedAt field")
	pf08Keys(t, input.(map[string]any)["field"], "id title description code fieldType required fieldOrder visible initialDisplay options createdAt updatedAt")
}
