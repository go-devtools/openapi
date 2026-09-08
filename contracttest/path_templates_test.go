package contracttest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-devtools/openapi"
)

// Build a native document without relying on the product's serialization or route discovery.
func pathContract(t *testing.T, paths, components map[string]any) []byte {
	t.Helper()
	value := map[string]any{"openapi": "3.2.0", "info": map[string]any{"title": "Paths", "version": "1"}, "paths": paths}
	if components != nil {
		value["components"] = components
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Express path bindings as ordinary public input, including unusual but legal names.
func pathParameter(name string) any {
	return map[string]any{"name": name, "in": "path", "required": true, "schema": true}
}

// Compare the normative path grammar with the independently pinned structural schema.
func TestNativePathTemplateGrammar(t *testing.T) {
	for _, tc := range []struct {
		name, path, parameter, code string
		official                    bool
	}{
		{"root", "/", "", "", true},
		{"trailing slash", "/items/", "", "", true},
		{"literal pchar", "/aZ09-._~!$&'()*+,;=:@", "", "", true},
		{"escaped literals", "/%E4%B8%AD/%2f/%7Bname%7D", "", "", true},
		{"embedded expression", "/file-{id}.json", "id", "", true},
		{"Unicode name", "/items/{编号}", "编号", "", true},
		{"query in name", "/items/{a?b}", "a?b", "", true},
		{"fragment in name", "/items/{a#b}", "a#b", "", true},
		{"slash in name", "/items/{a/b}", "a/b", "", true},
		{"space in name", "/items/{a b}", "a b", "", true},
		{"control in name", "/items/{a\x00b}", "a\x00b", "", true},
		{"percent in name", "/items/{%GG}", "%GG", "", true},
		{"pointer escapes in name", "/items/{~1/id}", "~1/id", "", true},
		{"missing leading slash", "items", "", "path", false},
		{"empty", "", "", "path", false},
		{"empty segment", "/items//other", "", "path", true},
		{"double leading slash", "//items", "", "path", true},
		{"unescaped space", "/items/a b", "", "path", true},
		{"unescaped Unicode", "/items/中文", "", "path", true},
		{"unescaped query", "/items?a=b", "", "path", true},
		{"unescaped fragment", "/items#part", "", "path", true},
		{"backslash", "/items\\other", "", "path", true},
		{"bad percent", "/items/%GG", "", "path", true},
		{"short percent", "/items/%A", "", "path", true},
		{"percent before expression", "/items/%{id}", "id", "path", true},
		{"empty expression", "/items/{}", "", "path", true},
		{"unclosed expression", "/items/{id", "", "path", true},
		{"unmatched closing brace", "/items/id}", "", "path", true},
		{"nested expression", "/items/{{id}}", "id", "path", true},
		{"repeated expression", "/items/{id}/{id}", "id", "path", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation := map[string]any{}
			if tc.parameter != "" {
				operation["parameters"] = []any{pathParameter(tc.parameter)}
			}
			raw := pathContract(t, map[string]any{tc.path: map[string]any{"get": operation}}, nil)
			value, err := decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err = official32(t).Validate(value); (err == nil) != tc.official {
				t.Fatalf("official structural schema: %v", err)
			}
			assertHTTPObjectReport(t, openapi.Check(raw), tc.code)
		})
	}
}

// Require every operation to retain all bindings after inheritance and identity-based overrides.
func TestNativePathTemplateBindings(t *testing.T) {
	for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query", "PROPFIND", "get-custom"} {
		for _, mode := range []string{"own", "inherited", "override", "missing", "unused", "wrong location", "case mismatch"} {
			t.Run(method+"/"+mode, func(t *testing.T) {
				item, operation := map[string]any{}, map[string]any{}
				code := ""
				switch mode {
				case "own":
					operation["parameters"] = []any{pathParameter("id")}
				case "inherited":
					item["parameters"] = []any{pathParameter("id")}
				case "override":
					item["parameters"] = []any{pathParameter("id")}
					operation["parameters"] = []any{pathParameter("id")}
				case "missing":
					code = "parameter.path.missing"
				case "unused":
					operation["parameters"] = []any{pathParameter("id"), pathParameter("other")}
					code = "parameter.path.unused"
				case "wrong location":
					operation["parameters"] = []any{map[string]any{"name": "id", "in": "query", "schema": true}}
					code = "parameter.path.missing"
				case "case mismatch":
					operation["parameters"] = []any{pathParameter("ID")}
					code = "parameter.path.missing"
				}
				if method == "PROPFIND" || method == "get-custom" {
					item["additionalOperations"] = map[string]any{method: operation}
				} else {
					item[method] = operation
				}
				raw := pathContract(t, map[string]any{"/items/{id}": item}, nil)
				value, err := decode(raw)
				if err != nil {
					t.Fatal(err)
				}
				// The official schema accepts these objects; semantic binding rules supply the negative outcomes.
				if err := official32(t).Validate(value); err != nil {
					t.Fatal(err)
				}
				assertHTTPObjectReport(t, openapi.Check(raw), code)
			})
		}
	}
	for _, tc := range []struct {
		name, path string
		item       map[string]any
		code       string
	}{
		{"ACL empty", "/items/{id}", map[string]any{}, ""},
		{"metadata without binding", "/items/{id}", map[string]any{"description": "Restricted"}, "parameter.path.missing"},
		{"parameters without operation", "/items/{id}", map[string]any{"parameters": []any{pathParameter("id")}}, ""},
		{"unused path item binding", "/items", map[string]any{"parameters": []any{pathParameter("id")}}, "parameter.path.unused"},
		{"missing in second operation", "/items/{id}", map[string]any{"get": map[string]any{"parameters": []any{pathParameter("id")}}, "put": map[string]any{}}, "parameter.path.missing"},
		{"multiple embedded names", "/files/{id}-{ext}", map[string]any{"get": map[string]any{"parameters": []any{pathParameter("id"), pathParameter("ext")}}}, ""},
		{"distinct case names", "/files/{id}/{ID}", map[string]any{"get": map[string]any{"parameters": []any{pathParameter("id"), pathParameter("ID")}}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertHTTPObjectReport(t, openapi.Check(pathContract(t, map[string]any{tc.path: tc.item}, nil)), tc.code)
		})
	}
}

// Resolve reusable and offline Path Items at each real endpoint instead of treating component keys as paths.
func TestNativePathTemplateReferences(t *testing.T) {
	external := []byte(`{"openapi":"3.2.0","info":{"title":"Shared","version":"1"},"components":{"parameters":{"ID":{"name":"id","in":"path","required":true,"schema":true},"Alias":{"$ref":"#/components/parameters/ID"}},"pathItems":{"Base":{"parameters":[{"$ref":"#/components/parameters/Alias"}],"query":{},"additionalOperations":{"PROPFIND":{}}},"Alias":{"$ref":"#/components/pathItems/Base"},"Empty":{}}}}`)
	for _, tc := range []struct{ name, path, target, code string }{
		{"resolved aliases", "/items/{id}", "Alias", ""},
		{"binding at literal endpoint", "/items", "Alias", "parameter.path.unused"},
		{"binding at different template", "/items/{name}", "Alias", "parameter.path.missing"},
		{"resolved empty ACL", "/items/{id}", "Empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := map[string]any{tc.path: map[string]any{"$ref": "shared.json#/components/pathItems/" + tc.target}}
			raw := pathContract(t, paths, nil)
			report := openapi.CheckWithOptions(raw, openapi.CheckOptions{BaseURI: "https://paths.test/main.json", Resources: map[string][]byte{"https://paths.test/shared.json": external}})
			assertHTTPObjectReport(t, report, tc.code)
			if tc.code != "" {
				found := false
				location := "#/paths/" + strings.ReplaceAll(strings.ReplaceAll(tc.path, "~", "~0"), "/", "~1")
				for _, d := range report.Diagnostics {
					if d.Code == "openapi.spec."+tc.code && strings.HasPrefix(d.Message, location+":") {
						found = true
					}
				}
				if !found {
					t.Fatalf("diagnostic lost endpoint binding location: %+v", report)
				}
			}
		})
	}
	t.Run("unbound components webhooks and callbacks", func(t *testing.T) {
		raw := []byte(`{"openapi":"3.2.0","info":{"title":"Unbound","version":"1"},"webhooks":{"changed":{"post":{"parameters":[{"name":"id","in":"path","required":true,"schema":true}]}}},"components":{"pathItems":{"Unused":{"get":{"parameters":[{"name":"id","in":"path","required":true,"schema":true}]}}},"callbacks":{"Notify":{"{$request.body#/callbackUrl}":{"post":{"parameters":[{"name":"id","in":"path","required":true,"schema":true}]}}}}}}`)
		assertHTTPObjectReport(t, openapi.Check(raw), "")
	})
}

// Reject identical template hierarchies within one Paths Object, retaining distinct literal routes and document scopes.
func TestNativePathTemplateHierarchy(t *testing.T) {
	for _, tc := range []struct{ first, second, code string }{
		{"/items/{id}", "/items/{name}", "paths.identical"},
		{"/items/{id}.json", "/items/{name}.json", "paths.identical"},
		{"/items/{a}-{b}", "/items/{c}-{d}", "paths.identical"},
		{"/items/{id}", "/items/mine", ""},
		{"/{entity}/me", "/books/{id}", ""},
		{"/items/{id}", "/items/{id}/", ""},
		{"/items/%7Bid%7D", "/items/{id}", ""},
	} {
		t.Run(tc.first+" vs "+tc.second, func(t *testing.T) {
			raw := pathContract(t, map[string]any{tc.first: map[string]any{}, tc.second: map[string]any{}, "x-debug": map[string]any{"bad/path": false}}, nil)
			assertHTTPObjectReport(t, openapi.Check(raw), tc.code)
		})
	}
	t.Run("different offline documents", func(t *testing.T) {
		raw := pathContract(t, map[string]any{"/items/{id}": map[string]any{}}, nil)
		other := pathContract(t, map[string]any{"/items/{name}": map[string]any{}}, nil)
		assertHTTPObjectReport(t, openapi.CheckWithOptions(raw, openapi.CheckOptions{Resources: map[string][]byte{"https://paths.test/other.json": other}}), "")
	})
}

// Bound reused path templates and deep aliases using the same index budget as reference checks.
func TestNativePathTemplateBudget(t *testing.T) {
	items := map[string]any{}
	for i := 0; i < 32; i++ {
		items[fmt.Sprintf("P%d", i)] = map[string]any{"$ref": fmt.Sprintf("#/components/pathItems/P%d", i+1)}
	}
	items["P32"] = map[string]any{"parameters": []any{pathParameter("id")}, "get": map[string]any{}}
	paths := map[string]any{}
	for i := 0; i < 12; i++ {
		paths[fmt.Sprintf("/items%d/{id}", i)] = map[string]any{"$ref": "#/components/pathItems/P0"}
	}
	raw := pathContract(t, paths, map[string]any{"pathItems": items})
	assertHTTPObjectReport(t, openapi.Check(raw), "")
	limited := openapi.CheckWithOptions(raw, openapi.CheckOptions{MaxIndexBytes: 32 << 10})
	for _, d := range limited.Diagnostics {
		if d.Code == "openapi.spec.budget" {
			return
		}
	}
	t.Fatalf("expected bounded traversal: %+v", limited)
}
