package contract_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	apiv1 "punchflow/internal/api/v1"
)

func TestContractRoutes(t *testing.T) {
	spec := loadSpec(t)
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths is not an object")
	}
	want := make(map[string]string)
	for path, raw := range paths {
		methods, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for method, operation := range methods {
			method = strings.ToUpper(method)
			if !isHTTPMethod(method) {
				continue
			}
			op, _ := operation.(map[string]any)
			id, _ := op["operationId"].(string)
			key := method + " " + path
			want[key] = id
		}
	}
	if len(want) != 96 {
		t.Fatalf("OpenAPI operation count = %d, want 96", len(want))
	}
	got := apiv1.Operations()
	if len(got) != 96 {
		t.Fatalf("punchflow operation count = %d, want 96", len(got))
	}
	seen := make(map[string]bool, len(got))
	for _, op := range got {
		op := op
		t.Run(op.Method+" "+op.Pattern, func(t *testing.T) {
			key := op.Method + " " + op.Pattern
			id, ok := want[key]
			if !ok {
				t.Fatalf("route %s is absent from OpenAPI", key)
			}
			if id != op.OperationID {
				t.Fatalf("operationId for %s = %q, want %q", key, op.OperationID, id)
			}
			seen[key] = true
		})
	}
	if len(seen) != len(want) {
		t.Errorf("matched %d/%d operations", len(seen), len(want))
	}
}

func isHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func responseSchemaURI(path, method string) string {
	return fmt.Sprintf("file:///kickflow-openapi.json#/paths/%s/%s/responses/200/content/application~1json/schema", escapePointer(path), method)
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
