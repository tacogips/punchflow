package model

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// User custom field input types (camelCase, as in User.customFields[].fieldType).
const (
	UserCustomFieldTypeText     = "text"
	UserCustomFieldTypeTextLong = "textLong"
	UserCustomFieldTypeNumber   = "number"
	UserCustomFieldTypeInteger  = "integer"
	UserCustomFieldTypeCheckbox = "checkbox"
	UserCustomFieldTypePullDown = "pullDown"
	UserCustomFieldTypeDate     = "date"
)

// UserCustomFieldTypes lists every supported user custom field type.
var UserCustomFieldTypes = []string{
	UserCustomFieldTypeText, UserCustomFieldTypeTextLong, UserCustomFieldTypeNumber,
	UserCustomFieldTypeInteger, UserCustomFieldTypeCheckbox, UserCustomFieldTypePullDown, UserCustomFieldTypeDate,
}

// UserCustomField is the definition of one user custom field. Only values for
// defined codes are accepted and presented; VisibleToEveryone controls whether
// callers without the user-management permission can see the value.
type UserCustomField struct {
	ID                string    `json:"id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	FieldType         string    `json:"fieldType"`
	Options           []string  `json:"options,omitempty"`
	VisibleToEveryone bool      `json:"visibleToEveryone"`
	FieldOrder        int       `json:"fieldOrder"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

var (
	userCustomNumberPattern  = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?$`)
	userCustomIntegerPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)
)

// NormalizeValue validates a decoded JSON value against the definition and
// returns the canonical value to store. A nil result means "no value" (the
// field is cleared). Number and integer values are stored as plain numeric
// strings; a JSON number is accepted and converted.
func (f UserCustomField) NormalizeValue(raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	switch f.FieldType {
	case UserCustomFieldTypeText, UserCustomFieldTypeTextLong:
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}
		return value, nil
	case UserCustomFieldTypeNumber:
		return normalizeNumeric(raw, userCustomNumberPattern, false)
	case UserCustomFieldTypeInteger:
		return normalizeNumeric(raw, userCustomIntegerPattern, true)
	case UserCustomFieldTypePullDown:
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}
		if !containsString(f.Options, value) {
			return nil, fmt.Errorf("%q is not an available option", value)
		}
		return value, nil
	case UserCustomFieldTypeCheckbox:
		list, ok := raw.([]any)
		if !ok {
			if typed, isStrings := raw.([]string); isStrings {
				list = make([]any, len(typed))
				for i, v := range typed {
					list[i] = v
				}
			} else {
				return nil, errors.New("must be an array of strings")
			}
		}
		out := make([]string, 0, len(list))
		seen := map[string]bool{}
		for _, item := range list {
			value, ok := item.(string)
			if !ok {
				return nil, errors.New("must be an array of strings")
			}
			if !containsString(f.Options, value) {
				return nil, fmt.Errorf("%q is not an available option", value)
			}
			if seen[value] {
				return nil, fmt.Errorf("%q is selected more than once", value)
			}
			seen[value] = true
			out = append(out, value)
		}
		return out, nil
	case UserCustomFieldTypeDate:
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("must be a date string")
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return nil, errors.New("must be a date in YYYY-MM-DD format")
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported field type %q", f.FieldType)
	}
}

func normalizeNumeric(raw any, pattern *regexp.Regexp, whole bool) (any, error) {
	switch value := raw.(type) {
	case string:
		if !pattern.MatchString(value) {
			return nil, errors.New("must be a plain numeric string without separators")
		}
		return value, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || (whole && value != math.Trunc(value)) {
			return nil, errors.New("must be a plain numeric value")
		}
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	default:
		return nil, errors.New("must be a numeric string")
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
