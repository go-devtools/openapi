package contracttest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openapi-golang/openapi"
)

// Verify resolved parameter identities and operation overrides through the public checker.
func TestNativeHTTPParameterContext(t *testing.T) {
	query := `{"name":"filter","in":"query","schema":true}`
	whole := `{"name":"all","in":"querystring","content":{"application/json":{}}}`
	ref := `{"$ref":"#/components/parameters/Query"}`
	for _, tc := range []struct{ name, item, operation, code string }{
		{"path duplicates", query + "," + query, "", "parameter.duplicate"},
		{"operation referenced duplicate", "", query + "," + ref, "parameter.duplicate"},
		{"path referenced duplicate", ref + "," + query, "", "parameter.duplicate"},
		{"same parameter override", ref, query, ""},
		{"path query operation querystring", ref, whole, "parameter.querystring"},
		{"path querystring operation query", whole, ref, "parameter.querystring"},
		{"path query conflict", query + "," + whole, "", "parameter.querystring"},
		{"two querystrings", "", whole + `,{"name":"other","in":"querystring","content":{"text/plain":{}}}`, "parameter.querystring"},
		{"inherited two querystrings", whole, `{"name":"other","in":"querystring","content":{"text/plain":{}}}`, "parameter.querystring"},
		{"querystring override", whole, whole, ""},
		{"different locations", query, `{"name":"filter","in":"header","schema":true}`, ""},
		{"case sensitive query names", query, `{"name":"Filter","in":"query","schema":true}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{"openapi":"3.2.0","info":{"title":"Context","version":"1"},"paths":{"/items":{"parameters":[%s],"get":{"parameters":[%s]}}},"components":{"parameters":{"Query":%s}}}`, tc.item, tc.operation, query))
			// Structural schema validation alone cannot establish uniqueness or inheritance semantics.
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			wantOfficial := tc.name != "path query conflict" && tc.name != "two querystrings"
			if err = official32(t).Validate(value); (err == nil) != wantOfficial {
				t.Fatalf("official schema disagreement: %v", err)
			}
			assertHTTPObjectReport(t, openapi.Check(raw), tc.code)
		})
	}
}

// Preserve parameter context across external Path Items, aliases and custom methods.
func TestNativeHTTPReferencedPathContext(t *testing.T) {
	external := []byte(`{"openapi":"3.2.0","$self":"https://contracts.test/shared.json","info":{"title":"Shared","version":"1"},"components":{"parameters":{"Query":{"name":"filter","in":"query","schema":true},"Alias":{"$ref":"#/components/parameters/Query"}},"pathItems":{"Base":{"parameters":[{"$ref":"#/components/parameters/Alias"}]},"Alias":{"$ref":"#/components/pathItems/Base"}}}}`)
	for _, method := range []string{"get", "query", "additionalOperations"} {
		t.Run(method, func(t *testing.T) {
			operation := `{"parameters":[{"name":"all","in":"querystring","content":{"application/json":{}}}]}`
			if method == "additionalOperations" {
				operation = `{"PROPFIND":` + operation + `}`
			}
			raw := []byte(`{"openapi":"3.2.0","info":{"title":"Main","version":"1"},"paths":{"/items":{"$ref":"shared.json#/components/pathItems/Alias","` + method + `":` + operation + `}}}`)
			report := openapi.CheckWithOptions(raw, openapi.CheckOptions{BaseURI: "https://contracts.test/main.json", Resources: map[string][]byte{"https://contracts.test/shared.json": external}})
			assertHTTPObjectReport(t, report, "parameter.querystring")
		})
	}
}

// Reject reference-only cycles that cannot resolve to a concrete parameter.
func TestNativeHTTPParameterReferenceCycle(t *testing.T) {
	raw := []byte(`{"openapi":"3.2.0","info":{"title":"Cycle","version":"1"},"paths":{"/items":{"get":{"parameters":[{"$ref":"#/components/parameters/A"}]}}},"components":{"parameters":{"A":{"$ref":"#/components/parameters/B"},"B":{"$ref":"#/components/parameters/A"}}}}`)
	assertHTTPObjectReport(t, openapi.Check(raw), "parameter.reference.cycle")
}

// Resolve Link operation IDs across supplied documents, preserving empty string identities.
func TestNativeHTTPOperationIdentity(t *testing.T) {
	for _, tc := range []struct{ name, id, target, code string }{
		{"matching", "next", "next", ""}, {"unknown", "next", "missing", "link.operationId"},
		{"case sensitive", "next", "Next", "link.operationId"}, {"empty identity", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, _ := json.Marshal(tc.id)
			target, _ := json.Marshal(tc.target)
			raw := []byte(`{"openapi":"3.2.0","info":{"title":"Main","version":"1"},"components":{"links":{"Next":{"operationId":` + string(target) + `}}}}`)
			external := []byte(`{"openapi":"3.2.0","info":{"title":"External","version":"1"},"paths":{"/next":{"get":{"operationId":` + string(id) + `}}}}`)
			assertHTTPObjectReport(t, openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://contracts.test/external.json": external}}), tc.code)
		})
	}
	t.Run("duplicate across documents", func(t *testing.T) {
		raw := []byte(`{"openapi":"3.2.0","info":{"title":"Main","version":"1"},"paths":{"/one":{"get":{"operationId":"next"}}},"components":{"links":{"Next":{"operationId":"next"}}}}`)
		external := []byte(`{"openapi":"3.2.0","info":{"title":"External","version":"1"},"paths":{"/two":{"get":{"operationId":"next"}}}}`)
		report := openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://contracts.test/external.json": external}})
		assertHTTPObjectReport(t, report, "link.operationId")
	})
}

// Accept bounded alias chains and report budget exhaustion instead of unbounded traversal.
func TestNativeHTTPContextBudget(t *testing.T) {
	items := map[string]any{}
	for i := 0; i < 64; i++ {
		items[fmt.Sprintf("P%02d", i)] = map[string]any{"$ref": fmt.Sprintf("#/components/pathItems/P%02d", i+1)}
	}
	items["P64"] = map[string]any{"parameters": []any{map[string]any{"name": "filter", "in": "query", "schema": true}}, "get": map[string]any{}}
	document := map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "Budget", "version": "1"}, "paths": map[string]any{"/items": map[string]any{"$ref": "#/components/pathItems/P00"}}, "components": map[string]any{"pathItems": items}}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	assertHTTPObjectReport(t, openapi.Check(raw), "")
	report := openapi.CheckWithOptions(raw, openapi.CheckOptions{MaxIndexBytes: 64 << 10})
	for _, d := range report.Diagnostics {
		if d.Code == "openapi.spec.budget" {
			return
		}
	}
	t.Fatalf("reference traversal did not respect the shared budget: %+v", report)
}
