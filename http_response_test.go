package openapi

import (
	"encoding/json"
	"github.com/openapi-golang/openapi/spec"
	"testing"
)

// HEAD retains response metadata without modifying the shared GET contract or original Bundle.
func TestHEADResponseProjection(t *testing.T) {
	response := spec.Response{Summary: "Original summary", Description: "ok", Headers: map[string]spec.RefOr[spec.Header]{"X-Version": spec.Inline(spec.Header{Schema: spec.Typed("string")})}, Content: map[string]spec.RefOr[spec.MediaType]{"application/json": spec.Inline(spec.MediaType{Schema: spec.Typed("string")})}}
	for _, shared := range []bool{false, true} {
		entry := spec.Inline(response)
		components := spec.Components{}
		if shared {
			components.Responses = map[string]spec.RefOr[spec.Response]{"Shared": entry}
			entry = spec.Ref[spec.Response]("#/components/responses/Shared")
			entry.Reference.Summary = "HEAD summary"
		}
		bundle, err := NewBundle(BundleData{FormatVersion: 1, SpecVersion: "3.2.0", Components: components, Templates: []Template{{Key: "resource", Operation: spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{"200": entry}}}}})
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(bundle.Snapshot())
		doc, err := Build(bundle, []Route{{Method: "HEAD", Path: "/resource", OperationKey: "resource"}, {Method: "GET", Path: "/resource", OperationKey: "resource"}}, Config{Title: "HTTP", Version: "1"})
		if err != nil {
			t.Fatal(err)
		}
		var specDoc spec.OpenAPI
		if err = json.Unmarshal(doc.JSON(), &specDoc); err != nil {
			t.Fatal(err)
		}
		head := specDoc.Paths["/resource"].Head.Responses["200"].Value
		if head == nil || len(head.Content) != 0 || head.Headers["X-Version"].Value == nil {
			t.Fatalf("HEAD did not preserve metadata without body: %#v", head)
		}
		if shared && head.Summary != "HEAD summary" {
			t.Fatal("response reference summary override was lost")
		}
		get := specDoc.Paths["/resource"].Get.Responses["200"]
		if shared {
			get = specDoc.Components.Responses["Shared"]
		}
		if get.Value == nil || len(get.Value.Content) != 1 {
			t.Fatal("HEAD changed GET content")
		}
		after, _ := json.Marshal(bundle.Snapshot())
		if string(before) != string(after) {
			t.Fatal("HEAD changed Bundle")
		}
	}
}

// Response components use names, while operation responses require status codes and x- names remain components.
func TestNamedResponseComponents(t *testing.T) {
	for _, name := range []string{"Shared", "x-reply"} {
		response := map[string]any{"description": "ok"}
		document := map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "Components", "version": "1"}, "paths": map[string]any{"/value": map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]any{"$ref": "#/components/responses/" + name}}}}}, "components": map[string]any{"responses": map[string]any{name: response}}}
		raw, _ := json.Marshal(document)
		if report := Check(raw); report.HasErrors() {
			t.Fatal(report)
		}
		response["description"] = 7
		raw, _ = json.Marshal(document)
		if !Check(raw).HasErrors() {
			t.Fatal("named response skipped field type validation")
		}
	}
	empty := []byte(`{"openapi":"3.2.0","info":{"title":"Optional","version":"1"},"paths":{"/empty":{"get":{"responses":{"200":{}}}}}}`)
	if report := Check(empty); report.HasErrors() {
		t.Fatal("OpenAPI 3.2 response description is optional", report)
	}
	invalid := []byte(`{"openapi":"3.2.0","info":{"title":"Status","version":"1"},"paths":{"/value":{"get":{"responses":{"Shared":{"description":"wrong status"}}}}}}`)
	if !Check(invalid).HasErrors() {
		t.Fatal("operation accepted a component name as status")
	}
}
