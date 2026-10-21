package present

import (
	"context"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf08Sections(ctx context.Context, q store.Querier, sections []model.WorkflowSection) ([]any, error) {
	formFields := pf08FormFieldsByCode(sections)
	out := []any{}
	for _, s := range sections {
		fields := []any{}
		for _, f := range s.FormFields {
			fields = append(fields, pf08FormFieldWithCustomValidations(ctx, q, f, formFields))
		}
		slips := []any{}
		for _, f := range s.SlipFields {
			slips = append(slips, pf08SlipFieldWithCustomValidations(f, s.SlipFields))
		}
		cfs := []any{}
		for _, f := range s.ConditionFields {
			cfs = append(cfs, pf08WorkflowConditionField(ctx, q, f, formFields))
		}
		section := map[string]any{"id": s.ID, "title": s.Title, "sectionType": s.SectionType, "description": nil, "viewRestriction": false, "sectionViewers": []any{}, "formFields": fields, "slipFields": slips, "conditionFields": cfs}
		switch s.SectionType {
		case model.SectionTypeForm:
			viewers, err := sectionViewersJSON(ctx, q, s, nil)
			if err != nil {
				return nil, err
			}
			section["viewRestriction"], section["sectionViewers"] = s.ViewRestriction, viewers
			if s.ViewRestriction {
				section["allowViewAuthor"], section["allowViewApprover"] = s.AllowViewAuthor, s.AllowViewApprover
			}
			section["conditional"] = s.Conditional
			if s.Conditional {
				section["combinationType"] = s.CombinationType
				if s.CombinationExpression == "" {
					section["combinationExpression"] = nil
				} else {
					section["combinationExpression"] = s.CombinationExpression
				}
			}
		case model.SectionTypeSlip:
			section["code"] = s.Code
			section["allowApproverToAddItems"] = s.AllowApproverToAddItems
		}
		out = append(out, section)
	}
	return out, nil
}

func pf08FormFieldWithCustomValidations(ctx context.Context, q store.Querier, f model.FormField, fields map[string]model.FormField) map[string]any {
	out := pf08FormField(ctx, q, f)
	validations := make([]any, 0, len(f.CustomValidations))
	for _, validation := range f.CustomValidations {
		conditions := make([]any, 0, len(validation.Fields))
		for _, condition := range validation.Fields {
			field, ok := fields[condition.FormFieldCode]
			var fieldJSON any
			if ok {
				fieldJSON = pf08FormField(ctx, q, field)
			}
			conditions = append(conditions, map[string]any{"id": condition.ID, "symbol": condition.Symbol, "value": condition.Value, "formField": fieldJSON, "generalMasterItem": nil, "grade": nil, "team": nil})
		}
		validations = append(validations, map[string]any{"id": validation.ID, "combinationType": validation.CombinationType, "errorMessage": validation.ErrorMessage, "fields": conditions})
	}
	out["customValidations"] = validations
	return out
}

func pf08SlipFieldWithCustomValidations(f model.SlipField, fields []model.SlipField) map[string]any {
	out := pf08SlipField(f)
	byCode := make(map[string]model.SlipField, len(fields))
	for _, field := range fields {
		byCode[field.Code] = field
	}
	validations := make([]any, 0, len(f.CustomValidations))
	for _, validation := range f.CustomValidations {
		conditions := make([]any, 0, len(validation.Fields))
		for _, condition := range validation.Fields {
			field, ok := byCode[condition.SlipFieldCode]
			var fieldJSON any
			if ok {
				fieldJSON = pf08SlipField(field)
			}
			conditions = append(conditions, map[string]any{"id": condition.ID, "symbol": condition.Symbol, "value": condition.Value, "slipField": fieldJSON, "generalMasterItem": nil, "grade": nil, "team": nil})
		}
		validations = append(validations, map[string]any{"id": validation.ID, "combinationType": validation.CombinationType, "errorMessage": validation.ErrorMessage, "fields": conditions})
	}
	out["customValidations"] = validations
	return out
}

func pf08FormField(ctx context.Context, q store.Querier, f model.FormField) map[string]any {
	allowed := f.AllowedExtensions
	if allowed == nil {
		allowed = []string{}
	}
	statuses := f.AllowedTicketStatus
	if statuses == nil {
		statuses = []string{}
	}
	opts := f.Options
	if opts == nil {
		opts = []string{}
	}
	out := map[string]any{"id": f.ID, "title": f.Title, "description": f.Description, "fieldType": f.FieldType, "required": f.Required, "approver": f.Approver, "options": opts, "code": f.Code, "size": f.Size, "regexpFormat": f.RegexpFormat, "formula": f.Formula, "defaultValue": f.DefaultValue, "minValue": f.MinValue, "maxValue": f.MaxValue, "minLength": f.MinLength, "maxLength": f.MaxLength, "decimalDigit": f.DecimalDigit, "delimited": f.Delimited, "prefix": f.Prefix, "suffix": f.Suffix, "multiple": f.Multiple, "orientation": f.Orientation, "author": f.Author, "hidden": f.Hidden, "readonlyOnUi": f.ReadonlyOnUI, "allowedExtensions": allowed, "autoLink": f.AutoLink, "useTodayForDefaultValue": f.UseTodayForDefaultValue, "allowedTicketStatus": statuses}
	if f.GeneralMasterCode != "" {
		if id, e := store.ResolveID(ctx, q, "general_masters", f.GeneralMasterCode); e == nil {
			if m, e := GeneralMaster(ctx, q, id); e == nil {
				out["generalMaster"] = m
			} else {
				out["generalMaster"] = nil
			}
		} else {
			out["generalMaster"] = nil
		}
		out["defaultGeneralMasterItem"] = nil
	} else {
		out["generalMaster"] = nil
		out["defaultGeneralMasterItem"] = nil
	}
	out["externalApiSetting"] = nil
	out["kintoneAppSetting"] = nil
	out["climberCloudSetting"] = nil
	out["generalMasterSearchFilters"] = []any{}
	out["approverEditRestriction"] = nil
	out["approverEditSetting"] = map[string]any{"permissionType": "all", "descendants": false, "gradeSymbol": nil, "grade": nil, "team": nil, "users": []any{}}
	out["customValidations"] = []any{}
	out["ticketCopySetting"] = nil
	out["selectableTicketWorkflows"] = []any{}
	return out
}
func pf08SlipField(f model.SlipField) map[string]any {
	allowed := f.AllowedExtensions
	if allowed == nil {
		allowed = []string{}
	}
	statuses := f.AllowedTicketStatus
	if statuses == nil {
		statuses = []string{}
	}
	opts := f.Options
	if opts == nil {
		opts = []string{}
	}
	out := map[string]any{"id": f.ID, "fieldType": f.FieldType, "code": f.Code, "title": f.Title, "required": f.Required, "showTotal": f.ShowTotal, "options": opts, "regexpFormat": f.RegexpFormat, "formula": f.Formula, "maxValue": f.MaxValue, "minValue": f.MinValue, "defaultValue": f.DefaultValue, "decimalDigit": f.DecimalDigit, "delimited": f.Delimited, "allowedExtensions": allowed, "prefix": f.Prefix, "suffix": f.Suffix, "approver": f.Approver, "author": f.Author, "hidden": f.Hidden, "readonlyOnUi": f.ReadonlyOnUI, "minLength": f.MinLength, "maxLength": f.MaxLength, "multiple": f.Multiple, "autoLink": f.AutoLink, "useTodayForDefaultValue": f.UseTodayForDefaultValue, "allowedTicketStatus": statuses, "generalMaster": nil, "defaultGeneralMasterItem": nil, "generalMasterSearchFilters": []any{}, "approverEditRestriction": nil, "approverEditSetting": map[string]any{"permissionType": "all", "descendants": false, "gradeSymbol": nil, "grade": nil, "team": nil, "users": []any{}}, "customValidations": []any{}, "selectableTicketWorkflows": []any{}}
	return out
}
