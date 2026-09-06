package contracttest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Pin official resources and compile full OAS 3.2 structure and dialect validators offline.
func official32(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(offlineLoader{})
	c.AssertFormat()
	checksums := map[string]string{
		"schema.json":      "7d48f01f37eeae4799041b371ad5f533f9f533fd2b0caa1011a8ba27c5b48b70",
		"schema-base.json": "423daa88e2285fa343856c08502fe63fd8aa3674cd5b4ef88746ba6f82647af3",
		"dialect.json":     "4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273",
		"meta.json":        "a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c",
	}
	for name, sum := range checksums {
		raw, err := os.ReadFile(filepath.Join("testdata", "oas32", name))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != sum {
			t.Fatalf("upstream resource checksum changed: %s", name)
		}
		v, err := decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.AddResource(v.(map[string]any)["$id"].(string), v); err != nil {
			t.Fatal(err)
		}
	}
	v, err := c.Compile("https://spec.openapis.org/oas/3.2/schema-base/2025-11-23")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Mutate standard fields in the complete valid example to prove independent validation rejects errors.
func TestOfficialOpenAPI32Matrix(t *testing.T) {
	v := official32(t)
	raw, err := os.ReadFile("../testdata/golden/openapi32-full.json")
	if err != nil {
		t.Fatal(err)
	}
	full, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Validate(full); err != nil {
		t.Fatalf("full standard fixture: %v", err)
	}
	cases := []struct {
		name   string
		path   []string
		field  string
		value  any
		remove bool
	}{
		{"Legacy specification version", nil, "openapi", "3.1.0", false},

		{"Empty security declaration", nil, "security", nil, false},
		{"license identifier conflict", []string{"info", "license"}, "url", "https://example.test/license", false},
		{"whole-query parameter is missing a name", []string{"components"}, "parameters", map[string]any{"Bad": map[string]any{"in": "querystring", "content": map[string]any{"application/json": map[string]any{"schema": true}}}}, false},
		{"Stream Schema type", []string{"components", "mediaTypes", "Events"}, "itemSchema", nil, false},
		{"media example conflict", []string{"components", "mediaTypes", "Events"}, "example", "event", false},
		{"device authorization endpoints are missing", []string{"components", "securitySchemes", "device", "flows", "deviceAuthorization"}, "tokenUrl", nil, true},
		{"Invalid XML node", []string{"components", "schemas", "XmlItem", "xml"}, "nodeType", "unknown", false},
		{"Legacy and current XML representations conflict", []string{"components", "schemas", "XmlItem", "xml"}, "attribute", true, false},
		{"invalid union type", []string{"components", "schemas", "Item"}, "type", []any{"object", "unknown"}, false},
		{"Negative length", []string{"components", "schemas", "Constraints"}, "minLength", json.Number("-1"), false},
		{"empty composition", []string{"components", "schemas", "Constraints"}, "anyOf", []any{}, false},
		{"duplicate required property", []string{"components", "schemas", "Item"}, "required", []any{"id", "id"}, false},
		{"Zero multiple", []string{"components", "schemas", "Constraints"}, "multipleOf", json.Number("0"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			node := copy.(map[string]any)
			for _, key := range tc.path {
				next, ok := node[key].(map[string]any)
				if !ok {
					t.Fatalf("test path does not exist: %v, %s", tc.path, key)
				}
				node = next
			}
			if tc.remove {
				delete(node, tc.field)
			} else {
				node[tc.field] = tc.value
			}
			if err = v.Validate(copy); err == nil {
				t.Fatal("independent validation did not reject an invalid sample")
			}
		})
	}
}
