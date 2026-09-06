package validate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Long prefixes must not amplify indexes through many short children; a larger budget accepts the same document.
func TestReferenceIndexTextBudget(t *testing.T) {
	properties := map[string]any{}
	for i := 0; i < 100; i++ {
		properties[fmt.Sprint(i)] = map[string]any{"type": "string"}
	}
	raw, err := json.Marshal(map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "a", "version": "1"}, "components": map[string]any{"schemas": map[string]any{strings.Repeat("p", 1024): map[string]any{"type": "object", "properties": properties}}}})
	if err != nil {
		t.Fatal(err)
	}
	if issues := CheckWithOptions(raw, Options{MaxIndexBytes: 64 << 10}); len(issues) == 0 {
		t.Fatal("index text was not bounded by the budget")
	}
	if issues := CheckWithOptions(raw, Options{MaxIndexBytes: 256 << 10}); len(issues) != 0 {
		t.Fatal(issues)
	}
}

// Normalized absolute references must not duplicate long resource URIs without a bound.
func TestNormalizedReferenceBudget(t *testing.T) {
	refs := make([]any, 100)
	for i := range refs {
		refs[i] = map[string]any{"$ref": "#/$defs/value"}
	}
	raw, err := json.Marshal(map[string]any{"$id": "https://example.test/" + strings.Repeat("p", 3000), "$defs": map[string]any{"value": map[string]any{"type": "string"}}, "allOf": refs})
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := BundleSchemas(raw, "", Options{MaxNormalizedBytes: 64 << 10}); len(issues) == 0 {
		t.Fatal("normalized references were not bounded by the budget")
	}
	if _, issues := BundleSchemas(raw, "", Options{MaxNormalizedBytes: 512 << 10}); len(issues) != 0 {
		t.Fatal(issues)
	}
}
