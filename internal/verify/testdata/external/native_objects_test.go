package consumer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/spec"
)

// Consume native object construction and explicit booleans from a separately versioned module.
func TestNativeObjectPublicConsumer(t *testing.T) {
	doc := spec.OpenAPI{OpenAPI: "3.2.0", Info: spec.Info{Title: "Native consumer", Version: "1"}, Paths: map[string]*spec.PathItem{}, Components: &spec.Components{
		Schemas:  map[string]*spec.Schema{"Items": {SchemaObject: &spec.SchemaObject{Type: spec.Types{"array"}, Items: spec.Typed("string"), UniqueItems: spec.Set(false), ReadOnly: spec.Set(false), XML: &spec.XML{Wrapped: spec.Set(false)}}}},
		Examples: map[string]spec.RefOr[spec.Example]{"Empty": spec.Inline(spec.Example{DataValue: spec.Set[any](nil), SerializedValue: spec.Set("")})},
	}}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if report := openapi.Check(raw); report.HasErrors() {
		t.Fatalf("typed consumer rejected: %+v", report)
	}
	for _, field := range []string{`"uniqueItems":false`, `"readOnly":false`, `"wrapped":false`, `"dataValue":null`, `"serializedValue":""`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("lost explicit value %s: %s", field, raw)
		}
	}
	var decoded spec.OpenAPI
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(decoded)
	if err != nil || string(again) != string(raw) {
		t.Fatalf("typed native round trip changed: %s (%v)", again, err)
	}
	doc.Components.Schemas["Items"].XML.NodeType = "element"
	invalid, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if report := openapi.Check(invalid); !report.HasErrors() {
		t.Fatal("explicit false did not trigger XML legacy conflict")
	}
}
