package compiler_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi/compiler"
	"github.com/openapi-golang/openapi/spec"
)

// Bundle retrieval aliases, relative identities, dynamic recursion, and boolean resources into one offline document.
func TestStandaloneEmbedsOfflineResources(t *testing.T) {
	root := &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "strict.json"}}
	resources := map[string][]byte{
		"https://example.test/api/strict.json": []byte(`{"$id":"models/strict","$dynamicAnchor":"node","$ref":"tree","unevaluatedProperties":false,"properties":{"forbidden":{"$ref":"../deny"}}}`),
		"https://example.test/api/models/tree": []byte(`{"$id":"tree","$dynamicAnchor":"node","type":"object","required":["name"],"properties":{"name":{"type":"string"},"children":{"type":"array","items":{"$dynamicRef":"#node"}}}}`),
		"https://example.test/api/deny":        []byte(`false`),
	}
	before, _ := json.Marshal(resources)
	projection := &compiler.Projection{Root: root}
	options := compiler.StandaloneOptions{BaseURI: "https://example.test/api/export.json", Resources: resources}
	raw, err := projection.StandaloneWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	validator := compileStandalone(t, raw)
	for _, sample := range []struct {
		value string
		valid bool
	}{
		{`{"name":"root","children":[{"name":"child"}]}`, true},
		{`{"name":"root","extra":true}`, false},
		{`{"name":"root","children":[{"name":"child","extra":true}]}`, false},
		{`{"name":"root","forbidden":null}`, false},
	} {
		var value any
		if err = json.Unmarshal([]byte(sample.value), &value); err != nil {
			t.Fatal(err)
		}
		if accepted := validator.Validate(value) == nil; accepted != sample.valid {
			t.Errorf("sample %s accepted=%v", sample.value, accepted)
		}
	}
	after, _ := json.Marshal(resources)
	if !bytes.Equal(before, after) || root.Ref != "strict.json" {
		t.Fatal("export mutated caller-owned resources")
	}
	again, err := projection.StandaloneWithOptions(options)
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("offline export is not deterministic")
	}
}

// Count every embedded resource toward the normalized budget, not just the root schema.
func TestStandaloneEmbeddedResourceBudget(t *testing.T) {
	projection := &compiler.Projection{Root: &spec.Schema{SchemaObject: &spec.SchemaObject{Ref: "https://example.test/large"}}}
	resource, _ := json.Marshal(map[string]any{"type": "string", "description": strings.Repeat("description", 1024)})
	options := compiler.StandaloneOptions{Resources: map[string][]byte{"https://example.test/large": resource}, MaxNormalizedBytes: 1024}
	raw, err := projection.StandaloneWithOptions(options)
	if err == nil || len(raw) > 0 || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("embedding budget not enforced: %s %v", raw, err)
	}
	options.MaxNormalizedBytes = 64 << 10
	raw, err = projection.StandaloneWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	validator := compileStandalone(t, raw)
	if validator.Validate("valid") != nil || validator.Validate(1) == nil {
		t.Fatal("resource constraints were lost")
	}
}

// Preserve OpenAPI-specific fields as standalone-schema annotations, not resource-loading instructions.
func TestStandalonePreservesOpenAPIAnnotations(t *testing.T) {
	root := spec.Typed("object")
	root.Discriminator = &spec.Discriminator{PropertyName: "kind", Mapping: map[string]string{"cat": "Cat", "remote": "https://example.test/annotation"}}
	root.XML = &spec.XML{Name: "item", NodeType: "element"}
	root.Examples = spec.Set([]any{map[string]any{"$id": "business-id", "$ref": "#/components/schemas/Cat"}})
	raw, err := (&compiler.Projection{Root: root}).Standalone()
	if err != nil {
		t.Fatal(err)
	}
	var value spec.Schema
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.Discriminator.Mapping["cat"] != "Cat" || value.XML.Name != "item" || value.Examples.Value[0].(map[string]any)["$ref"] != "#/components/schemas/Cat" {
		t.Fatalf("annotation data changed: %s", raw)
	}
	if compileStandalone(t, raw).Validate(map[string]any{"kind": "anything"}) != nil {
		t.Fatal("annotation became a validation constraint")
	}
}
