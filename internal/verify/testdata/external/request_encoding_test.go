package consumer

import (
	. "github.com/go-devtools/openapi"
	"github.com/go-devtools/openapi/spec"
	"testing"
)

// Merge encodings for fields read under different conditions while rejecting conflicts on the same field.
func TestConditionalRequestFieldEncoding(t *testing.T) {
	makeBundle := func(conflict bool) Bundle {
		t.Helper()
		field := func(name string, encoding spec.Encoding) spec.Operation {
			schema := spec.Typed("object")
			schema.Properties = map[string]*spec.Schema{name: spec.Typed("string")}
			body := spec.Inline(spec.RequestBody{Content: map[string]spec.RefOr[spec.MediaType]{"multipart/form-data": spec.Inline(spec.MediaType{Schema: schema, Encoding: map[string]spec.Encoding{name: encoding}})}})
			return spec.Operation{RequestBody: &body, Responses: map[string]spec.RefOr[spec.Response]{"200": spec.Inline(spec.Response{Description: "ok"})}}
		}
		second := "caption"
		if conflict {
			second = "asset"
		}
		data := BundleData{FormatVersion: 1, SpecVersion: "3.2.0", Capabilities: []string{RequestConditionsCapability}, Templates: []Template{{Key: "upload", Operation: spec.Operation{}, Variants: []OperationVariant{{When: RequestCondition{}, Operation: field("asset", spec.Encoding{ContentType: "application/octet-stream"})}, {When: RequestCondition{Methods: []string{"POST"}}, Operation: field(second, spec.Encoding{ContentType: "text/plain"})}}}}}
		bundle, err := NewBundle(data)
		if err != nil {
			t.Fatal(err)
		}
		return bundle
	}
	route := []Route{{Method: "POST", Path: "/upload", OperationKey: "upload"}}
	if _, err := Build(makeBundle(false), route, Config{Title: "Fields", Version: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(makeBundle(true), route, Config{Title: "Fields", Version: "1"}); err == nil {
		t.Fatal("conflicting encoding was accepted")
	}
}
