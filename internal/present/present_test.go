package present

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func pf08DB(t *testing.T) (context.Context, *store.DB) {
	t.Helper()
	ctx := context.Background()
	db, e := store.Open(ctx, filepath.Join(t.TempDir(), "present.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close() })
	q := db.Q()
	now := kftime.ToMillis(time.Date(2026, 1, 2, 3, 4, 5, 123000000, time.UTC))
	exec := func(s string, a ...any) {
		t.Helper()
		if _, e := q.ExecContext(ctx, s, a...); e != nil {
			t.Fatalf("seed: %v: %s", e, s)
		}
	}
	exec("INSERT INTO tenant(id,public_id,name,plan,mode,created_at,updated_at) VALUES('t','pub','Tenant','enterprise','normal',?,?)", now, now)
	exec("INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('u1','U1','u1@example.test','Ada','Lovelace','activated','en','normal','{}',?,?),('u2','U2','u2@example.test','Grace','Hopper','activated','en','normal','{}',?,?)", now, now, now, now)
	exec("INSERT INTO grades(id,code,name,level,is_default,created_at,updated_at) VALUES('g1','G1','Manager',1,1,?,?)", now, now)
	exec("INSERT INTO organization_charts(id,name,current,created_at,updated_at) VALUES('oc','Company',1,?,?)", now, now)
	exec("INSERT INTO teams(id,organization_chart_id,code,name,parent_id,approve_only,created_at,updated_at) VALUES('tm1','oc','ROOT','Company',NULL,0,?,?),('tm2','oc','SALES','Sales','tm1',0,?,?)", now, now, now, now)
	exec("INSERT INTO team_memberships(id,team_id,user_id,leader,grade_ids,created_at,updated_at) VALUES('mem','tm2','u1',1,'[\"g1\"]',?,?)", now, now)
	exec("INSERT INTO roles(id,name,editable,permission_list,created_at,updated_at) VALUES('r','Admin',1,'[{\"permission\":\"user\"}]',?,?)", now, now)
	exec("INSERT INTO role_memberships(role_id,user_id,created_at) VALUES('r','u1',?)", now)
	exec("INSERT INTO categories(id,code,name,created_at,updated_at) VALUES('cat','CAT','Category',?,?)", now, now)
	exec("INSERT INTO folders(id,code,name,description,parent_id,editable,created_at,updated_at) VALUES('f1','F1','Root',NULL,NULL,0,?,?),('f2','F2','Child','child','f1',1,?,?)", now, now, now, now)
	exec("INSERT INTO general_masters(id,code,name,default_sort_by,initial_display_code,initial_display_created_at,initial_display_description,created_at,updated_at) VALUES('gm','GM','Master','code',1,0,0,?,?)", now, now)
	exec("INSERT INTO general_master_fields(id,general_master_id,code,title,field_type,required,field_order,visible,initial_display,options,created_at,updated_at) VALUES('fld','gm','name','Name','text',1,1,1,0,'[]',?,?)", now, now)
	exec("INSERT INTO general_master_items(id,general_master_id,code,name,inputs,input_ids,created_at,updated_at) VALUES('it','gm','I1','Item','{\"fld\":\"value\"}','{\"fld\":\"in1\"}',?,?)", now, now)
	wf := model.WorkflowDef{ID: "wf", Code: "WF", Status: "visible", Name: "Workflow", Description: "desc", Notes: "notes", FolderCode: "F1", RouteCode: "ROUTE", CategoryCodes: []string{"CAT"}, TicketNumberFormat: pf08String("WF-{n}"), TitleInputMode: "manual", TicketNumberingTiming: "on_submit", AvailableTeams: []model.WorkflowTeam{{TeamCode: "SALES", Descendants: true, GradeSymbol: pf08String("equal"), GradeCodes: []string{"G1"}}}, Watchers: []model.WorkflowWatcher{{ID: "watcher", WatcherType: "user", UserCode: "U1"}}, TicketViewers: []model.WorkflowViewer{{ID: "viewer", UserCode: "U2"}}, SectionList: []model.WorkflowSection{{ID: "sec", Title: "Details", SectionType: "form", FormFields: []model.FormField{{ID: "ff", Code: "TITLE", Title: "Title", FieldType: "text", Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}, Orientation: "vertical"}}}}, RouteConditions: []model.RouteCondition{{ID: "cond", ConditionType: "field", CombinationType: "all", RouteCode: pf08String("ROUTE"), ConditionFields: []model.ConditionField{{ID: "wcf", FieldKey: "grade", Symbol: "equal", Value: "G1"}}}, {ID: "else", ConditionType: "field_otherwise", CombinationType: "all", ErrorMessage: pf08String("no route"), ConditionFields: []model.ConditionField{}}}, ReportFormats: []string{}}
	wf.SectionList[0].SlipFields = []model.SlipField{{ID: "slip", Code: "TOTAL", Title: "Total", FieldType: "number", Options: []string{}, AllowedExtensions: []string{}, AllowedTicketStatus: []string{}}}
	wfb, _ := json.Marshal(wf)
	exec("INSERT INTO workflows(id,code,status,folder_id,definition_json,ticket_number_counter,created_at,updated_at) VALUES('wf','WF','visible','f1',?,2,?,?)", string(wfb), now, now)
	rd := model.RouteDef{ID: "rt", Code: "ROUTE", Status: "visible", Name: "Route", Description: "route", FolderCode: "F1", Steps: []model.RouteStep{{ID: "s2", Code: "S2", StepOrder: 2, Title: "Second", StepType: "user", ActionType: "approve", UserCodes: []string{"U1"}, Targets: []model.RouteTarget{{TeamCode: "SALES", Descendants: true, GradeSymbol: pf08String("equal"), GradeCodes: []string{"G1"}}}, RequiredApprovalsNumber: 1, RequiredApprovalsPercent: 100, ManagerSelectionType: "closest"}, {ID: "s1", Code: "S1", StepOrder: 1, Title: "First", StepType: "user", ActionType: "approve", UserCodes: []string{"U1"}, Targets: []model.RouteTarget{{TeamCode: "SALES", Descendants: true, GradeSymbol: pf08String("equal"), GradeCodes: []string{"G1"}}}, RequiredApprovalsNumber: 1, RequiredApprovalsPercent: 100, ManagerSelectionType: "closest", RouteStepCondition: &model.RouteStepCondition{ID: "sc", ConditionType: "all", CombinationType: "all", RouteStepConditionFields: []model.ConditionField{{ID: "cf", FieldKey: "author_grade", Symbol: "equal", Value: "G1"}}}}}}
	rdb, _ := json.Marshal(rd)
	exec("INSERT INTO routes(id,code,status,folder_id,definition_json,created_at,updated_at) VALUES('rt','ROUTE','visible','f1',?,?,?)", string(rdb), now, now)
	exec("INSERT INTO proxy_applicants(id,user_id,proxy_user_id,workflow_ids,created_at,updated_at) VALUES('pa','u1','u2','[\"wf\"]',?,?)", now, now)
	exec("INSERT INTO proxy_approvers(id,user_id,proxy_user_id,workflow_ids,created_at,updated_at) VALUES('pr','u1','u2','[\"wf\"]',?,?)", now, now)
	return ctx, db
}
func pf08String(s string) *string { return &s }
func pf08Keys(t *testing.T, v any, want string) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	expected := strings.Fields(want)
	sort.Strings(expected)
	if fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Fatalf("keys mismatch got %v want %v", got, expected)
	}
}
func TestPresentCoreKeySets(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	u, _ := store.GetUser(ctx, q, "u1")
	ts := kftime.Format(u.CreatedAt)
	if !regexp.MustCompile("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.][0-9]{3}[+]09:00$").MatchString(ts) {
		t.Fatalf("timestamp=%s", ts)
	}
	opts := UserOpts{LastUsedOn: true, CustomFields: true, UserType: true}
	uj := UserFromModel(u, opts)
	pf08Keys(t, uj, "id email code firstName lastName fullName employeeId image status locale createdAt updatedAt lastUsedOn userType")
	if uj["fullName"] != "Lovelace Ada" {
		t.Fatalf("fullName=%v", uj["fullName"])
	}
	ud, e := UserDetail(ctx, q, "u1", opts)
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, ud, "id email code firstName lastName fullName employeeId image status locale createdAt updatedAt lastUsedOn userType userLineWorksAccount invitation")
	g, e := Grades(ctx, q, []string{"g1"})
	if e != nil || len(g) != 1 {
		t.Fatalf("grades %v %v", g, e)
	}
	pf08Keys(t, g[0], "id code name level isDefault createdAt updatedAt")
	teamBase, e := Team(ctx, q, "tm2")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, teamBase, "id name fullName code notes approveOnly usersCount createdAt updatedAt")
	team, e := TeamDetail(ctx, q, "tm2")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, team, "id name fullName code notes approveOnly usersCount createdAt updatedAt parent children users")
	if team["fullName"] != "Company/Sales" {
		t.Fatal(team["fullName"])
	}
	pf08Keys(t, team["users"].([]any)[0], "id email code firstName lastName fullName employeeId image status locale createdAt updatedAt grades leader")
	member := map[string]any(team["users"].([]any)[0].(MemberUserJSON))
	if member["leader"] != true || len(member["grades"].([]any)) == 0 {
		t.Fatal("member grades/leader not presented")
	}
	memberPage, err := MemberUsers(ctx, q, "tm2", 0, 10)
	if err != nil || len(memberPage) != 1 {
		t.Fatalf("member page: %v %v", memberPage, err)
	}
	pf08Keys(t, memberPage[0], "id email code firstName lastName fullName employeeId image status locale createdAt updatedAt grades leader")
	org, e := OrganizationChart(ctx, q, "oc")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, org, "id name current teamsCount membershipsCount createdAt updatedAt activationPlan")
	orgDetail, err := OrganizationChartDetail(ctx, q, "oc")
	if err != nil {
		t.Fatal(err)
	}
	pf08Keys(t, orgDetail, "id name current teamsCount membershipsCount createdAt updatedAt activationPlan")
	roleBase, err := Role(ctx, q, "r")
	if err != nil {
		t.Fatal(err)
	}
	pf08Keys(t, roleBase, "id name editable usersCount createdAt updatedAt")
	role, e := RoleDetail(ctx, q, "r")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, role, "id name editable usersCount createdAt updatedAt permissionList")
	pf08Keys(t, role["permissionList"].([]any)[0], "permission restricted folders generalMasters teams")
	cat, _ := store.GetCategory(ctx, q, "cat")
	pf08Keys(t, Category(cat), "id code name createdAt updatedAt")
	folder, e := FolderDetail(ctx, q, "f2")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, folder, "id name fullName code workflowsCount routesCount pipelinesCount editable createdAt updatedAt description ancestors children")
	if folder["fullName"] != "Root/Child" {
		t.Fatal(folder["fullName"])
	}
	root, e := Folder(ctx, q, "f1")
	if e != nil || root["editable"] != false {
		t.Fatalf("root folder editability: %v %v", root, e)
	}
	gm, e := GeneralMaster(ctx, q, "gm")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, gm, "id code name description createdAt updatedAt defaultSortBy itemsCount initialDisplayCode initialDisplayCreatedAt initialDisplayDescription fields")
	gmi, e := GeneralMasterItem(ctx, q, "it")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, gmi, "id code name description startsOn endsOn inputs createdAt updatedAt")
	w, e := Workflow(ctx, q, "wf")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, w, "id code versionId versionNumber name description status publicTicket visibleToManager visibleToTeamMembers titleDescription ticketNumberFormat overwritable createdAt updatedAt titleInputMode titleFormula availableToEveryone reportFormats hiddenOnWorkflowFilterForTicket hiddenOnWorkflowSelectionScreen approvalCancellable reportFileNameFormat current notes versionCreatedAt collectEmailOnExternalPublish notifyGuestOnCompletion allowCustomSteps commentingRequiredOnApproval commentingRequiredOnRejection commentingRequiredOnDenial allowEditingOfViewers commentingEnabled nextTicketNumberValue nextTicketNumberValueByKey author versionAuthor externalPublish folder categories")
	wd, e := WorkflowDetail(ctx, q, "wf")
	if e != nil {
		t.Fatal(e)
	}
	for _, k := range []string{"sectionList", "routeConditions", "availableTeams", "watchers", "workflowFilterVisibilities", "ticketNumberingTiming", "slackChannel", "chatworkRoom", "microsoftTeamsChannel", "googleChatSetting", "reportUploadGoogleDrive", "reportUploadBox", "cloudSignSetting", "ticketNumberKey", "excelTemplate", "customNumberings", "customNumberingSettings"} {
		if _, ok := wd[k]; !ok {
			t.Errorf("WorkflowDetail missing %s", k)
		}
	}
	routeBase, err := Route(ctx, q, "rt")
	if err != nil {
		t.Fatal(err)
	}
	pf08Keys(t, routeBase, "id code status versionId versionNumber name description notes current createdAt updatedAt versionCreatedAt folder author")
	r, e := RouteDetail(ctx, q, "rt")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, r, "id code status versionId versionNumber name description notes current createdAt updatedAt versionCreatedAt folder author versionAuthor steps")
	steps := r["steps"].([]any)
	if steps[0].(map[string]any)["code"] != "S1" {
		t.Fatal("route steps not ordered")
	}
	target := steps[0].(map[string]any)["targets"].([]any)[0].(map[string]any)
	if target["team"] == nil || len(target["grades"].([]any)) != 1 {
		t.Fatal("route target references not resolved")
	}
	pf08Keys(t, steps[0], "id stepOrder stepType title actionType instruction requiredApprovalsNumber requiredApprovalsPercent fallbackType allowSelfApproval managerSelectionType allowRaising minCustomAssignees approverAssignmentInstruction users code targets routeStepCondition subStatus")
	pa, e := ProxyApplicant(ctx, q, "pa")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, pa, "id createdAt updatedAt user proxyUser startsOn endsOn workflows")
	pr, e := ProxyApprover(ctx, q, "pr")
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, pr, "id createdAt updatedAt user proxyUser startsOn endsOn workflows")
	aj, e := AuditLog(ctx, q, model.AuditLog{ID: "a", Action: "login", CreatedAt: u.CreatedAt})
	if e != nil {
		t.Fatal(e)
	}
	pf08Keys(t, aj, "id user action data remoteIp requestId systemType createdAt updatedAt")
	branch := wd["routeConditions"].([]any)[1].(map[string]any)
	if branch["route"] != nil || branch["errorMessage"] == nil || *branch["errorMessage"].(*string) != "no route" {
		t.Fatal("errorMessage branch route/null mismatch")
	}
	pf08CheckTimestamps(t, team)
	pf08CheckTimestamps(t, org)
	pf08CheckTimestamps(t, role)
	pf08CheckTimestamps(t, folder)
	pf08CheckTimestamps(t, gm)
	pf08CheckTimestamps(t, gmi)
	pf08CheckTimestamps(t, w)
	pf08CheckTimestamps(t, wd)
	pf08CheckTimestamps(t, r)
	pf08CheckTimestamps(t, pa)
	pf08CheckTimestamps(t, pr)
	pf08CheckTimestamps(t, aj)
	categoryIDs, e := store.GetCategoryIDs(ctx, q, "wf")
	if e != nil || len(categoryIDs) != 1 || categoryIDs[0] != "cat" {
		t.Fatalf("category ids: %v %v", categoryIDs, e)
	}
	_, e = User(ctx, q, "missing", UserOpts{})
	if !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("not-found error=%v", e)
	}
	empty, e := Users(ctx, q, nil, UserOpts{})
	if e != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty users=%#v err=%v", empty, e)
	}
}

func TestRouteConditionValuesFollowNullableStringSchema(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field model.ConditionField
		want  any
	}{
		{name: "numeric threshold", field: model.ConditionField{FieldKey: "number_variable", Symbol: "greater_than", Value: float64(50000)}, want: "50000"},
		{name: "team reference", field: model.ConditionField{FieldKey: "author_team", Symbol: "equal"}, want: nil},
		{name: "empty comparison", field: model.ConditionField{FieldKey: "text_variable", Symbol: "is_empty", Value: "ignored"}, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pf08RouteStepConditionField(context.Background(), nil, tc.field)["value"]
			if got != tc.want {
				t.Fatalf("value=%#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestPresentMasterItemInputIDs(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	item, err := GeneralMasterItem(ctx, q, "it")
	if err != nil {
		t.Fatal(err)
	}
	input := item["inputs"].([]any)[0].(map[string]any)
	if input["id"] != "in1" {
		t.Fatalf("input id=%v, want persisted id in1", input["id"])
	}
	if got := input["field"].(map[string]any)["id"]; got != "fld" {
		t.Fatalf("field id=%v, want fld", got)
	}
	if _, err := q.ExecContext(ctx, `UPDATE general_master_items SET input_ids='{}' WHERE id='it'`); err != nil {
		t.Fatal(err)
	}
	if _, err := GeneralMasterItem(ctx, q, "it"); err == nil || !strings.Contains(err.Error(), "field fld has no input id") {
		t.Fatalf("missing input ID error=%v", err)
	}
}

func TestPresentSectionShapes(t *testing.T) {
	ctx, db := pf08DB(t)
	sections, err := pf08Sections(ctx, db.Q(), []model.WorkflowSection{
		{ID: "slip", Code: "lines", Title: "Line items", SectionType: model.SectionTypeSlip},
		{ID: "conditional", Title: "Conditional", SectionType: model.SectionTypeForm, Conditional: true, CombinationType: "all"},
		{ID: "plain", Title: "Plain", SectionType: model.SectionTypeForm},
	})
	if err != nil {
		t.Fatal(err)
	}
	slip := sections[0].(map[string]any)
	if slip["code"] != "lines" {
		t.Fatalf("slip code=%v, want lines", slip["code"])
	}
	form := sections[1].(map[string]any)
	if form["conditional"] != true || form["combinationType"] != "all" {
		t.Fatalf("conditional form shape=%v", form)
	}
	plain := sections[2].(map[string]any)
	if plain["conditional"] != false {
		t.Fatalf("plain form conditional=%v, want false", plain["conditional"])
	}
	if _, ok := plain["combinationType"]; ok {
		t.Fatalf("non-conditional form unexpectedly has combinationType: %v", plain)
	}
}

func TestPresentRouteTargetGradeSymbolNull(t *testing.T) {
	ctx, db := pf08DB(t)
	q := db.Q()
	route, err := store.RouteDefByID(ctx, q, "rt")
	if err != nil {
		t.Fatal(err)
	}
	for i := range route.Steps {
		for j := range route.Steps[i].Targets {
			route.Steps[i].Targets[j].GradeSymbol = nil
		}
	}
	definition, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.ExecContext(ctx, `UPDATE routes SET definition_json=? WHERE id='rt'`, string(definition)); err != nil {
		t.Fatal(err)
	}
	presented, err := RouteDetail(ctx, q, "rt")
	if err != nil {
		t.Fatal(err)
	}
	target := presented["steps"].([]any)[0].(map[string]any)["targets"].([]any)[0].(map[string]any)
	if _, ok := target["gradeSymbol"]; !ok {
		t.Fatal("gradeSymbol key is missing")
	}
	b, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]json.RawMessage
	if err = json.Unmarshal(b, &encoded); err != nil {
		t.Fatal(err)
	}
	if string(encoded["gradeSymbol"]) != "null" {
		t.Fatalf("encoded gradeSymbol=%s, want null", encoded["gradeSymbol"])
	}
}

func pf08CheckTimestamps(t *testing.T, v any) {
	t.Helper()
	pattern := regexp.MustCompile("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.][0-9]{3}[+]09:00$")
	var walk func(any)
	walk = func(x any) {
		switch y := x.(type) {
		case map[string]any:
			for k, v := range y {
				if k == "createdAt" || k == "updatedAt" || k == "versionCreatedAt" {
					if s, ok := v.(string); ok && !pattern.MatchString(s) {
						t.Errorf("%s timestamp %q", k, s)
					}
				}
				walk(v)
			}
		case []any:
			for _, v := range y {
				walk(v)
			}
		}
	}
	walk(v)
}
