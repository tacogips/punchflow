package present

import (
	"encoding/json"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"testing"
)

func TestWorkflowPresenterResolvesTicketWorkflowChoicesAndSlipDefaults(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	definition, err := store.WorkflowDefByID(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	definition.SectionList[0].FormFields[0].FieldType = "ticket"
	definition.SectionList[0].FormFields[0].SelectableTicketWorkflowCodes = []string{"WF2"}
	definition.SectionList[0].FormFields[0].ApproverEditRestriction = &model.ApproverEditRestriction{RouteStepCodes: []string{"accounting"}}
	definition.SectionList[0].SlipFields[0].FieldType = "master"
	definition.SectionList[0].SlipFields[0].GeneralMasterCode = "GM"
	definition.SectionList[0].SlipFields[0].DefaultGeneralMasterItemCode = "I1"
	definition.SectionList[0].SlipFields[0].SelectableTicketWorkflowCodes = []string{"WF2"}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.ExecContext(ctx, `UPDATE workflows SET definition_json=? WHERE id='wf'`, string(encoded)); err != nil {
		t.Fatal(err)
	}
	second := definition
	second.ID, second.Code, second.Name = "wf2", "WF2", "Source"
	encoded, err = json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.ExecContext(ctx, `INSERT INTO workflows(id,code,status,folder_id,definition_json,ticket_number_counter,created_at,updated_at) SELECT 'wf2','WF2','visible','f1',?,0,created_at,updated_at FROM workflows WHERE id='wf'`, string(encoded)); err != nil {
		t.Fatal(err)
	}
	got, err := WorkflowDetail(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	section := got["sectionList"].([]any)[0].(map[string]any)
	formField := section["formFields"].([]any)[0].(map[string]any)
	choices := formField["selectableTicketWorkflows"].([]any)
	restriction := formField["approverEditRestriction"].(map[string]any)
	if codes := restriction["routeStepCodes"].([]string); len(codes) != 1 || codes[0] != "accounting" {
		t.Fatalf("approver required route step codes=%v", codes)
	}
	if len(choices) != 1 {
		t.Fatalf("ticket workflow choices=%v", choices)
	}
	choice := choices[0].(WorkflowJSON)
	if choice["id"] != "wf2" || choice["code"] != "WF2" {
		t.Fatalf("resolved ticket workflow=%v", choice)
	}
	if _, exists := choice["folder"]; exists {
		t.Fatal("selectable ticket workflow unexpectedly contains folder")
	}
	slip := section["slipFields"].([]any)[0].(map[string]any)
	if got := slip["defaultGeneralMasterItem"].(GeneralMasterItemJSON)["id"]; got != "it" {
		t.Fatalf("detail default master item=%v", got)
	}
	if len(slip["selectableTicketWorkflows"].([]any)) != 1 {
		t.Fatal("detail ticket workflow choices were not resolved")
	}
}

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
	pf08Keys(t, section, "id title sectionType description viewRestriction sectionViewers formFields slipFields conditionFields conditional")
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

func TestWorkflowNextTicketNumberIsNullBeforeFirstAllocation(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	definition, err := store.WorkflowDefByID(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	format := "WF-[number]"
	definition.TicketNumberFormat = &format
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(ctx, `UPDATE workflows SET definition_json=?,ticket_number_counter=0 WHERE id='wf'`, string(encoded)); err != nil {
		t.Fatal(err)
	}
	workflow, err := Workflow(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	if workflow["nextTicketNumberValue"] != nil {
		t.Fatalf("unallocated workflow nextTicketNumberValue=%v, want null", workflow["nextTicketNumberValue"])
	}
	if _, err := q.ExecContext(ctx, `UPDATE workflows SET ticket_number_counter=1 WHERE id='wf'`); err != nil {
		t.Fatal(err)
	}
	workflow, err = Workflow(ctx, q, "wf")
	if err != nil {
		t.Fatal(err)
	}
	if workflow["nextTicketNumberValue"] != int64(2) {
		t.Fatalf("allocated workflow nextTicketNumberValue=%v, want 2", workflow["nextTicketNumberValue"])
	}
}
