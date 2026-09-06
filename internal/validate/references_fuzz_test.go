package validate

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Fuzz the actual reference graph with bounded inputs and deterministic-diagnostic checks.
func FuzzReferenceGraph(f *testing.F) {
	f.Add("#node", "https://example.test/openapi.json", "node")
	f.Add("#/$defs/a~1b", "https://example.test/openapi.json", "value")
	f.Add("file:///etc/passwd", "https://example.test/openapi.json", "node")
	f.Add("https://example.test/schema#missing", "https://example.test/openapi.json", "_valid")
	f.Fuzz(func(t *testing.T, ref, base, anchor string) {
		if len(ref) > 4096 || len(base) > 4096 || len(anchor) > 1024 {
			t.Skip()
		}
		value := map[string]any{
			"openapi": "3.2.0", "info": map[string]any{"title": "fuzz", "version": "1"},
			"components": map[string]any{"schemas": map[string]any{
				"A": map[string]any{"$anchor": anchor, "$ref": ref},
				"B": map[string]any{"$id": "https://example.test/schema", "$dynamicAnchor": "node", "$dynamicRef": ref},
			}},
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		options := Options{BaseURI: base, MaxBytes: 64 << 10, MaxResources: 8, MaxReferences: 8}
		first, second := CheckWithOptions(raw, options), CheckWithOptions(raw, options)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("reference diagnostics for identical input are unstable: %+v != %+v", first, second)
		}
	})
}
