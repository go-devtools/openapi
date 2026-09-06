package validate

import (
	"reflect"
	"testing"
)

// Reference normalization must preserve OpenAPI annotation data instead of interpreting mapping names as schema keywords.
func TestBundlePreservesDiscriminatorData(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","components":{"schemas":{"A":{"type":"object","discriminator":{"propertyName":"kind","mapping":{"$ref":"#/components/schemas/B","$dynamicRef":"#/components/schemas/B"}}},"B":{"type":"string"}}}}`)
	bundle, issues := BundleSchemas(raw, "/components/schemas/A", Options{BaseURI: "https://example.test/api"})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	resource := bundle.Resources["https://example.test/api"].(map[string]any)
	expected := map[string]any{"propertyName": "kind", "mapping": map[string]any{"$ref": "#/components/schemas/B", "$dynamicRef": "#/components/schemas/B"}}
	found := false
	for _, schema := range resource["$defs"].(map[string]any) {
		object, ok := schema.(map[string]any)
		if !ok {
			continue
		}
		if actual, exists := object["discriminator"]; exists {
			found = true
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("annotation was changed: %#v", actual)
			}
		}
	}
	if !found {
		t.Fatal("annotations were lost")
	}
}
