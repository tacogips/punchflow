package v1_test

import (
	"net/http"
	"testing"

	"punchflow/internal/testutil"
)

func TestUserEmailMustBeAddressOnly(t *testing.T) {
	env := testutil.New(t)
	rejected := []string{
		"Example User <display@example.test>",
		"<bracket@example.test>",
		"\"Quoted\" <quoted@example.test>",
		"comment@example.test (Example)",
		" padded@example.test",
		"no-domain",
	}
	for _, email := range rejected {
		resp := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": email, "firstName": "Mailbox", "lastName": "Form"})
		testutil.AssertError(t, resp, http.StatusBadRequest, "invalid_parameter")
	}
	created := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", map[string]any{"email": "plain-address@example.test", "firstName": "Plain", "lastName": "Address"})
	if created.Status != http.StatusOK {
		t.Fatalf("create user status=%d body=%s", created.Status, created.Body)
	}
	var user map[string]any
	created.JSON(t, &user)
	if user["email"] != "plain-address@example.test" {
		t.Fatalf("created email=%v", user["email"])
	}
	id := user["id"].(string)
	for _, email := range rejected {
		resp := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+id, map[string]any{"email": email})
		testutil.AssertError(t, resp, http.StatusBadRequest, "invalid_parameter")
	}
	updated := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+id, map[string]any{"email": "renamed-address@example.test"})
	if updated.Status != http.StatusOK {
		t.Fatalf("update user status=%d body=%s", updated.Status, updated.Body)
	}
	var after map[string]any
	updated.JSON(t, &after)
	if after["email"] != "renamed-address@example.test" {
		t.Fatalf("updated email=%v", after["email"])
	}
}
