package fixtures

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestValidateMinimal(t *testing.T) {
	s, err := Parse(pf06FSSub(t, "testdata/minimal"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(s); err != nil {
		t.Fatalf("Validate(minimal): %v", err)
	}
}

func TestValidateInvalidCases(t *testing.T) {
	cases := []struct {
		name string
		file string
		path string
	}{
		{"unknown-key", FileTenant, "unexpected"},
		{"duplicate-id", FileGrades, "$[1].id"},
		{"bad-enum", FileUsers, "[0].status"},
		{"dangling-user-code", FileRoles, "[0].memberCodes[1]"},
		{"two-current-charts", FileOrganizationCharts, "$"},
		{"custom-combination", FileWorkflows, "[0].routeConditions[0].combinationType"},
		{"other-variable", FileWorkflows, "[0].routeConditions[0].conditionFields[0].fieldKey"},
		{"both-thresholds", FileRoutes, "[0].steps[0]"},
		{"route-no-else", FileWorkflows, "[0].routeConditions"},
		{"pat-without-user", FileTokens, "[0].userCode"},
		{"step-type-grade", FileRoutes, "[0].steps[0].stepType"},
		{"permission-audit-read", FileRoles, "[0].permissionList[0].permission"},
		{"event-ticket-submitted", FileWebhooks, "[0].eventTypes[0]"},
		{"user-type-member", FileUsers, "[0].userType"},
		{"route-status-active", FileRoutes, "[0].status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set, err := Parse(pf06FSSub(t, "testdata/invalid/"+tc.name))
			var validationErr *ValidationError
			if err != nil {
				if !errors.As(err, &validationErr) {
					t.Fatalf("Parse error=%v; want ValidationError", err)
				}
			} else {
				err = Validate(set)
				if !errors.As(err, &validationErr) {
					t.Fatalf("Validate error=%v; want ValidationError", err)
				}
			}
			if len(validationErr.Problems) != 1 {
				t.Fatalf("got %d problems: %+v; want exactly one", len(validationErr.Problems), validationErr.Problems)
			}
			got := validationErr.Problems[0]
			if got.File != tc.file || got.Path != tc.path {
				t.Fatalf("problem=%+v; want file=%s path=%s", got, tc.file, tc.path)
			}
		})
	}
}

func pf06FSSub(t *testing.T, dir string) fs.FS {
	t.Helper()
	root := os.DirFS(".")
	sub, err := fs.Sub(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}
