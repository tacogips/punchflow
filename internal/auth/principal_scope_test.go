package auth

import (
	"reflect"
	"testing"

	"punchflow/internal/model"
)

func TestPrincipalScopeHelpers(t *testing.T) {
	p := &Principal{
		Permissions: map[string]bool{"master": true, "ticket_read": true, "team": true},
		Grants: map[string]model.PermissionGrant{
			"master":      {Targets: map[string]bool{"m2": true, "m1": true}},
			"ticket_read": {Unrestricted: true},
			"team":        {Targets: map[string]bool{}},
		},
	}
	if p.Unrestricted("master") || !p.Unrestricted("ticket_read") || p.Unrestricted("audit") {
		t.Fatal("Unrestricted reported the wrong scope")
	}
	if !p.Allows("master", "m1") || p.Allows("master", "m3") || !p.Allows("ticket_read", "anything") || p.Allows("audit", "x") {
		t.Fatal("Allows reported the wrong scope")
	}
	if p.Allows("team", "t1") {
		t.Fatal("a restricted grant without targets must cover nothing")
	}
	if !p.AllowsAny("m2", "team", "master") || p.AllowsAny("m3", "team", "master") {
		t.Fatal("AllowsAny reported the wrong scope")
	}
	if got, want := p.Targets("master"), []string{"m1", "m2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets = %v, want %v", got, want)
	}
	if !p.Has("team") {
		t.Fatal("Has must stay true for restricted permissions so handlers can reach the per-resource check")
	}
}

func TestPrincipalWithoutGrantsTreatsPermissionsAsUnrestricted(t *testing.T) {
	p := &Principal{Permissions: map[string]bool{"master": true}}
	if !p.Unrestricted("master") || !p.Allows("master", "any") || p.Allows("team", "any") {
		t.Fatal("principals built without grants must keep the legacy unrestricted meaning")
	}
	var nilPrincipal *Principal
	if nilPrincipal.Unrestricted("master") || nilPrincipal.Allows("master", "x") {
		t.Fatal("a nil principal holds nothing")
	}
}
