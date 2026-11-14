package contract_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var jstTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\+09:00$`)

func validateJSON(t *testing.T, schema *jsonschema.Schema, data []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode response JSON: %v", err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("response does not match schema: %v", err)
	}
}

func checkTimestamps(t *testing.T, value any) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if strings.HasSuffix(key, "At") {
				stamp, isString := child.(string)
				if isString && stamp != "" && !jstTimestamp.MatchString(stamp) {
					t.Errorf("timestamp %s = %v, want JST milliseconds", key, child)
				}
			}
			checkTimestamps(t, child)
		}
	case []any:
		for _, child := range current {
			checkTimestamps(t, child)
		}
	}
}
