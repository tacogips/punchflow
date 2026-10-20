package punchflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"testing"

	fixturetypes "punchflow/internal/fixtures"
	"punchflow/internal/model"
)

func TestDefaultFixturesStrictDecode(t *testing.T) {
	files := []struct {
		name string
		new  func() any
	}{
		{fixturetypes.FileTenant, func() any { return &fixturetypes.TenantFile{} }},
		{fixturetypes.FileGrades, func() any { return &[]fixturetypes.GradeFixture{} }},
		{fixturetypes.FileUsers, func() any { return &[]fixturetypes.UserFixture{} }},
		{fixturetypes.FileOrganizationCharts, func() any { return &[]fixturetypes.OrgChartFixture{} }},
		{fixturetypes.FileRoles, func() any { return &[]fixturetypes.RoleFixture{} }},
		{fixturetypes.FileCategories, func() any { return &[]fixturetypes.CategoryFixture{} }},
		{fixturetypes.FileFolders, func() any { return &[]fixturetypes.FolderFixture{} }},
		{fixturetypes.FileGeneralMasters, func() any { return &[]fixturetypes.GeneralMasterFixture{} }},
		{fixturetypes.FileRoutes, func() any { return &[]model.RouteDef{} }},
		{fixturetypes.FileWorkflows, func() any { return &[]model.WorkflowDef{} }},
		{fixturetypes.FileProxyApplicants, func() any { return &[]fixturetypes.ProxyFixture{} }},
		{fixturetypes.FileProxyApprovers, func() any { return &[]fixturetypes.ProxyFixture{} }},
		{fixturetypes.FileTokens, func() any { return &[]fixturetypes.TokenFixture{} }},
		{fixturetypes.FileWebhooks, func() any { return &[]fixturetypes.WebhookFixture{} }},
	}
	for _, file := range files {
		t.Run(file.name, func(t *testing.T) {
			data, err := fs.ReadFile(DefaultFixtures, "fixtures/default/"+file.name)
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(file.new()); err != nil {
				t.Fatalf("strict decode: %v", err)
			}
		})
	}
}

func TestDefaultFixturePersonasAndDefinitions(t *testing.T) {
	var tokens []fixturetypes.TokenFixture
	if err := decodeDefaultFixture(fixturetypes.FileTokens, &tokens); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"pf_pat_dev_admin": "admin", "pf_pat_dev_manager": "sales-manager",
		"pf_pat_dev_finance": "finance-manager", "pf_pat_dev_employee": "sales-rep",
		"pf_sat_dev_integration": "",
	}
	for _, token := range tokens {
		owner, ok := want[token.Token]
		if !ok {
			t.Errorf("unexpected fixture token %q", token.Token)
			continue
		}
		if token.UserCode != owner {
			t.Errorf("token %q owner = %q, want %q", token.Token, token.UserCode, owner)
		}
		delete(want, token.Token)
	}
	for token := range want {
		t.Errorf("missing fixture token %q", token)
	}
	var definitions fixturetypes.DefinitionsFile
	if err := decodeDefaultFixture(fixturetypes.FileWorkflows, &definitions.Workflows); err != nil {
		t.Fatal(err)
	}
	if err := decodeDefaultFixture(fixturetypes.FileRoutes, &definitions.Routes); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"expense-claim", "purchase-request", "leave-request"} {
		if !containsDefaultCodeWorkflow(definitions.Workflows, code) {
			t.Errorf("missing workflow %q", code)
		}
	}
	for _, code := range []string{"expense-standard", "expense-large", "purchase", "leave"} {
		if !containsDefaultCodeRoute(definitions.Routes, code) {
			t.Errorf("missing route %q", code)
		}
	}
	for _, route := range definitions.Routes {
		for _, step := range route.Steps {
			number := step.RequiredApprovalsNumber > 0
			percent := step.RequiredApprovalsPercent > 0
			if number == percent {
				t.Errorf("route %s step %s must have exactly one positive approval requirement", route.Code, step.Code)
			}
		}
	}
}

func TestKeycloakRealmMatchesFixtureUsersAndPKCE(t *testing.T) {
	var fixtureUsers []fixturetypes.UserFixture
	if err := decodeDefaultFixture(fixturetypes.FileUsers, &fixtureUsers); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("deploy/keycloak/realm-punchflow.json")
	if err != nil {
		t.Fatal(err)
	}
	var realm struct {
		Realm   string `json:"realm"`
		Clients []struct {
			ClientID                  string            `json:"clientId"`
			PublicClient              bool              `json:"publicClient"`
			StandardFlowEnabled       bool              `json:"standardFlowEnabled"`
			DirectAccessGrantsEnabled bool              `json:"directAccessGrantsEnabled"`
			ImplicitFlowEnabled       bool              `json:"implicitFlowEnabled"`
			Attributes                map[string]string `json:"attributes"`
			RedirectURIs              []string          `json:"redirectUris"`
			WebOrigins                []string          `json:"webOrigins"`
		} `json:"clients"`
		Users []struct {
			Username string `json:"username"`
			Email    string `json:"email"`
		} `json:"users"`
	}
	if err := json.Unmarshal(data, &realm); err != nil {
		t.Fatal(err)
	}
	if realm.Realm != "punchflow" || len(realm.Clients) != 1 {
		t.Fatalf("unexpected realm/client definition: realm=%q clients=%d", realm.Realm, len(realm.Clients))
	}
	client := realm.Clients[0]
	if client.ClientID != "punchflow-ui" || !client.PublicClient || !client.StandardFlowEnabled || client.DirectAccessGrantsEnabled || client.ImplicitFlowEnabled || client.Attributes["pkce.code.challenge.method"] != "S256" || !containsDefaultString(client.RedirectURIs, "http://localhost:8080/auth/oidc/callback") || !containsDefaultString(client.WebOrigins, "http://localhost:8080") {
		t.Errorf("Keycloak client does not match the PKCE callback contract")
	}
	wantEmails := make(map[string]bool, len(fixtureUsers))
	for _, user := range fixtureUsers {
		wantEmails[user.Email] = true
	}
	for _, user := range realm.Users {
		if !wantEmails[user.Email] {
			t.Errorf("realm user %s has unexpected e-mail %s", user.Username, user.Email)
		}
		delete(wantEmails, user.Email)
	}
	for email := range wantEmails {
		t.Errorf("fixture user e-mail missing from realm: %s", email)
	}
	if len(realm.Users) != len(fixtureUsers) {
		t.Errorf("realm user count = %d, want %d", len(realm.Users), len(fixtureUsers))
	}
}

func decodeDefaultFixture(name string, dst any) error {
	data, err := fs.ReadFile(DefaultFixtures, "fixtures/default/"+name)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func containsDefaultCodeWorkflow(values []model.WorkflowDef, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

func containsDefaultCodeRoute(values []model.RouteDef, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

func containsDefaultString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
