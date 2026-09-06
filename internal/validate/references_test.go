package validate

import (
	"encoding/json"
	"strings"
	"testing"
)

// Isolate reference semantics with minimal native documents and no unrelated business fields.
func referenceDocument(schemas string, extra string) []byte {
	return []byte(`{"openapi":"3.2.0","info":{"title":"Reference test","version":"1"},"components":{"schemas":` + schemas + `}` + extra + `}`)
}

// Verify base URIs, resource anchors, and recursive references without expanding cyclic schemas.
func TestLocalReferenceResources(t *testing.T) {
	cases := []struct{ name, schemas, extra string }{
		{"anchor", `{"Node":{"$anchor":"node","properties":{"next":{"$ref":"#node"}}}}`, ""},
		{"dynamic-anchor", `{"Node":{"$dynamicAnchor":"node","properties":{"next":{"$dynamicRef":"#node"}}}}`, ""},
		{"relative-self", `{"Node":{"$ref":"openapi.json#/components/schemas/Value"},"Value":true}`, `,"$self":"/api/openapi.json"`},
		{"resource-pointer", `{"Node":{"$id":"schemas/node","$defs":{"Value":true},"properties":{"value":{"$ref":"#/$defs/Value"}}}}`, `,"$self":"https://example.test/api/openapi.json"`},
		{"sibling-resource", `{"Node":{"$id":"schemas/node","$ref":"value"},"Value":{"$id":"schemas/value","type":"string"}}`, `,"$self":"https://example.test/api/openapi.json"`},
		{"nested-resource", `{"Node":{"$id":"schemas/node","$defs":{"Inner":{"$id":"inner","$anchor":"inner","$ref":"#inner"}},"$ref":"inner#inner"}}`, ""},
		{"empty-schema-ref", `{"Node":{"$id":"https://example.test/node","$ref":""}}`, ""},
		{"encoded-pointer", `{"Node":{"$ref":"#/components/schemas/a~1b~0c%20d"},"a/b~c d":false}`, ""},
		{"schema-property-x", `{"Node":{"properties":{"x-child":{"$anchor":"child","type":"string"}},"$ref":"#child"}}`, ""},
		{"annotation-data", `{"Node":{"examples":[{"$ref":"https://unloaded.test/value"}],"default":{"$id":"bad id"},"const":{"$dynamicRef":"#missing"}}}`, ""},
		{"same-anchor-different-resource", `{"A":{"$id":"a","$anchor":"node","$ref":"#node"},"B":{"$id":"b","$anchor":"node","$ref":"#node"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if issues := Check(referenceDocument(tc.schemas, tc.extra)); len(issues) != 0 {
				t.Fatalf("valid reference was rejected: %+v", issues)
			}
		})
	}
}

// Preserve a separate location for every invalid reference, including repeated reference text.
func TestInvalidReferenceResources(t *testing.T) {
	cases := []struct{ name, schemas, extra, code string }{
		{"cross-id-pointer", `{"A":{"$id":"https://example.test/a","type":"string"},"B":{"$ref":"#/components/schemas/A"}}`, "", "ref.scope"},
		{"missing-anchor", `{"Node":{"$ref":"#missing"}}`, "", "ref.missing"},
		{"missing-dynamic-anchor", `{"Node":{"$dynamicRef":"#missing"}}`, "", "ref.missing"},
		{"non-schema", `{"Node":{"$ref":"#/info"}}`, "", "ref.type"},
		{"empty-openapi-ref", `{"Node":{"$ref":""}}`, "", "ref.type"},
		{"wrong-ref-type", `{"Node":{"$ref":42}}`, "", "ref.uri"},
		{"bad-pointer-escape", `{"Node":{"$ref":"#/components/schemas/a~2b"},"a~2b":true}`, "", "ref.pointer"},
		{"array-leading-zero", `{"Node":{"prefixItems":[true],"$ref":"#/components/schemas/Node/prefixItems/00"}}`, "", "ref.pointer"},
		{"bad-id-fragment", `{"Node":{"$id":"https://example.test/node#nonempty"}}`, "", "schema.id"},
		{"bad-id-type", `{"Node":{"$id":false}}`, "", "schema.id"},
		{"bad-anchor", `{"Node":{"$anchor":"0node"}}`, "", "schema.anchor"},
		{"duplicate-anchor", `{"A":{"$anchor":"node"},"B":{"$dynamicAnchor":"node"}}`, "", "schema.anchor.duplicate"},
		{"duplicate-id", `{"A":{"$id":"same"},"B":{"$id":"same"}}`, "", "resource.duplicate"},
		{"unknown-resource", `{"Node":{"$ref":"https://unloaded.test/schema"}}`, "", "external.denied"},
		{"wrong-base", `{"Node":{"$id":"node","$ref":"#node"},"Other":{"$anchor":"node"}}`, "", "ref.missing"},
		{"x-property-checked", `{"Node":{"properties":{"x-bad":{"$ref":"#missing"}}}}`, "", "ref.missing"},
		{"bad-self", `{}`, `,"$self":"bad uri"`, "self"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := Check(referenceDocument(tc.schemas, tc.extra))
			for _, issue := range issues {
				if issue.Code == "openapi.spec."+tc.code {
					return
				}
			}
			t.Fatalf("%s was not reported: %+v", tc.code, issues)
		})
	}
	issues := Check(referenceDocument(`{"A":{"$ref":"#missing"},"B":{"$ref":"#missing"}}`, ""))
	paths := map[string]bool{}
	for _, issue := range issues {
		if issue.Code == "openapi.spec.ref.missing" {
			paths[issue.Path] = true
		}
	}
	if !paths["#/components/schemas/A/$ref"] || !paths["#/components/schemas/B/$ref"] {
		t.Fatalf("duplicate reference lost its location: %+v", issues)
	}
}

// Require Reference Objects and Links to target the correct kind, not merely an existing JSON Pointer.
func TestReferenceTargetRoles(t *testing.T) {
	for _, fragment := range []string{
		`"paths":{"/x":{"get":{"responses":{"200":{"$ref":"#/components/schemas/A"}}}}},"components":{"schemas":{"A":true}}`,
		`"components":{"schemas":{"A":true},"links":{"L":{"operationRef":"#/info"}}}`,
	} {
		raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},` + fragment + `}`)
		issues := Check(raw)
		found := false
		for _, issue := range issues {
			found = found || strings.HasPrefix(issue.Code, "openapi.spec.ref.")
		}
		if !found {
			t.Fatalf("invalid target was not rejected: %s %+v", raw, issues)
		}
	}
}

// Report duplicate identities deterministically for reproducible CI comparisons.
func TestReferenceDiagnosticsDeterministic(t *testing.T) {
	raw := referenceDocument(`{"B":{"$anchor":"same"},"A":{"$anchor":"same"}}`, "")
	want, _ := json.Marshal(Check(raw))
	for i := 0; i < 30; i++ {
		got, _ := json.Marshal(Check(raw))
		if string(got) != string(want) {
			t.Fatalf("reference diagnostics are unstable: %s != %s", got, want)
		}
	}
}

// Treat callback extensions as business data without triggering reference checks.
func TestCallbackExtensionReferencesAreData(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"a","version":"1"},"components":{"callbacks":{"Notify":{"x-note":{"$ref":"https://data.invalid/note"},"{$request.body#/callback}":{"post":{"responses":{"200":{"description":"OK"}}}}}}}}`)
	if issues := Check(raw); len(issues) != 0 {
		t.Fatalf("extension data was incorrectly treated as a reference: %+v", issues)
	}
}
