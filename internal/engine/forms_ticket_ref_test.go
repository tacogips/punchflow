package engine

import (
	"testing"

	"punchflow/internal/model"
)

func TestTicketReferenceSelectableWorkflowRestriction(t *testing.T) {
	e := pf14Setup(t)
	allowedID, err := pf14cCreate(t, e, "expense-claim", "sales-rep", nil, nil)
	if err != nil {
		t.Fatalf("create allowed source ticket: %v", err)
	}
	disallowedID, err := pf14cCreate(t, e, "leave-request", "sales-rep", nil, nil)
	if err != nil {
		t.Fatalf("create disallowed source ticket: %v", err)
	}
	var fieldID string
	pf16Redefine(t, e, "purchase-request", func(def *model.WorkflowDef) {
		for sectionIndex := range def.SectionList {
			for i := range def.SectionList[sectionIndex].FormFields {
				if def.SectionList[sectionIndex].FormFields[i].Code == "related" {
					def.SectionList[sectionIndex].FormFields[i].SelectableTicketWorkflowCodes = []string{"expense-claim"}
					fieldID = def.SectionList[sectionIndex].FormFields[i].ID
				}
			}
		}
	})
	_, fields := pf14FormDefinition(t, e, "purchase-request")
	for _, source := range []struct {
		name, id  string
		wantError bool
	}{
		{name: "allowed workflow", id: allowedID},
		{name: "disallowed workflow", id: disallowedID, wantError: true},
	} {
		t.Run(source.name, func(t *testing.T) {
			_, createErr := pf14cCreate(t, e, "purchase-request", "sales-rep", map[string]any{"related": source.id}, nil)
			if source.wantError && createErr == nil {
				t.Fatal("ticket from an unselectable workflow was accepted")
			}
			if !source.wantError && createErr != nil {
				t.Fatalf("allowed ticket reference rejected: %v", createErr)
			}
		})
	}
	if fieldID == "" || fields["related"] != fieldID {
		t.Fatal("ticket reference restriction was not retained in workflow fixture")
	}
}
