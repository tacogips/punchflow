package engine

import (
	"encoding/json"
	"testing"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestCustomValidationResourceConditions(t *testing.T) {
	e := pf14Setup(t)
	ctx := t.Context()
	q := e.db.Q()
	chartID, err := store.ActiveOrgChartID(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := store.TeamIDByCode(ctx, q, chartID, "company")
	if err != nil {
		t.Fatal(err)
	}
	engineeringID, err := store.TeamIDByCode(ctx, q, chartID, "engineering")
	if err != nil {
		t.Fatal(err)
	}
	gradeID, err := store.GradeIDByCode(ctx, q, "manager")
	if err != nil {
		t.Fatal(err)
	}
	masterID, err := store.ResolveID(ctx, q, "general_masters", "cost-centers")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := store.MasterItemIDByCode(ctx, q, masterID, "cc-eng")
	if err != nil {
		t.Fatal(err)
	}
	n := &normalizer{ctx: ctx, q: q}

	form := model.FormCustomValidation{CombinationType: "all", Fields: []model.FormCustomValidationField{
		{FormFieldCode: "approving-team", Symbol: "descendants_or_equal", TeamCode: "company"},
		{FormFieldCode: "cost-center", Symbol: "equal", GeneralMasterCode: "cost-centers", GeneralMasterItemCode: "cc-eng"},
		{FormFieldCode: "grade", Symbol: "equal", GradeCode: "manager"},
		{FormFieldCode: "reviewer", Symbol: "equal", GradeCode: "manager"},
		{FormFieldCode: "reviewer", Symbol: "descendants_or_equal", TeamCode: "sales"},
	}}
	values := map[string]any{"approving-team": []string{engineeringID}, "cost-center": []string{itemID}, "grade": gradeID, "reviewer": e.users["sales-manager"]}
	if !n.formCustomValidationMatches(form, func(code string) (any, bool) { value, ok := values[code]; return value, ok }) {
		t.Fatal("form resource conditions should match team and user memberships and the master item")
	}
	values["approving-team"] = []string{"unrelated-team"}
	if n.formCustomValidationMatches(form, func(code string) (any, bool) { value, ok := values[code]; return value, ok }) {
		t.Fatal("form descendant condition matched an unrelated team")
	}

	slip := model.SlipCustomValidation{CombinationType: "all", Fields: []model.SlipCustomValidationField{{
		SlipFieldCode: "team", Symbol: "descendants_or_equal", TeamCode: "company",
	}}}
	if !n.slipCustomValidationMatches(slip, func(code string) (any, bool) { return []string{engineeringID}, true }) {
		t.Fatal("detail resource condition should match a child team")
	}
	if n.teamDescendsFrom(companyID, engineeringID) {
		t.Fatal("parent team must not be considered a descendant of its child")
	}
}

func TestCustomValidationResourceConditionsRejectMatchingFormInput(t *testing.T) {
	e := pf14Setup(t)
	teamCode := "sales"
	itemCode := "vendor-acme"
	pf16Redefine(t, e, "purchase-request", func(def *model.WorkflowDef) {
		for si := range def.SectionList {
			for fi := range def.SectionList[si].FormFields {
				field := &def.SectionList[si].FormFields[fi]
				if field.Code == "item" {
					field.CustomValidations = []model.FormCustomValidation{{ID: "custom-resource-rule", CombinationType: "all", ErrorMessage: "resource rule matched", Fields: []model.FormCustomValidationField{
						{ID: "team-condition", FormFieldCode: "approving-team", Symbol: "descendants_or_equal", TeamCode: teamCode},
						{ID: "master-condition", FormFieldCode: "vendor", Symbol: "equal", GeneralMasterCode: "vendors", GeneralMasterItemCode: itemCode},
					}}}
				}
			}
		}
	})
	workflow, fields := pf14FormDefinition(t, e, "purchase-request")
	teamID, err := store.TeamIDByCode(e.ctx, e.db.Q(), mustActiveChartID(t, e), "sales-east")
	if err != nil {
		t.Fatal(err)
	}
	masterID, err := store.ResolveID(e.ctx, e.db.Q(), "general_masters", "vendors")
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := store.MasterItemIDByCode(e.ctx, e.db.Q(), masterID, itemCode)
	if err != nil {
		t.Fatal(err)
	}
	inputs := pf14CompleteInputs(fields, map[string]any{"amount": "100", "item": "Computer"})
	for i := range inputs {
		switch inputs[i].FormFieldCode {
		case "approving-team":
			inputs[i].TeamID, _ = json.Marshal(teamID)
			inputs[i].Value = nil
		case "vendor":
			inputs[i].GeneralMasterItemID, _ = json.Marshal(itemID)
			inputs[i].Value = nil
		}
	}
	err = e.db.WithTx(e.ctx, func(tx *store.Tx) error {
		_, _, createErr := e.engine.Create(e.ctx, tx, e.actor("sales-rep"), CreateInput{Status: model.TicketStatusDraft, WorkflowID: workflow.ID, AuthorTeamID: e.team("sales"), Inputs: &inputs})
		return createErr
	})
	api, ok := kferr.As(err)
	if !ok || api.Code != "validation_failed" || len(api.Errors["item"]) == 0 || api.Errors["item"][0] != "resource rule matched" {
		t.Fatalf("create error=%v, want resource custom validation on item", err)
	}
}

func mustActiveChartID(t *testing.T, e *pf14TestEnv) string {
	t.Helper()
	id, err := store.ActiveOrgChartID(e.ctx, e.db.Q())
	if err != nil {
		t.Fatal(err)
	}
	return id
}
