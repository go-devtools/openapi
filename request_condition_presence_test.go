package openapi

import (
	"bytes"
	"testing"
)

// Diagnose media-dependent body presence when it cannot be merged without losing requirements.
func TestConditionalBodyPresence(t *testing.T) {
	raw := []byte(`{"formatVersion":1,"specVersion":"3.2.0","capabilities":["request-conditions-v1"],"components":{},"profile":{},"templates":[{"key":"body","operation":{},"variants":[
 {"when":{"mediaTypes":["application/json"]},"operation":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"string"}}}},"responses":{"200":{"description":"ok"}}}},
 {"when":{"mediaTypes":["text/plain"]},"operation":{"requestBody":{"content":{"text/plain":{"schema":{"type":"string"}}}},"responses":{"200":{"description":"ok"}}}}
 ]}]}`)
	bundle, err := ParseBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(bundle, []Route{{Method: "POST", Path: "/body", OperationKey: "body", RequestMediaTypes: []string{"application/json", "text/plain"}}}, Config{Title: "Presence", Version: "1"}); err == nil {
		t.Fatal("media-dependent required flag was silently merged")
	}
	original := bundle.JSON()
	snapshot := bundle.Snapshot()
	snapshot.Templates[0].Variants[0].When.MediaTypes[0] = "application/xml"
	if !bytes.Equal(original, bundle.JSON()) {
		t.Fatal("snapshot mutated the decision table")
	}
	doc, err := Build(bundle, []Route{{Method: "POST", Path: "/body", OperationKey: "body", RequestMediaTypes: []string{"application/json"}}}, Config{Title: "Presence", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range doc.Report().Facts {
		if fact.Kind == "declared" && fact.When != nil && len(fact.When.MediaTypes) == 1 && fact.When.MediaTypes[0] == "application/json" {
			found = true
		}
	}
	if !found {
		t.Fatal("media configuration lacks declared provenance")
	}
	if !bytes.Equal(original, bundle.JSON()) {
		t.Fatal("linking mutated the decision table")
	}
}
