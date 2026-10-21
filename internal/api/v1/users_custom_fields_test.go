package v1_test

import (
	"net/http"
	"reflect"
	"testing"

	"punchflow/internal/testutil"
)

type customFieldView struct {
	Code      string `json:"code"`
	Value     any    `json:"value"`
	FieldType string `json:"fieldType"`
}

func customFieldsOf(t *testing.T, user map[string]any) []customFieldView {
	t.Helper()
	raw, ok := user["customFields"].([]any)
	if !ok {
		return nil
	}
	out := make([]customFieldView, 0, len(raw))
	for _, item := range raw {
		entry := item.(map[string]any)
		out = append(out, customFieldView{Code: entry["code"].(string), Value: entry["value"], FieldType: entry["fieldType"].(string)})
	}
	return out
}

func customCodes(fields []customFieldView) []string {
	codes := make([]string, len(fields))
	for i, f := range fields {
		codes[i] = f.Code
	}
	return codes
}

func newUserBody(email string, fields []map[string]any) map[string]any {
	return map[string]any{"email": email, "firstName": "Custom", "lastName": "Fields", "customFields": fields}
}

func TestUserCustomFieldsAreValidatedAgainstDefinitions(t *testing.T) {
	env := testutil.New(t)
	invalid := []struct {
		name  string
		field map[string]any
	}{
		{"object value", map[string]any{"code": "cost-center", "value": map[string]any{"a": 1}}},
		{"boolean value", map[string]any{"code": "cost-center", "value": true}},
		{"number for text", map[string]any{"code": "cost-center", "value": 5}},
		{"undefined code", map[string]any{"code": "no-such-field", "value": "x"}},
		{"option not offered", map[string]any{"code": "employment-type", "value": "temp"}},
		{"bad date", map[string]any{"code": "hire-date", "value": "2024-13-40"}},
		{"checkbox needs array", map[string]any{"code": "skills", "value": "go"}},
		{"checkbox option unknown", map[string]any{"code": "skills", "value": []string{"cobol"}}},
		{"empty code", map[string]any{"code": "", "value": "x"}},
	}
	for i, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", newUserBody("invalid"+string(rune('a'+i))+"@example.test", []map[string]any{tc.field}))
			if res.Status != http.StatusUnprocessableEntity && res.Status != http.StatusBadRequest {
				t.Fatalf("status=%d want 422/400 body=%s", res.Status, res.Body)
			}
		})
	}

	res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/users", newUserBody("valid@example.test", []map[string]any{
		{"code": "skills", "value": []string{"sql", "go"}},
		{"code": "hire-date", "value": "2024-01-02"},
		{"code": "employment-type", "value": "full-time"},
		{"code": "cost-center", "value": "CC-9"},
	}))
	created := decodeObject(t, res)
	fields := customFieldsOf(t, created)
	// Definition order, not request order, and every entry carries its field type.
	if got, want := customCodes(fields), []string{"cost-center", "employment-type", "hire-date", "skills"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("custom field order = %v, want %v", got, want)
	}
	if fields[0].FieldType != "text" || fields[1].FieldType != "pullDown" || fields[2].FieldType != "date" || fields[3].FieldType != "checkbox" {
		t.Fatalf("field types = %+v", fields)
	}
}

func TestUserCustomFieldsPatchMergesAndNullClears(t *testing.T) {
	env := testutil.New(t)
	id := env.UserID(t, "admin")
	before := customFieldsOf(t, decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+id, nil)))
	if got, want := customCodes(before), []string{"cost-center", "hire-date", "skills"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fixture custom fields = %v, want %v", got, want)
	}

	// Entries that are not mentioned keep their value.
	merged := customFieldsOf(t, decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+id, map[string]any{
		"customFields": []map[string]any{{"code": "cost-center", "value": "CC-101"}},
	})))
	if got, want := customCodes(merged), []string{"cost-center", "hire-date", "skills"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after partial patch fields = %v, want %v", got, want)
	}
	if merged[0].Value != "CC-101" {
		t.Fatalf("cost-center = %v", merged[0].Value)
	}

	// null clears exactly that field.
	cleared := customFieldsOf(t, decodeObject(t, env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+id, map[string]any{
		"customFields": []map[string]any{{"code": "hire-date", "value": nil}},
	})))
	if got, want := customCodes(cleared), []string{"cost-center", "skills"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after null clear fields = %v, want %v", got, want)
	}

	// A rejected patch leaves the stored values untouched.
	bad := env.Do(t, testutil.Admin, http.MethodPatch, "/v1/users/"+id, map[string]any{
		"customFields": []map[string]any{{"code": "cost-center", "value": "CC-102"}, {"code": "skills", "value": map[string]any{"x": 1}}},
	})
	if bad.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid patch status=%d body=%s", bad.Status, bad.Body)
	}
	after := customFieldsOf(t, decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+id, nil)))
	if after[0].Value != "CC-101" {
		t.Fatalf("rejected patch changed stored value: %+v", after)
	}
}

func TestUserCustomFieldsOmitUndefinedCodesAndHideRestrictedFields(t *testing.T) {
	env := testutil.New(t)
	id := env.UserID(t, "sales-rep")
	// Stale data for a code without a definition must never reach a response.
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE users SET custom_fields=? WHERE id=?`,
		`{"cost-center":"CC-200","hire-date":"2021-05-06","ghost":{"nested":true}}`, id); err != nil {
		t.Fatal(err)
	}
	asAdmin := customFieldsOf(t, decodeObject(t, env.Do(t, testutil.Admin, http.MethodGet, "/v1/users/"+id, nil)))
	if got, want := customCodes(asAdmin), []string{"cost-center", "hire-date"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("admin view = %v, want %v (undefined code omitted, hidden field visible to user managers)", got, want)
	}
	// Without the user permission only fields visible to everyone are returned.
	asSelf := customFieldsOf(t, decodeObject(t, env.Do(t, testutil.Employee, http.MethodGet, "/v1/user", nil)))
	if got, want := customCodes(asSelf), []string{"cost-center"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("self view = %v, want %v", got, want)
	}
	list := env.Do(t, testutil.Admin, http.MethodGet, "/v1/users?perPage=100", nil)
	var users []map[string]any
	list.JSON(t, &users)
	for _, u := range users {
		for _, f := range customFieldsOf(t, u) {
			if f.Code == "ghost" {
				t.Fatalf("list exposed undefined custom field: %v", u)
			}
		}
	}
}
