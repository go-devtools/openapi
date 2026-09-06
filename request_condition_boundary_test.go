package openapi

import (
	"reflect"
	"testing"

	"github.com/openapi-golang/openapi/contracttest"
	"github.com/openapi-golang/openapi/spec"
)

// Condition intersections must not widen or erase reachable domains or mutate caller input.
func TestRequestConditionIntersection(t *testing.T) {
	a := RequestCondition{Methods: []string{"POST", "GET", "GET"}, ExceptMethods: []string{"GET"}, ExceptMediaTypes: []string{"multipart/form-data"}}
	original := copyJSON(a)
	combined, ok, err := a.Intersect(RequestCondition{ExceptMethods: []string{"DELETE"}, MediaTypes: []string{"application/json"}})
	if err != nil || !ok {
		t.Fatal(combined, ok, err)
	}
	if !reflect.DeepEqual(combined.Methods, []string{"POST"}) || !reflect.DeepEqual(combined.MediaTypes, []string{"application/json"}) {
		t.Fatalf("wrong intersection: %+v", combined)
	}
	combined.Methods[0] = "PATCH"
	if !reflect.DeepEqual(a, original) {
		t.Fatal("condition mutated caller input")
	}
	if _, ok, err := a.Intersect(RequestCondition{Methods: []string{"GET"}}); err != nil || ok {
		t.Fatal("empty intersection became unrestricted")
	}
	for _, condition := range []RequestCondition{{Methods: []string{"GET\n"}}, {MediaTypes: []string{"application/json; charset=utf-8"}}, {MediaTypes: []string{"*/*"}}} {
		if _, _, err := condition.Intersect(RequestCondition{}); err == nil {
			t.Fatal("invalid decision accepted")
		}
	}
}

// Conditional merging must preserve composition siblings without narrowing another valid branch.
func TestConditionalSchemaAlternatives(t *testing.T) {
	constrained := &spec.Schema{SchemaObject: &spec.SchemaObject{AnyOf: []*spec.Schema{spec.Typed("string")}, MinLength: spec.Set(uint64(3))}}
	variants := []OperationVariant{}
	for i, media := range []string{"application/json", "text/plain"} {
		schema := constrained
		if i == 1 {
			schema = spec.Typed("string")
		}
		variants = append(variants, OperationVariant{When: RequestCondition{MediaTypes: []string{media}}, Operation: spec.Operation{Responses: map[string]spec.RefOr[spec.Response]{"200": spec.Inline(spec.Response{Description: "result", Content: map[string]spec.RefOr[spec.MediaType]{"application/json": spec.Inline(spec.MediaType{Schema: schema})}})}}})
	}
	bundle, err := NewBundle(BundleData{FormatVersion: 1, SpecVersion: "3.2.0", Capabilities: []string{RequestConditionsCapability}, Templates: []Template{{Key: "value", Variants: variants}}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := Build(bundle, []Route{{Method: "POST", Path: "/value", OperationKey: "value", RequestMediaTypes: []string{"application/json", "text/plain"}}}, Config{Title: "Union", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := contracttest.Compile(document.JSON(), "/paths/~1value/post/responses/200/content/application~1json/schema", contracttest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.Value("a"); err != nil {
		t.Fatal("unconstrained alternative was lost:", err)
	}
	if validator.Value(1) == nil {
		t.Fatal("type constraints were dropped")
	}
	if constrained.MinLength.Value != 3 {
		t.Fatal("caller schema mutated")
	}
}
