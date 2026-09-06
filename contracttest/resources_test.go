package contracttest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Compile nested pointers within their parent resource without losing the nearest identifier or anchor.
func TestNestedSchemaResourceScope(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","components":{"schemas":{"Product":{"$id":"https://example.test/product","$defs":{"Name":{"$anchor":"name","type":"string","minLength":3}},"type":"object","properties":{"name":{"$ref":"#name"}}}}}}`)
	v, err := Compile(raw, "/components/schemas/Product/properties/name", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"alice"`)); err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"ab"`)); err == nil {
		t.Fatal("target constraints in the parent resource were not checked")
	}
}

// Components without their own identifiers share the OpenAPI document's anchor scope.
func TestOpenAPISharedAnchors(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","$self":"https://example.test/api/openapi","components":{"schemas":{"A":{"$ref":"#name"},"B":{"$anchor":"name","type":"string","const":"yes"}}}}`)
	v, err := Compile(raw, "/components/schemas/A", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"yes"`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`"no"`)) == nil {
		t.Fatal("shared anchor lost constraints")
	}
}

// Dynamic recursion applies extension constraints to children while static references keep the base resource.
func TestDynamicReferenceInstances(t *testing.T) {
	document := `{"openapi":"3.2.0","components":{"schemas":{
 "Tree":{"$id":"https://example.test/tree","$dynamicAnchor":"node","type":"object","properties":{"data":true,"children":{"type":"array","items":{"$dynamicRef":"#node"}}}},
 "Strict":{"$id":"https://example.test/strict","$dynamicAnchor":"node","$ref":"https://example.test/tree","unevaluatedProperties":false}
 }}}`
	strict, err := Compile([]byte(document), "/components/schemas/Strict", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = strict.JSON([]byte(`{"data":1,"children":[{"data":2}]}`)); err != nil {
		t.Fatal(err)
	}
	if strict.JSON([]byte(`{"children":[{"data":2,"extra":true}]}`)) == nil {
		t.Fatal("dynamic reference did not inherit recursive extension constraints")
	}
	for _, variant := range []string{
		strings.Replace(document, `"$dynamicAnchor":"node"`, `"$anchor":"node"`, 1),
		strings.ReplaceAll(document, `"$dynamicRef":"#node"`, `"$dynamicRef":"#"`),
	} {
		fallback, err := Compile([]byte(variant), "/components/schemas/Strict", Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err = fallback.JSON([]byte(`{"children":[{"extra":true}]}`)); err != nil {
			t.Fatal("nondynamic initial target must preserve static semantics", err)
		}
	}
	base, err := Compile([]byte(document), "/components/schemas/Tree", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = base.JSON([]byte(`{"children":[{"extra":true}]}`)); err != nil {
		t.Fatal("extension polluted the base resource", err)
	}
	static, err := Compile([]byte(strings.ReplaceAll(document, `"$dynamicRef":"#node"`, `"$ref":"#node"`)), "/components/schemas/Strict", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = static.JSON([]byte(`{"children":[{"extra":true}]}`)); err != nil {
		t.Fatal("static reference was incorrectly treated as dynamic", err)
	}
}

// Resource and reference fields inside example data do not participate in schema indexing.
func TestContractIgnoresExampleResourceFields(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","components":{"schemas":{"X":{"type":"string","examples":[{"$id":"https://example.test/x","$ref":"file:///denied"}]}}}}`)
	v, err := Compile(raw, "/components/schemas/X", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"yes"`)); err != nil {
		t.Fatal(err)
	}
}

// Explicit retrieval addresses, relative self/id values, and preloaded resources determine reference targets.
func TestContractExplicitOfflineResources(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","$self":"/api/root","components":{"schemas":{"Container":{"$id":"schemas/container","properties":{"item":{"$ref":"item#item"}}}}}}`)
	resource := []byte(`{"$id":"./canonical-item","$anchor":"item","type":"integer","minimum":3}`)
	options := Options{BaseURI: "https://example.test/staging/root", Resources: map[string][]byte{"https://example.test/api/schemas/item": resource}}
	original := bytes.Clone(resource)
	v, err := Compile(raw, "/components/schemas/Container", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`{"item":3}`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`{"item":2}`)) == nil {
		t.Fatal("cross-resource boundary was lost")
	}
	if !bytes.Equal(resource, original) {
		t.Fatal("preloaded bytes were modified")
	}
	options.Resources["https://example.test/api/schemas/item"] = []byte(`false`)
	if err = v.JSON([]byte(`{"item":3}`)); err != nil {
		t.Fatal("compiled result retained mutable caller resources", err)
	}
}

// External OpenAPI components, boolean schemas, and explicitly supplied meta-schemas stay offline.
func TestContractResourceKindsAndDialect(t *testing.T) {
	options := Options{Resources: map[string][]byte{
		"https://example.test/shared": []byte(`{"openapi":"3.2.0","components":{"schemas":{"Name":{"type":"string","minLength":3}}}}`),
		"https://example.test/no":     []byte(`false`),
	}}
	v, err := Compile([]byte(`{"anyOf":[{"$ref":"https://example.test/shared#/components/schemas/Name"},{"$ref":"https://example.test/no"}]}`), "", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`"alice"`)); err != nil {
		t.Fatal(err)
	}
	if v.JSON([]byte(`"ab"`)) == nil {
		t.Fatal("external OpenAPI component constraints were lost")
	}
	for _, dialect := range []string{"https://spec.openapis.org/oas/3.1/dialect/base", "https://spec.openapis.org/oas/3.2/dialect/2025-09-17"} {
		if _, err = Compile([]byte(`{"$schema":"`+dialect+`","type":"string"}`), "", Options{}); err != nil {
			t.Fatal(err)
		}
	}
	options.Resources["https://example.test/dialect"] = []byte(`{"$id":"https://example.test/dialect","$schema":"https://json-schema.org/draft/2020-12/schema","$vocabulary":{"https://json-schema.org/draft/2020-12/vocab/validation":true},"type":["object","boolean"]}`)
	if _, err = Compile([]byte(`{"$schema":"https://example.test/dialect","type":"integer"}`), "", options); err != nil {
		t.Fatal(err)
	}
	options.Resources["https://example.test/dialect"] = []byte(`{"$id":"https://example.test/dialect","$schema":"https://json-schema.org/draft/2020-12/schema","$vocabulary":{"https://example.test/unknown-vocabulary":true},"type":["object","boolean"]}`)
	if _, err = Compile([]byte(`{"$schema":"https://example.test/dialect"}`), "", options); err == nil {
		t.Fatal("unknown required vocabulary was not rejected")
	}
}

// Missing resources never trigger HTTP requests; preloading the same URI also avoids server access.
func TestContractNeverFetches(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); _, _ = w.Write([]byte(`false`)) }))
	defer server.Close()
	raw, _ := json.Marshal(map[string]any{"$ref": server.URL + "/schema"})
	if _, err := Compile(raw, "", Options{}); err == nil {
		t.Fatal("missing resource must fail")
	}
	if _, err := Compile(raw, "", Options{Resources: map[string][]byte{server.URL + "/schema": []byte(`true`)}}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatal("validator made a network request")
	}
}

// Schema locations, identity conflicts, and aggregate resource budgets are checked before independent compilation.
func TestContractResourceErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		pointer string
		options Options
	}{
		{"Invalid location", `{"openapi":"3.2.0","info":{"title":"a"}}`, "/info", Options{}},
		{"Example data", `{"type":"string","examples":[{}]}`, "/examples/0", Options{}},
		{"invalid escape", `{"type":"object"}`, "/~invalid", Options{}},
		{"duplicate key", `{"type":"string","type":"integer"}`, "", Options{}},
		{"preloaded duplicate key", `true`, "", Options{Resources: map[string][]byte{"https://example.test/a": []byte(`{"type":"string","type":"integer"}`)}}},
		{"Cumulative bytes", `true`, "", Options{MaxBytes: 8, Resources: map[string][]byte{"https://example.test/a": []byte(`false`)}}},
		{"Embedded resource", `{"$defs":{"a":{"$id":"https://example.test/a"}}}`, "", Options{MaxResources: 1}},
		{"Reference budget", `{"allOf":[{"$ref":"#"},{"$ref":"#"}]}`, "", Options{MaxReferences: 1}},
		{"duplicate identity", `{"$defs":{"a":{"$id":"https://example.test/a"},"b":{"$id":"https://example.test/a"}}}`, "", Options{}},
		{"Crossed identity", `{"$ref":"#/$defs/a","$defs":{"a":{"$id":"https://example.test/a","type":"string"}}}`, "", Options{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile([]byte(tc.raw), tc.pointer, tc.options); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
}

// Budgets before the independent numeric engine prevent exponent expansion and bound decoded sample depth.
func TestContractEngineBudgets(t *testing.T) {
	if _, err := Compile([]byte(`{"minimum":1e5000}`), "", Options{}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected a numeric budget diagnostic: %v", err)
	}
	v, err := Compile([]byte(`true`), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = v.JSON([]byte(`1e999999999999999999`)); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected a sample numeric budget diagnostic: %v", err)
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if err = v.Value(cyclic); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected a sample depth budget diagnostic: %v", err)
	}
	small, err := Compile([]byte(`true`), "", Options{MaxBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	if err = small.Value("abcdef"); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected a content byte budget diagnostic: %v", err)
	}
}
