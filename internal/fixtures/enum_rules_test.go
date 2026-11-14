package fixtures

import (
	"os"
	"strconv"
	"testing"

	"punchflow/internal/model"
)

func TestEnumRuleCases(t *testing.T) {
	checks := []struct {
		name     string
		file     string
		path     string
		mutate   func(*Set, string)
		invalid  string
		accepted string
	}{
		{"step type rejects grade", FileRoutes, "[0].steps[0].stepType", func(s *Set, value string) { s.Routes[0].Steps[0].StepType = value }, "grade", "manager"},
		{"permission rejects audit_read", FileRoles, "[0].permissionList[0].permission", func(s *Set, value string) { s.Roles[0].PermissionList[0].Permission = value }, "audit_read", "workflow_misc"},
		{"user type rejects member", FileUsers, "[0].userType", func(s *Set, value string) { s.Users[0].UserType = value }, "member", "normal"},
		{"route status rejects active", FileRoutes, "[0].status", func(s *Set, value string) { s.Routes[0].Status = value }, "active", "visible"},
		{"section type rejects conditional", FileWorkflows, "[0].sectionList[0].sectionType", func(s *Set, value string) { s.Workflows[0].SectionList[0].SectionType = value }, "conditional", "form"},
		{"master sort rejects createdAt", FileGeneralMasters, "[0].defaultSortBy", func(s *Set, value string) { s.GeneralMasters[0].DefaultSortBy = value }, "createdAt", "code"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			set := pf02FixtureSet(t)
			check.mutate(set, check.accepted)
			if pf02HasProblemAt(Validate(set), check.file, check.path) {
				t.Fatalf("accepted value %q produced a problem at %s:%s", check.accepted, check.file, check.path)
			}
			check.mutate(set, check.invalid)
			if !pf02HasProblemAt(Validate(set), check.file, check.path) {
				t.Fatalf("invalid value %q produced no problem at %s:%s", check.invalid, check.file, check.path)
			}
		})
	}

	t.Run("grade symbol empty string rejected and nil accepted", func(t *testing.T) {
		set := pf02FixtureSet(t)
		target, path := pf02FirstTarget(set)
		target.GradeSymbol = nil
		if pf02HasProblemAt(Validate(set), FileRoutes, path) {
			t.Fatalf("nil gradeSymbol produced a problem at %s:%s", FileRoutes, path)
		}
		empty := ""
		target.GradeSymbol = &empty
		if !pf02HasProblemAt(Validate(set), FileRoutes, path) {
			t.Fatalf("empty gradeSymbol produced no problem at %s:%s", FileRoutes, path)
		}
	})

	t.Run("webhook rejects invented event and accepts canonical event", func(t *testing.T) {
		set := pf02FixtureSet(t)
		set.Webhooks = append(set.Webhooks, WebhookFixture{ID: "00000000-0000-4000-8000-000000000099", EventTypes: []string{"ticket_opened"}})
		path := "[0].eventTypes[0]"
		if pf02HasProblemAt(Validate(set), FileWebhooks, path) {
			t.Fatalf("canonical event produced a problem at %s:%s", FileWebhooks, path)
		}
		set.Webhooks[0].EventTypes[0] = "ticket_submitted"
		if !pf02HasProblemAt(Validate(set), FileWebhooks, path) {
			t.Fatalf("invented event produced no problem at %s:%s", FileWebhooks, path)
		}
	})
}

func pf02FixtureSet(t *testing.T) *Set {
	t.Helper()
	set, err := Parse(os.DirFS("../../fixtures/default"))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func pf02FirstTarget(set *Set) (*model.RouteTarget, string) {
	for ri := range set.Routes {
		for si := range set.Routes[ri].Steps {
			if len(set.Routes[ri].Steps[si].Targets) > 0 {
				return &set.Routes[ri].Steps[si].Targets[0], pf02RouteTargetPath(ri, si)
			}
		}
	}
	panic("default route fixtures contain no target")
}

func pf02RouteTargetPath(route, step int) string {
	return "[" + strconv.Itoa(route) + "].steps[" + strconv.Itoa(step) + "].targets[0].gradeSymbol"
}

func pf02HasProblemAt(err error, file, path string) bool {
	if err == nil {
		return false
	}
	validation, ok := err.(*ValidationError)
	if !ok {
		return false
	}
	for _, problem := range validation.Problems {
		if problem.File == file && problem.Path == path {
			return true
		}
	}
	return false
}
