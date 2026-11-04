package v1

import (
	"bytes"
	"encoding/json"
	"fmt"

	"punchflow/internal/kferr"
)

// pf22ValidateTicketRequestRequiredProperties validates required properties
// that encoding/json cannot distinguish from omitted fields in request structs.
// Unknown properties remain ignored, matching the REST request convention.
func pf22ValidateTicketRequestRequiredProperties(raw json.RawMessage, operation string) error {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return kferr.InvalidJSON()
	}
	if body == nil {
		return kferr.InvalidParameter("Invalid ticket request")
	}

	if err := pf22ValidateTicketArray(body, "inputs", operation, func(item map[string]json.RawMessage, path string) error {
		return pf22RequireOneOf(item, path, "formFieldId", "formFieldCode")
	}); err != nil {
		return err
	}
	if err := pf22ValidateTicketArray(body, "slipItems", operation, func(item map[string]json.RawMessage, path string) error {
		if operation == "updateTicketForm" {
			if err := pf22RequireOneOf(item, path, "slipSectionId", "slipSectionCode"); err != nil {
				return err
			}
		}
		if err := pf22RequirePresent(item, path, "inputs"); err != nil {
			return err
		}
		return pf22ValidateNestedArray(item, "inputs", path, func(field map[string]json.RawMessage, fieldPath string) error {
			return pf22RequireOneOf(field, fieldPath, "slipFieldId", "slipFieldCode")
		})
	}); err != nil {
		return err
	}
	if err := pf22ValidateTicketArray(body, "approvers", operation, func(item map[string]json.RawMessage, path string) error {
		if err := pf22RequirePresent(item, path, "userId"); err != nil {
			return err
		}
		return pf22RequirePresent(item, path, "routeStepCode")
	}); err != nil {
		return err
	}
	if err := pf22ValidateTicketArray(body, "viewers", operation, func(item map[string]json.RawMessage, path string) error {
		return pf22RequirePresent(item, path, "userId")
	}); err != nil {
		return err
	}
	if value, ok := body["cloudSignDocument"]; ok && !pf22IsJSONNull(value) {
		var document map[string]json.RawMessage
		if err := json.Unmarshal(value, &document); err == nil && document != nil {
			for _, property := range []string{"id", "title", "sandbox"} {
				if err := pf22RequirePresent(document, "cloudSignDocument", property); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func pf22ValidateTicketArray(body map[string]json.RawMessage, key, operation string, validate func(map[string]json.RawMessage, string) error) error {
	value, ok := body[key]
	if !ok || pf22IsJSONNull(value) {
		return nil
	}
	return pf22ValidateNestedArray(body, key, operation, validate)
}

func pf22ValidateNestedArray(parent map[string]json.RawMessage, key, path string, validate func(map[string]json.RawMessage, string) error) error {
	value, ok := parent[key]
	if !ok || pf22IsJSONNull(value) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err != nil {
		return nil // The typed request decoder reports the wrong container type.
	}
	for index, rawItem := range items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &item); err != nil {
			continue // The typed request decoder reports malformed item types.
		}
		if err := validate(item, fmt.Sprintf("%s.%s[%d]", path, key, index)); err != nil {
			return err
		}
	}
	return nil
}

func pf22RequireOneOf(item map[string]json.RawMessage, path string, properties ...string) error {
	for _, property := range properties {
		if value, ok := item[property]; ok && !pf22IsJSONNull(value) {
			return nil
		}
	}
	return kferr.InvalidParameter(fmt.Sprintf("%s requires one of %v", path, properties))
}

func pf22RequirePresent(item map[string]json.RawMessage, path, property string) error {
	value, ok := item[property]
	if !ok || pf22IsJSONNull(value) {
		return kferr.InvalidParameter(fmt.Sprintf("%s.%s is required", path, property))
	}
	return nil
}

func pf22IsJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}
