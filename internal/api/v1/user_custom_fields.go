package v1

import (
	"context"
	"encoding/json"
	"fmt"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// pf18ApplyCustomFields validates custom field entries against the configured
// definitions and applies them over base without modifying it. An entry whose
// value is null clears that field. Unknown codes and values that do not fit the
// field type are rejected with a validation error.
func pf18ApplyCustomFields(ctx context.Context, q store.Querier, base map[string]any, entries []pf18CustomField) (map[string]any, error) {
	defs, err := store.UserCustomFieldList(ctx, q)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]model.UserCustomField, len(defs))
	for _, def := range defs {
		byCode[def.Code] = def
	}
	out := make(map[string]any, len(base)+len(entries))
	for code, value := range base {
		out[code] = value
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Code == "" || len(entry.Value) == 0 {
			return nil, kferr.InvalidParameter("Invalid customFields")
		}
		def, ok := byCode[entry.Code]
		if !ok {
			return nil, kferr.ValidationFailed(map[string][]string{"customFields": {fmt.Sprintf("contains an unknown code %q", entry.Code)}})
		}
		if seen[entry.Code] {
			return nil, kferr.ValidationFailed(map[string][]string{"customFields": {fmt.Sprintf("contains the code %q more than once", entry.Code)}})
		}
		seen[entry.Code] = true
		var rawValue any
		if err := json.Unmarshal(entry.Value, &rawValue); err != nil {
			return nil, kferr.InvalidParameter("Invalid customFields")
		}
		value, err := def.NormalizeValue(rawValue)
		if err != nil {
			return nil, kferr.ValidationFailed(map[string][]string{"customFields": {fmt.Sprintf("value for %q %s", entry.Code, err.Error())}})
		}
		if value == nil {
			delete(out, entry.Code)
			continue
		}
		out[entry.Code] = value
	}
	return out, nil
}
