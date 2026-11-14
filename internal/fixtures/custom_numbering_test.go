package fixtures

import (
	"strings"
	"testing"

	"punchflow"
	"punchflow/internal/model"
)

func TestValidateCustomNumberingFixtureDefinitions(t *testing.T) {
	fsys, err := DefaultFS(punchflow.DefaultFixtures)
	if err != nil {
		t.Fatal(err)
	}
	set, err := Parse(fsys)
	if err != nil {
		t.Fatal(err)
	}
	format := "DEC-[number]"
	numberingID := "424a3c20-74e3-4a73-9466-1a45375486b1"
	set.Workflows[0].CustomNumberings = []model.CustomNumbering{{ID: numberingID, Code: "DECISION", Name: "Decision number", Value: 1}}
	set.Workflows[0].CustomNumberingSettings = []model.CustomNumberingSetting{{ID: "524a3c20-74e3-4a4a-924b-693fbb3bffb8", CustomNumberingID: numberingID, TicketNumberFormat: &format, TimingType: "opened", SortOrder: 1}}
	if err := Validate(set); err != nil {
		t.Fatalf("valid custom numbering fixture rejected: %v", err)
	}

	set.Workflows[0].CustomNumberingSettings[0].TicketNumberFormat = ptrFixtureString("DEC-[year]")
	if err := Validate(set); err == nil || !strings.Contains(err.Error(), ".ticketNumberFormat") {
		t.Fatalf("format without [number] error=%v", err)
	}
	set.Workflows[0].CustomNumberingSettings[0].TicketNumberFormat = &format
	set.Workflows[0].CustomNumberingSettings[0].CustomNumberingID = "624a3c20-74e3-4a4a-924b-693fbb3bffb8"
	if err := Validate(set); err == nil || !strings.Contains(err.Error(), ".customNumberingId") {
		t.Fatalf("unknown numbering reference error=%v", err)
	}
}

func ptrFixtureString(value string) *string { return &value }
