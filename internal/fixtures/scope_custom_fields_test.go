package fixtures

import (
	"context"
	"reflect"
	"testing"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

func TestValidateUserCustomFieldDefinitionsAndValues(t *testing.T) {
	checks := []struct {
		name   string
		file   string
		path   string
		mutate func(*Set)
	}{
		{"unknown field type", FileUserCustomFields, "[0].fieldType", func(s *Set) { s.UserCustomFields[0].FieldType = "boolean" }},
		{"selection field without options", FileUserCustomFields, "[1].options", func(s *Set) { s.UserCustomFields[1].Options = nil }},
		{"duplicate code", FileUserCustomFields, "[1].code", func(s *Set) { s.UserCustomFields[1].Code = s.UserCustomFields[0].Code }},
		{"blank name", FileUserCustomFields, "[0].name", func(s *Set) { s.UserCustomFields[0].Name = " " }},
		{"user value for undefined code", FileUsers, "[1].customFields.ghost", func(s *Set) { s.Users[1].CustomFields = map[string]any{"ghost": "x"} }},
		{"user value of the wrong type", FileUsers, "[1].customFields.cost-center", func(s *Set) { s.Users[1].CustomFields = map[string]any{"cost-center": map[string]any{"a": 1}} }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			set := pf02FixtureSet(t)
			if err := Validate(set); err != nil {
				t.Fatalf("default fixtures must be valid: %v", err)
			}
			check.mutate(set)
			if !pf02HasProblemAt(Validate(set), check.file, check.path) {
				t.Fatalf("no validation problem at %s:%s", check.file, check.path)
			}
		})
	}
}

// TestRestrictedRoleScopeRoundTrips guards that restricted role permissions
// (flag and target IDs) survive loading and exporting fixtures.
func TestRestrictedRoleScopeRoundTrips(t *testing.T) {
	ctx := context.Background()
	set := pf02FixtureSet(t)
	want := model.RolePermission{
		Permission: model.PermissionTicketRead, Restricted: true,
		FolderIDs: []string{"00000000-0000-4000-8000-000000000037"},
	}
	set.Roles[1].PermissionList = []model.RolePermission{want}
	if err := Validate(set); err != nil {
		t.Fatalf("validate: %v", err)
	}
	db := pf06OpenDB(t)
	if err := db.WithTx(ctx, func(tx *store.Tx) error {
		_, err := pf06InsertTx(ctx, tx, set, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	exported, err := Export(ctx, db.Q())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var got *RoleFixture
	reparsed, err := Parse(pf06MapFS(exported))
	if err != nil {
		t.Fatalf("reparse export: %v", err)
	}
	for i := range reparsed.Roles {
		if reparsed.Roles[i].ID == set.Roles[1].ID {
			got = &reparsed.Roles[i]
		}
	}
	if got == nil || len(got.PermissionList) != 1 || !reflect.DeepEqual(got.PermissionList[0], want) {
		t.Fatalf("exported role permissions = %+v, want %+v", got, want)
	}
	memberID := ""
	for _, u := range set.Users {
		if u.Code == set.Roles[1].MemberCodes[0] {
			memberID = u.ID
		}
	}
	grants, err := store.UserGrants(ctx, db.Q(), memberID)
	if err != nil {
		t.Fatalf("grants: %v", err)
	}
	grant := grants[model.PermissionTicketRead]
	if grant.Unrestricted || !grant.Targets["00000000-0000-4000-8000-000000000037"] {
		t.Fatalf("grant = %+v, want restricted to the finance folder", grant)
	}
}
