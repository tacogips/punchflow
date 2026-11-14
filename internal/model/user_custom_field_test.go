package model

import (
	"reflect"
	"testing"
)

func TestUserCustomFieldNormalizeValue(t *testing.T) {
	number := UserCustomField{Code: "n", FieldType: UserCustomFieldTypeNumber}
	integer := UserCustomField{Code: "i", FieldType: UserCustomFieldTypeInteger}
	pull := UserCustomField{Code: "p", FieldType: UserCustomFieldTypePullDown, Options: []string{"a", "b"}}
	check := UserCustomField{Code: "c", FieldType: UserCustomFieldTypeCheckbox, Options: []string{"a", "b"}}
	date := UserCustomField{Code: "d", FieldType: UserCustomFieldTypeDate}
	text := UserCustomField{Code: "t", FieldType: UserCustomFieldTypeText}
	long := UserCustomField{Code: "l", FieldType: UserCustomFieldTypeTextLong}

	valid := []struct {
		name  string
		field UserCustomField
		in    any
		want  any
	}{
		{"null clears", text, nil, nil},
		{"text", text, "x", "x"},
		{"textLong", long, "multi\nline", "multi\nline"},
		{"number string", number, "-12.50", "-12.50"},
		{"number JSON float is stringified", number, 3.25, "3.25"},
		{"integer string", integer, "42", "42"},
		{"integer JSON whole number is stringified", integer, float64(7), "7"},
		{"pullDown", pull, "b", "b"},
		{"checkbox", check, []any{"b", "a"}, []string{"b", "a"}},
		{"empty checkbox", check, []any{}, []string{}},
		{"date", date, "2024-02-29", "2024-02-29"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.field.NormalizeValue(tc.in)
			if err != nil {
				t.Fatalf("NormalizeValue(%v) error = %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("NormalizeValue(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}

	invalid := []struct {
		name  string
		field UserCustomField
		in    any
	}{
		{"text object", text, map[string]any{"a": 1}},
		{"text bool", text, true},
		{"number with separators", number, "1,000"},
		{"number text", number, "abc"},
		{"number bool", number, false},
		{"integer fraction string", integer, "1.5"},
		{"integer fraction float", integer, 1.5},
		{"pullDown unknown option", pull, "z"},
		{"pullDown array", pull, []any{"a"}},
		{"checkbox duplicate", check, []any{"a", "a"}},
		{"checkbox non-string item", check, []any{1}},
		{"checkbox string", check, "a"},
		{"date bad format", date, "2024/01/02"},
		{"date impossible", date, "2023-02-29"},
		{"date number", date, 20240102},
		{"unsupported type", UserCustomField{Code: "x", FieldType: "mystery"}, "v"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := tc.field.NormalizeValue(tc.in); err == nil {
				t.Fatalf("NormalizeValue(%v) = %#v, want an error", tc.in, got)
			}
		})
	}
}
