package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func loadSpec(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "design-docs", "references", "kickflow-openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse OpenAPI YAML: %v", err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("normalize OpenAPI YAML: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(encoded, &spec); err != nil {
		t.Fatalf("decode normalized OpenAPI JSON: %v", err)
	}
	return spec
}

func schemaCompiler(t *testing.T, spec map[string]any) *jsonschema.Compiler {
	t.Helper()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("encode spec: %v", err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode OpenAPI resource: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("file:///kickflow-openapi.json", document); err != nil {
		t.Fatalf("add OpenAPI resource: %v", err)
	}
	return c
}
