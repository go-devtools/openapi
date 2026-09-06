package compiler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openapi-golang/openapi/spec"
)

// Preserve large integers in roots and examples; never rewrite business fields named $ref inside examples.
func TestStandalonePreservesValuesAndDataReferences(t *testing.T) {
	root := spec.Typed("object")
	root.Properties = map[string]*spec.Schema{"ID": {SchemaObject: &spec.SchemaObject{Ref: "#/components/schemas/ID"}}}
	root.Examples = spec.Set([]any{map[string]any{"ID": json.Number("9007199254740993"), "$ref": "#/components/schemas/ID"}})
	root.Defs = map[string]*spec.Schema{"local": spec.Boolean(false)}
	id := spec.Typed("integer")
	id.Minimum = spec.Set(json.Number("9007199254740993"))
	projection := &Projection{Root: root, Components: map[string]*spec.Schema{"ID": id}}
	raw, err := projection.Standalone()
	if err != nil {
		t.Fatal(err)
	}
	var output spec.Schema
	if err = json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output.Properties["ID"].Ref != "#/$defs/ID" || output.Defs["ID"].Minimum.Value != json.Number("9007199254740993") {
		t.Fatal("Schema reference or component numeric value is incorrect")
	}
	if output.Defs["local"] == nil || output.Defs["local"].Bool == nil || *output.Defs["local"].Bool {
		t.Fatal("root-owned $defs were overwritten")
	}
	example := output.Examples.Value[0].(map[string]any)
	if example["ID"] != json.Number("9007199254740993") || example["$ref"] != "#/components/schemas/ID" {
		t.Fatalf("example value was changed: %v", example)
	}
	if root.Properties["ID"].Ref != "#/components/schemas/ID" {
		t.Fatal("export changed the original projection")
	}
}

// Reject conflicting definitions and preserve root bounds without floating-point conversion.
func TestStandaloneRootNumberAndDefinitionConflict(t *testing.T) {
	root := spec.Typed("integer")
	root.Maximum = spec.Set(json.Number("9007199254740993"))
	projection := &Projection{Root: root}
	raw, err := projection.Standalone()
	if err != nil || !strings.Contains(string(raw), `"maximum":9007199254740993`) {
		t.Fatalf("root numeric bound lost precision: %s %v", raw, err)
	}
	root.Defs = map[string]*spec.Schema{"same": spec.Typed("string")}
	projection.Components = map[string]*spec.Schema{"same": spec.Typed("integer")}
	if _, err = projection.Standalone(); err == nil {
		t.Fatal("conflicting $defs were silently overwritten")
	}
}
